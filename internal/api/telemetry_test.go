package api

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/oplog"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/querylog"
)

type fakeQueryReader struct {
	page       querylog.Page
	stats      querylog.Stats
	filterSeen querylog.Filter
	statsSeen  querylog.StatsQuery
}

func (f *fakeQueryReader) ListQueries(_ context.Context, filter querylog.Filter) (querylog.Page, error) {
	f.filterSeen = filter
	return f.page, nil
}

func (f *fakeQueryReader) QueryStats(_ context.Context, query querylog.StatsQuery) (querylog.Stats, error) {
	f.statsSeen = query
	return f.stats, nil
}

type fakeSystemLogs struct {
	entries []oplog.Entry
}

func (f fakeSystemLogs) Entries() []oplog.Entry {
	return f.entries
}

type fakeLogStats struct {
	stats querylog.WriterStats
}

func (f fakeLogStats) Stats() querylog.WriterStats {
	return f.stats
}

func TestListQueriesParsesFiltersAndPaginates(t *testing.T) {
	t.Parallel()

	seenAt := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	ruleID := int64(4)
	reader := &fakeQueryReader{
		page: querylog.Page{
			Entries: []querylog.Entry{{
				ID: 42,
				Event: querylog.Event{
					Timestamp:    seenAt,
					ClientIP:     "192.0.2.5",
					Domain:       "ads.example.com",
					QueryType:    "A",
					Outcome:      querylog.OutcomeBlocked,
					RuleID:       ruleID,
					RuleKind:     "deny",
					RuleDomain:   "example.com",
					ResponseCode: "NXDOMAIN",
					DurationMS:   2,
				},
			}},
			NextCursor: 42,
		},
	}
	handler := NewHandler(Dependencies{Queries: reader})

	since := "2026-11-20T00:00:00Z"
	until := "2026-11-21T00:00:00Z"
	query := url.Values{
		"limit":     {"1"},
		"before_id": {"100"},
		"client_ip": {"192.0.2.5"},
		"domain":    {"Ads.Example.COM."},
		"outcome":   {"blocked"},
		"since":     {since},
		"until":     {until},
	}
	response := performRequest(handler, http.MethodGet, "/api/queries?"+query.Encode(), "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var body queriesResponse
	decodeResponse(t, response, &body)
	if len(body.Queries) != 1 || body.Queries[0].ID != 42 {
		t.Fatalf("queries = %#v", body.Queries)
	}
	if body.Queries[0].RuleID == nil || *body.Queries[0].RuleID != ruleID {
		t.Errorf("rule_id = %#v", body.Queries[0].RuleID)
	}
	if body.NextCursor == nil || *body.NextCursor != 42 {
		t.Errorf("next_cursor = %#v", body.NextCursor)
	}

	filter := reader.filterSeen
	if filter.Limit != 1 || filter.BeforeID != 100 {
		t.Errorf("pagination filter = %#v", filter)
	}
	if filter.ClientIP != "192.0.2.5" || filter.Domain != "ads.example.com" {
		t.Errorf("identity filter = %#v", filter)
	}
	if filter.Outcome != querylog.OutcomeBlocked {
		t.Errorf("outcome = %q", filter.Outcome)
	}
	wantSince, _ := time.Parse(time.RFC3339, since)
	wantUntil, _ := time.Parse(time.RFC3339, until)
	if !filter.Since.Equal(wantSince) || !filter.Until.Equal(wantUntil) {
		t.Errorf("time filter = %s to %s", filter.Since, filter.Until)
	}
}

func TestListQueriesRejectsInvalidFilters(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{Queries: &fakeQueryReader{}})
	invalidQueries := []string{
		"limit=0",
		"limit=501",
		"limit=one",
		"before_id=-1",
		"client_ip=not-an-ip",
		"domain=*.example.com",
		"outcome=allowed",
		"since=yesterday",
		"since=2026-11-21T00:00:00Z&until=2026-11-20T00:00:00Z",
	}

	for _, query := range invalidQueries {
		t.Run(query, func(t *testing.T) {
			response := performRequest(handler, http.MethodGet, "/api/queries?"+query, "")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s",
					response.Code, http.StatusBadRequest, response.Body.String())
			}
		})
	}
}

