package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandlerServesFilesAndFallback(t *testing.T) {
	t.Parallel()

	content := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>dashboard</html>")},
		"favicon.svg":   &fstest.MapFile{Data: []byte("<svg></svg>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}
	handler := newSPAHandler(content)

	tests := []struct {
		name        string
		method      string
		path        string
		status      int
		body        string
		contentType string
		cache       string
	}{
		{
			name: "index", method: http.MethodGet, path: "/", status: http.StatusOK,
			body: "dashboard", contentType: "text/html; charset=utf-8", cache: "no-store",
		},
		{
			name: "hashed asset", method: http.MethodGet, path: "/assets/app.js", status: http.StatusOK,
			body: "console.log", cache: "public, max-age=31536000, immutable",
		},
		{
			name: "public asset", method: http.MethodGet, path: "/favicon.svg", status: http.StatusOK,
			body: "<svg", cache: "no-cache",
		},
		{
			name: "client route", method: http.MethodGet, path: "/settings", status: http.StatusOK,
			body: "dashboard", contentType: "text/html; charset=utf-8", cache: "no-store",
		},
		{
			name: "missing asset", method: http.MethodGet, path: "/assets/missing.js", status: http.StatusNotFound,
		},
		{
			name: "head index", method: http.MethodHead, path: "/", status: http.StatusOK,
			contentType: "text/html; charset=utf-8", cache: "no-store",
		},
		{
			name: "post", method: http.MethodPost, path: "/", status: http.StatusMethodNotAllowed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if test.body != "" && !strings.Contains(response.Body.String(), test.body) {
				t.Errorf("body = %q, want it to contain %q", response.Body.String(), test.body)
			}
			if test.contentType != "" && response.Header().Get("Content-Type") != test.contentType {
				t.Errorf("Content-Type = %q, want %q", response.Header().Get("Content-Type"), test.contentType)
			}
			if test.cache != "" && response.Header().Get("Cache-Control") != test.cache {
				t.Errorf("Cache-Control = %q, want %q", response.Header().Get("Cache-Control"), test.cache)
			}
			if test.method == http.MethodHead && response.Body.Len() != 0 {
				t.Errorf("HEAD response body = %q, want empty", response.Body.String())
			}
		})
	}
}

func TestSPAHandlerReportsMissingIndex(t *testing.T) {
	t.Parallel()

	handler := newSPAHandler(fstest.MapFS{})
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
