package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/runtime"
)

// Runs against the real Docker engine: HUB_DOCKER_TESTS=1 go test ./...
func TestDockerDriver(t *testing.T) {
	if os.Getenv("HUB_DOCKER_TESTS") != "1" {
		t.Skip("set HUB_DOCKER_TESTS=1 to run against Docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	m, err := manifest.Load(filepath.Join("testdata", "apps", "echo", manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	d, err := New(Options{
		OverrideDir: t.TempDir(),
		AppURL:      func(id string) string { return "http://" + id + ".hub.localhost:7000" },
		CoreURL:     "http://hub.localhost:7000",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	in := runtime.Instance{App: m, Project: "hub-test-echo"}
	t.Cleanup(func() { _ = d.Stop(context.Background(), in, runtime.StopOptions{RemoveVolumes: true}) })

	evs, _ := d.Events(ctx)
	if err := d.Start(ctx, in, runtime.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	st, err := d.Status(ctx, in)
	if err != nil || !st.Running || !strings.HasSuffix(st.Addr, ":8080") {
		t.Fatalf("status = %+v, %v", st, err)
	}
	waitEvent(t, evs, runtime.EventStarted, "web")

	ris, err := d.Running(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ri := range ris {
		found = found || (ri.AppID == "echo" && ri.Project == "hub-test-echo")
	}
	if !found {
		t.Errorf("Running() = %+v, missing echo", ris)
	}

	// Sidecar reaches web over the default network; logs cover every service.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if hasLine(t, ctx, d, in.Project, "sidecar", "SIDECAR_OK") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sidecar never reached web over the default network")
		}
		time.Sleep(300 * time.Millisecond)
	}

	if err := d.Stop(ctx, in, runtime.StopOptions{RemoveVolumes: true}); err != nil {
		t.Fatal(err)
	}
	waitEvent(t, evs, runtime.EventDied, "web")
	if st, _ := d.Status(ctx, in); st.Running {
		t.Error("still running after Stop")
	}
}

func hasLine(t *testing.T, ctx context.Context, d *Driver, project, service, text string) bool {
	lines, err := d.Logs(ctx, project, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	for l := range lines {
		ok = ok || (l.Service == service && strings.Contains(l.Text, text))
	}
	return ok
}

func waitEvent(t *testing.T, evs <-chan runtime.Event, kind runtime.EventKind, service string) {
	t.Helper()
	timeout := time.After(30 * time.Second)
	for {
		select {
		case ev := <-evs:
			if ev.Kind == kind && ev.Service == service && ev.AppID == "echo" && ev.Project == "hub-test-echo" {
				return
			}
		case <-timeout:
			t.Fatalf("no %s event for %s", kind, service)
		}
	}
}
