// Package querylog defines DNS query telemetry and its asynchronous writer.
package querylog

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Outcome is the final disposition of a DNS query.
type Outcome string

const (
	OutcomeForwarded Outcome = "forwarded"
	OutcomeBlocked   Outcome = "blocked"
	OutcomeFailed    Outcome = "failed"
)

// ParseOutcome validates an outcome supplied by an API client.
func ParseOutcome(raw string) (Outcome, error) {
	outcome := Outcome(strings.ToLower(strings.TrimSpace(raw)))
	switch outcome {
	case OutcomeForwarded, OutcomeBlocked, OutcomeFailed:
		return outcome, nil
	default:
		return "", fmt.Errorf("outcome must be forwarded, blocked, or failed")
	}
}

// Event records one completed DNS query. Rule fields describe the active
// decision even when an allowed query later fails upstream.
type Event struct {
	Timestamp     time.Time
	ClientIP      string
	Domain        string
	QueryType     string
	Outcome       Outcome
	RuleID        int64
	RuleKind      string
	RuleDomain    string
	BlocklistID   int64
	BlocklistName string
	Upstream      string
	ResponseCode  string
	DurationMS    int64
}

// Entry is a persisted event with its database ID.
type Entry struct {
	ID int64
	Event
}

// Filter selects a newest-first page of query history.
type Filter struct {
	BeforeID int64
	Limit    int
	ClientIP string
	Domain   string
	Outcome  Outcome
	Since    time.Time
	Until    time.Time
}

// Page is one cursor-paginated query-log result.
type Page struct {
	Entries    []Entry
	NextCursor int64
}

// StatsQuery defines the aggregate statistics time range.
type StatsQuery struct {
	Since    time.Time
	Until    time.Time
	Bucket   time.Duration
	TopLimit int
}

// DomainCount is the number of matching queries for a domain.
type DomainCount struct {
	Domain string
	Count  int64
}

// ClientCount summarizes one client address.
type ClientCount struct {
	ClientIP string
	Total    int64
	Blocked  int64
}

// TimeBucket summarizes one interval.
type TimeBucket struct {
	Start   time.Time
	Total   int64
	Blocked int64
}

// Stats summarizes persisted query events.
type Stats struct {
	Since             time.Time
	Until             time.Time
	Bucket            time.Duration
	Total             int64
	Blocked           int64
	Forwarded         int64
	Failed            int64
	UniqueClients     int64
	TopBlockedDomains []DomainCount
	TopClients        []ClientCount
	Buckets           []TimeBucket
}

// WriterStats exposes query-writer health and loss counters.
type WriterStats struct {
	QueueCapacity int
	QueueDepth    int
	Enqueued      uint64
	Written       uint64
	Dropped       uint64
	QueueDropped  uint64
	WriteDropped  uint64
	WriteErrors   uint64
}

// Repository persists query events and performs retention.
type Repository interface {
	InsertQueries(ctx context.Context, events []Event) error
	DeleteQueriesBefore(ctx context.Context, before time.Time, limit int) (int64, error)
}

// Reader provides dashboard query history and aggregate statistics.
type Reader interface {
	ListQueries(ctx context.Context, filter Filter) (Page, error)
	QueryStats(ctx context.Context, query StatsQuery) (Stats, error)
}
