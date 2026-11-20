package lifecycle

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/registry"
	"hub.localhost/platform/core/internal/runtime"
	"hub.localhost/platform/core/internal/runtime/fake"
)

func testApp(id string) *manifest.Manifest {
	return &manifest.Manifest{
		ManifestVersion: 1, ID: id, Name: id,
		Runtime: manifest.Runtime{Driver: "docker", Compose: "./compose.yaml", Service: "web", Port: 8080},
		Health:  manifest.Health{Path: "/healthz", StartTimeout: manifest.Duration{Duration: 2 * time.Second}},
	}
}

var ok = func(w http.ResponseWriter, _ *http.Request) {}

func setup(t *testing.T, f *fake.Driver, apps ...*manifest.Manifest) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mg := New(ctx, Options{
		Driver: f, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AppHost:        func(id string) string { return id + ".hub.localhost:7000" },
		HealthInterval: 10 * time.Millisecond,
	})
	mg.Sync(registry.Result{Apps: apps})
	go mg.Run(ctx)
	return mg
}

func waitFor(t *testing.T, mg *Manager, id string, want State) Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s, _ := mg.Get(id)
		if s.State == want {
			return s
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: want %s, still %s (error %q)", id, want, s.State, s.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStartStop(t *testing.T) {
	var host atomic.Value
	f := fake.New(t, func(w http.ResponseWriter, r *http.Request) { host.Store(r.Host) })
	mg := setup(t, f, testApp("notes"))

	if err := mg.Start("notes"); err != nil {
		t.Fatal(err)
	}
	s := waitFor(t, mg, "notes", Running)
	if s.Addr != f.Addr {
		t.Errorf("addr = %q, want %q", s.Addr, f.Addr)
	}
	if host.Load() != "notes.hub.localhost:7000" {
		t.Errorf("health check Host = %v", host.Load())
	}
	if err := mg.Stop("notes"); err != nil {
		t.Fatal(err)
	}
	if s := waitFor(t, mg, "notes", Stopped); s.Addr != "" {
		t.Errorf("addr kept after stop: %q", s.Addr)
	}
	if err := mg.Start("nope"); err == nil {
		t.Error("start of unknown app succeeded")
	}
}

