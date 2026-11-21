package blocklists

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPFetcherConditionalRequest(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("If-None-Match") == `"fixture-v1"` {
			w.Header().Set("ETag", `"fixture-v1"`)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"fixture-v1"`)
		w.Header().Set("Last-Modified", "Sat, 21 Nov 2026 00:00:00 GMT")
		_, _ = w.Write([]byte("0.0.0.0 ads.example\n"))
	}))
	t.Cleanup(server.Close)

	fetcher := &HTTPFetcher{
		client:    server.Client(),
		maxBytes:  1024,
		userAgent: "sinkhole-test",
	}
	ctx := context.Background()

	result, err := fetcher.Fetch(ctx, server.URL, "", "")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if result.NotModified || string(result.Body) != "0.0.0.0 ads.example\n" {
		t.Errorf("first result = %#v", result)
	}
	if result.ETag != `"fixture-v1"` || result.LastModified == "" {
		t.Errorf("validators = %q, %q", result.ETag, result.LastModified)
	}

	unchanged, err := fetcher.Fetch(ctx, server.URL, result.ETag, result.LastModified)
	if err != nil {
		t.Fatalf("conditional fetch: %v", err)
	}
	if !unchanged.NotModified {
		t.Errorf("conditional result = %#v, want NotModified", unchanged)
	}
	if unchanged.ETag != `"fixture-v1"` {
		t.Errorf("ETag = %q, want fixture-v1", unchanged.ETag)
	}
}

func TestHTTPFetcherLimitsResponseSize(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("response is too large"))
	}))
	t.Cleanup(server.Close)

	fetcher := &HTTPFetcher{client: server.Client(), maxBytes: 4}
	_, err := fetcher.Fetch(context.Background(), server.URL, "", "")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Fetch error = %v, want size limit error", err)
	}
}

func TestHTTPFetcherRejectsInsecureURL(t *testing.T) {
	t.Parallel()

	fetcher := &HTTPFetcher{maxBytes: 1024}
	_, err := fetcher.Fetch(context.Background(), "http://example.com/list.txt", "", "")
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("Fetch error = %v, want HTTPS validation error", err)
	}
}

func TestHTTPFetcherReportsHTTPFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	fetcher := &HTTPFetcher{client: server.Client(), maxBytes: 1024}
	_, err := fetcher.Fetch(context.Background(), server.URL, "", "")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("Fetch error = %v, want status error", err)
	}
}
