// Package shellui serves the shell's built files (embedded) or proxies a
// shell dev server.
package shellui

import (
	"embed"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

// static is filled by the shell build (`pnpm nx build @hub/shell`).
//
//go:embed all:static
var static embed.FS

// Handler serves the shell. With devURL it proxies to that dev server
// (HMR websockets included) instead.
func Handler(devURL string) (http.Handler, error) {
	if devURL != "" {
		u, err := url.Parse(devURL)
		if err != nil {
			return nil, err
		}
		return &httputil.ReverseProxy{
			Rewrite:       func(pr *httputil.ProxyRequest) { pr.SetURL(u) },
			FlushInterval: -1,
		}, nil
	}
	files, err := fs.Sub(static, "static")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(files, "index.html"); err != nil {
		return http.HandlerFunc(notBuilt), nil
	}
	fileServer := http.FileServerFS(files)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." {
			p = "index.html"
		}
		if fi, err := fs.Stat(files, p); err != nil || fi.IsDir() {
			// SPA route: let the client router handle it.
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
	}), nil
}

func notBuilt(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!doctype html><meta charset="utf-8"><title>hub</title>
<style>:root{color-scheme:light dark;font-family:system-ui,sans-serif}body{display:grid;place-items:center;min-height:100vh;margin:0}</style>
<main><h1>hub core is running</h1><p>The shell UI isn't built into this binary yet.</p>
<p>Run <code>pnpm nx build @hub/shell</code>, then rebuild the core. API: <a href="/api/apps">/api/apps</a></p></main>`))
}
