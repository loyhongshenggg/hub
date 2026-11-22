// Package process is the dev-mode runtime driver: it runs an app's
// `dev.processes` on the host (live reload) instead of its containers.
//
// Each process runs as `/bin/sh -c <command>` in its own process group, gets
// a free port in $PORT, and sees every sibling's port as $HUB_PORT_<NAME>.
// Processes with `watch` patterns are restarted when a matching file changes;
// if such a process exits, it waits for the next change instead of failing
// the app (compile errors are normal while editing). Dev instances are not
// re-adopted after a core restart: Close stops them, and Prepare kills any
// left behind by a crashed core.
package process

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/broker"
	"hub.localhost/platform/core/internal/runtime"
)

// ErrNoDev means the app's manifest has no dev block.
var ErrNoDev = errors.New("this app has no dev mode (add a dev: block to its app.manifest.yaml)")

type Options struct {
	// StateDir holds pid files and each app's dev data dir.
	StateDir string
	AppURL   func(id string) string
	CoreURL  string
	Log      *slog.Logger
	// WatchInterval is how often watched files are polled. Default 500ms.
	WatchInterval time.Duration
	// StopTimeout is how long a process group gets after SIGTERM. Default 5s.
	StopTimeout time.Duration
}

type Driver struct {
	opts   Options
	events *broker.Broker[runtime.Event]

	mu    sync.Mutex
	insts map[string]*instance // by compose-style project name
	logs  map[string]*logBuffer
}

var _ runtime.Driver = (*Driver)(nil)

func New(opts Options) *Driver {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	if opts.WatchInterval == 0 {
		opts.WatchInterval = 500 * time.Millisecond
	}
	if opts.StopTimeout == 0 {
		opts.StopTimeout = 5 * time.Second
	}
	return &Driver{opts: opts, events: broker.New[runtime.Event](), insts: map[string]*instance{}, logs: map[string]*logBuffer{}}
}

type instance struct {
	// opMu serialises watcher restarts with Stop, so nothing respawns after
	// Stop returns.
	opMu    sync.Mutex
	in      runtime.Instance
	ports   map[string]int
	procs   []*proc
	cancel  context.CancelFunc // stops watchers
	stopped bool
}

type proc struct {
	spec manifest.DevProcess
	dir  string
	env  []string

	mu        sync.Mutex
	cmd       *exec.Cmd
	done      chan struct{} // closed when the current cmd exits
	running   bool
	expecting bool // exit is expected (stop/restart): don't report it
}

