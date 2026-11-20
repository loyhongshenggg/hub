// Package validate runs `hub validate <id>`: schema-check the manifest, build
// and start a throwaway instance, wait for health, check the UI page can be
// framed, then tear it down (volumes too).
package validate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hub.localhost/libs/manifest"
	"hub.localhost/platform/core/internal/lifecycle"
	"hub.localhost/platform/core/internal/runtime"
)

type Options struct {
	Driver  runtime.Driver
	AppsDir string
	ID      string
	AppHost func(id string) string
	Out     io.Writer
	Keep    bool // leave the instance running for inspection
}

// ErrFailed means at least one check failed (details were printed).
var ErrFailed = errors.New("validation failed")

type run struct {
	Options
	failed bool
}

func (r *run) ok(step, format string, a ...any) {
	fmt.Fprintf(r.Out, "  ✓ %-9s %s\n", step, fmt.Sprintf(format, a...))
}
func (r *run) warn(step, format string, a ...any) {
	fmt.Fprintf(r.Out, "  ! %-9s %s\n", step, fmt.Sprintf(format, a...))
}
func (r *run) fail(step string, err error) error {
	r.failed = true
	msg := strings.ReplaceAll(err.Error(), "\n", "\n              ")
	fmt.Fprintf(r.Out, "  ✗ %-9s %s\n", step, msg)
	return ErrFailed
}
func (r *run) info(format string, a ...any) { fmt.Fprintf(r.Out, "  … "+format+"\n", a...) }

func Run(ctx context.Context, opts Options) (err error) {
	r := &run{Options: opts}
	fmt.Fprintf(r.Out, "hub validate %s\n", opts.ID)
	defer func() {
		if r.failed {
			fmt.Fprintln(r.Out, "FAIL")
		} else if err == nil {
			fmt.Fprintln(r.Out, "PASS")
		}
	}()

	path := filepath.Join(opts.AppsDir, opts.ID, manifest.FileName)
	if _, err := os.Stat(path); err != nil {
		return r.fail("manifest", fmt.Errorf("%s not found", path))
	}
	m, err := manifest.Load(path)
	if err != nil {
		return r.fail("manifest", err)
	}
	if m.ID != opts.ID {
		return r.fail("manifest", fmt.Errorf("id %q must match its folder name %q", m.ID, opts.ID))
	}
	shown := path
	if rel, err := filepath.Rel(filepath.Dir(opts.AppsDir), path); err == nil {
		shown = rel
	}
	r.ok("manifest", "%s (v%d)", shown, m.ManifestVersion)

	if err := opts.Driver.Prepare(ctx); err != nil {
		return r.fail("docker", err)
	}
	r.ok("docker", "engine reachable, network ready")

	in := runtime.Instance{App: m, Project: "hub-validate-" + m.ID}
	defer func() {
		if opts.Keep {
			r.info("kept running as compose project %q (remove: docker compose -p %s down -v)", in.Project, in.Project)
			return
		}
		// Clean up even if ctx was cancelled (Ctrl-C).
		cctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := opts.Driver.Stop(cctx, in, runtime.StopOptions{RemoveVolumes: true}); err != nil {
			r.fail("cleanup", err)
			return
		}
		r.ok("cleanup", "removed containers and volumes of %s", in.Project)
	}()

	r.info("building and starting %s (first build can take a while)", in.Project)
	t0 := time.Now()
	if err := opts.Driver.Start(ctx, in, runtime.StartOptions{Build: true}); err != nil {
		return r.fail("start", err)
	}
	r.ok("start", "%s", since(t0))

	t0 = time.Now()
	host := opts.AppHost(m.ID)
	addr, err := lifecycle.HealthCheck{Driver: opts.Driver, Host: host}.Wait(ctx, in)
	if err != nil {
		r.fail("health", err)
		r.dumpLogs(ctx, in.Project)
		return ErrFailed
	}
	r.ok("health", "GET %s → 2xx after %s", m.Health.Path, since(t0))

	if err := r.checkUI(ctx, addr, host, m); err != nil {
		r.dumpLogs(ctx, in.Project)
		return err
	}
	return nil
}

func since(t time.Time) string { return time.Since(t).Round(100 * time.Millisecond).String() }

func (r *run) checkUI(ctx context.Context, addr, host string, m *manifest.Manifest) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+m.UI.Path, nil)
	if err != nil {
		return r.fail("ui", err)
	}
	req.Host = host
	c := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		return r.fail("ui", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return r.fail("ui", fmt.Errorf("GET %s → %s (want 2xx; ui.path must serve the page directly)", m.UI.Path, resp.Status))
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if ct != "text/html" && ct != "application/xhtml+xml" {
		r.warn("ui", "GET %s → Content-Type %q, not HTML; the tab will show it as-is", m.UI.Path, ct)
	}
	if resp.Header.Get("X-Frame-Options") != "" || strings.Contains(strings.ToLower(resp.Header.Get("Content-Security-Policy")), "frame-ancestors") {
		r.warn("ui", "app sends X-Frame-Options / frame-ancestors; the gateway strips them so the shell can frame it")
	}
	r.ok("ui", "GET %s → %s (frameable)", m.UI.Path, resp.Status)
	return nil
}

func (r *run) dumpLogs(ctx context.Context, project string) {
	lines, err := r.Driver.Logs(ctx, project, 40, false)
	if err != nil {
		r.warn("logs", "%v", err)
		return
	}
	fmt.Fprintln(r.Out, "  ── last log lines ──")
	for l := range lines {
		fmt.Fprintf(r.Out, "  %s | %s\n", l.Service, l.Text)
	}
}
