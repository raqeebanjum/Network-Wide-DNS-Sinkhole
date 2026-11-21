package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/querylog"
)

func TestInsertAndListQueries(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	ctx := context.Background()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)

	events := []querylog.Event{
		{
			Timestamp: base, ClientIP: "192.0.2.1", Domain: "a.example",
			QueryType: "A", Outcome: querylog.OutcomeForwarded,
			Upstream: "1.1.1.1:53", ResponseCode: "NOERROR", DurationMS: 2,
		},
		{
			Timestamp: base.Add(time.Minute), ClientIP: "192.0.2.2", Domain: "b.example",
			QueryType: "AAAA", Outcome: querylog.OutcomeBlocked,
			RuleID: 7, RuleKind: "deny", RuleDomain: "example",
			ResponseCode: "NXDOMAIN", DurationMS: 0,
		},
		{
			Timestamp: base.Add(2 * time.Minute), ClientIP: "192.0.2.1", Domain: "c.example",
			QueryType: "A", Outcome: querylog.OutcomeFailed,
			Upstream: "1.0.0.1:53", ResponseCode: "SERVFAIL", DurationMS: 5,
		},
	}
	if err := database.InsertQueries(ctx, events); err != nil {
		t.Fatalf("insert queries: %v", err)
	}

	firstPage, err := database.ListQueries(ctx, querylog.Filter{Limit: 2})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(firstPage.Entries) != 2 {
		t.Fatalf("first page entries = %d, want 2", len(firstPage.Entries))
	}
	if firstPage.Entries[0].Domain != "c.example" || firstPage.Entries[1].Domain != "b.example" {
		t.Errorf("first page order = %#v", firstPage.Entries)
	}
	if firstPage.NextCursor != firstPage.Entries[1].ID || firstPage.NextCursor == 0 {
		t.Errorf("next cursor = %d, want %d", firstPage.NextCursor, firstPage.Entries[1].ID)
	}

	secondPage, err := database.ListQueries(ctx, querylog.Filter{
		Limit:    2,
		BeforeID: firstPage.NextCursor,
	})
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if len(secondPage.Entries) != 1 || secondPage.Entries[0].Domain != "a.example" {
		t.Fatalf("second page = %#v", secondPage)
	}
	if secondPage.NextCursor != 0 {
		t.Errorf("second page next cursor = %d, want 0", secondPage.NextCursor)
	}

	blocked, err := database.ListQueries(ctx, querylog.Filter{
		Outcome: querylog.OutcomeBlocked,
	})
	if err != nil {
		t.Fatalf("filter blocked queries: %v", err)
	}
	if len(blocked.Entries) != 1 || blocked.Entries[0].RuleID != 7 ||
		blocked.Entries[0].RuleKind != "deny" || blocked.Entries[0].RuleDomain != "example" {
		t.Errorf("blocked queries = %#v", blocked.Entries)
	}

	filtered, err := database.ListQueries(ctx, querylog.Filter{
		ClientIP: "192.0.2.1",
		Domain:   "a.example",
		Since:    base,
		Until:    base,
	})
	if err != nil {
		t.Fatalf("filter queries: %v", err)
	}
	if len(filtered.Entries) != 1 || filtered.Entries[0].Domain != "a.example" {
		t.Errorf("filtered queries = %#v", filtered.Entries)
	}
}

func TestInsertRejectsInvalidOutcome(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	err := database.InsertQueries(context.Background(), []querylog.Event{{
		Timestamp: time.Now(),
		Outcome:   querylog.Outcome("unknown"),
	}})
	if err == nil {
		t.Fatal("InsertQueries returned nil error")
	}
}

