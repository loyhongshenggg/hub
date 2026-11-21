// Package api is the shell-facing REST + SSE API under /api.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"hub.localhost/platform/core/internal/lifecycle"
	"hub.localhost/platform/core/internal/registry"
	"hub.localhost/platform/core/internal/runtime"
	"hub.localhost/platform/core/internal/store"
)

// AppInfo is one app as the shell sees it.
type AppInfo struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon,omitempty"` // URL path
	URL          string    `json:"url"`            // app origin, e.g. http://notes.hub.localhost:7000
	UIPath       string    `json:"uiPath"`
	KeepAlive    bool      `json:"keepAlive"`
	AutoStart    bool      `json:"autoStart"` // effective: user override, else manifest
	Pinned       bool      `json:"pinned"`
	Position     *int      `json:"position"`
	Status       string    `json:"status"` // stopped|starting|running|stopping|error
	Mode         string    `json:"mode"`   // run|dev: of the current (or last) instance
	DevAvailable bool      `json:"devAvailable"`
	Error        string    `json:"error,omitempty"`
	Since        time.Time `json:"since"`
	Removed      bool      `json:"removed,omitempty"`
}

type AppsResponse struct {
	Apps     []AppInfo          `json:"apps"`
	Problems []registry.Problem `json:"problems"`
}

type API struct {
	Manager *lifecycle.Manager
	Store   *store.Store
	Driver  runtime.Driver
	AppURL  func(id string) string
	Log     *slog.Logger
	// Shutdown asks the core to exit (nil: not offered). The server's
	// same-origin check keeps other sites from calling it.
	Shutdown func()
}

func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /api/system/shutdown", a.shutdown)
	mux.HandleFunc("GET /api/apps", a.listApps)
	mux.HandleFunc("GET /api/apps/{id}", a.getApp)
	mux.HandleFunc("PATCH /api/apps/{id}", a.patchApp)
	mux.HandleFunc("POST /api/apps/{id}/{action}", a.action)
	mux.HandleFunc("GET /api/apps/{id}/icon", a.icon)
	mux.HandleFunc("GET /api/apps/{id}/logs", a.logs)
	mux.HandleFunc("GET /api/events", a.events)
	mux.HandleFunc("GET /api/state/{key}", a.getState)
	mux.HandleFunc("PUT /api/state/{key}", a.putState)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { writeError(w, http.StatusNotFound, "no such endpoint") })
	return mux
}

func (a *API) snapshot(ctx context.Context) (AppsResponse, error) {
	prefs, err := a.Store.AllPrefs(ctx)
	if err != nil {
		return AppsResponse{}, err
	}
	snaps := a.Manager.List()
	resp := AppsResponse{Apps: make([]AppInfo, 0, len(snaps)), Problems: a.Manager.Problems()}
	if resp.Problems == nil {
		resp.Problems = []registry.Problem{}
	}
	for _, s := range snaps {
		resp.Apps = append(resp.Apps, a.info(s, prefs[s.App.ID]))
	}
	return resp, nil
}

func (a *API) info(s lifecycle.Snapshot, p store.Prefs) AppInfo {
	m := s.App
	auto := m.AutoStart
	if p.AutoStart != nil {
		auto = *p.AutoStart
	}
	icon := ""
	if m.Icon != "" {
		icon = "/api/apps/" + m.ID + "/icon"
	}
	return AppInfo{
		ID: m.ID, Name: m.Name, Description: m.Description, Icon: icon, URL: a.AppURL(m.ID),
		UIPath: m.UI.Path, KeepAlive: m.UI.KeepAlive, AutoStart: auto,
		Pinned: p.Pinned, Position: p.Position,
		Status: string(s.State), Mode: string(s.Mode), DevAvailable: a.Manager.DevAvailable(m),
		Error: s.Error, Since: s.Since, Removed: s.Removed,
	}
}

func (a *API) listApps(w http.ResponseWriter, r *http.Request) {
	resp, err := a.snapshot(r.Context())
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, resp)
}

func (a *API) appInfo(ctx context.Context, id string) (AppInfo, bool, error) {
	s, ok := a.Manager.Get(id)
	if !ok {
		return AppInfo{}, false, nil
	}
	p, err := a.Store.Prefs(ctx, id)
	return a.info(s, p), true, err
}

func (a *API) writeApp(w http.ResponseWriter, r *http.Request, status int) {
	info, ok, err := a.appInfo(r.Context(), r.PathValue("id"))
	switch {
	case err != nil:
		writeError(w, 500, err.Error())
	case !ok:
		writeError(w, 404, "unknown app")
	default:
		writeJSON(w, status, info)
	}
}

func (a *API) getApp(w http.ResponseWriter, r *http.Request) { a.writeApp(w, r, 200) }

