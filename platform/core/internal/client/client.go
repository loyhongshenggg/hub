// Package client is the CLI's HTTP client for a running core.
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"syscall"

	"hub.localhost/platform/core/internal/api"
	"hub.localhost/platform/core/internal/runtime"
)

type Client struct {
	Addr string // dial address, e.g. 127.0.0.1:7000
	Host string // Host header, e.g. hub.localhost:7000
	HTTP *http.Client
}

var ErrNotRunning = errors.New("hub core is not running (start it with `hub serve`)")

func (c *Client) do(ctx context.Context, method, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, "http://"+c.Addr+path, nil)
	if err != nil {
		return err
	}
	req.Host = c.Host
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		if errors.Is(err, syscall.ECONNREFUSED) {
			return ErrNotRunning
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	if out == nil {
		return nil
	}
	if s, ok := out.(*sseHandler); ok {
		return s.read(resp.Body)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Healthy reports whether a core answers at c.Addr.
func (c *Client) Healthy(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/api/healthz", nil)
}

// Shutdown asks the core to exit; it answers before it stops.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/api/system/shutdown", nil)
}

func (c *Client) Apps(ctx context.Context) (api.AppsResponse, error) {
	var r api.AppsResponse
	return r, c.do(ctx, http.MethodGet, "/api/apps", &r)
}

func (c *Client) App(ctx context.Context, id string) (api.AppInfo, error) {
	var r api.AppInfo
	return r, c.do(ctx, http.MethodGet, "/api/apps/"+id, &r)
}

// Action runs start, stop or restart; mode ("run", "dev" or "") picks how.
func (c *Client) Action(ctx context.Context, id, action, mode string) (api.AppInfo, error) {
	var r api.AppInfo
	path := "/api/apps/" + id + "/" + action
	if mode != "" {
		path += "?mode=" + mode
	}
	return r, c.do(ctx, http.MethodPost, path, &r)
}

// Logs streams log lines to fn until the stream ends or ctx is cancelled.
func (c *Client) Logs(ctx context.Context, id string, tail int, follow bool, fn func(runtime.LogLine)) error {
	f := "0"
	if follow {
		f = "1"
	}
	h := &sseHandler{fn: func(event string, data []byte) error {
		if event != "log" {
			return nil
		}
		var l runtime.LogLine
		if err := json.Unmarshal(data, &l); err != nil {
			return err
		}
		fn(l)
		return nil
	}}
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/apps/%s/logs?tail=%d&follow=%s", id, tail, f), h)
	if ctx.Err() != nil {
		return nil
	}
	return err
}

type sseHandler struct {
	fn func(event string, data []byte) error
}

func (s *sseHandler) read(r io.Reader) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	event, data := "message", ""
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if data != "" {
				if err := s.fn(event, []byte(data)); err != nil {
					return err
				}
			}
			event, data = "message", ""
		case strings.HasPrefix(line, "event: "):
			event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			data += strings.TrimPrefix(line, "data: ")
		}
	}
	return sc.Err()
}
