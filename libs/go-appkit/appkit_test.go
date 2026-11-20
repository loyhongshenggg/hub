package appkit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestHealth(t *testing.T) {
	a := New()
	rec := httptest.NewRecorder()
	a.HealthHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("no checks: %d %s", rec.Code, rec.Body)
	}

	a.AddCheck("db", func(context.Context) error { return errors.New("connection refused") })
	a.AddCheck("cache", func(context.Context) error { return nil })
	rec = httptest.NewRecorder()
	a.HealthHandler().ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	var body struct {
		Status string
		Checks map[string]string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 503 || body.Status != "fail" || body.Checks["db"] != "connection refused" || body.Checks["cache"] != "ok" {
		t.Errorf("failing check: %d %+v", rec.Code, body)
	}
}

func TestSPA(t *testing.T) {
	files := fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html>app")},
		"assets/app-1.js":  {Data: []byte("js")},
		"favicon.svg":      {Data: []byte("<svg/>")},
		"nested/page.html": {Data: []byte("page")},
	}
	h := SPA(files)
	cases := []struct {
		path, body, cache string
		code              int
	}{
		{"/", "<!doctype html>app", "no-cache", 200},
		{"/notes/42", "<!doctype html>app", "no-cache", 200}, // client route
		{"/assets/app-1.js", "js", "public, max-age=31536000, immutable", 200},
		{"/favicon.svg", "<svg/>", "no-cache", 200},
		{"/missing.js", "", "", 404},
		{"/api/nope", "", "", 404},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", c.path, nil))
		if rec.Code != c.code {
			t.Errorf("%s: status %d, want %d", c.path, rec.Code, c.code)
			continue
		}
		if c.code == 200 && (rec.Body.String() != c.body || rec.Header().Get("Cache-Control") != c.cache) {
			t.Errorf("%s: body %q cache %q", c.path, rec.Body, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestServeAndShutdown(t *testing.T) {
	a := New()
	a.Mux.HandleFunc("GET /api/hi", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "hi") })
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.serve(ctx, ln) }()

	base := "http://" + ln.Addr().String()
	for _, p := range []string{"/healthz", "/api/hi"} {
		resp, err := http.Get(base + p)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: %d", p, resp.StatusCode)
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no shutdown")
	}
}

func TestEnv(t *testing.T) {
	t.Setenv("HUB_APP_ID", "notes")
	t.Setenv("PORT", "")
	c := LoadConfig()
	if c.AppID != "notes" || c.Port != "8080" {
		t.Errorf("config = %+v", c)
	}
}
