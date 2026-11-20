// Package gateway reverse-proxies <id>.hub.localhost to the app's container,
// only while the app is running (ADR-0006).
package gateway

import (
	"html/template"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"hub.localhost/platform/core/internal/lifecycle"
)

type Gateway struct {
	Lookup      func(id string) (lifecycle.Snapshot, bool)
	ShellOrigin string // only the shell may frame apps
	Log         *slog.Logger
	Transport   http.RoundTripper
}

func New(lookup func(string) (lifecycle.Snapshot, bool), shellOrigin string, log *slog.Logger) *Gateway {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = 0 // apps may stream
	t.IdleConnTimeout = 90 * time.Second
	return &Gateway{Lookup: lookup, ShellOrigin: shellOrigin, Log: log, Transport: t}
}

// ServeApp handles a request for app id.
func (g *Gateway) ServeApp(w http.ResponseWriter, r *http.Request, id string) {
	s, ok := g.Lookup(id)
	if !ok {
		g.page(w, http.StatusNotFound, pageData{Title: "No such app", Message: "There is no app called “" + id + "”.", Shell: g.ShellOrigin})
		return
	}
	if s.State != lifecycle.Running || s.Addr == "" {
		g.offPage(w, s)
		return
	}
	target := &url.URL{Scheme: "http", Host: s.Addr}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = pr.In.Host // apps see their public host (CSRF/origin checks)
			pr.SetXForwarded()
		},
		Transport:      g.Transport,
		FlushInterval:  -1,
		ModifyResponse: func(resp *http.Response) error { AllowFraming(resp.Header, g.ShellOrigin); return nil },
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			g.Log.Warn("gateway", "app", id, "path", r.URL.Path, "error", err)
			g.page(w, http.StatusBadGateway, pageData{Title: s.App.Name + " is not responding", Message: err.Error(), Shell: g.ShellOrigin})
		},
	}
	proxy.ServeHTTP(w, r)
}

// AllowFraming removes app headers that block the shell's iframe
// (X-Frame-Options, CSP frame-ancestors) and allows only the shell to frame.
func AllowFraming(h http.Header, shellOrigin string) {
	h.Del("X-Frame-Options")
	for _, name := range []string{"Content-Security-Policy", "Content-Security-Policy-Report-Only"} {
		vals := h.Values(name)
		h.Del(name)
		for _, v := range vals {
			if v = stripDirective(v, "frame-ancestors"); v != "" {
				h.Add(name, v)
			}
		}
	}
	h.Add("Content-Security-Policy", "frame-ancestors "+shellOrigin)
}

func stripDirective(policy, directive string) string {
	var keep []string
	for _, d := range strings.Split(policy, ";") {
		d = strings.TrimSpace(d)
		name, _, _ := strings.Cut(d, " ")
		if d != "" && !strings.EqualFold(name, directive) {
			keep = append(keep, d)
		}
	}
	return strings.Join(keep, "; ")
}

func (g *Gateway) offPage(w http.ResponseWriter, s lifecycle.Snapshot) {
	d := pageData{Title: s.App.Name + " is off", Message: "Turn it on from the hub.", Shell: g.ShellOrigin}
	switch s.State {
	case lifecycle.Starting:
		d.Title, d.Message, d.Refresh = s.App.Name+" is starting…", "This page reloads when it's ready.", true
	case lifecycle.Stopping:
		d.Title = s.App.Name + " is stopping…"
	case lifecycle.Error:
		d.Title, d.Message = s.App.Name+" failed", s.Error
	}
	w.Header().Set("Retry-After", "2")
	g.page(w, http.StatusServiceUnavailable, d)
}

type pageData struct {
	Title, Message, Shell string
	Refresh               bool
}

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>{{.Title}}</title>
{{if .Refresh}}<meta http-equiv="refresh" content="2">{{end}}
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>
:root{color-scheme:light dark;font-family:system-ui,sans-serif}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:Canvas;color:CanvasText}
main{max-width:36rem;padding:2rem;text-align:center}
h1{font-size:1.4rem;margin:0 0 .5rem}
p{opacity:.75;white-space:pre-wrap;word-break:break-word}
a{color:inherit}
</style></head>
<body><main><h1>{{.Title}}</h1><p>{{.Message}}</p><p><a href="{{.Shell}}" target="_top">Open the hub</a></p></main></body></html>
`))

func (g *Gateway) page(w http.ResponseWriter, status int, d pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	AllowFraming(w.Header(), g.ShellOrigin)
	w.WriteHeader(status)
	_ = pageTmpl.Execute(w, d)
}
