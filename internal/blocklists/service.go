package blocklists

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const defaultRefreshInterval = 24 * time.Hour

// Store persists blocklist metadata and active domain memberships.
type Store interface {
	ListBlocklists(ctx context.Context) ([]List, error)
	GetBlocklist(ctx context.Context, id int64) (List, error)
	CreateBlocklist(ctx context.Context, input Input) (List, error)
	UpdateBlocklist(ctx context.Context, id int64, input Input) (List, error)
	DeleteBlocklist(ctx context.Context, id int64) error
	ReplaceBlocklistDomains(ctx context.Context, id int64, domains []string, checkedAt time.Time) (List, error)
}

// PolicyReloader rebuilds the DNS rule snapshot.
type PolicyReloader interface {
	Reload(ctx context.Context) error
}

// Service coordinates persisted sources, HTTP refresh, and policy activation.
type Service struct {
	store           Store
	fetcher         Fetcher
	policy          PolicyReloader
	logger          *slog.Logger
	refreshInterval time.Duration

	refreshMu  sync.Mutex
	refreshing map[int64]struct{}

	startOnce sync.Once
	wg        sync.WaitGroup
}

// NewService creates a blocklist service.
func NewService(store Store, fetcher Fetcher, policy PolicyReloader, refreshInterval time.Duration, logger *slog.Logger) (*Service, error) {
	if store == nil {
		return nil, errors.New("blocklist store must not be nil")
	}
	if fetcher == nil {
		return nil, errors.New("blocklist fetcher must not be nil")
	}
	if policy == nil {
		return nil, errors.New("blocklist policy reloader must not be nil")
	}
	if refreshInterval <= 0 {
		refreshInterval = defaultRefreshInterval
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Service{
		store:           store,
		fetcher:         fetcher,
		policy:          policy,
		logger:          logger,
		refreshInterval: refreshInterval,
		refreshing:      make(map[int64]struct{}),
	}, nil
}

// List returns all configured sources.
func (s *Service) List(ctx context.Context) ([]List, error) {
	return s.store.ListBlocklists(ctx)
}

// Create validates and persists a new source.
func (s *Service) Create(ctx context.Context, input Input) (List, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return List{}, err
	}
	return s.store.CreateBlocklist(ctx, normalized)
}

// Update edits a source and republishes the policy snapshot.
func (s *Service) Update(ctx context.Context, id int64, input Input) (List, error) {
	normalized, err := normalizeInput(input)
	if err != nil {
		return List{}, err
	}
	list, err := s.store.UpdateBlocklist(ctx, id, normalized)
	if err != nil {
		return List{}, err
	}
	if err := s.reloadPolicy(ctx); err != nil {
		return List{}, fmt.Errorf("activate blocklist update: %w", err)
	}
	return list, nil
}

// Delete removes a source and republishes the policy snapshot.
func (s *Service) Delete(ctx context.Context, id int64) error {
	if err := s.store.DeleteBlocklist(ctx, id); err != nil {
		return err
	}
	if err := s.reloadPolicy(ctx); err != nil {
		return fmt.Errorf("activate blocklist deletion: %w", err)
	}
	return nil
}

// Refresh downloads and activates one source. Failed downloads leave the
// previous active domains untouched.
func (s *Service) Refresh(ctx context.Context, id int64) (List, error) {
	s.wg.Add(1)
	defer s.wg.Done()

	if !s.beginRefresh(id) {
		return List{}, ErrRefreshInProgress
	}
	defer s.endRefresh(id)

	list, err := s.store.GetBlocklist(ctx, id)
	if err != nil {
		return List{}, err
	}

	body, err := s.fetcher.Fetch(ctx, list.URL)
	if err != nil {
		if ctx.Err() != nil {
			return list, ctx.Err()
		}
		s.logger.Warn("blocklist download failed", "name", list.Name, "error", err)
		return list, fmt.Errorf("%w: %v", ErrRefresh, err)
	}

	domains, err := Parse(body)
	if err != nil {
		s.logger.Warn("blocklist parse failed", "name", list.Name, "error", err)
		return list, fmt.Errorf("%w: %v", ErrRefresh, err)
	}

	updated, err := s.store.ReplaceBlocklistDomains(ctx, id, domains, time.Now().UTC())
	if err != nil {
		return list, err
	}
	if err := s.reloadPolicy(ctx); err != nil {
		return updated, fmt.Errorf("%w: activate downloaded list: %v", ErrRefresh, err)
	}

	s.logger.Info("blocklist refreshed", "name", updated.Name, "domains", updated.DomainCount)
	return updated, nil
}

// RefreshAll refreshes every enabled source sequentially.
func (s *Service) RefreshAll(ctx context.Context) ([]RefreshResult, error) {
	lists, err := s.store.ListBlocklists(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]RefreshResult, 0, len(lists))
	for _, list := range lists {
		if !list.Enabled {
			continue
		}
		updated, refreshErr := s.Refresh(ctx, list.ID)
		result := RefreshResult{List: updated}
		if refreshErr != nil {
			result.Error = refreshErr.Error()
		}
		results = append(results, result)
	}
	return results, nil
}

// Start launches the periodic refresh loop. It refreshes stale enabled lists
// immediately, then once per configured interval.
func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			timer := time.NewTimer(0)
			defer timer.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
					s.refreshDue(ctx)
					timer.Reset(s.refreshInterval)
				}
			}
		}()
	})
}

// Wait blocks until refresh work stops or ctx expires.
func (s *Service) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) refreshDue(ctx context.Context) {
	lists, err := s.store.ListBlocklists(ctx)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Error("list blocklists for refresh", "error", err)
		}
		return
	}

	now := time.Now().UTC()
	for _, list := range lists {
		if !list.Enabled {
			continue
		}
		if list.LastUpdatedAt != nil && now.Sub(*list.LastUpdatedAt) < s.refreshInterval {
			continue
		}
		if _, err := s.Refresh(ctx, list.ID); err != nil && ctx.Err() == nil {
			s.logger.Warn("automatic blocklist refresh failed", "name", list.Name, "error", err)
		}
		if ctx.Err() != nil {
			return
		}
	}
}

func (s *Service) beginRefresh(id int64) bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if _, exists := s.refreshing[id]; exists {
		return false
	}
	s.refreshing[id] = struct{}{}
	return true
}

func (s *Service) endRefresh(id int64) {
	s.refreshMu.Lock()
	delete(s.refreshing, id)
	s.refreshMu.Unlock()
}

func (s *Service) reloadPolicy(ctx context.Context) error {
	reloadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return s.policy.Reload(reloadCtx)
}

func normalizeInput(input Input) (Input, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return Input{}, fmt.Errorf("%w: name must be between 1 and 100 characters", ErrInvalid)
	}
	address, err := normalizeListURL(input.URL)
	if err != nil {
		return Input{}, err
	}
	return Input{Name: name, URL: address, Enabled: input.Enabled}, nil
}
