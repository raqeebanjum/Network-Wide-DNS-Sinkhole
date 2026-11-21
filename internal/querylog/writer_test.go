package querylog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type recordingRepository struct {
	mu        sync.Mutex
	events    []Event
	insertErr error

	started   chan struct{}
	startOnce sync.Once
	release   chan struct{}
}

func (r *recordingRepository) InsertQueries(_ context.Context, events []Event) error {
	if r.started != nil {
		r.startOnce.Do(func() { close(r.started) })
		<-r.release
	}
	if r.insertErr != nil {
		return r.insertErr
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, events...)
	return nil
}

func (r *recordingRepository) DeleteQueriesBefore(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

func (r *recordingRepository) recorded() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

func TestWriterFlushesEveryEventExactlyOnce(t *testing.T) {
	t.Parallel()

	repository := &recordingRepository{}
	writer := newTestWriter(t, repository, WriterOptions{
		QueueSize:     16,
		BatchSize:     3,
		FlushInterval: time.Hour,
		Retention:     24 * time.Hour,
	})

	for index := range 10 {
		writer.Record(Event{
			Timestamp: time.Now().UTC(),
			Domain:    string(rune('a'+index)) + ".example",
			Outcome:   OutcomeForwarded,
		})
	}
	closeWriter(t, writer)

	events := repository.recorded()
	if len(events) != 10 {
		t.Fatalf("recorded events = %d, want 10", len(events))
	}

	counts := make(map[string]int)
	for _, event := range events {
		counts[event.Domain]++
	}
	for index := range 10 {
		domain := string(rune('a'+index)) + ".example"
		if counts[domain] != 1 {
			t.Errorf("domain %q recorded %d times, want 1", domain, counts[domain])
		}
	}

	stats := writer.Stats()
	if stats.Written != 10 || stats.Dropped != 0 {
		t.Errorf("writer stats = %#v", stats)
	}
}

func TestWriterDropsWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	repository := &recordingRepository{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	writer := newTestWriter(t, repository, WriterOptions{
		QueueSize:     1,
		BatchSize:     1,
		FlushInterval: time.Hour,
		Retention:     24 * time.Hour,
	})

	writer.Record(Event{Outcome: OutcomeForwarded, Domain: "first.example"})
	select {
	case <-repository.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first insert")
	}

	writer.Record(Event{Outcome: OutcomeForwarded, Domain: "second.example"})
	writer.Record(Event{Outcome: OutcomeForwarded, Domain: "dropped.example"})
	close(repository.release)
	closeWriter(t, writer)

	stats := writer.Stats()
	if stats.QueueDropped != 1 {
		t.Errorf("QueueDropped = %d, want 1; stats = %#v", stats.QueueDropped, stats)
	}
	if stats.Written != 2 {
		t.Errorf("Written = %d, want 2; stats = %#v", stats.Written, stats)
	}
	if len(repository.recorded()) != 2 {
		t.Errorf("recorded events = %d, want 2", len(repository.recorded()))
	}
}

func TestWriterCountsPersistenceFailures(t *testing.T) {
	t.Parallel()

	repository := &recordingRepository{insertErr: errors.New("database unavailable")}
	writer := newTestWriter(t, repository, WriterOptions{
		QueueSize:     4,
		BatchSize:     1,
		FlushInterval: time.Hour,
		Retention:     24 * time.Hour,
	})

	writer.Record(Event{Outcome: OutcomeForwarded, Domain: "lost.example"})
	closeWriter(t, writer)

	stats := writer.Stats()
	if stats.WriteErrors != 1 || stats.WriteDropped != 1 || stats.Dropped != 1 {
		t.Errorf("writer stats = %#v", stats)
	}
}

func newTestWriter(t *testing.T, repository Repository, options WriterOptions) *Writer {
	t.Helper()

	options.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	writer, err := NewWriter(repository, options)
	if err != nil {
		t.Fatalf("create query-log writer: %v", err)
	}
	return writer
}

func closeWriter(t *testing.T, writer *Writer) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writer.Close(ctx); err != nil {
		t.Fatalf("close query-log writer: %v", err)
	}
}
