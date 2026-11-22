// Command hub is the hub core: `hub serve` runs the control plane, gateway
// and shell; the other subcommands talk to it or validate an app.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"hub.localhost/platform/core/internal/api"
	"hub.localhost/platform/core/internal/client"
	"hub.localhost/platform/core/internal/config"
	"hub.localhost/platform/core/internal/gateway"
	"hub.localhost/platform/core/internal/lifecycle"
	"hub.localhost/platform/core/internal/registry"
	"hub.localhost/platform/core/internal/runtime"
	"hub.localhost/platform/core/internal/runtime/docker"
	"hub.localhost/platform/core/internal/runtime/process"
	"hub.localhost/platform/core/internal/server"
	"hub.localhost/platform/core/internal/session"
	"hub.localhost/platform/core/internal/shellui"
	"hub.localhost/platform/core/internal/store"
	"hub.localhost/platform/core/internal/validate"
)

var version = "dev" // set with -ldflags "-X main.version=…"

const usage = `hub — local multi-app platform

Usage:
  hub serve [--addr 127.0.0.1:7000] [--root DIR] [--data DIR] [--shell-dev URL] [--session] [--native]
  hub stop
  hub apps [ls]
  hub apps start|restart <id> [--dev] [--no-wait]
  hub apps stop <id> [--no-wait]
  hub apps logs <id> [-f] [--tail N]
  hub validate <id> [--keep] [--root DIR] [--data DIR]
  hub version

Environment: HUB_ADDR (default 127.0.0.1:7000), HUB_ROOT, HUB_DATA,
HUB_LOG_REQUESTS=1 (log every HTTP request).
`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		if !errors.Is(err, validate.ErrFailed) {
			fmt.Fprintln(os.Stderr, "hub:", err)
		}
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Print(usage)
		return nil
	}
	switch args[0] {
	case "serve":
		return serve(ctx, args[1:])
	case "stop":
		return stopCmd(ctx)
	case "apps":
		return apps(ctx, args[1:])
	case "validate":
		return validateCmd(ctx, args[1:])
	case "version", "--version":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	}
	return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// flags parses args where flags may come before or after positionals.
func flags(fs *flag.FlagSet, args []string) ([]string, error) {
	fs.SetOutput(io.Discard)
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, fmt.Errorf("%s: %w", fs.Name(), err)
		}
		args = fs.Args()
		if len(args) == 0 {
			return pos, nil
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
}

func baseConfig(root, data, addr string) (config.Config, error) {
	cfg := config.Config{Addr: addr, Root: root, DataDir: data}
	if cfg.Root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return cfg, err
		}
		if cfg.Root, err = config.FindRoot(wd); err != nil {
			return cfg, err
		}
	}
	if cfg.DataDir == "" {
		var err error
		if cfg.DataDir, err = config.DefaultDataDir(); err != nil {
			return cfg, err
		}
	}
	return cfg, os.MkdirAll(cfg.DataDir, 0o755)
}

func newDriver(cfg config.Config) (*docker.Driver, error) {
	return docker.New(docker.Options{OverrideDir: cfg.OverrideDir(), AppURL: cfg.AppURL, CoreURL: cfg.ShellOrigin()})
}

