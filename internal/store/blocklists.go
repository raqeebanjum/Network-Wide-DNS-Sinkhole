package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/blocklists"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"modernc.org/sqlite"
)

// ListBlocklists returns all configured sources, including disabled ones.
func (s *Store) ListBlocklists(ctx context.Context) ([]blocklists.List, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, url, enabled, last_updated_at,
		       domain_count, created_at, updated_at
		FROM blocklists
		ORDER BY name, id
	`)
	if err != nil {
		return nil, fmt.Errorf("query blocklists: %w", err)
	}
	defer rows.Close()

	lists := make([]blocklists.List, 0)
	for rows.Next() {
		list, err := scanBlocklist(rows)
		if err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blocklists: %w", err)
	}
	return lists, nil
}

// GetBlocklist returns one configured source.
func (s *Store) GetBlocklist(ctx context.Context, id int64) (blocklists.List, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, url, enabled, last_updated_at,
		       domain_count, created_at, updated_at
		FROM blocklists
		WHERE id = ?
	`, id)
	list, err := scanBlocklist(row)
	if errors.Is(err, sql.ErrNoRows) {
		return blocklists.List{}, blocklists.ErrNotFound
	}
	return list, err
}

// CreateBlocklist inserts a new source.
func (s *Store) CreateBlocklist(ctx context.Context, input blocklists.Input) (blocklists.List, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO blocklists (name, url, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, input.Name, input.URL, input.Enabled, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return blocklists.List{}, translateBlocklistError("create blocklist", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return blocklists.List{}, fmt.Errorf("read created blocklist ID: %w", err)
	}
	return s.GetBlocklist(ctx, id)
}

// UpdateBlocklist replaces editable metadata.
func (s *Store) UpdateBlocklist(ctx context.Context, id int64, input blocklists.Input) (blocklists.List, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE blocklists
		SET name = ?, url = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, input.Name, input.URL, input.Enabled, time.Now().UTC().UnixMilli(), id)
	if err != nil {
		return blocklists.List{}, translateBlocklistError("update blocklist", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return blocklists.List{}, fmt.Errorf("read updated blocklist count: %w", err)
	}
	if affected == 0 {
		return blocklists.List{}, blocklists.ErrNotFound
	}
	return s.GetBlocklist(ctx, id)
}

// DeleteBlocklist removes a source and all of its domain memberships.
func (s *Store) DeleteBlocklist(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM blocklists WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete blocklist: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted blocklist count: %w", err)
	}
	if affected == 0 {
		return blocklists.ErrNotFound
	}
	return nil
}

// ReplaceBlocklistDomains replaces one source's domain snapshot.
func (s *Store) ReplaceBlocklistDomains(ctx context.Context, id int64, domains []string, checkedAt time.Time) (blocklists.List, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return blocklists.List{}, fmt.Errorf("begin blocklist replacement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		"DELETE FROM blocklist_domains WHERE blocklist_id = ?", id,
	); err != nil {
		return blocklists.List{}, fmt.Errorf("delete old blocklist domains: %w", err)
	}

	statement, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO blocklist_domains (domain, blocklist_id)
		VALUES (?, ?)
	`)
	if err != nil {
		return blocklists.List{}, fmt.Errorf("prepare blocklist domain insert: %w", err)
	}
	for _, domain := range domains {
		if _, err := statement.ExecContext(ctx, domain, id); err != nil {
			statement.Close()
			return blocklists.List{}, fmt.Errorf("insert blocklist domain: %w", err)
		}
	}
	if err := statement.Close(); err != nil {
		return blocklists.List{}, fmt.Errorf("close blocklist domain insert: %w", err)
	}

	var domainCount int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM blocklist_domains WHERE blocklist_id = ?", id,
	).Scan(&domainCount); err != nil {
		return blocklists.List{}, fmt.Errorf("count refreshed blocklist domains: %w", err)
	}

	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, `
		UPDATE blocklists
		SET last_updated_at = ?, domain_count = ?, updated_at = ?
		WHERE id = ?
	`, checkedAt.UTC().UnixMilli(), domainCount, now.UnixMilli(), id)
	if err != nil {
		return blocklists.List{}, fmt.Errorf("update refreshed blocklist: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return blocklists.List{}, fmt.Errorf("read refreshed blocklist count: %w", err)
	}
	if affected == 0 {
		return blocklists.List{}, blocklists.ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return blocklists.List{}, fmt.Errorf("commit blocklist replacement: %w", err)
	}
	return s.GetBlocklist(ctx, id)
}

// ListBlocklistDomains returns enabled domain memberships for matcher rebuilds.
func (s *Store) ListBlocklistDomains(ctx context.Context) ([]rules.BlocklistDomain, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.domain, b.id, b.name
		FROM blocklist_domains d
		JOIN blocklists b ON b.id = d.blocklist_id
		WHERE b.enabled = 1
		ORDER BY d.domain, b.id
	`)
	if err != nil {
		return nil, fmt.Errorf("query blocklist domains: %w", err)
	}
	defer rows.Close()

	entries := make([]rules.BlocklistDomain, 0)
	for rows.Next() {
		var entry rules.BlocklistDomain
		if err := rows.Scan(&entry.Domain, &entry.BlocklistID, &entry.BlocklistName); err != nil {
			return nil, fmt.Errorf("scan blocklist domain: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blocklist domains: %w", err)
	}
	return entries, nil
}

type blocklistScanner interface {
	Scan(dest ...any) error
}

func scanBlocklist(row blocklistScanner) (blocklists.List, error) {
	var (
		list        blocklists.List
		enabled     int
		lastUpdated sql.NullInt64
		createdAt   int64
		updatedAt   int64
	)
	if err := row.Scan(
		&list.ID,
		&list.Name,
		&list.URL,
		&enabled,
		&lastUpdated,
		&list.DomainCount,
		&createdAt,
		&updatedAt,
	); err != nil {
		return blocklists.List{}, err
	}

	list.Enabled = enabled == 1
	if lastUpdated.Valid {
		value := time.UnixMilli(lastUpdated.Int64).UTC()
		list.LastUpdatedAt = &value
	}
	list.CreatedAt = time.UnixMilli(createdAt).UTC()
	list.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return list, nil
}

func translateBlocklistError(operation string, err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 1555, 2067:
			return blocklists.ErrConflict
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
