// Package server routes requests by Host: the shell + API at hub.localhost,
// apps at <id>.hub.localhost, plain localhost redirects to the shell.
package server

import (
	"net"
	"net/http"
	"strings"

	"hub.localhost/platform/core/internal/config"
	"hub.localhost/platform/core/internal/gateway"
)

type Server struct {
	ShellOrigin string
	API         http.Handler
	Shell       http.Handler
	Gateway     *gateway.Gateway
}

func hostname(hostport string) string {
	h, _, err := net.SplitHostPort(hostport)
	if err != nil {
		h = hostport
	}
	return strings.ToLower(strings.Trim(h, "[]"))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := hostname(r.Host)
	switch {
	case host == config.Domain:
		if r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") {
			if !s.sameOrigin(r) {
				http.Error(w, "cross-site request refused", http.StatusForbidden)
				return
			}
			s.API.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		s.Shell.ServeHTTP(w, r)
	case strings.HasSuffix(host, "."+config.Domain):
		id := strings.TrimSuffix(host, "."+config.Domain)
		if strings.Contains(id, ".") {
			http.NotFound(w, r)
			return
		}
		s.Gateway.ServeApp(w, r, id)
	case host == "localhost" || host == "127.0.0.1" || host == "::1":
		http.Redirect(w, r, s.ShellOrigin+r.URL.RequestURI(), http.StatusFound)
	default:
		// Unknown Host: refuse, so DNS-rebinding pages can't reach the core.
		http.Error(w, "unknown host "+r.Host, http.StatusMisdirectedRequest)
	}
}

// sameOrigin blocks state-changing API calls from other sites and from app
// iframes (same-site but cross-origin). Non-browser clients send neither header.
func (s *Server) sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != s.ShellOrigin {
		return false
	}
	return true
}
