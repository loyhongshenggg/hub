package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hub.localhost/platform/core/internal/api"
	"hub.localhost/platform/core/internal/config"
	"hub.localhost/platform/core/internal/gateway"
	"hub.localhost/platform/core/internal/lifecycle"
	"hub.localhost/platform/core/internal/registry"
	"hub.localhost/platform/core/internal/runtime"
	"hub.localhost/platform/core/internal/runtime/fake"
	"hub.localhost/platform/core/internal/server"
	"hub.localhost/platform/core/internal/shellui"
	"hub.localhost/platform/core/internal/store"
)

const notesManifest = `manifestVersion: 1
id: notes
name: Notes
icon: ./icon.svg
runtime: {driver: docker, service: web, port: 8080}
health: {path: /healthz}
`

type env struct {
	url  string // test server base URL
	mg   *lifecycle.Manager
	drv  *fake.Driver
	quit chan struct{} // closed by POST /api/system/shutdown
}

func setup(t *testing.T) *env {
	t.Helper()
	root := t.TempDir()
	appDir := filepath.Join(root, "apps", "notes")
	must(t, os.MkdirAll(appDir, 0o755))
	must(t, os.WriteFile(filepath.Join(appDir, "app.manifest.yaml"), []byte(notesManifest), 0o644))
	must(t, os.WriteFile(filepath.Join(appDir, "compose.yaml"), []byte("services: {}\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(appDir, "icon.svg"), []byte("<svg/>"), 0o644))
	broken := filepath.Join(root, "apps", "broken")
	must(t, os.MkdirAll(broken, 0o755))
	must(t, os.WriteFile(filepath.Join(broken, "app.manifest.yaml"), []byte("manifestVersion: 1\n"), 0o644))

	cfg := config.Config{Root: root, DataDir: t.TempDir(), Addr: "127.0.0.1:7000"}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	drv := fake.New(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		w.Header().Set("X-Seen-Host", r.Host)
		w.Header().Set("X-Seen-Forwarded-Host", r.Header.Get("X-Forwarded-Host"))
		io.WriteString(w, "hello from notes")
	})
	drv.LogLines = []runtime.LogLine{{Service: "web", Stream: "stdout", Text: "listening"}}
	st, err := store.Open(ctx, cfg.DBPath())
	must(t, err)
	t.Cleanup(func() { st.Close() })

	mg := lifecycle.New(ctx, lifecycle.Options{Driver: drv, Log: log, AppHost: cfg.AppHost, HealthInterval: 10 * time.Millisecond})
	res, err := registry.Scan(cfg.AppsDir())
	must(t, err)
	mg.Sync(res)
	go mg.Run(ctx)

	shell, err := shellui.Handler("")
	must(t, err)
	quit := make(chan struct{})
	srv := &server.Server{
		ShellOrigin: cfg.ShellOrigin(),
		API: (&api.API{Manager: mg, Store: st, Driver: drv, AppURL: cfg.AppURL, Log: log,
			Shutdown: func() { close(quit) }}).Handler(),
		Shell:   shell,
		Gateway: gateway.New(mg.Get, cfg.ShellOrigin(), log),
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return &env{url: ts.URL, mg: mg, drv: drv, quit: quit}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func (e *env) do(t *testing.T, method, host, path, body string, headers ...string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, e.url+path, strings.NewReader(body))
	must(t, err)
	req.Host = host
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	resp, err := noRedirect.Do(req)
	must(t, err)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

const shellHost, notesHost = "hub.localhost:7000", "notes.hub.localhost:7000"

func TestHostRouting(t *testing.T) {
	e := setup(t)
	resp, _ := e.do(t, "GET", "localhost:7000", "/x?y=1", "")
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "http://hub.localhost:7000/x?y=1" {
		t.Errorf("localhost: %d → %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := e.do(t, "GET", "evil.example:7000", "/api/apps", ""); resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("unknown host: %d", resp.StatusCode)
	}
	resp, body := e.do(t, "GET", shellHost, "/", "")
	// Either the embedded shell build or the "not built yet" page.
	if resp.StatusCode != 200 || !strings.Contains(body, "<title>hub</title>") {
		t.Errorf("shell: %d %q", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Errorf("shell CSP = %q", got)
	}
	if resp, _ := e.do(t, "GET", "nope.hub.localhost:7000", "/", ""); resp.StatusCode != 404 {
		t.Errorf("unknown app: %d", resp.StatusCode)
	}
}

func TestAPIAndGateway(t *testing.T) {
	e := setup(t)

	var list api.AppsResponse
	_, body := e.do(t, "GET", shellHost, "/api/apps", "")
	must(t, json.Unmarshal([]byte(body), &list))
	if len(list.Apps) != 1 || list.Apps[0].Status != "stopped" || list.Apps[0].URL != "http://notes.hub.localhost:7000" ||
		list.Apps[0].Icon != "/api/apps/notes/icon" || !list.Apps[0].KeepAlive {
		t.Fatalf("apps = %+v", list.Apps)
	}
	if len(list.Problems) != 1 || list.Problems[0].ID != "broken" || list.Problems[0].Path != "apps/broken/app.manifest.yaml" ||
		!strings.HasPrefix(list.Problems[0].Error, "invalid manifest:") {
		t.Errorf("problems = %+v", list.Problems)
	}

	// Off page while stopped.
	resp, body := e.do(t, "GET", notesHost, "/", "")
	if resp.StatusCode != 503 || !strings.Contains(body, "Notes is off") {
		t.Errorf("off page: %d %q", resp.StatusCode, body)
	}

	// CSRF guard: app iframes and other sites can't drive the API.
	if resp, _ := e.do(t, "POST", shellHost, "/api/apps/notes/start", "", "Sec-Fetch-Site", "same-site"); resp.StatusCode != 403 {
		t.Errorf("same-site POST: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", shellHost, "/api/apps/notes/start", "", "Origin", "http://notes.hub.localhost:7000"); resp.StatusCode != 403 {
		t.Errorf("foreign Origin POST: %d", resp.StatusCode)
	}
	resp, _ = e.do(t, "POST", shellHost, "/api/apps/notes/start", "", "Origin", "http://hub.localhost:7000", "Sec-Fetch-Site", "same-origin")
	if resp.StatusCode != 202 {
		t.Fatalf("start: %d", resp.StatusCode)
	}
	waitRunning(t, e.mg, "notes")

	// Proxied: framing headers fixed, Host preserved.
	resp, body = e.do(t, "GET", notesHost, "/anything", "")
	if resp.StatusCode != 200 || body != "hello from notes" {
		t.Fatalf("proxy: %d %q", resp.StatusCode, body)
	}
	if resp.Header.Get("X-Frame-Options") != "" {
		t.Error("X-Frame-Options not stripped")
	}
	csp := resp.Header.Values("Content-Security-Policy")
	if strings.Join(csp, " | ") != "default-src 'self' | frame-ancestors http://hub.localhost:7000" {
		t.Errorf("CSP = %q", csp)
	}
	if resp.Header.Get("X-Seen-Host") != notesHost || resp.Header.Get("X-Seen-Forwarded-Host") != notesHost {
		t.Errorf("app saw Host %q, X-Forwarded-Host %q", resp.Header.Get("X-Seen-Host"), resp.Header.Get("X-Seen-Forwarded-Host"))
	}

	// Icon is sandboxed.
	resp, body = e.do(t, "GET", shellHost, "/api/apps/notes/icon", "")
	if resp.StatusCode != 200 || body != "<svg/>" || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("icon: %d %q %q", resp.StatusCode, body, resp.Header.Get("Content-Security-Policy"))
	}

	if resp, _ := e.do(t, "POST", shellHost, "/api/apps/notes/explode", ""); resp.StatusCode != 404 {
		t.Errorf("bad action: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "POST", shellHost, "/api/apps/ghost/start", ""); resp.StatusCode != 404 {
		t.Errorf("unknown app start: %d", resp.StatusCode)
	}
}

func waitRunning(t *testing.T, mg *lifecycle.Manager, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if s, _ := mg.Get(id); s.State == lifecycle.Running {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("app never reached running")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestPrefsAndState(t *testing.T) {
	e := setup(t)
	resp, body := e.do(t, "PATCH", shellHost, "/api/apps/notes", `{"pinned":true,"position":3,"autoStart":true}`)
	var info api.AppInfo
	must(t, json.Unmarshal([]byte(body), &info))
	if resp.StatusCode != 200 || !info.Pinned || info.Position == nil || *info.Position != 3 || !info.AutoStart {
		t.Errorf("patch: %d %+v", resp.StatusCode, info)
	}
	_, body = e.do(t, "PATCH", shellHost, "/api/apps/notes", `{"autoStart":null,"position":null}`)
	info = api.AppInfo{}
	must(t, json.Unmarshal([]byte(body), &info))
	if !info.Pinned || info.Position != nil || info.AutoStart {
		t.Errorf("patch reset: %+v", info)
	}
	if resp, _ := e.do(t, "PATCH", shellHost, "/api/apps/notes", `{"colour":"red"}`); resp.StatusCode != 400 {
		t.Errorf("unknown field: %d", resp.StatusCode)
	}

	if resp, _ := e.do(t, "GET", shellHost, "/api/state/tabs", ""); resp.StatusCode != 404 {
		t.Errorf("unset state: %d", resp.StatusCode)
	}
	if resp, _ := e.do(t, "PUT", shellHost, "/api/state/tabs", `{"open":["notes"]}`); resp.StatusCode != 204 {
		t.Errorf("put state: %d", resp.StatusCode)
	}
	if _, body := e.do(t, "GET", shellHost, "/api/state/tabs", ""); body != `{"open":["notes"]}` {
		t.Errorf("state = %q", body)
	}
	if resp, _ := e.do(t, "PUT", shellHost, "/api/state/tabs", `{`); resp.StatusCode != 400 {
		t.Errorf("bad JSON state: %d", resp.StatusCode)
	}
}

// readEvents returns the first n SSE events (name + data) from path.
func readEvents(t *testing.T, e *env, path string, n int) [][2]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", e.url+path, nil)
	req.Host = shellHost
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	var out [][2]string
	sc := bufio.NewScanner(resp.Body)
	var ev [2]string
	for len(out) < n && sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			ev[0] = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			ev[1] = strings.TrimPrefix(line, "data: ")
		case line == "" && ev[0] != "":
			out = append(out, ev)
			ev = [2]string{}
		}
	}
	return out
}

func TestSSE(t *testing.T) {
	e := setup(t)
	go func() {
		time.Sleep(100 * time.Millisecond)
		e.mg.Start("notes")
	}()
	evs := readEvents(t, e, "/api/events", 2)
	if len(evs) != 2 || evs[0][0] != "apps" || !strings.Contains(evs[0][1], `"status":"stopped"`) {
		t.Fatalf("events = %v", evs)
	}
	if !strings.Contains(evs[1][1], `"id":"notes"`) {
		t.Errorf("second event = %v", evs[1])
	}

	logs := readEvents(t, e, "/api/apps/notes/logs?follow=0", 2)
	if len(logs) != 2 || logs[0][0] != "log" || !strings.Contains(logs[0][1], `"text":"listening"`) || logs[1][0] != "end" {
		t.Errorf("logs = %v", logs)
	}
}

func TestShutdown(t *testing.T) {
	e := setup(t)
	// Another site (or an app iframe) can't stop the hub.
	for _, h := range [][]string{
		{"Sec-Fetch-Site", "cross-site"},
		{"Origin", "http://evil.example"},
		{"Origin", "http://notes.hub.localhost:7000"},
	} {
		if resp, _ := e.do(t, "POST", shellHost, "/api/system/shutdown", "", h...); resp.StatusCode != http.StatusForbidden {
			t.Errorf("%v: %d, want 403", h, resp.StatusCode)
		}
	}
	select {
	case <-e.quit:
		t.Fatal("refused request still shut down")
	default:
	}
	// The CLI and the desktop launcher send no browser headers.
	if resp, _ := e.do(t, "POST", shellHost, "/api/system/shutdown", ""); resp.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown: %d", resp.StatusCode)
	}
	select {
	case <-e.quit:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown not requested")
	}
}
