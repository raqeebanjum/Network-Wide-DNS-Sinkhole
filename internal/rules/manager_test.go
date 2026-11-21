package rules_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/rules"
	"github.com/raqeebanjum/network-wide-dns-sinkhole/internal/store"
)

func TestManagerReloadsAndPersistsRules(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "sinkhole.db")

	first := openStore(t, databasePath)
	manager, err := rules.NewManager(ctx, first)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	deny, err := manager.CreateRule(ctx, rules.Input{
		Domain: "Ads.Example.com.",
		Kind:   rules.KindDeny,
		Note:   "manual block",
	})
	if err != nil {
		t.Fatalf("create deny rule: %v", err)
	}
	if deny.Domain != "ads.example.com" {
		t.Errorf("stored domain = %q, want normalized", deny.Domain)
	}

	if _, err := manager.CreateRule(ctx, rules.Input{
		Domain: "safe.ads.example.com",
		Kind:   rules.KindAllow,
	}); err != nil {
		t.Fatalf("create allow rule: %v", err)
	}

	match, ok := manager.Match("tracker.ads.example.com.")
	if !ok || match.Rule.Kind != rules.KindDeny {
		t.Fatalf("tracker match = %#v, %v", match, ok)
	}
	match, ok = manager.Match("safe.ads.example.com")
	if !ok || match.Rule.Kind != rules.KindAllow {
		t.Fatalf("allow match = %#v, %v", match, ok)
	}
	if manager.Count() != 2 {
		t.Errorf("Count = %d, want 2", manager.Count())
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	second := openStore(t, databasePath)
	reloaded, err := rules.NewManager(ctx, second)
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	if _, ok := reloaded.Match("x.ads.example.com"); !ok {
		t.Fatal("deny rule did not survive restart")
	}
	if err := reloaded.DeleteRule(ctx, deny.ID); err != nil {
		t.Fatalf("delete deny rule: %v", err)
	}
	if _, ok := reloaded.Match("x.ads.example.com"); ok {
		t.Fatal("deleted deny rule is still active")
	}
}

func TestManagerRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	database := openStore(t, filepath.Join(t.TempDir(), "sinkhole.db"))
	manager, err := rules.NewManager(ctx, database)
	if err != nil {
		t.Fatalf("create manager: %v", err)
	}

	_, err = manager.CreateRule(ctx, rules.Input{
		Domain: "*.example.com",
		Kind:   rules.KindDeny,
	})
	if !errors.Is(err, rules.ErrInvalid) {
		t.Fatalf("create invalid rule error = %v, want ErrInvalid", err)
	}
	if manager.Count() != 0 {
		t.Errorf("Count = %d after invalid creation, want 0", manager.Count())
	}
}

func openStore(t *testing.T, path string) *store.Store {
	t.Helper()

	database, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return database
}
