package appkit

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// SPA serves a built single-page app from files (e.g. an embed.FS sub-tree):
// existing files as-is, anything else as index.html so client-side routes
// work. Vite's hashed files under assets/ are cached for a year; everything
// else is revalidated. Paths under /api/ never fall back to index.html.
func SPA(files fs.FS) http.Handler {
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if fi, err := fs.Stat(files, p); err != nil || fi.IsDir() {
			if strings.HasPrefix(p, "api/") || path.Ext(p) != "" {
				http.NotFound(w, r)
				return
			}
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			p = "index.html"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
