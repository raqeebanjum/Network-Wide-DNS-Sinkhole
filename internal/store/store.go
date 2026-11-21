package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var (
	// ErrRuleNotFound is returned when a requested rule does not exist.
	ErrRuleNotFound = errors.New("rule not found")
	// ErrRuleConflict is returned when a domain already has a rule.
	ErrRuleConflict = errors.New("a rule for this domain already exists")
)

// Store provides persistent application state backed by SQLite.
type Store struct {
	db *sql.DB
}

// Open creates or opens the database, applies migrations, and configures
// SQLite for the single-instance application.
func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path must not be empty")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	// SQLite PRAGMAs are connection-scoped. Keep one connection until they are
	// moved into the DSN for a larger pool.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("connect to SQLite: %w", err)
	}

	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
	} {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure SQLite with %q: %w", pragma, err)
		}
	}

	if err := s.migrate(ctx); err != nil {
		return fmt.Errorf("migrate SQLite database: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := migrationVersion(entry.Name())
		if err != nil {
			return err
		}

		applied, err := s.migrationApplied(ctx, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if err := s.applyMigration(ctx, version, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (s *Store) migrationApplied(ctx context.Context, version int) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations WHERE version = ?",
		version,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("check migration %d: %w", version, err)
	}
	return count > 0, nil
}

func (s *Store) applyMigration(ctx context.Context, version int, body string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		version, time.Now().UTC().UnixMilli(),
	); err != nil {
		return err
	}
	return tx.Commit()
}

func migrationVersion(name string) (int, error) {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return 0, fmt.Errorf("migration %q must start with a numeric version", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("migration %q has an invalid version", name)
	}
	return version, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// ListRules returns all rules ordered by domain.
func (s *Store) ListRules(ctx context.Context) ([]rules.Rule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, domain, kind, note, created_at, updated_at
		FROM rules
		ORDER BY domain, id
	`)
	if err != nil {
		return nil, fmt.Errorf("query rules: %w", err)
	}
	defer rows.Close()

	entries := make([]rules.Rule, 0)
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	return entries, nil
}

// GetRule returns one rule by ID.
func (s *Store) GetRule(ctx context.Context, id int64) (rules.Rule, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, domain, kind, note, created_at, updated_at
		FROM rules
		WHERE id = ?
	`, id)
	rule, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return rules.Rule{}, ErrRuleNotFound
	}
	return rule, err
}

// CreateRule inserts a normalized rule.
func (s *Store) CreateRule(ctx context.Context, input rules.Input) (rules.Rule, error) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO rules (domain, kind, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, input.Domain, input.Kind, input.Note, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return rules.Rule{}, translateRuleError("create rule", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return rules.Rule{}, fmt.Errorf("read created rule ID: %w", err)
	}
	return s.GetRule(ctx, id)
}

// UpdateRule replaces a rule's mutable fields.
func (s *Store) UpdateRule(ctx context.Context, id int64, input rules.Input) (rules.Rule, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE rules
		SET domain = ?, kind = ?, note = ?, updated_at = ?
		WHERE id = ?
	`, input.Domain, input.Kind, input.Note, time.Now().UTC().UnixMilli(), id)
	if err != nil {
		return rules.Rule{}, translateRuleError("update rule", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return rules.Rule{}, fmt.Errorf("read updated rule count: %w", err)
	}
	if affected == 0 {
		return rules.Rule{}, ErrRuleNotFound
	}
	return s.GetRule(ctx, id)
}

// DeleteRule removes one rule by ID.
func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM rules WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read deleted rule count: %w", err)
	}
	if affected == 0 {
		return ErrRuleNotFound
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRule(row rowScanner) (rules.Rule, error) {
	var (
		rule      rules.Rule
		kind      string
		createdAt int64
		updatedAt int64
	)
	if err := row.Scan(
		&rule.ID,
		&rule.Domain,
		&kind,
		&rule.Note,
		&createdAt,
		&updatedAt,
	); err != nil {
		return rules.Rule{}, err
	}

	rule.Kind = rules.Kind(kind)
	rule.CreatedAt = time.UnixMilli(createdAt).UTC()
	rule.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	return rule, nil
}

func translateRuleError(operation string, err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 1555, 2067: // SQLITE_CONSTRAINT_PRIMARYKEY, SQLITE_CONSTRAINT_UNIQUE
			return ErrRuleConflict
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