func serve(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", env("HUB_ADDR", "127.0.0.1:7000"), "listen address")
	root := fs.String("root", os.Getenv("HUB_ROOT"), "repo root (default: found from the current folder)")
	data := fs.String("data", os.Getenv("HUB_DATA"), "data folder (default ~/.local/share/hub)")
	shellDev := fs.String("shell-dev", "", "proxy the shell to this dev server URL instead of the embedded build")
	sess := fs.Bool("session", false, "desktop session: on exit stop all apps, and restart them on the next start")
	native := fs.Bool("native", false, "no Docker: run apps as host processes from their dev: block (macOS release, ADR-0012)")
	if _, err := flags(fs, args); err != nil {
		return err
	}
	cfg, err := baseConfig(*root, *data, *addr)
	if err != nil {
		return err
	}
	cfg.ShellDevURL = *shellDev
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer st.Close()
	devDrv := process.New(process.Options{
		StateDir: filepath.Join(cfg.DataDir, "dev"), AppURL: cfg.AppURL, CoreURL: cfg.ShellOrigin(), Log: log,
	})
	if err := devDrv.Prepare(ctx); err != nil {
		return err
	}
	defer devDrv.Close() // dev processes don't outlive the core

	// Run mode uses Docker, except with --native, where the host-process
	// driver runs every app (from its dev: block) and Docker isn't needed.
	var drv runtime.Driver = devDrv
	if !*native {
		dd, err := newDriver(cfg)
		if err != nil {
			return err
		}
		defer dd.Close()
		if err := dd.Prepare(ctx); err != nil {
			return err
		}
		drv = dd
	}

	// The manager outlives the shutdown signal so a session can still stop
	// its apps; life ends when serve returns.
	life, endLife := context.WithCancel(context.Background())
	defer endLife()
	quit := make(chan struct{})
	var quitOnce sync.Once
	requestQuit := func() { quitOnce.Do(func() { close(quit) }) }

	mg := lifecycle.New(life, lifecycle.Options{Driver: drv, DevDriver: devDrv, Log: log, AppHost: cfg.AppHost})
	res, err := registry.Scan(cfg.AppsDir())
	if err != nil {
		return err
	}
	mg.Sync(res)
	for _, p := range res.Problems {
		log.Warn("invalid app", "id", p.ID, "error", p.Error)
	}
	go mg.Run(life)
	go registry.Watch(life, cfg.AppsDir(), 2*time.Second, func(res registry.Result, err error) {
		if err != nil {
			log.Warn("scan apps", "error", err)
			return
		}
		mg.Sync(res)
	})
	if err := autostart(life, mg, st, log); err != nil {
		return err
	}
	if err := session.Restore(life, mg, st, log); err != nil {
		log.Warn("session restore", "error", err)
	}

	shell, err := shellui.Handler(cfg.ShellDevURL)
	if err != nil {
		return err
	}
	srv := &server.Server{
		ShellOrigin: cfg.ShellOrigin(),
		API:         (&api.API{Manager: mg, Store: st, Driver: drv, AppURL: cfg.AppURL, Log: log, Shutdown: requestQuit}).Handler(),
		Shell:       shell,
		Gateway:     gateway.New(mg.Get, cfg.ShellOrigin(), log),
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen %s (is another hub running?): %w", cfg.Addr, err)
	}
	var handler http.Handler = srv
	if os.Getenv("HUB_LOG_REQUESTS") == "1" {
		handler = logRequests(srv, log)
	}
	hs := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return life }, // ends SSE streams on shutdown
	}
	log.Info("hub is up", "shell", cfg.ShellOrigin(), "apps", cfg.AppsDir(), "data", cfg.DataDir, "version", version, "session", *sess)
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	case <-quit:
	}
	if *sess {
		log.Info("shutting down the session: stopping apps (they restart on the next start)")
		if n, err := session.Stop(context.Background(), mg, st, log, 2*time.Minute); err != nil {
			log.Error("session stop", "error", err)
		} else {
			log.Info("session apps stopped", "count", n)
		}
	} else {
		log.Info("shutting down (Run-mode apps keep running and are re-adopted on next start; dev processes stop)")
	}
	endLife()
	sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return hs.Shutdown(sctx)
}

func autostart(ctx context.Context, mg *lifecycle.Manager, st *store.Store, log *slog.Logger) error {
	prefs, err := st.AllPrefs(ctx)
	if err != nil {
		return err
	}
	for _, s := range mg.List() {
		on := s.App.AutoStart
		if p := prefs[s.App.ID].AutoStart; p != nil {
			on = *p
		}
		if on {
			log.Info("autostart", "app", s.App.ID)
			if err := mg.Start(s.App.ID); err != nil {
				log.Warn("autostart", "app", s.App.ID, "error", err)
			}
		}
	}
	return nil
}

func cliClient() *client.Client {
	addr := env("HUB_ADDR", "127.0.0.1:7000")
	cfg := config.Config{Addr: addr}
	return &client.Client{Addr: addr, Host: cfg.ShellHost()}
}

