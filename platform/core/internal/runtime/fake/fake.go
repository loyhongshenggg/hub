// Package fake is an in-memory runtime.Driver for tests: "running" an app
// points it at a test HTTP server.
package fake

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"hub.localhost/platform/core/internal/runtime"
)

type Driver struct {
	Addr      string        // where every running app "listens"
	StartGate chan struct{} // if set, Start blocks until closed or ctx ends
	Starts    atomic.Int32
	Stops     atomic.Int32
	EventsC   chan runtime.Event
	LogLines  []runtime.LogLine

	mu      sync.Mutex
	running map[string]bool // project → up
}

// New returns a driver whose apps are served by handler.
func New(t testing.TB, handler http.HandlerFunc) *Driver {
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Driver{Addr: strings.TrimPrefix(srv.URL, "http://"), running: map[string]bool{}, EventsC: make(chan runtime.Event)}
}

func (f *Driver) Prepare(context.Context) error { return nil }

func (f *Driver) Start(ctx context.Context, in runtime.Instance, _ runtime.StartOptions) error {
	f.Starts.Add(1)
	if f.StartGate != nil {
		select {
		case <-f.StartGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.Set(in.Project, true)
	return nil
}

func (f *Driver) Stop(_ context.Context, in runtime.Instance, _ runtime.StopOptions) error {
	f.Stops.Add(1)
	f.Set(in.Project, false)
	return nil
}

// Set marks a compose project as up or down.
func (f *Driver) Set(project string, up bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running[project] = up
}

func (f *Driver) Status(_ context.Context, in runtime.Instance) (runtime.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.running[in.Project] {
		return runtime.Status{Running: true, Addr: f.Addr, State: "running"}, nil
	}
	return runtime.Status{State: "exited"}, nil
}

func (f *Driver) Running(context.Context) ([]runtime.RunningInstance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []runtime.RunningInstance
	for p, up := range f.running {
		if up {
			out = append(out, runtime.RunningInstance{AppID: strings.TrimPrefix(p, "hub-"), Project: p})
		}
	}
	return out, nil
}

func (f *Driver) Events(context.Context) (<-chan runtime.Event, <-chan error) {
	return f.EventsC, make(chan error)
}

func (f *Driver) Logs(context.Context, string, int, bool) (<-chan runtime.LogLine, error) {
	ch := make(chan runtime.LogLine, len(f.LogLines))
	for _, l := range f.LogLines {
		ch <- l
	}
	close(ch)
	return ch, nil
}
