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
	Format        string
	License       string
	Homepage      string
	ETag          string
	LastModified  string
	LastCheckedAt *time.Time
	LastUpdatedAt *time.Time
	LastError     string
	DomainCount   int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Input contains normalized blocklist metadata supplied by the API.
type Input struct {
	Name     string
	URL      string
	Enabled  bool
	License  string
	Homepage string
}

// Preset is a curated, one-click blocklist source.
type Preset struct {
	Slug        string
	Name        string
	URL         string
	License     string
	Homepage    string
	Description string
}

// FetchResult is the HTTP download outcome for one list.
type FetchResult struct {
	NotModified  bool
	Body         []byte
	ETag         string
	LastModified string
}

// ParseResult is a validated set of domains from one list.
type ParseResult struct {
	Domains []string
	Format  string
	Ignored int
}

// RefreshResult summarizes one list in a refresh-all operation.
type RefreshResult struct {
	List  List
	Error string
}