func TestQueryStats(t *testing.T) {
	t.Parallel()

	start := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	reader := &fakeQueryReader{
		stats: querylog.Stats{
			Since:         start,
			Until:         start.Add(24 * time.Hour),
			Bucket:        time.Hour,
			Total:         40,
			Blocked:       10,
			Forwarded:     28,
			Failed:        2,
			UniqueClients: 3,
			TopBlockedDomains: []querylog.DomainCount{
				{Domain: "ads.example.com", Count: 10},
			},
			TopClients: []querylog.ClientCount{
				{ClientIP: "192.0.2.5", Total: 20, Blocked: 5},
			},
			Buckets: []querylog.TimeBucket{
				{Start: start, Total: 40, Blocked: 10},
			},
		},
	}
	handler := NewHandler(Dependencies{Queries: reader})

	response := performRequest(handler, http.MethodGet, "/api/stats?range=24h", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var body statsResponse
	decodeResponse(t, response, &body)
	if body.Total != 40 || body.Blocked != 10 || body.BlockedPercentage != 25 {
		t.Errorf("stats totals = %#v", body)
	}
	if body.BucketSeconds != 3600 {
		t.Errorf("bucket_seconds = %d, want 3600", body.BucketSeconds)
	}
	if len(body.TopBlockedDomains) != 1 || len(body.TopClients) != 1 || len(body.Buckets) != 1 {
		t.Errorf("stats lists = %#v", body)
	}
	if reader.statsSeen.Bucket != time.Hour || reader.statsSeen.TopLimit != 10 {
		t.Errorf("stats query = %#v", reader.statsSeen)
	}
}

func TestQueryStatsRejectsInvalidRange(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{Queries: &fakeQueryReader{}})
	for _, rangeValue := range []string{"0h", "-1h", "next week", "400d"} {
		response := performRequest(handler, http.MethodGet, "/api/stats?range="+url.QueryEscape(rangeValue), "")
		if response.Code != http.StatusBadRequest {
			t.Errorf("range %q status = %d, want %d", rangeValue, response.Code, http.StatusBadRequest)
		}
	}
}

func TestSystemLogsAndWriterStats(t *testing.T) {
	t.Parallel()

	buffer := oplog.NewBuffer(2)
	buffer.Add(oplog.Entry{
		Time:       time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC),
		Level:      "INFO",
		Message:    "first",
		Attributes: map[string]any{"count": 1},
	})
	buffer.Add(oplog.Entry{
		Time:       time.Date(2026, 11, 20, 12, 1, 0, 0, time.UTC),
		Level:      "WARN",
		Message:    "second",
		Attributes: map[string]any{},
	})
	handler := NewHandler(Dependencies{
		SystemLogs: buffer,
		LogStats: fakeLogStats{stats: querylog.WriterStats{
			QueueCapacity: 4096,
			QueueDepth:    2,
			Enqueued:      10,
			Written:       8,
			Dropped:       2,
			QueueDropped:  1,
			WriteDropped:  1,
			WriteErrors:   1,
		}},
	})

	response := performRequest(handler, http.MethodGet, "/api/system-logs", "")
	if response.Code != http.StatusOK {
		t.Fatalf("system logs status = %d", response.Code)
	}
	var logs systemLogsResponse
	decodeResponse(t, response, &logs)
	if !logs.Ephemeral || len(logs.Logs) != 2 {
		t.Fatalf("system logs = %#v", logs)
	}
	if logs.Logs[0].Message != "second" {
		t.Errorf("newest system log = %q, want second", logs.Logs[0].Message)
	}

	response = performRequest(handler, http.MethodGet, "/api/logging/stats", "")
	if response.Code != http.StatusOK {
		t.Fatalf("logging stats status = %d", response.Code)
	}
	var stats writerStatsResponse
	decodeResponse(t, response, &stats)
	if stats.QueueDepth != 2 || stats.Dropped != 2 || stats.WriteErrors != 1 {
		t.Errorf("logging stats = %#v", stats)
	}
}

func TestTelemetryEndpointsUnavailable(t *testing.T) {
	t.Parallel()

	handler := NewHandler(Dependencies{})
	for _, path := range []string{"/api/queries", "/api/stats", "/api/system-logs", "/api/logging/stats"} {
		response := performRequest(handler, http.MethodGet, path, "")
		if response.Code != http.StatusServiceUnavailable {
			t.Errorf("%s status = %d, want %d", path, response.Code, http.StatusServiceUnavailable)
		}
	}
}
