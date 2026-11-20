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

var (
	ErrUnknownApp = errors.New("unknown app")
	ErrNoDevMode  = errors.New("this app has no dev mode (add a dev: block to its app.manifest.yaml)")
)

// Mode is how an app runs: its containers (Run) or host processes with live
// reload (Dev).
type Mode string

const (
	ModeRun Mode = "run"
	ModeDev Mode = "dev"
)

// devMinStartTimeout: dev servers compile on first start, so allow longer.
const devMinStartTimeout = 2 * time.Minute

// Snapshot is the public view of one app.
type Snapshot struct {
	App   *manifest.Manifest
	State State
	Error string
	Since time.Time
	Addr  string // gateway target, set while Running
	Mode  Mode   // of the current (or last) instance
	// Removed: the manifest is gone; the app disappears once stopped.
	Removed bool
}

type Options struct {
	Driver    runtime.Driver // Run mode (containers)
	DevDriver runtime.Driver // Dev mode (host processes); nil disables it
	Log       *slog.Logger
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

	target   State // Running, Stopped, or "" = hold (after errors)
	restart  bool  // stop first, then go to target
	adopt    bool  // next start skips `compose up`: containers already run
	mode     Mode  // of the current (or last) instance
	wantMode Mode  // mode of the next start
	cancel   context.CancelFunc
	kick     chan struct{}
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
		a := &app{m: m, state: Stopped, since: time.Now(), mode: ModeRun, wantMode: ModeRun, kick: make(chan struct{}, 1)}
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
	return Snapshot{App: a.m, State: a.state, Error: a.err, Since: a.since, Addr: a.addr, Mode: a.mode, Removed: a.removed}
}

func (mg *Manager) lookup(id string) (*app, error) {
	a, ok := mg.apps[id]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknownApp, id)
	}
	return a, nil
}

// Start powers an app on in Run mode. See StartMode.
func (mg *Manager) Start(id string) error { return mg.StartMode(id, ModeRun) }

// DevAvailable reports whether an app can run in Dev mode.
func (mg *Manager) DevAvailable(m *manifest.Manifest) bool {
	return mg.opts.DevDriver != nil && m.Dev != nil
}

// StartMode powers an app on in the given mode. If it is running in the
// other mode, it is stopped first. Returns at once; watch Changes.
func (mg *Manager) StartMode(id string, mode Mode) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, err := mg.lookup(id)
	if err != nil {
		return err
	}
	if a.removed {
		return fmt.Errorf("app %q was removed from the apps folder", id)
	}
	if mode == ModeDev && !mg.DevAvailable(a.m) {
		return ErrNoDevMode
	}
	a.target, a.wantMode = Running, mode
	switch {
	case a.state != Stopped && a.mode != mode: // switch modes: stop the old instance first
		a.restart = true
		if a.cancel != nil {
			a.cancel()
		}
		if a.state == Running || a.state == Error {
			mg.setState(a, Stopping, "")
		}
	case a.state == Stopped || a.state == Error:
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

// Restart stops and starts an app in the mode it last ran in.
func (mg *Manager) Restart(id string) error { return mg.RestartMode(id, "") }

// RestartMode stops and starts an app; mode "" keeps the current mode.
func (mg *Manager) RestartMode(id string, mode Mode) error {
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, err := mg.lookup(id)
	if err != nil {
		return err
	}
	if a.removed {
		return fmt.Errorf("app %q was removed from the apps folder", id)
	}
	if mode == "" {
		mode = a.mode
	}
	if mode == ModeDev && !mg.DevAvailable(a.m) {
		return ErrNoDevMode
	}
	a.target, a.restart, a.wantMode = Running, true, mode
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
	a.mode = a.wantMode
	drv := mg.driver(a.mode)
	dev := a.mode == ModeDev
	mg.setState(a, Starting, "")
	mg.mu.Unlock()
	defer cancel()

	var err error
	if !adopt {
		err = drv.Start(ctx, in, runtime.StartOptions{})
	}
	var addr string
	if err == nil {
		hc := HealthCheck{Driver: drv, Host: mg.opts.AppHost(in.App.ID),
			Client: mg.opts.HealthClient, Interval: mg.opts.HealthInterval}
		if dev {
			hc.MinTimeout = devMinStartTimeout
		}
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
	drv := mg.driver(a.mode)
	mg.setState(a, Stopping, "")
	mg.mu.Unlock()

	err := drv.Stop(mg.ctx, in, runtime.StopOptions{})

	mg.mu.Lock()
	defer mg.mu.Unlock()
	if err != nil {
		a.target, a.restart = "", false
		mg.setState(a, Error, err.Error())
		return
	}
	mg.setState(a, Stopped, "")
}

// driver returns the runtime for a mode.
func (mg *Manager) driver(m Mode) runtime.Driver {
	if m == ModeDev && mg.opts.DevDriver != nil {
		return mg.opts.DevDriver
	}
	return mg.opts.Driver
}

// Logs streams the logs of an app's current (or last) instance.
func (mg *Manager) Logs(ctx context.Context, id string, tail int, follow bool) (<-chan runtime.LogLine, error) {
	mg.mu.Lock()
	a, err := mg.lookup(id)
	var drv runtime.Driver
	if err == nil {
		drv = mg.driver(a.mode)
	}
	mg.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return drv.Logs(ctx, runtime.ProjectName(id), tail, follow)
}

// Run adopts already-running apps and follows runtime events until ctx ends.
func (mg *Manager) Run(ctx context.Context) {
	if mg.opts.DevDriver != nil {
		go mg.follow(ctx, ModeDev, mg.opts.DevDriver, false)
	}
	mg.follow(ctx, ModeRun, mg.opts.Driver, true)
}

// follow consumes one driver's events, reconnecting on errors. With
// reconcile, it first re-syncs states with what the driver runs (startup,
// and after missed events).
func (mg *Manager) follow(ctx context.Context, mode Mode, drv runtime.Driver, reconcile bool) {
	for {
		if reconcile {
			mg.reconcile(ctx)
		}
		cctx, cancel := context.WithCancel(ctx)
		evs, errs := drv.Events(cctx)
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
				mg.onEvent(mode, ev)
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
			a.adopt, a.target, a.mode, a.wantMode = true, Running, ModeRun, ModeRun
			kick(a)
		case !up[id] && a.state == Running && a.mode == ModeRun:
			a.target, a.crashed = "", true
			mg.setState(a, Error, "stopped outside the hub")
		}
	}
}

func (mg *Manager) onEvent(mode Mode, ev runtime.Event) {
	if ev.Project != runtime.ProjectName(ev.AppID) {
		return // throwaway instance, e.g. `hub validate`
	}
	mg.mu.Lock()
	defer mg.mu.Unlock()
	a, ok := mg.apps[ev.AppID]
	if !ok {
		return
	}
	if mode == ModeDev {
		// A dev process without watch patterns exiting breaks the app.
		if a.mode == ModeDev && a.state == Running && ev.Kind == runtime.EventDied {
			a.target = ""
			mg.setState(a, Error, fmt.Sprintf("dev process %q exited (code %s)", ev.Service, ev.ExitCode))
		}
		return
	}
	if ev.Service != a.m.Runtime.Service || a.mode != ModeRun {
		return // not the routed container, or the app runs in dev mode
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