func (a *API) action(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mode := lifecycle.Mode(r.URL.Query().Get("mode"))
	if mode != "" && mode != lifecycle.ModeRun && mode != lifecycle.ModeDev {
		writeError(w, 400, "mode must be run or dev")
		return
	}
	var err error
	switch r.PathValue("action") {
	case "start":
		if mode == "" {
			mode = lifecycle.ModeRun
		}
		err = a.Manager.StartMode(id, mode)
	case "stop":
		err = a.Manager.Stop(id)
	case "restart":
		err = a.Manager.RestartMode(id, mode)
	default:
		writeError(w, 404, "unknown action (start, stop, restart)")
		return
	}
	switch {
	case errors.Is(err, lifecycle.ErrUnknownApp):
		writeError(w, 404, err.Error())
	case errors.Is(err, lifecycle.ErrNoDevMode):
		writeError(w, 409, err.Error())
	case err != nil:
		writeError(w, 409, err.Error())
	default:
		a.writeApp(w, r, http.StatusAccepted)
	}
}

// patchApp updates prefs. Body fields are optional: pinned (bool),
// position (int|null), autoStart (bool|null; null = manifest default).
func (a *API) patchApp(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.Manager.Get(id); !ok {
		writeError(w, 404, "unknown app")
		return
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err != nil {
		writeError(w, 400, "body must be a JSON object")
		return
	}
	p, err := a.Store.Prefs(r.Context(), id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	for k, v := range body {
		var err error
		switch k {
		case "pinned":
			err = json.Unmarshal(v, &p.Pinned)
		case "position":
			p.Position = nil
			err = json.Unmarshal(v, &p.Position)
		case "autoStart":
			p.AutoStart = nil
			err = json.Unmarshal(v, &p.AutoStart)
		default:
			err = errors.New("unknown field")
		}
		if err != nil {
			writeError(w, 400, fmt.Sprintf("%s: %v", k, err))
			return
		}
	}
	if err := a.Store.SetPrefs(r.Context(), id, p); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	a.Manager.Notify()
	a.writeApp(w, r, 200)
}

func (a *API) icon(w http.ResponseWriter, r *http.Request) {
	s, ok := a.Manager.Get(r.PathValue("id"))
	if !ok || s.App.Icon == "" {
		http.NotFound(w, r)
		return
	}
	// SVGs may hold scripts; never let them run on the shell's origin.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, s.App.IconPath())
}

// logs streams the logs of the app's current instance (containers or dev
// processes) as SSE "log" events (data: runtime.LogLine).
// Query: tail (default 200, 0 = all), follow (default 1).
func (a *API) logs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := a.Manager.Get(id); !ok {
		writeError(w, 404, "unknown app")
		return
	}
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, 400, "tail must be a non-negative integer")
			return
		}
		tail = n
	}
	follow := r.URL.Query().Get("follow") != "0"
	lines, err := a.Manager.Logs(r.Context(), id, tail, follow)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	s := startSSE(w)
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if s.ping() != nil {
				return
			}
		case l, ok := <-lines:
			if !ok {
				_ = s.send("end", struct{}{})
				return
			}
			if s.send("log", l) != nil {
				return
			}
		}
	}
}

// events streams the full app list as an SSE "apps" event on connect and
// after every change.
func (a *API) events(w http.ResponseWriter, r *http.Request) {
	changes, unsubscribe := a.Manager.Changes()
	defer unsubscribe()
	s := startSSE(w)
	send := func() error {
		resp, err := a.snapshot(r.Context())
		if err != nil {
			return err
		}
		return s.send("apps", resp)
	}
	if send() != nil {
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if s.ping() != nil {
				return
			}
		case _, ok := <-changes:
			if !ok {
				return // fell behind; client reconnects and gets a snapshot
			}
			// Coalesce bursts of transitions into one message.
			time.Sleep(30 * time.Millisecond)
			for drained := false; !drained; {
				select {
				case _, ok := <-changes:
					if !ok {
						return
					}
				default:
					drained = true
				}
			}
			if send() != nil {
				return
			}
		}
	}
}

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func (a *API) getState(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !keyRe.MatchString(key) {
		writeError(w, 400, "bad key")
		return
	}
	v, err := a.Store.UIState(r.Context(), key)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, 404, "not set")
	case err != nil:
		writeError(w, 500, err.Error())
	default:
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(v)
	}
}

func (a *API) putState(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !keyRe.MatchString(key) {
		writeError(w, 400, "bad key")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := a.Store.SetUIState(r.Context(), key, body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type sse struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func startSSE(w http.ResponseWriter) *sse {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	s := &sse{w: w, rc: http.NewResponseController(w)}
	_ = s.rc.Flush()
	return s
}

func (s *sse) send(event string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, b); err != nil {
		return err
	}
	return s.rc.Flush()
}

func (s *sse) ping() error {
	if _, err := io.WriteString(s.w, ": ping\n\n"); err != nil {
		return err
	}
	return s.rc.Flush()
}

func (a *API) shutdown(w http.ResponseWriter, _ *http.Request) {
	if a.Shutdown == nil {
		writeError(w, http.StatusNotFound, "shutdown is not available")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "stopping"})
	go a.Shutdown()
}
