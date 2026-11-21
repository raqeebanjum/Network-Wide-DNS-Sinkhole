package rules

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Store persists rules. Implementations receive already-normalized input.
type Store interface {
	ListRules(ctx context.Context) ([]Rule, error)
	GetRule(ctx context.Context, id int64) (Rule, error)
	CreateRule(ctx context.Context, input Input) (Rule, error)
	UpdateRule(ctx context.Context, id int64, input Input) (Rule, error)
	DeleteRule(ctx context.Context, id int64) error
}

// Manager persists rule changes and atomically publishes an immutable matcher
// snapshot to DNS request handlers.
type Manager struct {
	store   Store
	matcher atomic.Pointer[Matcher]
	opMu    sync.Mutex
}

// NewManager loads the initial matcher snapshot.
func NewManager(ctx context.Context, store Store) (*Manager, error) {
	manager := &Manager{store: store}
	if err := manager.Reload(ctx); err != nil {
		return nil, err
	}
	return manager, nil
}

// Reload recompiles rules and swaps the matcher in one atomic operation.
func (m *Manager) Reload(ctx context.Context) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.reloadLocked(ctx)
}

func (m *Manager) reloadLocked(ctx context.Context) error {
	entries, err := m.store.ListRules(ctx)
	if err != nil {
		return fmt.Errorf("list rules: %w", err)
	}
	m.matcher.Store(NewMatcher(entries))
	return nil
}

// Match checks the active immutable matcher without touching SQLite.
func (m *Manager) Match(name string) (Match, bool) {
	matcher := m.matcher.Load()
	if matcher == nil {
		return Match{}, false
	}
	return matcher.Match(name)
}

// Count reports the number of active rules.
func (m *Manager) Count() int {
	matcher := m.matcher.Load()
	if matcher == nil {
		return 0
	}
	return matcher.Count()
}

// ListRules returns all persisted rules.
func (m *Manager) ListRules(ctx context.Context) ([]Rule, error) {
	return m.store.ListRules(ctx)
}

// GetRule returns one persisted rule.
func (m *Manager) GetRule(ctx context.Context, id int64) (Rule, error) {
	return m.store.GetRule(ctx, id)
}

// CreateRule validates and persists a rule, then republishes the matcher.
func (m *Manager) CreateRule(ctx context.Context, input Input) (Rule, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return Rule{}, err
	}

	m.opMu.Lock()
	defer m.opMu.Unlock()

	rule, err := m.store.CreateRule(ctx, normalized)
	if err != nil {
		return Rule{}, err
	}
	if err := m.reloadLocked(ctx); err != nil {
		return Rule{}, fmt.Errorf("activate created rule: %w", err)
	}
	return rule, nil
}

// UpdateRule replaces an existing rule and republishes the matcher.
func (m *Manager) UpdateRule(ctx context.Context, id int64, input Input) (Rule, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return Rule{}, err
	}

	m.opMu.Lock()
	defer m.opMu.Unlock()

	rule, err := m.store.UpdateRule(ctx, id, normalized)
	if err != nil {
		return Rule{}, err
	}
	if err := m.reloadLocked(ctx); err != nil {
		return Rule{}, fmt.Errorf("activate updated rule: %w", err)
	}
	return rule, nil
}

// DeleteRule removes a rule and republishes the matcher.
func (m *Manager) DeleteRule(ctx context.Context, id int64) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if err := m.store.DeleteRule(ctx, id); err != nil {
		return err
	}
	if err := m.reloadLocked(ctx); err != nil {
		return fmt.Errorf("activate deleted rule: %w", err)
	}
	return nil
}
