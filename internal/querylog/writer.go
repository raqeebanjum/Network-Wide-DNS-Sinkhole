package querylog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultQueueSize     = 4096
	defaultBatchSize     = 100
	defaultFlushInterval = 500 * time.Millisecond
	defaultCleanupEvery  = time.Hour
	defaultRetention     = 30 * 24 * time.Hour
	retentionDeleteBatch = 1000
	flushTimeout         = 5 * time.Second
	cleanupTimeout       = 30 * time.Second
)

// WriterOptions controls batching and retention. Zero values use safe defaults.
type WriterOptions struct {
	QueueSize       int
	BatchSize       int
	FlushInterval   time.Duration
	Retention       time.Duration
	CleanupInterval time.Duration
	Logger          *slog.Logger
}

// Writer drains query events into SQLite without blocking DNS handlers.
type Writer struct {
	repository Repository
	logger     *slog.Logger
	queue      chan Event
	batchSize  int
	flushEvery time.Duration
	retention  time.Duration
	cleanupFor time.Duration

	mu        sync.RWMutex
	closed    bool
	closeOnce sync.Once
	done      chan struct{}

	enqueued     atomic.Uint64
	written      atomic.Uint64
	queueDropped atomic.Uint64
	writeDropped atomic.Uint64
	writeErrors  atomic.Uint64
}

// NewWriter starts the asynchronous query-log worker.
func NewWriter(repository Repository, options WriterOptions) (*Writer, error) {
	if repository == nil {
		return nil, errors.New("query-log repository must not be nil")
	}
	if options.QueueSize == 0 {
		options.QueueSize = defaultQueueSize
	}
	if options.BatchSize == 0 {
		options.BatchSize = defaultBatchSize
	}
	if options.FlushInterval == 0 {
		options.FlushInterval = defaultFlushInterval
	}
	if options.Retention == 0 {
		options.Retention = defaultRetention
	}
	if options.CleanupInterval == 0 {
		options.CleanupInterval = defaultCleanupEvery
	}
	if options.QueueSize < 0 || options.BatchSize < 0 ||
		options.FlushInterval < 0 || options.Retention < 0 || options.CleanupInterval < 0 {
		return nil, errors.New("query-log writer options must not be negative")
	}

	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	writer := &Writer{
		repository: repository,
		logger:     logger,
		queue:      make(chan Event, options.QueueSize),
		batchSize:  options.BatchSize,
		flushEvery: options.FlushInterval,
		retention:  options.Retention,
		cleanupFor: options.CleanupInterval,
		done:       make(chan struct{}),
	}
	go writer.run()
	return writer, nil
}

// Record enqueues a completed query. It never waits for the database.
func (w *Writer) Record(event Event) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.closed {
		w.queueDropped.Add(1)
		return
	}

	select {
	case w.queue <- event:
		w.enqueued.Add(1)
	default:
		w.queueDropped.Add(1)
	}
}

// Close stops accepting events, flushes the remaining batch, and waits for the
// worker to exit or ctx to expire.
func (w *Writer) Close(ctx context.Context) error {
	w.closeOnce.Do(func() {
		w.mu.Lock()
		w.closed = true
		close(w.queue)
		w.mu.Unlock()
	})

	select {
	case <-w.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stats returns a consistent-enough snapshot of queue and persistence health.
func (w *Writer) Stats() WriterStats {
	queueDropped := w.queueDropped.Load()
	writeDropped := w.writeDropped.Load()
	return WriterStats{
		QueueCapacity: cap(w.queue),
		QueueDepth:    len(w.queue),
		Enqueued:      w.enqueued.Load(),
		Written:       w.written.Load(),
		Dropped:       queueDropped + writeDropped,
		QueueDropped:  queueDropped,
		WriteDropped:  writeDropped,
		WriteErrors:   w.writeErrors.Load(),
	}
}

func (w *Writer) run() {
	defer close(w.done)

	batch := make([]Event, 0, w.batchSize)
	flushTicker := time.NewTicker(w.flushEvery)
	cleanupTicker := time.NewTicker(w.cleanupFor)
	defer flushTicker.Stop()
	defer cleanupTicker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		err := w.repository.InsertQueries(ctx, batch)
		cancel()

		if err != nil {
			w.writeErrors.Add(1)
			w.writeDropped.Add(uint64(len(batch)))
			w.logger.Error("persist query log batch", "events", len(batch), "error", err)
		} else {
			w.written.Add(uint64(len(batch)))
		}
		batch = batch[:0]
	}

	for {
		select {
		case event, ok := <-w.queue:
			if !ok {
				flush()
				return
			}
			batch = append(batch, event)
			if len(batch) >= w.batchSize {
				flush()
			}
		case <-flushTicker.C:
			flush()
		case <-cleanupTicker.C:
			w.cleanup()
		}
	}
}

func (w *Writer) cleanup() {
	cutoff := time.Now().UTC().Add(-w.retention)
	var deleted int64

	for {
		ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		count, err := w.repository.DeleteQueriesBefore(ctx, cutoff, retentionDeleteBatch)
		cancel()
		if err != nil {
			w.logger.Error("delete expired query logs", "error", err)
			return
		}
		deleted += count
		if count < retentionDeleteBatch {
			break
		}
	}

	if deleted > 0 {
		w.logger.Info("expired query logs deleted", "count", deleted)
	}
}
