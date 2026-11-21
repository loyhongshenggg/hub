package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"hub.localhost/apps/radar/api/internal/engine"
	"hub.localhost/apps/radar/api/internal/store"
)

// learning serves the learning dashboard. Each request loads the (small)
// state, runs the engine, and saves if something changed; mu serialises
// changes so two clicks can't lose an update.
type learning struct {
	mu    sync.Mutex
	store *store.Store
	eng   *engine.Engine
	now   func() time.Time
}

func (l *learning) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/learning", l.read(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		return l.eng.Dashboard(s, pool, l.now()), nil
	}))
	mux.HandleFunc("GET /api/learning/tracks/{track}", l.read(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		return l.eng.Track(s, pool, r.PathValue("track"), l.now())
	}))
	mux.HandleFunc("GET /api/learning/tracks/{track}/chapters/{chapter}", l.read(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		return l.eng.Chapter(s, pool, r.PathValue("track"), r.PathValue("chapter"), l.now())
	}))
	mux.HandleFunc("GET /api/learning/history", l.read(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		limit, before := 50, time.Time{}
		if v := r.URL.Query().Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 200 {
				return nil, invalid("limit must be 1–200")
			}
			limit = n
		}
		if v := r.URL.Query().Get("before"); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, invalid("before must be an RFC 3339 time")
			}
			before = t
		}
		h := l.eng.History(s, pool, before, limit+1)
		next := ""
		if len(h) > limit {
			h, next = h[:limit], h[limit-1].At.Format(time.RFC3339)
		}
		return map[string]any{"items": h, "nextBefore": next}, nil
	}))
	mux.HandleFunc("GET /api/learning/assessment", l.read(func(s *engine.State, _ []engine.Item, _ *http.Request) (any, error) {
		return map[string]any{"done": s.Assessed, "topics": l.eng.Assessment(s)}, nil
	}))
	mux.HandleFunc("GET /api/learning/settings", l.read(func(s *engine.State, _ []engine.Item, _ *http.Request) (any, error) {
		return s.Settings, nil
	}))

	// Changes. Step-level ones answer with the chapter they happened in, so
	// the UI can update without a second request.
	mux.HandleFunc("POST /api/learning/done", l.change(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		var b struct {
			Ref    string        `json:"ref"`
			Rating engine.Rating `json:"rating"`
			Worth  *bool         `json:"worth"`
		}
		if err := decode(r, &b); err != nil {
			return nil, err
		}
		if err := l.eng.Done(s, pool, b.Ref, b.Rating, b.Worth, l.now()); err != nil {
			return nil, err
		}
		return l.context(s, pool, b.Ref), nil
	}))
	mux.HandleFunc("POST /api/learning/undo", l.change(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		var b struct{ Ref string }
		if err := decode(r, &b); err != nil {
			return nil, err
		}
		if err := l.eng.Undo(s, b.Ref); err != nil {
			return nil, err
		}
		return l.context(s, pool, b.Ref), nil
	}))
	mux.HandleFunc("POST /api/learning/dismiss", l.change(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		var b struct{ Ref string }
		if err := decode(r, &b); err != nil {
			return nil, err
		}
		if err := l.eng.Dismiss(s, pool, b.Ref, l.now()); err != nil {
			return nil, err
		}
		return l.context(s, pool, b.Ref), nil
	}))
	mux.HandleFunc("POST /api/learning/tracks/{track}/chapters/{chapter}/skip-to-practice", l.change(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		if err := l.eng.SkipToPractice(s, r.PathValue("track"), r.PathValue("chapter"), l.now()); err != nil {
			return nil, err
		}
		return l.chapter(s, pool, r)
	}))
	mux.HandleFunc("POST /api/learning/tracks/{track}/chapters/{chapter}/checkpoint", l.change(func(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
		var b struct {
			Met    []bool        `json:"met"`
			Rating engine.Rating `json:"rating"`
			Worth  *bool         `json:"worth"`
			Force  bool          `json:"force"`
		}
		if err := decode(r, &b); err != nil {
			return nil, err
		}
		res, err := l.eng.Checkpoint(s, pool, r.PathValue("track"), r.PathValue("chapter"), b.Met, b.Rating, b.Worth, b.Force, l.now())
		if err != nil {
			return nil, err
		}
		ch, _ := l.chapter(s, pool, r)
		return map[string]any{"result": res, "chapter": ch}, nil
	}))
	mux.HandleFunc("POST /api/learning/tracks/{track}/chapters/{chapter}/snooze", l.change(func(s *engine.State, _ []engine.Item, r *http.Request) (any, error) {
		return nil, l.eng.Snooze(s, r.PathValue("track"), r.PathValue("chapter"))
	}))
	mux.HandleFunc("PUT /api/learning/assessment", l.change(func(s *engine.State, _ []engine.Item, r *http.Request) (any, error) {
		var b struct {
			Answers map[string]string `json:"answers"`
		}
		if err := decode(r, &b); err != nil {
			return nil, err
		}
		return nil, l.eng.Assess(s, b.Answers)
	}))
	mux.HandleFunc("PUT /api/learning/settings", l.change(func(s *engine.State, _ []engine.Item, r *http.Request) (any, error) {
		next := s.Settings
		if err := decode(r, &next); err != nil {
			return nil, err
		}
		if next.Priorities == nil {
			next.Priorities = map[string]float64{}
		}
		if err := l.eng.ValidSettings(next); err != nil {
			return nil, err
		}
		s.Settings = next
		return s.Settings, nil
	}))
}

// context renders the chapter a ref belongs to ({"chapter": null} if none).
func (l *learning) context(s *engine.State, pool []engine.Item, ref string) map[string]any {
	out := map[string]any{"chapter": nil}
	if tr, ch := l.eng.Context(s, ref); tr != "" {
		v, _ := l.eng.Chapter(s, pool, tr, ch, l.now())
		out["chapter"] = v
	}
	return out
}

func (l *learning) chapter(s *engine.State, pool []engine.Item, r *http.Request) (any, error) {
	return l.eng.Chapter(s, pool, r.PathValue("track"), r.PathValue("chapter"), l.now())
}

type handler func(s *engine.State, pool []engine.Item, r *http.Request) (any, error)

func (l *learning) load(ctx context.Context) (*engine.State, []engine.Item, error) {
	s, err := l.store.LoadLearning(ctx)
	if err != nil {
		return nil, nil, err
	}
	pool, err := l.store.MentorPool(ctx)
	return s, pool, err
}

func (l *learning) read(h handler) http.HandlerFunc { return l.serve(h, false) }

func (l *learning) change(h handler) http.HandlerFunc { return l.serve(h, true) }

func (l *learning) serve(h handler, save bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l.mu.Lock()
		defer l.mu.Unlock()
		s, pool, err := l.load(r.Context())
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		out, err := h(s, pool, r)
		if err == nil && save {
			err = l.store.SaveLearning(r.Context(), s)
		}
		switch {
		case errors.Is(err, engine.ErrNotFound):
			writeError(w, 404, "not found")
		case errors.Is(err, engine.ErrInvalid):
			writeError(w, 400, err.Error())
		case err != nil:
			writeError(w, 500, err.Error())
		case out == nil:
			w.WriteHeader(http.StatusNoContent)
		default:
			writeJSON(w, 200, out)
		}
	}
}

func invalid(msg string) error { return errors.Join(engine.ErrInvalid, errors.New(msg)) }

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return invalid("bad JSON body: " + err.Error())
	}
	return nil
}
