package session

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/lifecycle"
	"hub.localhost/platform/core/internal/registry"
	"hub.localhost/platform/core/internal/runtime/fake"
	"hub.localhost/platform/core/internal/store"
)

func testApp(id string) *manifest.Manifest {
	return &manifest.Manifest{
		ManifestVersion: 1, ID: id, Name: id,
		Runtime: manifest.Runtime{Driver: "docker", Compose: "./compose.yaml", Service: "web", Port: 8080},
		Health:  manifest.Health{Path: "/healthz", StartTimeout: manifest.Duration{Duration: 2 * time.Second}},
	}
}

func manager(t *testing.T, f *fake.Driver) *lifecycle.Manager {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mg := lifecycle.New(ctx, lifecycle.Options{
		Driver: f, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		AppHost:        func(id string) string { return id + ".hub.localhost:7000" },
		HealthInterval: 10 * time.Millisecond,
	})
	mg.Sync(registry.Result{Apps: []*manifest.Manifest{testApp("notes"), testApp("radar"), testApp("idle")}})
	go mg.Run(ctx)
	return mg
}

func waitFor(t *testing.T, mg *lifecycle.Manager, id string, want lifecycle.State) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if s, _ := mg.Get(id); s.State == want {
			return
		}
		if time.Now().After(deadline) {
			s, _ := mg.Get(id)
			t.Fatalf("%s: want %s, still %s", id, want, s.State)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStopAndRestore(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ok := func(w http.ResponseWriter, _ *http.Request) {}

	mg := manager(t, fake.New(t, ok))
	mg.Start("notes")
	mg.Start("radar")
	waitFor(t, mg, "notes", lifecycle.Running)
	waitFor(t, mg, "radar", lifecycle.Running)

	n, err := Stop(ctx, mg, st, log, 5*time.Second)
	if err != nil || n != 2 {
		t.Fatalf("stop: %d %v", n, err)
	}
	for _, id := range []string{"notes", "radar", "idle"} {
		if s, _ := mg.Get(id); s.State != lifecycle.Stopped {
			t.Errorf("%s still %s", id, s.State)
		}
	}

	// A fresh core (new manager) brings back exactly those two.
	mg2 := manager(t, fake.New(t, ok))
	if err := Restore(ctx, mg2, st, log); err != nil {
		t.Fatal(err)
	}
	waitFor(t, mg2, "notes", lifecycle.Running)
	waitFor(t, mg2, "radar", lifecycle.Running)
	if s, _ := mg2.Get("idle"); s.State != lifecycle.Stopped {
		t.Errorf("idle restored: %s", s.State)
	}
	// The record is used once.
	if left, _ := st.TakeSession(ctx); len(left) != 0 {
		t.Errorf("session record not cleared: %v", left)
	}
}
