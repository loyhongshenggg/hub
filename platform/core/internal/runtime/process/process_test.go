package process

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/runtime"
)

func newDriver(t *testing.T) *Driver {
	d := New(Options{
		StateDir:      t.TempDir(),
		AppURL:        func(id string) string { return "http://" + id + ".hub.localhost:7000" },
		CoreURL:       "http://hub.localhost:7000",
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		WatchInterval: 50 * time.Millisecond,
		StopTimeout:   2 * time.Second,
	})
	if err := d.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Close)
	return d
}

func app(t *testing.T, procs ...manifest.DevProcess) runtime.Instance {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{ID: "demo", Name: "Demo", Dir: dir, Dev: &manifest.Dev{Route: "web", Processes: procs}}
	return runtime.Instance{App: m, Project: "hub-demo"}
}

// lines collects the current log lines of the instance (no follow).
func lines(t *testing.T, d *Driver) string {
	t.Helper()
	ch, err := d.Logs(context.Background(), "hub-demo", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for l := range ch {
		b.WriteString(l.Service + "|" + l.Text + "\n")
	}
	return b.String()
}

func waitLog(t *testing.T, d *Driver, re string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		out := lines(t, d)
		if regexp.MustCompile(re).MatchString(out) {
			return out
		}
		if time.Now().After(deadline) {
			t.Fatalf("log never matched %q:\n%s", re, out)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartEnvPortsWatchStop(t *testing.T) {
	d := newDriver(t)
	in := app(t,
		manifest.DevProcess{Name: "api", Dir: "api", Command: `echo "api up port=$PORT"; exec sleep 30`, Watch: []string{"*.txt"}},
		manifest.DevProcess{Name: "web", Command: `echo "web port=$PORT api=$HUB_PORT_API id=$HUB_APP_ID dev=$HUB_DEV data=$HUB_DATA_DIR"; exec sleep 30`},
	)
	if err := d.Start(context.Background(), in, runtime.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	out := waitLog(t, d, `web\|web port=\d+ api=\d+ id=demo dev=1 data=\S+/demo/data`)
	webPort := regexp.MustCompile(`web port=(\d+)`).FindStringSubmatch(out)[1]
	apiPort := regexp.MustCompile(`api=(\d+)`).FindStringSubmatch(out)[1]
	if !strings.Contains(waitLog(t, d, `api\|api up`), "api up port="+apiPort) {
		t.Errorf("api did not get its own port %s", apiPort)
	}
	st, _ := d.Status(context.Background(), in)
	if !st.Running || st.Addr != "127.0.0.1:"+webPort {
		t.Errorf("status = %+v, want route to web on %s", st, webPort)
	}

	// A watched file changes → api restarts; web keeps running.
	if err := os.WriteFile(filepath.Join(in.App.Dir, "api", "x.txt"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}
	waitLog(t, d, `(?s)file changed; restarting.*api\|api up`)
	if n := strings.Count(lines(t, d), "api up"); n != 2 {
		t.Errorf("api started %d times, want 2", n)
	}

	if err := d.Stop(context.Background(), in, runtime.StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if st, _ := d.Status(context.Background(), in); st.Running {
		t.Error("still running after Stop")
	}
	if _, err := os.Stat(pidFile(d.opts.StateDir, "hub-demo")); !os.IsNotExist(err) {
		t.Error("pid file left behind")
	}
}

func TestStopKillsChildren(t *testing.T) {
	d := newDriver(t)
	in := app(t, manifest.DevProcess{Name: "web", Command: `sleep 30 & echo "child=$!"; wait`})
	if err := d.Start(context.Background(), in, runtime.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	out := waitLog(t, d, `child=\d+`)
	child, _ := strconv.Atoi(regexp.MustCompile(`child=(\d+)`).FindStringSubmatch(out)[1])
	if err := d.Stop(context.Background(), in, runtime.StopOptions{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(child, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatal("child process survived Stop")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestExitWithoutWatchIsReported(t *testing.T) {
	d := newDriver(t)
	evs, _ := d.Events(t.Context())
	in := app(t, manifest.DevProcess{Name: "web", Command: `echo bye; exit 3`})
	if err := d.Start(context.Background(), in, runtime.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-evs:
		if ev.Kind != runtime.EventDied || ev.Service != "web" || ev.ExitCode != "3" || ev.AppID != "demo" {
			t.Errorf("event = %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no died event")
	}
	if st, _ := d.Status(context.Background(), in); st.Running {
		t.Error("exited route process should not be running")
	}
}

func TestExitWithWatchWaitsForChange(t *testing.T) {
	d := newDriver(t)
	evs, _ := d.Events(t.Context())
	in := app(t, manifest.DevProcess{Name: "web", Command: `echo boom; exit 1`, Watch: []string{"*.txt"}})
	if err := d.Start(context.Background(), in, runtime.StartOptions{}); err != nil {
		t.Fatal(err)
	}
	waitLog(t, d, `restarts when a watched file changes`)
	select {
	case ev := <-evs:
		t.Fatalf("unexpected event %+v (watched processes wait for a fix)", ev)
	case <-time.After(150 * time.Millisecond):
	}
	if st, _ := d.Status(context.Background(), in); !st.Running {
		t.Error("a watched app stays up while waiting for a fix")
	}
}

func TestNoDev(t *testing.T) {
	d := newDriver(t)
	in := runtime.Instance{App: &manifest.Manifest{ID: "x"}, Project: "hub-x"}
	if err := d.Start(context.Background(), in, runtime.StartOptions{}); err != ErrNoDev {
		t.Fatalf("err = %v", err)
	}
}

func TestKillStale(t *testing.T) {
	dir := t.TempDir()
	start := func() *exec.Cmd {
		cmd := exec.Command("sleep", "30")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	stale, innocent := start(), start()
	pids := strconv.Itoa(stale.Process.Pid) + " " + startTime(stale.Process.Pid) + "\n" +
		strconv.Itoa(innocent.Process.Pid) + " 1\n" // wrong start time: PID reused
	if err := os.WriteFile(filepath.Join(dir, "hub-x.pids"), []byte(pids), 0o644); err != nil {
		t.Fatal(err)
	}
	killStale(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))

	done := make(chan error, 1)
	go func() { done <- stale.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stale process not killed")
	}
	if syscall.Kill(innocent.Process.Pid, 0) != nil {
		t.Error("killed a process whose start time didn't match")
	}
	if _, err := os.Stat(filepath.Join(dir, "hub-x.pids")); !os.IsNotExist(err) {
		t.Error("pid file not removed")
	}
}

// The shell dies with the core (Pdeathsig) but its children keep the group.
func TestKillStaleOrphanedGroup(t *testing.T) {
	dir := t.TempDir()
	leader := exec.Command("/bin/sh", "-c", `sleep 30 & echo $!; sleep 0.3`)
	leader.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := leader.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := leader.Start(); err != nil {
		t.Fatal(err)
	}
	started := startTime(leader.Process.Pid)
	var childPid int
	if _, err := fmt.Fscan(out, &childPid); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(childPid, syscall.SIGKILL) })
	_ = leader.Wait() // leader gone; child still in its group
	if syscall.Kill(childPid, 0) != nil {
		t.Fatal("test setup: child should be alive")
	}
	line := fmt.Sprintf("%d %s\n", leader.Process.Pid, started)
	if err := os.WriteFile(filepath.Join(dir, "hub-y.pids"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	killStale(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	deadline := time.Now().Add(2 * time.Second)
	for !isGone(childPid) {
		if time.Now().After(deadline) {
			t.Fatal("orphaned group member survived")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// isGone: no such process, or a zombie (killed, not yet reaped by init).
func isGone(pid int) bool {
	f := statFields(pid)
	return len(f) == 0 || f[0] == "Z"
}