func (d *Driver) Prepare(context.Context) error {
	if _, err := os.Stat("/bin/sh"); err != nil {
		return fmt.Errorf("dev mode needs /bin/sh: %w", err)
	}
	if err := os.MkdirAll(d.opts.StateDir, 0o755); err != nil {
		return err
	}
	killStale(d.opts.StateDir, d.opts.Log)
	return nil
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func envName(name string) string {
	return "HUB_PORT_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// DataDir is where an app keeps its files in dev mode ($HUB_DATA_DIR).
func (d *Driver) DataDir(appID string) string { return filepath.Join(d.opts.StateDir, appID, "data") }

func (d *Driver) Start(ctx context.Context, in runtime.Instance, _ runtime.StartOptions) error {
	m := in.App
	if m.Dev == nil {
		return ErrNoDev
	}
	d.mu.Lock()
	if inst, ok := d.insts[in.Project]; ok && !inst.stopped {
		d.mu.Unlock()
		return nil // already running
	}
	logs := newLogBuffer(2000)
	d.logs[in.Project] = logs
	d.mu.Unlock()

	ports := map[string]int{}
	for _, p := range m.Dev.Processes {
		port, err := freePort()
		if err != nil {
			return err
		}
		ports[p.Name] = port
	}
	dataDir := d.DataDir(m.ID)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	base := append(os.Environ(),
		"HUB_APP_ID="+m.ID,
		"HUB_APP_URL="+d.opts.AppURL(m.ID),
		"HUB_CORE_URL="+d.opts.CoreURL,
		"HUB_DEV=1",
		"HUB_DATA_DIR="+dataDir,
	)
	for name, port := range ports {
		base = append(base, envName(name)+"="+strconv.Itoa(port))
	}

	wctx, cancel := context.WithCancel(context.Background())
	inst := &instance{in: in, ports: ports, cancel: cancel}
	for _, spec := range m.Dev.Processes {
		p := &proc{
			spec: spec,
			dir:  m.DevDir(spec),
			env:  append(append([]string{}, base...), "PORT="+strconv.Itoa(ports[spec.Name])),
		}
		inst.procs = append(inst.procs, p)
	}
	for _, p := range inst.procs {
		if err := d.spawn(inst, p, logs); err != nil {
			cancel()
			d.stopProcs(inst)
			return fmt.Errorf("start %s: %w", p.spec.Name, err)
		}
		if len(p.spec.Watch) > 0 {
			go d.watch(wctx, inst, p, logs)
		}
	}
	d.mu.Lock()
	d.insts[in.Project] = inst
	d.mu.Unlock()
	writePids(d.opts.StateDir, in.Project, inst)
	return nil
}

func (d *Driver) spawn(inst *instance, p *proc, logs *logBuffer) error {
	cmd := exec.Command("/bin/sh", "-c", p.spec.Command)
	cmd.Dir = p.dir
	cmd.Env = p.env
	// Own process group, so stopping kills children (go run → binary, vite
	// → workers). On Linux, Pdeathsig also covers the shell if the core dies
	// abruptly (sysproc_*.go).
	cmd.SysProcAttr = sysProcAttr()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	p.mu.Lock()
	p.cmd, p.done, p.running, p.expecting = cmd, done, true, false
	p.mu.Unlock()

	var wg sync.WaitGroup
	pipe := func(r io.Reader, stream string) {
		defer wg.Done()
		s := bufio.NewScanner(r)
		s.Buffer(make([]byte, 64*1024), 1024*1024)
		for s.Scan() {
			logs.add(runtime.LogLine{Service: p.spec.Name, Stream: stream, Text: s.Text()})
		}
	}
	wg.Add(2)
	go pipe(stdout, "stdout")
	go pipe(stderr, "stderr")
	go func() {
		wg.Wait() // read all output before Wait closes the pipes
		err := cmd.Wait()
		p.mu.Lock()
		p.running = false
		expected := p.expecting
		p.mu.Unlock()
		close(done)
		if expected {
			return
		}
		code := exitCode(err)
		if len(p.spec.Watch) > 0 {
			logs.add(hubLine(p.spec.Name, fmt.Sprintf("exited (code %s); restarts when a watched file changes", code)))
			return
		}
		logs.add(hubLine(p.spec.Name, fmt.Sprintf("exited (code %s)", code)))
		d.events.Publish(runtime.Event{
			Kind: runtime.EventDied, AppID: inst.in.App.ID, Project: inst.in.Project,
			Service: p.spec.Name, ExitCode: code,
		})
	}()
	return nil
}

func exitCode(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if st, ok := ee.Sys().(syscall.WaitStatus); ok && st.Signaled() {
			return st.Signal().String()
		}
		return strconv.Itoa(ee.ExitCode())
	}
	if err != nil {
		return err.Error()
	}
	return "0"
}

func hubLine(service, text string) runtime.LogLine {
	return runtime.LogLine{Service: service, Stream: "stderr", Text: "[hub] " + text}
}

// kill stops one process group: SIGTERM, then SIGKILL after the timeout.
func (d *Driver) kill(p *proc) {
	p.mu.Lock()
	cmd, done, running := p.cmd, p.done, p.running
	p.expecting = true
	p.mu.Unlock()
	if cmd == nil || !running {
		return
	}
	pgid := cmd.Process.Pid
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	select {
	case <-done:
	case <-time.After(d.opts.StopTimeout):
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		<-done
	}
	// Children that ignored SIGTERM but outlived the shell.
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

func (d *Driver) stopProcs(inst *instance) {
	var wg sync.WaitGroup
	for _, p := range inst.procs {
		wg.Go(func() { d.kill(p) })
	}
	wg.Wait()
}

func (d *Driver) Stop(_ context.Context, in runtime.Instance, _ runtime.StopOptions) error {
	d.mu.Lock()
	inst, ok := d.insts[in.Project]
	if ok {
		inst.stopped = true
		delete(d.insts, in.Project)
	}
	logs := d.logs[in.Project]
	d.mu.Unlock()
	if !ok {
		return nil
	}
	inst.cancel()
	inst.opMu.Lock()
	d.stopProcs(inst)
	inst.opMu.Unlock()
	removePids(d.opts.StateDir, in.Project)
	if logs != nil {
		logs.add(hubLine("hub", "dev processes stopped"))
		logs.endFollowers()
	}
	return nil
}

// Close stops every dev instance (core shutdown).
func (d *Driver) Close() {
	d.mu.Lock()
	var all []runtime.Instance
	for _, inst := range d.insts {
		all = append(all, inst.in)
	}
	d.mu.Unlock()
	for _, in := range all {
		_ = d.Stop(context.Background(), in, runtime.StopOptions{})
	}
}

func (d *Driver) Status(_ context.Context, in runtime.Instance) (runtime.Status, error) {
	d.mu.Lock()
	inst, ok := d.insts[in.Project]
	d.mu.Unlock()
	if !ok || in.App.Dev == nil {
		return runtime.Status{}, nil
	}
	route := in.App.Dev.RouteProcess().Name
	for _, p := range inst.procs {
		if p.spec.Name != route {
			continue
		}
		p.mu.Lock()
		running := p.running
		p.mu.Unlock()
		if !running && len(p.spec.Watch) == 0 {
			return runtime.Status{State: "exited"}, nil
		}
		return runtime.Status{Running: true, State: "running", Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(inst.ports[route]))}, nil
	}
	return runtime.Status{}, nil
}

// Running is always empty: dev instances don't outlive the core.
func (d *Driver) Running(context.Context) ([]runtime.RunningInstance, error) { return nil, nil }

func (d *Driver) Events(ctx context.Context) (<-chan runtime.Event, <-chan error) {
	ch, unsubscribe := d.events.Subscribe()
	go func() { <-ctx.Done(); unsubscribe() }()
	return ch, make(chan error)
}

func (d *Driver) Logs(ctx context.Context, project string, tail int, follow bool) (<-chan runtime.LogLine, error) {
	d.mu.Lock()
	logs := d.logs[project]
	_, running := d.insts[project]
	d.mu.Unlock()
	out := make(chan runtime.LogLine, 64)
	if logs == nil {
		close(out)
		return out, nil
	}
	lines, sub, unsubscribe := logs.snapshot(tail, follow && running)
	go func() {
		defer close(out)
		defer unsubscribe()
		for _, l := range lines {
			select {
			case out <- l:
			case <-ctx.Done():
				return
			}
		}
		if sub == nil {
			return
		}
		for {
			select {
			case l, ok := <-sub:
				if !ok {
					return
				}
				select {
				case out <- l:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