func apps(ctx context.Context, args []string) error {
	c := cliClient()
	if len(args) == 0 || args[0] == "ls" {
		return appsLs(ctx, c)
	}
	switch cmd := args[0]; cmd {
	case "start", "stop", "restart":
		fs := flag.NewFlagSet("apps "+cmd, flag.ContinueOnError)
		noWait := fs.Bool("no-wait", false, "return once the request is accepted")
		dev := fs.Bool("dev", false, "dev mode: run the app's dev processes on the host (live reload)")
		pos, err := flags(fs, args[1:])
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return fmt.Errorf("usage: hub apps %s <id>", cmd)
		}
		mode := ""
		if *dev {
			mode = "dev"
		}
		return appsAction(ctx, c, pos[0], cmd, mode, !*noWait)
	case "logs":
		fs := flag.NewFlagSet("apps logs", flag.ContinueOnError)
		follow := fs.Bool("f", false, "follow")
		tail := fs.Int("tail", 200, "lines from the end (0 = all)")
		pos, err := flags(fs, args[1:])
		if err != nil {
			return err
		}
		if len(pos) != 1 {
			return errors.New("usage: hub apps logs <id> [-f] [--tail N]")
		}
		return c.Logs(ctx, pos[0], *tail, *follow, func(l runtime.LogLine) {
			w := os.Stdout
			if l.Stream == "stderr" {
				w = os.Stderr
			}
			fmt.Fprintf(w, "%s | %s\n", l.Service, l.Text)
		})
	}
	return fmt.Errorf("unknown apps command %q\n\n%s", args[0], usage)
}

func appsLs(ctx context.Context, c *client.Client) error {
	r, err := c.Apps(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tSTATUS\tURL\tNAME")
	for _, a := range r.Apps {
		status := a.Status
		if a.Mode == "dev" && a.Status != "stopped" {
			status += " [dev]"
		}
		if a.Error != "" {
			status += " (" + firstLine(a.Error) + ")"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.ID, status, a.URL, a.Name)
	}
	for _, p := range r.Problems {
		fmt.Fprintf(tw, "%s\tinvalid\t-\t%s\n", p.ID, firstLine(p.Error))
	}
	return tw.Flush()
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

func appsAction(ctx context.Context, c *client.Client, id, action, mode string, wait bool) error {
	info, err := c.Action(ctx, id, action, mode)
	if err != nil {
		return err
	}
	if !wait {
		fmt.Printf("%s: %s\n", id, info.Status)
		return nil
	}
	want := lifecycle.Running
	if action == "stop" {
		want = lifecycle.Stopped
	}
	last := ""
	for {
		if info.Status != last {
			fmt.Printf("%s: %s\n", id, info.Status)
			last = info.Status
		}
		switch info.Status {
		case string(want):
			if want == lifecycle.Running {
				fmt.Printf("open %s%s\n", info.URL, info.UIPath)
			}
			return nil
		case string(lifecycle.Error):
			return fmt.Errorf("%s failed: %s", id, info.Error)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
		if info, err = c.App(ctx, id); err != nil {
			return err
		}
	}
}

func validateCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	keep := fs.Bool("keep", false, "leave the test instance running")
	root := fs.String("root", os.Getenv("HUB_ROOT"), "repo root")
	data := fs.String("data", os.Getenv("HUB_DATA"), "data folder (default ~/.local/share/hub)")
	pos, err := flags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: hub validate <id> [--keep]")
	}
	cfg, err := baseConfig(*root, *data, env("HUB_ADDR", "127.0.0.1:7000"))
	if err != nil {
		return err
	}
	drv, err := newDriver(cfg)
	if err != nil {
		return err
	}
	defer drv.Close()
	return validate.Run(ctx, validate.Options{
		Driver: drv, AppsDir: cfg.AppsDir(), ID: pos[0], AppHost: cfg.AppHost, Out: os.Stdout, Keep: *keep,
	})
}

// logRequests logs every request when HUB_LOG_REQUESTS=1 (debugging).
func logRequests(next http.Handler, log *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		log.Info("http", "method", r.Method, "host", r.Host, "path", r.URL.RequestURI(), "status", rec.status, "dur", time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int)        { s.status = code; s.ResponseWriter.WriteHeader(code) }
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// stopCmd asks a running core to exit and waits until it has.
func stopCmd(ctx context.Context) error {
	c := cliClient()
	if err := c.Shutdown(ctx); err != nil {
		return err
	}
	fmt.Println("hub: stopping…")
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if err := c.Healthy(ctx); errors.Is(err, client.ErrNotRunning) {
			fmt.Println("hub: stopped")
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return errors.New("the core is still running after 3 minutes")
}
