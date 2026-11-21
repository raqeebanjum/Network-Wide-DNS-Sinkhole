package blocklists

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultMaxDownloadSize = 32 << 20
	defaultFetchTimeout    = 30 * time.Second
)

// Fetcher downloads a blocklist and supports conditional requests.
type Fetcher interface {
	Fetch(ctx context.Context, address, etag, lastModified string) (FetchResult, error)
}

// HTTPFetcher fetches blocklists over HTTPS with a hard size limit.
type HTTPFetcher struct {
	client    *http.Client
	maxBytes  int64
	userAgent string
}

// NewHTTPFetcher creates a fetcher with production timeout and redirect rules.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		client: &http.Client{
			Timeout: defaultFetchTimeout,
			CheckRedirect: func(request *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				if request.URL.Scheme != "https" {
					return fmt.Errorf("redirect target must use HTTPS")
				}
				if request.URL.User != nil {
					return fmt.Errorf("redirect target must not include credentials")
				}
				return nil
			},
		},
		maxBytes:  defaultMaxDownloadSize,
		userAgent: "network-wide-dns-sinkhole/0.1",
	}
}

// Fetch performs a conditional GET. A 304 response is returned as NotModified.
func (f *HTTPFetcher) Fetch(ctx context.Context, address, etag, lastModified string) (FetchResult, error) {
	if _, err := normalizeListURL(address); err != nil {
		return FetchResult{}, err
	}
	maxBytes := f.maxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxDownloadSize
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return FetchResult{}, fmt.Errorf("create blocklist request: %w", err)
	}
	request.Header.Set("User-Agent", f.userAgent)
	request.Header.Set("Accept", "text/plain, text/*;q=0.9, */*;q=0.1")
	if etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		request.Header.Set("If-Modified-Since", lastModified)
	}

	response, err := f.client.Do(request)
	if err != nil {
		return FetchResult{}, fmt.Errorf("download blocklist: %w", err)
	}
	defer response.Body.Close()

	result := FetchResult{
		ETag:         response.Header.Get("ETag"),
		LastModified: response.Header.Get("Last-Modified"),
	}
	if response.StatusCode == http.StatusNotModified {
		if result.ETag == "" {
			result.ETag = etag
		}
		if result.LastModified == "" {
			result.LastModified = lastModified
		}
		result.NotModified = true
		return result, nil
	}
	if response.StatusCode != http.StatusOK {
		return FetchResult{}, fmt.Errorf("download blocklist: upstream returned %s", response.Status)
	}
	if response.ContentLength > maxBytes {
		return FetchResult{}, fmt.Errorf("download blocklist: response exceeds %d bytes", maxBytes)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return FetchResult{}, fmt.Errorf("read blocklist: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return FetchResult{}, fmt.Errorf("download blocklist: response exceeds %d bytes", maxBytes)
	}
	result.Body = body
	return result, nil
}
