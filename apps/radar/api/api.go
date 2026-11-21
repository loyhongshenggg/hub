package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

// fetcher is the part of fetch.Fetcher the API uses.
type fetcher interface {
	Trigger()
	Status() (running bool, last time.Time)
}

type api struct {
	store      *store.Store
	fetcher    fetcher
	categories []content.Category
	curriculum *content.Curriculum
	// window is how far back News goes (mentor feeds keep older items for
	// learning). Zero: no limit.
	window time.Duration
}

// freshDays is how recent a mentor item must be to count as fresh.
const freshDays = 60

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/news", a.news)
	mux.HandleFunc("POST /api/news/read-all", a.readAll)
	mux.HandleFunc("POST /api/items/{id}/read", a.flag(a.store.SetRead, "read"))
	mux.HandleFunc("POST /api/items/{id}/saved", a.flag(a.store.SetSaved, "saved"))
	mux.HandleFunc("PUT /api/items/{id}/topics", a.setTopics)
	mux.HandleFunc("GET /api/curriculum", a.curriculumHandler)
	mux.HandleFunc("GET /api/learning/fresh", a.fresh)
	mux.HandleFunc("GET /api/sources", a.sources)
	mux.HandleFunc("GET /api/summary", a.summary)
	mux.HandleFunc("POST /api/refresh", a.refresh)
	mux.HandleFunc("POST /api/visit", a.visit)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { writeError(w, 404, "no such endpoint") })
}

func filterFrom(r *http.Request) (store.Filter, error) {
	q := r.URL.Query()
	f := store.Filter{
		Category: q.Get("category"),
		Source:   q.Get("source"),
		Query:    q.Get("q"),
		Unread:   q.Get("unread") == "1",
		Saved:    q.Get("saved") == "1",
		Topic:    q.Get("topic"),
		Tagged:   q.Get("tagged") == "1",
		Mentor:   q.Get("mentor"),
	}
	if f.Category == "all" {
		f.Category = ""
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return f, errors.New("limit must be a positive integer")
		}
		f.Limit = n
	}
	if c := q.Get("cursor"); c != "" {
		if _, err := fmt.Sscanf(c, "%d.%d", &f.BeforeTime, &f.BeforeID); err != nil {
			return f, errors.New("bad cursor")
		}
	}
	return f, nil
}

// since is the start of the News window.
func (a *api) since() time.Time {
	if a.window == 0 {
		return time.Unix(0, 0)
	}
	return time.Now().Add(-a.window)
}

func (a *api) news(w http.ResponseWriter, r *http.Request) {
	f, err := filterFrom(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	f.Since = a.since().Unix()
	items, next, err := a.store.Items(r.Context(), f)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

func (a *api) readAll(w http.ResponseWriter, r *http.Request) {
	f, err := filterFrom(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	f.Since = a.since().Unix()
	n, err := a.store.MarkAllRead(r.Context(), f)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int64{"marked": n})
}

// flag handles POST /api/items/{id}/<field> with body {"<field>": bool}.
func (a *api) flag(set func(context.Context, int64, bool) error, field string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeError(w, 400, "bad item id")
			return
		}
		var body map[string]bool
		if err := json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&body); err != nil {
			writeError(w, 400, fmt.Sprintf(`body must be {"%s": true|false}`, field))
			return
		}
		on, ok := body[field]
		if !ok {
			writeError(w, 400, fmt.Sprintf(`body must be {"%s": true|false}`, field))
			return
		}
		switch err := set(r.Context(), id, on); {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, 404, "no such item")
		case err != nil:
			writeError(w, 500, err.Error())
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func (a *api) sources(w http.ResponseWriter, r *http.Request) {
	srcs, err := a.store.Sources(r.Context(), false)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if srcs == nil {
		srcs = []store.Source{}
	}
	running, last := a.fetcher.Status()
	writeJSON(w, 200, map[string]any{"categories": a.categories, "sources": srcs, "fetch": fetchStatus(running, last)})
}

func fetchStatus(running bool, last time.Time) map[string]any {
	s := map[string]any{"running": running, "lastRound": nil}
	if !last.IsZero() {
		s["lastRound"] = last.UTC()
	}
	return s
}

func (a *api) summary(w http.ResponseWriter, r *http.Request) {
	c, err := a.store.Counts(r.Context(), a.since())
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	running, last := a.fetcher.Status()
	writeJSON(w, 200, map[string]any{"counts": c, "fetch": fetchStatus(running, last)})
}

func (a *api) curriculumHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, a.curriculum)
}

// fresh lists recent mentor items ("Fresh from <mentor>"), newest first.
func (a *api) fresh(w http.ResponseWriter, r *http.Request) {
	f, err := filterFrom(r)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	f.MentorsOnly = true
	f.Since = time.Now().AddDate(0, 0, -freshDays).Unix()
	items, next, err := a.store.Items(r.Context(), f)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "nextCursor": next})
}

// setTopics is the hand fix for tags: PUT {"topics": [...]} (best first).
// The rules tagger never overwrites it.
func (a *api) setTopics(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, 400, "bad item id")
		return
	}
	var body struct {
		Topics []string `json:"topics"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil || body.Topics == nil {
		writeError(w, 400, `body must be {"topics": ["topic-id", ...]}`)
		return
	}
	if len(body.Topics) > 5 {
		writeError(w, 400, "at most 5 topics")
		return
	}
	for _, t := range body.Topics {
		if a.curriculum.Topic(t) == nil {
			writeError(w, 400, fmt.Sprintf("unknown topic %q", t))
			return
		}
	}
	tags, _, err := a.store.ItemTags(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, 404, "no such item")
		return
	} else if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	tags.Topics = body.Topics
	if err := a.store.SetTags(r.Context(), map[int64]store.Tags{id: tags}, store.TaggedByUser, ""); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) refresh(w http.ResponseWriter, _ *http.Request) {
	a.fetcher.Trigger()
	w.WriteHeader(http.StatusAccepted)
}

func (a *api) visit(w http.ResponseWriter, r *http.Request) {
	if err := a.store.SetLastVisit(r.Context(), time.Now()); err != nil {
		writeError(w, 500, err.Error())
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
