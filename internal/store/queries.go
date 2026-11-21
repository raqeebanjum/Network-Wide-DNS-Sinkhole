package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/querylog"
)

const (
	defaultQueryPageSize = 100
	maxQueryPageSize     = 500
	defaultTopLimit      = 10
	maxTopLimit          = 50
	retentionDeleteLimit = 1000
)

// InsertQueries persists a batch of completed queries in one transaction.
func (s *Store) InsertQueries(ctx context.Context, events []querylog.Event) error {
	if len(events) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin query-log transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO query_logs (
			queried_at, client_ip, domain, query_type, outcome,
			rule_id, rule_kind, rule_domain, upstream, response_code, duration_ms
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare query-log insert: %w", err)
	}
	defer statement.Close()

	for _, event := range events {
		if event.Outcome != querylog.OutcomeForwarded &&
			event.Outcome != querylog.OutcomeBlocked &&
			event.Outcome != querylog.OutcomeFailed {
			return fmt.Errorf("insert query log: invalid outcome %q", event.Outcome)
		}
		if event.DurationMS < 0 {
			return fmt.Errorf("insert query log: negative duration")
		}

		var ruleID any
		if event.RuleID > 0 {
			ruleID = event.RuleID
		}
		if _, err := statement.ExecContext(ctx,
			event.Timestamp.UTC().UnixMilli(),
			event.ClientIP,
			event.Domain,
			event.QueryType,
			string(event.Outcome),
			ruleID,
			event.RuleKind,
			event.RuleDomain,
			event.Upstream,
			event.ResponseCode,
			event.DurationMS,
		); err != nil {
			return fmt.Errorf("insert query log: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit query-log transaction: %w", err)
	}
	return nil
}

// DeleteQueriesBefore removes at most limit expired queries and returns the
// number deleted.
func (s *Store) DeleteQueriesBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = retentionDeleteLimit
	}

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM query_logs
		WHERE id IN (
			SELECT id
			FROM query_logs
			WHERE queried_at < ?
			ORDER BY queried_at, id
			LIMIT ?
		)
	`, before.UTC().UnixMilli(), limit)
	if err != nil {
		return 0, fmt.Errorf("delete expired query logs: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("read deleted query-log count: %w", err)
	}
	return deleted, nil
}

// ListQueries returns a newest-first page using ID keyset pagination.
func (s *Store) ListQueries(ctx context.Context, filter querylog.Filter) (querylog.Page, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultQueryPageSize
	}
	if limit > maxQueryPageSize {
		limit = maxQueryPageSize
	}

	var (
		query strings.Builder
		args  []any
	)
	query.WriteString(`
		SELECT id, queried_at, client_ip, domain, query_type, outcome,
		       rule_id, rule_kind, rule_domain, upstream, response_code, duration_ms
		FROM query_logs
		WHERE 1 = 1
	`)

	if filter.BeforeID > 0 {
		query.WriteString(" AND id < ?")
		args = append(args, filter.BeforeID)
	}
	if filter.ClientIP != "" {
		query.WriteString(" AND client_ip = ?")
		args = append(args, filter.ClientIP)
	}
	if filter.Domain != "" {
		query.WriteString(" AND domain = ?")
		args = append(args, filter.Domain)
	}
	if filter.Outcome != "" {
		query.WriteString(" AND outcome = ?")
		args = append(args, string(filter.Outcome))
	}
	if !filter.Since.IsZero() {
		query.WriteString(" AND queried_at >= ?")
		args = append(args, filter.Since.UTC().UnixMilli())
	}
	if !filter.Until.IsZero() {
		query.WriteString(" AND queried_at <= ?")
		args = append(args, filter.Until.UTC().UnixMilli())
	}

	query.WriteString(" ORDER BY id DESC LIMIT ?")
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return querylog.Page{}, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	entries := make([]querylog.Entry, 0, limit)
	for rows.Next() {
		entry, err := scanQueryEntry(rows)
		if err != nil {
			return querylog.Page{}, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return querylog.Page{}, fmt.Errorf("iterate query history: %w", err)
	}

	var nextCursor int64
	if len(entries) > limit {
		entries = entries[:limit]
		nextCursor = entries[len(entries)-1].ID
	}
	return querylog.Page{Entries: entries, NextCursor: nextCursor}, nil
}

// QueryStats aggregates totals, top lists, and time buckets in one read snapshot.
func (s *Store) QueryStats(ctx context.Context, query querylog.StatsQuery) (querylog.Stats, error) {
	if query.Bucket < time.Millisecond {
		query.Bucket = time.Hour
	}
	if query.TopLimit <= 0 {
		query.TopLimit = defaultTopLimit
	}
	if query.TopLimit > maxTopLimit {
		query.TopLimit = maxTopLimit
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return querylog.Stats{}, fmt.Errorf("begin stats transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	since := query.Since.UTC().UnixMilli()
	until := query.Until.UTC().UnixMilli()
	stats := querylog.Stats{
		Since:             query.Since.UTC(),
		Until:             query.Until.UTC(),
		Bucket:            query.Bucket,
		TopBlockedDomains: make([]querylog.DomainCount, 0),
		TopClients:        make([]querylog.ClientCount, 0),
		Buckets:           make([]querylog.TimeBucket, 0),
	}

	if err := tx.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN outcome = 'blocked' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = 'forwarded' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN outcome = 'failed' THEN 1 ELSE 0 END), 0),
			COUNT(DISTINCT client_ip)
		FROM query_logs
		WHERE queried_at >= ? AND queried_at <= ?
	`, since, until).Scan(
		&stats.Total,
		&stats.Blocked,
		&stats.Forwarded,
		&stats.Failed,
		&stats.UniqueClients,
	); err != nil {
		return querylog.Stats{}, fmt.Errorf("aggregate query totals: %w", err)
	}

	domainRows, err := tx.QueryContext(ctx, `
		SELECT domain, COUNT(*)
		FROM query_logs
		WHERE queried_at >= ? AND queried_at <= ? AND outcome = 'blocked'
		GROUP BY domain
		ORDER BY COUNT(*) DESC, domain
		LIMIT ?
	`, since, until, query.TopLimit)
	if err != nil {
		return querylog.Stats{}, fmt.Errorf("aggregate top blocked domains: %w", err)
	}
	for domainRows.Next() {
		var item querylog.DomainCount
		if err := domainRows.Scan(&item.Domain, &item.Count); err != nil {
			domainRows.Close()
			return querylog.Stats{}, fmt.Errorf("scan top blocked domain: %w", err)
		}
		stats.TopBlockedDomains = append(stats.TopBlockedDomains, item)
	}
	if err := domainRows.Close(); err != nil {
		return querylog.Stats{}, fmt.Errorf("close top blocked domains: %w", err)
	}

	clientRows, err := tx.QueryContext(ctx, `
		SELECT client_ip, COUNT(*),
		       COALESCE(SUM(CASE WHEN outcome = 'blocked' THEN 1 ELSE 0 END), 0)
		FROM query_logs
		WHERE queried_at >= ? AND queried_at <= ?
		GROUP BY client_ip
		ORDER BY COUNT(*) DESC, client_ip
		LIMIT ?
	`, since, until, query.TopLimit)
	if err != nil {
		return querylog.Stats{}, fmt.Errorf("aggregate top clients: %w", err)
	}
	for clientRows.Next() {
		var item querylog.ClientCount
		if err := clientRows.Scan(&item.ClientIP, &item.Total, &item.Blocked); err != nil {
			clientRows.Close()
			return querylog.Stats{}, fmt.Errorf("scan top client: %w", err)
		}
		stats.TopClients = append(stats.TopClients, item)
	}
	if err := clientRows.Close(); err != nil {
		return querylog.Stats{}, fmt.Errorf("close top clients: %w", err)
	}

	bucketMS := query.Bucket.Milliseconds()
	bucketRows, err := tx.QueryContext(ctx, `
		SELECT (queried_at / ?) * ? AS bucket_start,
		       COUNT(*),
		       COALESCE(SUM(CASE WHEN outcome = 'blocked' THEN 1 ELSE 0 END), 0)
		FROM query_logs
		WHERE queried_at >= ? AND queried_at <= ?
		GROUP BY bucket_start
		ORDER BY bucket_start
	`, bucketMS, bucketMS, since, until)
	if err != nil {
		return querylog.Stats{}, fmt.Errorf("aggregate query buckets: %w", err)
	}
	for bucketRows.Next() {
		var (
			start   int64
			total   int64
			blocked int64
		)
		if err := bucketRows.Scan(&start, &total, &blocked); err != nil {
			bucketRows.Close()
			return querylog.Stats{}, fmt.Errorf("scan query bucket: %w", err)
		}
		stats.Buckets = append(stats.Buckets, querylog.TimeBucket{
			Start:   time.UnixMilli(start).UTC(),
			Total:   total,
			Blocked: blocked,
		})
	}
	if err := bucketRows.Close(); err != nil {
		return querylog.Stats{}, fmt.Errorf("close query buckets: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return querylog.Stats{}, fmt.Errorf("commit stats transaction: %w", err)
	}
	return stats, nil
}

type queryScanner interface {
	Scan(dest ...any) error
}

func scanQueryEntry(row queryScanner) (querylog.Entry, error) {
	var (
		entry      querylog.Entry
		queriedAt  int64
		outcome    string
		ruleID     sql.NullInt64
		durationMS int64
	)
	if err := row.Scan(
		&entry.ID,
		&queriedAt,
		&entry.ClientIP,
		&entry.Domain,
		&entry.QueryType,
		&outcome,
		&ruleID,
		&entry.RuleKind,
		&entry.RuleDomain,
		&entry.Upstream,
		&entry.ResponseCode,
		&durationMS,
	); err != nil {
		return querylog.Entry{}, fmt.Errorf("scan query entry: %w", err)
	}

	entry.Timestamp = time.UnixMilli(queriedAt).UTC()
	entry.Outcome = querylog.Outcome(outcome)
	if ruleID.Valid {
		entry.RuleID = ruleID.Int64
	}
	entry.DurationMS = durationMS
	return entry, nil
}
