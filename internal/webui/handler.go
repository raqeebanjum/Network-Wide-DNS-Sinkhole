// Package webui serves the embedded dashboard in production builds.
package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// Embedded reports whether this binary includes the production frontend.
func Embedded() bool {
	_, ok := frontend()
	return ok
}

// Handler serves the dashboard when it is embedded, or a development notice
// when the binary was built without the frontend.
func Handler() http.Handler {
	content, ok := frontend()
	if !ok {
		return withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			_, _ = w.Write([]byte(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>Sinkhole development</title></head>
<body style="font-family:system-ui;max-width:44rem;margin:4rem auto;padding:0 1rem">
<h1>Sinkhole API is running</h1>
<p>The dashboard is not embedded in this development build.</p>
<p>Start the Vite frontend with <code>make web-dev</code> and open
<a href="http://localhost:5173">http://localhost:5173</a>.</p>
</body></html>`))
		}))
	}
	return withSecurityHeaders(newSPAHandler(content))
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; font-src 'self'; connect-src 'self'; "+
				"object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=()")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, request)
	})
}

func newSPAHandler(content fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(content))

	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		requestPath := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
		if requestPath != "" && requestPath != "index.html" {
			info, err := fs.Stat(content, requestPath)
			if err == nil && !info.IsDir() {
				if strings.HasPrefix(requestPath, "assets/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				fileServer.ServeHTTP(w, request)
				return
			}
			if path.Ext(requestPath) != "" {
				http.NotFound(w, request)
				return
			}
		}

		index, err := fs.ReadFile(content, "index.html")
		if err != nil {
			http.Error(w, "dashboard index is unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Length", strconv.Itoa(len(index)))
		w.WriteHeader(http.StatusOK)
		if request.Method != http.MethodHead {
			_, _ = w.Write(index)
		}
	})
}
