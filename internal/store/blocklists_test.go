package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/blocklists"
)

func TestBlocklistCRUDAndDomainReplacement(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	ctx := context.Background()

	initial, err := database.ListBlocklists(ctx)
	if err != nil {
		t.Fatalf("list initial blocklists: %v", err)
	}
	if len(initial) != 1 || initial[0].Name != "StevenBlack Unified Hosts" {
		t.Fatalf("initial blocklists = %#v", initial)
	}

	created, err := database.CreateBlocklist(ctx, blocklists.Input{
		Name:     "Test source",
		URL:      "https://lists.example/test.txt",
		Enabled:  true,
		License:  "MIT",
		Homepage: "https://lists.example",
	})
	if err != nil {
		t.Fatalf("create blocklist: %v", err)
	}
	if !created.Enabled || created.DomainCount != 0 {
		t.Errorf("created blocklist = %#v", created)
	}

	if _, err := database.CreateBlocklist(ctx, blocklists.Input{
		Name:    "Duplicate",
		URL:     created.URL,
		Enabled: true,
	}); !errors.Is(err, blocklists.ErrConflict) {
		t.Fatalf("duplicate create error = %v, want ErrConflict", err)
	}

	checkedAt := time.Date(2026, 11, 21, 1, 0, 0, 0, time.UTC)
	refreshed, err := database.ReplaceBlocklistDomains(ctx, created.ID, blocklists.ParseResult{
		Domains: []string{"ads.example", "tracker.example", "ads.example"},
		Format:  "hosts",
	}, `"etag-1"`, "yesterday", checkedAt)
	if err != nil {
		t.Fatalf("replace domains: %v", err)
	}
	if refreshed.DomainCount != 2 || refreshed.Format != "hosts" || refreshed.LastUpdatedAt == nil {
		t.Errorf("refreshed blocklist = %#v", refreshed)
	}

	domains, err := database.ListBlocklistDomains(ctx)
	if err != nil {
		t.Fatalf("list blocklist domains: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("blocklist domains = %#v", domains)
	}
	if domains[0].BlocklistID != created.ID || domains[0].BlocklistName != "Test source" {
		t.Errorf("domain attribution = %#v", domains[0])
	}

	failedAt := checkedAt.Add(time.Hour)
	failed, err := database.RecordBlocklistError(ctx, created.ID, "download failed", failedAt)
	if err != nil {
		t.Fatalf("record blocklist error: %v", err)
	}
	if failed.LastError != "download failed" || failed.DomainCount != 2 {
		t.Errorf("failed blocklist = %#v", failed)
	}
	if failed.LastUpdatedAt == nil || !failed.LastUpdatedAt.Equal(*refreshed.LastUpdatedAt) {
		t.Error("failed refresh changed the last successful update")
	}

	unchanged, err := database.RecordBlocklistUnchanged(
		ctx, created.ID, `"etag-2"`, "today", failedAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("record unchanged blocklist: %v", err)
	}
	if unchanged.LastError != "" || unchanged.ETag != `"etag-2"` || unchanged.DomainCount != 2 {
		t.Errorf("unchanged blocklist = %#v", unchanged)
	}

	disabled, err := database.UpdateBlocklist(ctx, created.ID, blocklists.Input{
		Name:     created.Name,
		URL:      created.URL,
		Enabled:  false,
		License:  created.License,
		Homepage: created.Homepage,
	})
	if err != nil {
		t.Fatalf("disable blocklist: %v", err)
	}
	if disabled.Enabled {
		t.Error("blocklist is still enabled")
	}
	domains, err = database.ListBlocklistDomains(ctx)
	if err != nil {
		t.Fatalf("list disabled blocklist domains: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("disabled blocklist domains = %#v, want none", domains)
	}

	if err := database.DeleteBlocklist(ctx, created.ID); err != nil {
		t.Fatalf("delete blocklist: %v", err)
	}
	if _, err := database.GetBlocklist(ctx, created.ID); !errors.Is(err, blocklists.ErrNotFound) {
		t.Fatalf("get deleted blocklist error = %v, want ErrNotFound", err)
	}
}

func TestBlocklistDuplicateDomainKeepsBothSources(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	ctx := context.Background()

	first, err := database.CreateBlocklist(ctx, blocklists.Input{
		Name: "First", URL: "https://lists.example/first.txt", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create first blocklist: %v", err)
	}
	second, err := database.CreateBlocklist(ctx, blocklists.Input{
		Name: "Second", URL: "https://lists.example/second.txt", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create second blocklist: %v", err)
	}

	for _, list := range []blocklists.List{first, second} {
		if _, err := database.ReplaceBlocklistDomains(ctx, list.ID, blocklists.ParseResult{
			Domains: []string{"shared.example"},
			Format:  "domains",
		}, "", "", time.Now()); err != nil {
			t.Fatalf("replace domains for %s: %v", list.Name, err)
		}
	}

	domains, err := database.ListBlocklistDomains(ctx)
	if err != nil {
		t.Fatalf("list domains: %v", err)
	}
	if len(domains) != 2 || domains[0].BlocklistID != first.ID || domains[1].BlocklistID != second.ID {
		t.Errorf("domain sources = %#v", domains)
	}

	if _, err := database.UpdateBlocklist(ctx, first.ID, blocklists.Input{
		Name: first.Name, URL: first.URL, Enabled: false,
	}); err != nil {
		t.Fatalf("disable first blocklist: %v", err)
	}
	domains, err = database.ListBlocklistDomains(ctx)
	if err != nil {
		t.Fatalf("list domains after disable: %v", err)
	}
	if len(domains) != 1 || domains[0].BlocklistID != second.ID {
		t.Errorf("remaining domain sources = %#v", domains)
	}
}
