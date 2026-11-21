// Package blocklists downloads, parses, and manages external DNS blocklists.
package blocklists

import (
	"errors"
	"time"
)

var (
	// ErrInvalid identifies invalid list metadata or URLs.
	ErrInvalid = errors.New("invalid blocklist")
	// ErrNotFound identifies a missing blocklist.
	ErrNotFound = errors.New("blocklist not found")
	// ErrConflict identifies a duplicate blocklist URL.
	ErrConflict = errors.New("a blocklist with this URL already exists")
	// ErrRefreshInProgress identifies a list already being refreshed.
	ErrRefreshInProgress = errors.New("blocklist refresh is already in progress")
	// ErrRefresh identifies a failed download, parse, or activation.
	ErrRefresh = errors.New("blocklist refresh failed")
)

// List is persisted blocklist metadata.
type List struct {
	ID            int64
	Name          string
	URL           string
	Enabled       bool
	LastUpdatedAt *time.Time
	DomainCount   int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Input contains normalized blocklist metadata supplied by the API.
type Input struct {
	Name    string
	URL     string
	Enabled bool
}

// RefreshResult summarizes one list in a refresh-all operation.
type RefreshResult struct {
	List  List
	Error string
}
