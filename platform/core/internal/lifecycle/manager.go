// Package lifecycle owns each app's on/off state machine:
//
//	stopped → starting → running → stopping → stopped, plus error.
//
// Every app has a worker goroutine that drives its actual state toward the
// desired one, so overlapping start/stop/restart requests resolve in order.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/broker"
	"hub.localhost/platform/core/internal/registry"
	"hub.localhost/platform/core/internal/runtime"
)

type State string

const (
	Stopped  State = "stopped"
	Starting State = "starting"
	Running  State = "running"
	Stopping State = "stopping"
	Error    State = "error"
)

var ErrUnknownApp = errors.New("unknown app")

// Snapshot is the public view of one app.
type Snapshot struct {
	App   *manifest.Manifest
	State State
	Error string
	Since time.Time
	Addr  string // gateway target, set while Running
	// Removed: the manifest is gone; the app disappears once stopped.
	Removed bool
}

type Options struct {
	Driver runtime.Driver
	Log    *slog.Logger
	// AppHost returns the Host header an app sees, e.g. notes.hub.localhost:7000.
	AppHost func(id string) string
	// Health tuning (tests); zero values use HealthCheck defaults.
	HealthClient   *http.Client
	HealthInterval time.Duration
}

type Manager struct {
	opts    Options
	ctx     context.Context
	changes *broker.Broker[struct{}]

	mu       sync.Mutex
	apps     map[string]*app
	problems []registry.Problem
}

type app struct {
	m       *manifest.Manifest
	state   State
	err     string
	since   time.Time
	addr    string
	removed bool
	crashed bool // error came from the container dying while running

	target  State // Running, Stopped, or "" = hold (after errors)
	restart bool  // stop first, then go to target
	adopt   bool  // next start skips `compose up`: containers already run
	cancel  context.CancelFunc
	kick    chan struct{}
}

// New returns a manager whose workers live until ctx ends.
func New(ctx context.Context, opts Options) *Manager {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	return &Manager{opts: opts, ctx: ctx, changes: broker.New[struct{}](), apps: map[string]*app{}}
}

// Changes notifies (coalesced, contentless) whenever any snapshot changes.
func (mg *Manager) Changes() (<-chan struct{}, func()) { return mg.changes.Subscribe() }

func (mg *Manager) publish() { mg.changes.Publish(struct{}{}) }

// Notify tells Changes subscribers that something outside the manager
// (e.g. user prefs) changed.
func (mg *Manager) Notify() { mg.publish() }

// Sync applies a registry scan: adds new apps, updates manifests (used from
// the next start), and drops removed apps once they are stopped.
func (mg *Manager) Sync(res registry.Result) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	seen := map[string]bool{}
	for _, m := range res.Apps {
		seen[m.ID] = true
		if a, ok := mg.apps[m.ID]; ok {
			a.m, a.removed = m, false
			continue
		}
		a := &app{m: m, state: Stopped, since: time.Now(), kick: make(chan struct{}, 1)}
		mg.apps[m.ID] = a
		go mg.worker(a)
	}
	for id, a := range mg.apps {
		if !seen[id] {
			a.removed = true
			mg.dropIfDone(id, a)
		}
	}
	mg.problems = res.Problems
	mg.publish()
}

// dropIfDone forgets a removed app once it is stopped. Caller holds mu.
func (mg *Manager) dropIfDone(id string, a *app) {
	if a.removed && a.state == Stopped && mg.apps[id] == a {
		delete(mg.apps, id)
		close(a.kick)
	}
}

func (mg *Manager) List() []Snapshot {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	out := make([]Snapshot, 0, len(mg.apps))
	for _, a := range mg.apps {
		out = append(out, a.snapshot())
	}
	slices.SortFunc(out, func(x, y Snapshot) int { return strings.Compare(x.App.ID, y.App.ID) })
	return out
}

func (mg *Manager) Get(id string) (Snapshot, bool) {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, ok := mg.apps[id]
	if !ok {
		return Snapshot{}, false
	}
	return a.snapshot(), true
}

func (mg *Manager) Problems() []registry.Problem {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	return slices.Clone(mg.problems)
}

func (a *app) snapshot() Snapshot {
	return Snapshot{App: a.m, State: a.state, Error: a.err, Since: a.since, Addr: a.addr, Removed: a.removed}
}

func (mg *Manager) lookup(id string) (*app, error) {
	a, ok := mg.apps[id]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownApp, id)
	}
	return a, nil
}

// Start powers an app on. It returns at once; watch Changes for progress.
func (mg *Manager) Start(id string) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, err := mg.lookup(id)
	if err != nil {
		return err
	}
	if a.removed {
		return fmt.Errorf("app %q was removed from the apps folder", id)
	}
	a.target = Running
	if a.state == Stopped || a.state == Error {
		mg.setState(a, Starting, "") // show progress at once; the worker takes over
	}
	kick(a)
	return nil
}

// Stop powers an app off (volumes are kept). It cancels a start in progress.
func (mg *Manager) Stop(id string) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, err := mg.lookup(id)
	if err != nil {
		return err
	}
	a.target, a.restart = Stopped, false
	if a.cancel != nil {
		a.cancel()
	}
	if a.state == Running || a.state == Error {
		mg.setState(a, Stopping, "")
	}
	kick(a)
	return nil
}

