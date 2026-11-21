package blocklists_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/blocklists"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/store"
)

type fakeFetcher struct {
	mu      sync.Mutex
	results []blocklists.FetchResult
	errors  []error
	calls   int
}

func (f *fakeFetcher) Fetch(_ context.Context, _, _, _ string) (blocklists.FetchResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	index := f.calls
	f.calls++
	if index < len(f.errors) && f.errors[index] != nil {
		return blocklists.FetchResult{}, f.errors[index]
	}
	if index >= len(f.results) {
		return blocklists.FetchResult{}, errors.New("no fake result configured")
	}
	return f.results[index], nil
}

func TestRefreshActivatesAndPreservesLastGoodSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "sinkhole.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})

	policy, err := rules.NewManager(ctx, database)
	if err != nil {
		t.Fatalf("create rule manager: %v", err)
	}

	fetcher := &fakeFetcher{
		results: []blocklists.FetchResult{
			{
				Body: []byte("0.0.0.0 blocked.example\n"),
				ETag: `"v1"`,
			},
			{},
			{
				Body: []byte("0.0.0.0 blocked.example\n0.0.0.0 second.example\n"),
				ETag: `"v2"`,
			},
		},
		errors: []error{nil, errors.New("network unavailable"), nil},
	}
	service, err := blocklists.NewService(
		database,
		fetcher,
		policy,
		time.Hour,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("create blocklist service: %v", err)
	}

	created, err := service.Create(ctx, blocklists.Input{
		Name:     "Test list",
		URL:      "https://lists.example/blocklist.txt",
		Enabled:  true,
		License:  "MIT",
		Homepage: "https://lists.example",
	})
	if err != nil {
		t.Fatalf("create blocklist: %v", err)
	}

	refreshed, err := service.Refresh(ctx, created.ID)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	if refreshed.DomainCount != 1 || refreshed.Format != "hosts" || refreshed.ETag != `"v1"` {
		t.Errorf("refreshed list = %#v", refreshed)
	}
	match, ok := policy.Match("ads.blocked.example")
	if !ok || !match.Blocks() || match.Blocklist.BlocklistName != "Test list" {
		t.Fatalf("blocklist matcher = %#v, %v", match, ok)
	}

	if _, err := service.Refresh(ctx, created.ID); !errors.Is(err, blocklists.ErrRefresh) {
		t.Fatalf("failed refresh error = %v, want ErrRefresh", err)
	}
	failed, err := database.GetBlocklist(ctx, created.ID)
	if err != nil {
		t.Fatalf("get failed list: %v", err)
	}
	if failed.LastError == "" {
		t.Error("failed list did not record last_error")
	}
	if failed.DomainCount != 1 {
		t.Errorf("failed list domain count = %d, want previous snapshot", failed.DomainCount)
	}
	if _, ok := policy.Match("blocked.example"); !ok {
		t.Error("previous blocklist snapshot is no longer active")
	}

	updated, err := service.Update(ctx, created.ID, blocklists.Input{
		Name:     created.Name,
		URL:      created.URL,
		Enabled:  false,
		License:  created.License,
		Homepage: created.Homepage,
	})
	if err != nil {
		t.Fatalf("disable blocklist: %v", err)
	}
	if updated.Enabled {
		t.Error("disabled blocklist is still enabled")
	}
	if _, ok := policy.Match("blocked.example"); ok {
		t.Error("disabled blocklist is still active")
	}

	third, err := service.Refresh(ctx, created.ID)
	if err != nil {
		t.Fatalf("third refresh: %v", err)
	}
	if third.DomainCount != 2 {
		t.Errorf("third refresh domain count = %d, want 2", third.DomainCount)
	}

	if err := service.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete blocklist: %v", err)
	}
	if _, ok := policy.Match("blocked.example"); ok {
		t.Error("deleted blocklist is still active")
	}
}

func TestCreateRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database, err := store.Open(filepath.Join(t.TempDir(), "sinkhole.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	policy, err := rules.NewManager(ctx, database)
	if err != nil {
		t.Fatalf("create rule manager: %v", err)
	}
	service, err := blocklists.NewService(
		database,
		&fakeFetcher{},
		policy,
		time.Hour,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	_, err = service.Create(ctx, blocklists.Input{
		Name:    "Insecure",
		URL:     "http://example.com/list.txt",
		Enabled: true,
	})
	if !errors.Is(err, blocklists.ErrInvalid) {
		t.Fatalf("Create error = %v, want ErrInvalid", err)
	}
}
