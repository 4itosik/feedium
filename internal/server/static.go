package server

import (
	"net/http"
	"path"
	"strings"
)

// NewSPAHandler wraps an http.FileSystem so that:
//   - Existing files are served directly.
//   - Paths that look like assets (have a file extension) return 404 on miss.
//   - All other missing paths fall back to index.html for client-side routing.
func NewSPAHandler(fs http.FileSystem) http.Handler {
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		f, err := fs.Open(p)
		if err == nil {
			f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}

		// If the path has a file extension, it's an asset — return 404.
		if ext := path.Ext(p); ext != "" {
			http.NotFound(w, r)
			return
		}

		// SPA fallback: serve index.html for all routes without extension.
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		fileServer.ServeHTTP(w, r2)
	})
}
