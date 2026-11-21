package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
)

func TestRuleCRUDAndPersistence(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "sinkhole.db")
	first := openTestStore(t, databasePath)
	ctx := context.Background()

	created, err := first.CreateRule(ctx, rules.Input{
		Domain: "ads.example.com",
		Kind:   rules.KindDeny,
		Note:   "test deny rule",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if created.ID == 0 {
		t.Error("created rule has zero ID")
	}
	if created.Domain != "ads.example.com" {
		t.Errorf("domain = %q", created.Domain)
	}
	if created.Kind != rules.KindDeny {
		t.Errorf("kind = %q", created.Kind)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Error("created timestamps are zero")
	}

	if _, err := first.CreateRule(ctx, rules.Input{
		Domain: "ads.example.com",
		Kind:   rules.KindAllow,
	}); !errors.Is(err, ErrRuleConflict) {
		t.Fatalf("duplicate create error = %v, want ErrRuleConflict", err)
	}

	updated, err := first.UpdateRule(ctx, created.ID, rules.Input{
		Domain: "tracker.example.com",
		Kind:   rules.KindAllow,
		Note:   "updated",
	})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if updated.Domain != "tracker.example.com" || updated.Kind != rules.KindAllow {
		t.Errorf("updated rule = %#v", updated)
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Errorf("UpdatedAt = %s, want at or after %s", updated.UpdatedAt, created.UpdatedAt)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	reopened := openTestStore(t, databasePath)
	persisted, err := reopened.GetRule(ctx, created.ID)
	if err != nil {
		t.Fatalf("get persisted rule: %v", err)
	}
	if persisted.Domain != "tracker.example.com" || persisted.Kind != rules.KindAllow {
		t.Errorf("persisted rule = %#v", persisted)
	}

	if err := reopened.DeleteRule(ctx, created.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	if _, err := reopened.GetRule(ctx, created.ID); !errors.Is(err, ErrRuleNotFound) {
		t.Fatalf("get deleted rule error = %v, want ErrRuleNotFound", err)
	}
}

func TestMigrationsRunOnce(t *testing.T) {
	t.Parallel()

	databasePath := filepath.Join(t.TempDir(), "sinkhole.db")
	first := openTestStore(t, databasePath)

	embedded, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read embedded migrations: %v", err)
	}
	expected := len(embedded)

	var version int
	if err := first.db.QueryRow(
		"SELECT MAX(version) FROM schema_migrations",
	).Scan(&version); err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != expected {
		t.Errorf("migration version = %d, want %d", version, expected)
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first database: %v", err)
	}

	second := openTestStore(t, databasePath)
	var count int
	if err := second.db.QueryRow(
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != expected {
		t.Errorf("migration count = %d, want %d", count, expected)
	}
}

func TestSQLiteConfiguration(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))

	var foreignKeys int
	if err := database.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var journalMode string
	if err := database.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal", journalMode)
	}
}

func TestUpdateAndDeleteMissingRule(t *testing.T) {
	t.Parallel()

	database := openTestStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	ctx := context.Background()

	if _, err := database.UpdateRule(ctx, 999, rules.Input{
		Domain: "missing.example",
		Kind:   rules.KindDeny,
	}); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("update missing rule error = %v, want ErrRuleNotFound", err)
	}
	if err := database.DeleteRule(ctx, 999); !errors.Is(err, ErrRuleNotFound) {
		t.Errorf("delete missing rule error = %v, want ErrRuleNotFound", err)
	}
}

func openTestStore(t *testing.T, path string) *Store {
	t.Helper()

	database, err := Open(path)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	return database
}