func TestHealthTimeout(t *testing.T) {
	f := fake.New(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
	a := testApp("slow")
	a.Health.StartTimeout.Duration = 100 * time.Millisecond
	mg := setup(t, f, a)
	mg.Start("slow")
	s := waitFor(t, mg, "slow", Error)
	if !strings.Contains(s.Error, "not healthy after 100ms") || !strings.Contains(s.Error, "503") {
		t.Errorf("error = %q", s.Error)
	}
	// Error holds: no automatic stop or retry.
	time.Sleep(50 * time.Millisecond)
	if f.Stops.Load() != 0 || f.Starts.Load() != 1 {
		t.Errorf("starts=%d stops=%d after failure", f.Starts.Load(), f.Stops.Load())
	}
	// Power on from error shows progress immediately (no stale "error").
	mg.Start("slow")
	if s, _ := mg.Get("slow"); s.State != Starting || s.Error != "" {
		t.Errorf("right after Start from error: %s %q", s.State, s.Error)
	}
	waitFor(t, mg, "slow", Error)
	// Power off from error cleans up.
	mg.Stop("slow")
	if s, _ := mg.Get("slow"); s.State != Stopping {
		t.Errorf("right after Stop from error: %s", s.State)
	}
	waitFor(t, mg, "slow", Stopped)
}

func TestStopDuringStart(t *testing.T) {
	f := fake.New(t, ok)
	f.StartGate = make(chan struct{})
	mg := setup(t, f, testApp("notes"))
	mg.Start("notes")
	waitFor(t, mg, "notes", Starting)
	mg.Stop("notes")
	waitFor(t, mg, "notes", Stopped)
	if f.Stops.Load() != 1 {
		t.Errorf("stops = %d, want 1 (cleanup after cancelled start)", f.Stops.Load())
	}
}

func TestRestart(t *testing.T) {
	f := fake.New(t, ok)
	mg := setup(t, f, testApp("notes"))
	mg.Start("notes")
	waitFor(t, mg, "notes", Running)
	mg.Restart("notes")
	deadline := time.Now().Add(5 * time.Second)
	for f.Starts.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	waitFor(t, mg, "notes", Running)
	if f.Starts.Load() != 2 || f.Stops.Load() != 1 {
		t.Errorf("starts=%d stops=%d, want 2/1", f.Starts.Load(), f.Stops.Load())
	}
}

func TestCrashAndRecover(t *testing.T) {
	f := fake.New(t, ok)
	mg := setup(t, f, testApp("notes"))
	mg.Start("notes")
	waitFor(t, mg, "notes", Running)

	f.Set("hub-notes", false)
	f.EventsC <- runtime.Event{Kind: runtime.EventDied, AppID: "notes", Project: "hub-notes", Service: "web", ExitCode: "137"}
	if s := waitFor(t, mg, "notes", Error); !strings.Contains(s.Error, "exited (code 137)") {
		t.Errorf("error = %q", s.Error)
	}

	// A restart policy (or the user) brings it back: adopted without compose up.
	f.Set("hub-notes", true)
	f.EventsC <- runtime.Event{Kind: runtime.EventStarted, AppID: "notes", Project: "hub-notes", Service: "web"}
	waitFor(t, mg, "notes", Running)
	if f.Starts.Load() != 1 {
		t.Errorf("starts = %d, want 1 (adopt must not run compose up)", f.Starts.Load())
	}
}

func TestIgnoresOtherProjectsAndServices(t *testing.T) {
	f := fake.New(t, ok)
	mg := setup(t, f, testApp("notes"))
	mg.Start("notes")
	waitFor(t, mg, "notes", Running)
	f.EventsC <- runtime.Event{Kind: runtime.EventDied, AppID: "notes", Project: "hub-validate-notes", Service: "web"}
	f.EventsC <- runtime.Event{Kind: runtime.EventDied, AppID: "notes", Project: "hub-notes", Service: "db"}
	time.Sleep(20 * time.Millisecond)
	if s, _ := mg.Get("notes"); s.State != Running {
		t.Errorf("state = %s after unrelated events", s.State)
	}
}

func TestAdoptOnBoot(t *testing.T) {
	f := fake.New(t, ok)
	f.Set("hub-notes", true)
	f.Set("hub-validate-todo", true)
	mg := setup(t, f, testApp("notes"), testApp("todo"))
	waitFor(t, mg, "notes", Running)
	if s, _ := mg.Get("todo"); s.State != Stopped {
		t.Errorf("todo adopted from a validate project: %s", s.State)
	}
	if f.Starts.Load() != 0 {
		t.Errorf("adopt ran compose up")
	}
}

func TestSyncRemoval(t *testing.T) {
	f := fake.New(t, ok)
	mg := setup(t, f, testApp("notes"), testApp("todo"))
	mg.Start("notes")
	waitFor(t, mg, "notes", Running)

	mg.Sync(registry.Result{})
	if _, found := mg.Get("todo"); found {
		t.Error("stopped app not dropped")
	}
	s, found := mg.Get("notes")
	if !found || !s.Removed {
		t.Fatalf("running app should stay, flagged removed: %+v", s)
	}
	if err := mg.Start("notes"); err == nil {
		t.Error("start of removed app succeeded")
	}
	mg.Stop("notes")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, found := mg.Get("notes"); !found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("removed app not dropped after stop")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestChangesNotify(t *testing.T) {
	f := fake.New(t, ok)
	mg := setup(t, f, testApp("notes"))
	ch, cancel := mg.Changes()
	defer cancel()
	mg.Start("notes")
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no change notification")
	}
}

func setupModes(t *testing.T) (*Manager, *fake.Driver, *fake.Driver) {
	run, dev := fake.New(t, ok), fake.New(t, ok)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	withDev := testApp("notes")
	withDev.Dev = &manifest.Dev{Processes: []manifest.DevProcess{{Name: "web", Command: "x"}}}
	mg := New(ctx, Options{
		Driver: run, DevDriver: dev, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AppHost:        func(id string) string { return id + ".hub.localhost:7000" },
		HealthInterval: 10 * time.Millisecond,
	})
	mg.Sync(registry.Result{Apps: []*manifest.Manifest{withDev, testApp("plain")}})
	go mg.Run(ctx)
	return mg, run, dev
}

func TestDevMode(t *testing.T) {
	mg, run, dev := setupModes(t)
	if err := mg.StartMode("plain", ModeDev); err != ErrNoDevMode {
		t.Fatalf("dev without a dev block: %v", err)
	}

	if err := mg.StartMode("notes", ModeDev); err != nil {
		t.Fatal(err)
	}
	if s := waitFor(t, mg, "notes", Running); s.Mode != ModeDev || dev.Starts.Load() != 1 || run.Starts.Load() != 0 {
		t.Fatalf("mode=%s dev starts=%d run starts=%d", s.Mode, dev.Starts.Load(), run.Starts.Load())
	}

	// A container event for the app is stale while it runs in dev mode.
	run.EventsC <- runtime.Event{Kind: runtime.EventDied, AppID: "notes", Project: "hub-notes", Service: "web"}
	time.Sleep(20 * time.Millisecond)
	if s, _ := mg.Get("notes"); s.State != Running {
		t.Fatalf("container event broke a dev app: %s", s.State)
	}

	// Restart keeps the mode.
	mg.Restart("notes")
	for dev.Starts.Load() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if s := waitFor(t, mg, "notes", Running); s.Mode != ModeDev || run.Starts.Load() != 0 {
		t.Fatalf("restart changed mode: %s", s.Mode)
	}

	// Switching to run stops the dev instance first.
	mg.StartMode("notes", ModeRun)
	for run.Starts.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	if s := waitFor(t, mg, "notes", Running); s.Mode != ModeRun || dev.Stops.Load() != 2 {
		t.Fatalf("after switch: mode=%s dev stops=%d", s.Mode, dev.Stops.Load())
	}
}

func TestRestartIntoDev(t *testing.T) {
	mg, run, dev := setupModes(t)
	mg.Start("notes")
	waitFor(t, mg, "notes", Running)
	if err := mg.RestartMode("notes", ModeDev); err != nil {
		t.Fatal(err)
	}
	for dev.Starts.Load() < 1 {
		time.Sleep(5 * time.Millisecond)
	}
	if s := waitFor(t, mg, "notes", Running); s.Mode != ModeDev || run.Stops.Load() != 1 {
		t.Fatalf("mode=%s run stops=%d", s.Mode, run.Stops.Load())
	}
	if err := mg.RestartMode("plain", ModeDev); err != ErrNoDevMode {
		t.Errorf("plain: %v", err)
	}
}

func TestDevProcessDeathFailsApp(t *testing.T) {
	mg, _, dev := setupModes(t)
	mg.StartMode("notes", ModeDev)
	waitFor(t, mg, "notes", Running)
	dev.EventsC <- runtime.Event{Kind: runtime.EventDied, AppID: "notes", Project: "hub-notes", Service: "api", ExitCode: "2"}
	if s := waitFor(t, mg, "notes", Error); !strings.Contains(s.Error, `dev process "api" exited (code 2)`) {
		t.Errorf("error = %q", s.Error)
	}
}
