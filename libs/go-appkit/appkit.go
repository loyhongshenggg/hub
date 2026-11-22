// Package appkit is the optional Go starter kit for hub apps: config from
// the environment, a /healthz endpoint with checks, SPA static serving, and
// an HTTP server that shuts down cleanly when the container stops.
//
//	app := appkit.New()
//	app.Mux.HandleFunc("GET /api/hello", hello)
//	app.Mux.Handle("/", appkit.SPA(webFS))
//	app.Run()
package appkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"
)

// Config comes from the environment the hub core injects.
type Config struct {
	AppID   string // HUB_APP_ID, e.g. "notes"
	AppURL  string // HUB_APP_URL, browser URL of this app
	CoreURL string // HUB_CORE_URL, browser URL of the hub shell
	Port    string // PORT, default 8080 — must match runtime.port in the manifest
	// Bind is the address to listen on: HUB_BIND, set to 127.0.0.1 by the
	// core for host processes (Dev mode, native releases). Empty = all
	// interfaces, as a container needs.
	Bind string
	// DataDir holds persistent files: DATA_DIR, else HUB_DATA_DIR (set by
	// the core in dev mode), else /data (the volume in the app's image).
	DataDir string
}

// LoadConfig reads Config from the environment.
func LoadConfig() Config {
	return Config{
		AppID:   os.Getenv("HUB_APP_ID"),
		AppURL:  os.Getenv("HUB_APP_URL"),
		CoreURL: os.Getenv("HUB_CORE_URL"),
		Port:    Env("PORT", "8080"),
		Bind:    os.Getenv("HUB_BIND"),
		DataDir: Env("DATA_DIR", Env("HUB_DATA_DIR", "/data")),
	}
}

// Env returns the environment variable key, or def when unset or empty.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Check reports whether a dependency (database, cache…) is ready.
type Check func(ctx context.Context) error

// App bundles config, a logger, a mux with /healthz, and Run.
type App struct {
	Config Config
	Log    *slog.Logger
	Mux    *http.ServeMux

	// HealthPath is where health is served; it must match health.path in
	// app.manifest.yaml. Default /healthz.
	HealthPath string
	// ShutdownTimeout bounds graceful shutdown. Docker sends SIGKILL 10s
	// after SIGTERM by default, so keep this below that. Default 8s.
	ShutdownTimeout time.Duration

	mu     sync.Mutex
	checks map[string]Check
}

// New returns an App configured from the environment. /healthz is
// registered when Run starts, so HealthPath can still be changed.
func New() *App {
	return &App{
		Config:          LoadConfig(),
		Log:             slog.New(slog.NewTextHandler(os.Stdout, nil)),
		Mux:             http.NewServeMux(),
		HealthPath:      "/healthz",
		ShutdownTimeout: 8 * time.Second,
		checks:          map[string]Check{},
	}
}

// AddCheck makes /healthz fail (503) while check returns an error.
func (a *App) AddCheck(name string, check Check) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.checks[name] = check
}

// HealthHandler runs every check (2s budget) and answers 200 or 503 with
// {"status": "ok"|"fail", "checks": {name: "ok"|error}}.
func (a *App) HealthHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		a.mu.Lock()
		names := make([]string, 0, len(a.checks))
		for n := range a.checks {
			names = append(names, n)
		}
		checks := a.checks
		a.mu.Unlock()
		sort.Strings(names)

		status, code := "ok", http.StatusOK
		results := map[string]string{}
		for _, n := range names {
			if err := checks[n](ctx); err != nil {
				results[n] = err.Error()
				status, code = "fail", http.StatusServiceUnavailable
			} else {
				results[n] = "ok"
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "checks": results})
	})
}

// Run serves the app on :PORT until SIGINT/SIGTERM, then drains requests.
func (a *App) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return a.Serve(ctx, net.JoinHostPort(a.Config.Bind, a.Config.Port))
}

// Serve is Run with an explicit context and address (handy in tests).
func (a *App) Serve(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return a.serve(ctx, ln)
}

func (a *App) serve(ctx context.Context, ln net.Listener) error {
	mux := http.NewServeMux()
	mux.Handle("GET "+a.HealthPath, a.HealthHandler())
	mux.Handle("/", a.Mux)
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	a.Log.Info("listening", "addr", ln.Addr().String(), "app", a.Config.AppID, "url", a.Config.AppURL)
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	a.Log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), a.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