func TestQueryStats(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	ctx := context.Background()
	base := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

	events := []querylog.Event{
		{
			Timestamp: base.Add(5 * time.Minute), ClientIP: "192.0.2.1",
			Domain: "ads.example.com", QueryType: "A", Outcome: querylog.OutcomeBlocked,
			ResponseCode: "NXDOMAIN", DurationMS: 0,
		},
		{
			Timestamp: base.Add(15 * time.Minute), ClientIP: "192.0.2.1",
			Domain: "ads.example.com", QueryType: "A", Outcome: querylog.OutcomeBlocked,
			ResponseCode: "NXDOMAIN", DurationMS: 0,
		},
		{
			Timestamp: base.Add(65 * time.Minute), ClientIP: "192.0.2.2",
			Domain: "example.com", QueryType: "A", Outcome: querylog.OutcomeForwarded,
			Upstream: "1.1.1.1:53", ResponseCode: "NOERROR", DurationMS: 2,
		},
		{
			Timestamp: base.Add(125 * time.Minute), ClientIP: "192.0.2.2",
			Domain: "broken.example", QueryType: "A", Outcome: querylog.OutcomeFailed,
			Upstream: "1.1.1.1:53", ResponseCode: "SERVFAIL", DurationMS: 100,
		},
	}
	if err := database.InsertQueries(ctx, events); err != nil {
		t.Fatalf("insert queries: %v", err)
	}

	stats, err := database.QueryStats(ctx, querylog.StatsQuery{
		Since:    base,
		Until:    base.Add(3 * time.Hour),
		Bucket:   time.Hour,
		TopLimit: 2,
	})
	if err != nil {
		t.Fatalf("query stats: %v", err)
	}

	if stats.Total != 4 || stats.Blocked != 2 || stats.Forwarded != 1 || stats.Failed != 1 {
		t.Errorf("totals = %#v", stats)
	}
	if stats.UniqueClients != 2 {
		t.Errorf("unique clients = %d, want 2", stats.UniqueClients)
	}
	if len(stats.TopBlockedDomains) != 1 ||
		stats.TopBlockedDomains[0].Domain != "ads.example.com" ||
		stats.TopBlockedDomains[0].Count != 2 {
		t.Errorf("top blocked domains = %#v", stats.TopBlockedDomains)
	}
	if len(stats.TopClients) != 2 || stats.TopClients[0].ClientIP != "192.0.2.1" {
		t.Errorf("top clients = %#v", stats.TopClients)
	}
	if stats.TopClients[0].Blocked != 2 {
		t.Errorf("first client's blocked count = %d, want 2", stats.TopClients[0].Blocked)
	}
	if len(stats.Buckets) != 3 {
		t.Fatalf("buckets = %#v", stats.Buckets)
	}
	if stats.Buckets[0].Total != 2 || stats.Buckets[0].Blocked != 2 {
		t.Errorf("first bucket = %#v", stats.Buckets[0])
	}
	if stats.Buckets[1].Total != 1 || stats.Buckets[2].Total != 1 {
		t.Errorf("later buckets = %#v", stats.Buckets[1:])
	}
}

func TestDeleteQueriesBeforeInBatches(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	ctx := context.Background()
	cutoff := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)

	events := []querylog.Event{
		{Timestamp: cutoff.Add(-time.Hour), Domain: "old1.example", Outcome: querylog.OutcomeForwarded},
		{Timestamp: cutoff.Add(-2 * time.Hour), Domain: "old2.example", Outcome: querylog.OutcomeForwarded},
		{Timestamp: cutoff.Add(time.Hour), Domain: "new.example", Outcome: querylog.OutcomeForwarded},
	}
	if err := database.InsertQueries(ctx, events); err != nil {
		t.Fatalf("insert queries: %v", err)
	}

	deleted, err := database.DeleteQueriesBefore(ctx, cutoff, 1)
	if err != nil {
		t.Fatalf("delete first batch: %v", err)
	}
	if deleted != 1 {
		t.Errorf("first deleted count = %d, want 1", deleted)
	}
	deleted, err = database.DeleteQueriesBefore(ctx, cutoff, 1)
	if err != nil {
		t.Fatalf("delete second batch: %v", err)
	}
	if deleted != 1 {
		t.Errorf("second deleted count = %d, want 1", deleted)
	}

	page, err := database.ListQueries(ctx, querylog.Filter{})
	if err != nil {
		t.Fatalf("list remaining queries: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Domain != "new.example" {
		t.Errorf("remaining queries = %#v", page.Entries)
	}
}
