package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"hub.localhost/platform/core/internal/runtime"
)

// HealthCheck polls an instance until its health path returns 2xx.
type HealthCheck struct {
	Driver runtime.Driver
	// Host is sent as the Host header, so apps that check it (CSRF, dev
	// servers' allowed hosts) see the same value as through the gateway.
	Host     string
	Client   *http.Client  // default: 2s timeout, no redirects
	Interval time.Duration // default 500ms
	// MinTimeout raises the manifest's startTimeout (dev servers compile first).
	MinTimeout time.Duration
}

// Wait returns the instance's address once healthy. It fails fast if the
// routed container exits, and after the manifest's startTimeout otherwise.
func (h HealthCheck) Wait(ctx context.Context, in runtime.Instance) (string, error) {
	c := h.Client
	if c == nil {
		c = &http.Client{
			Timeout:       2 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	interval := h.Interval
	if interval == 0 {
		interval = 500 * time.Millisecond
	}
	timeout := max(in.App.Health.StartTimeout.Duration, h.MinTimeout)
	deadline := time.Now().Add(timeout)
	svc := in.App.Runtime.Service
	var last error
	for {
		st, err := h.Driver.Status(ctx, in)
		switch {
		case err != nil:
			last = err
		case !st.Running && (st.State == "exited" || st.State == "dead"):
			return "", fmt.Errorf("service %q %s before becoming healthy", svc, st.State)
		case !st.Running:
			last = fmt.Errorf("service %q is %s", svc, orUnknown(st.State))
		case st.Addr == "":
			last = fmt.Errorf("service %q has no address on the hub network", svc)
		default:
			if last = probe(ctx, c, st.Addr, h.Host, in.App.Health.Path); last == nil {
				return st.Addr, nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("not healthy after %s: %w", timeout, last)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
}

func orUnknown(s string) string {
	if s == "" {
		return "not created"
	}
	return s
}

func probe(ctx context.Context, c *http.Client, addr, host, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
	if err != nil {
		return err
	}
	req.Host = host
	resp, err := c.Do(req)
	if err != nil {
		var uerr interface{ Unwrap() error }
		if errors.As(err, &uerr) {
			err = uerr.Unwrap()
		}
		return fmt.Errorf("GET %s: %w", path, err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return nil
}