func (mg *Manager) Restart(id string) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, err := mg.lookup(id)
	if err != nil {
		return err
	}
	if a.removed {
		return fmt.Errorf("app %q was removed from the apps folder", id)
	}
	a.target, a.restart = Running, true
	if a.cancel != nil {
		a.cancel()
	}
	if a.state == Running || a.state == Error {
		mg.setState(a, Stopping, "")
	}
	kick(a)
	return nil
}

func kick(a *app) {
	select {
	case a.kick <- struct{}{}:
	default:
	}
}

// setState records a transition. Caller holds mu.
func (mg *Manager) setState(a *app, s State, errMsg string) {
	a.state, a.err, a.since = s, errMsg, time.Now()
	if s != Running {
		a.addr = ""
	}
	if s != Error {
		a.crashed = false
	}
	attrs := []any{"app", a.m.ID, "state", s}
	if errMsg != "" {
		attrs = append(attrs, "error", errMsg)
	}
	mg.opts.Log.Info("app state", attrs...)
	mg.publish()
}

func (mg *Manager) worker(a *app) {
	for range a.kick {
		mg.converge(a)
	}
}

func (mg *Manager) converge(a *app) {
	for mg.ctx.Err() == nil {
		mg.mu.Lock()
		if a.restart && a.state == Stopped {
			a.restart = false
		}
		var op func(*app)
		switch {
		case a.restart, a.target == Stopped && a.state != Stopped:
			op = mg.doStop
		case a.target == Running && a.state != Running:
			op = mg.doStart
		default:
			mg.dropIfDone(a.m.ID, a)
			mg.mu.Unlock()
			return
		}
		mg.mu.Unlock()
		op(a)
	}
}

func (mg *Manager) doStart(a *app) {
	mg.mu.Lock()
	ctx, cancel := context.WithCancel(mg.ctx)
	a.cancel = cancel
	in := runtime.Default(a.m)
	adopt := a.adopt
	a.adopt = false
	mg.setState(a, Starting, "")
	mg.mu.Unlock()
	defer cancel()

	var err error
	if !adopt {
		err = mg.opts.Driver.Start(ctx, in, runtime.StartOptions{})
	}
	var addr string
	if err == nil {
		hc := HealthCheck{Driver: mg.opts.Driver, Host: mg.opts.AppHost(in.App.ID),
			Client: mg.opts.HealthClient, Interval: mg.opts.HealthInterval}
		addr, err = hc.Wait(ctx, in)
	}

	mg.mu.Lock()
	defer mg.mu.Unlock()
	a.cancel = nil
	if ctx.Err() != nil && mg.ctx.Err() == nil {
		return // cancelled by Stop/Restart; converge takes it from here
	}
	if err != nil {
		a.target = ""
		mg.setState(a, Error, err.Error())
		return
	}
	a.addr = addr
	mg.setState(a, Running, "")
}

func (mg *Manager) doStop(a *app) {
	mg.mu.Lock()
	in := runtime.Default(a.m)
	mg.setState(a, Stopping, "")
	mg.mu.Unlock()

	err := mg.opts.Driver.Stop(mg.ctx, in, runtime.StopOptions{})

	mg.mu.Lock()
	defer mg.mu.Unlock()
	if err != nil {
		a.target, a.restart = "", false
		mg.setState(a, Error, err.Error())
		return
	}
	mg.setState(a, Stopped, "")
}

// Run adopts already-running apps and follows runtime events until ctx ends.
func (mg *Manager) Run(ctx context.Context) {
	for {
		mg.reconcile(ctx)
		cctx, cancel := context.WithCancel(ctx)
		evs, errs := mg.opts.Driver.Events(cctx)
	loop:
		for {
			select {
			case <-ctx.Done():
				cancel()
				return
			case ev, ok := <-evs:
				if !ok {
					break loop
				}
				mg.onEvent(ev)
			case err := <-errs:
				mg.opts.Log.Warn("runtime events disconnected; retrying", "error", err)
				break loop
			}
		}
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// reconcile matches states to what the runtime actually runs: adopts apps
// that are up, flags running apps that vanished (missed events).
func (mg *Manager) reconcile(ctx context.Context) {
	ris, err := mg.opts.Driver.Running(ctx)
	if err != nil {
		mg.opts.Log.Warn("list running apps", "error", err)
		return
	}
	up := map[string]bool{}
	for _, ri := range ris {
		if ri.Project == runtime.ProjectName(ri.AppID) {
			up[ri.AppID] = true
		}
	}
	mg.mu.Lock()
	defer mg.mu.Unlock()
	for id, a := range mg.apps {
		switch {
		case up[id] && a.state == Stopped && a.target != Stopped:
			a.adopt, a.target = true, Running
			kick(a)
		case !up[id] && a.state == Running:
			a.target, a.crashed = "", true
			mg.setState(a, Error, "stopped outside the hub")
		}
	}
}

func (mg *Manager) onEvent(ev runtime.Event) {
	if ev.Project != runtime.ProjectName(ev.AppID) {
		return // throwaway instance, e.g. `hub validate`
	}
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, ok := mg.apps[ev.AppID]
	if !ok || ev.Service != a.m.Runtime.Service {
		return
	}
	switch ev.Kind {
	case runtime.EventDied:
		if a.state == Running {
			a.target, a.crashed = "", true
			mg.setState(a, Error, fmt.Sprintf("service %q exited (code %s)", ev.Service, ev.ExitCode))
		}
	case runtime.EventStarted:
		// Started outside the hub, or restarted by a restart policy.
		if a.state == Stopped || (a.state == Error && a.crashed) {
			a.adopt, a.target = true, Running
			kick(a)
		}
	}
}
