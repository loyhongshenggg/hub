package main

import (
	"errors"
	"net/http"

	"hub.localhost/apps/radar/api/internal/engine"
	"hub.localhost/apps/radar/api/internal/reader"
	"hub.localhost/apps/radar/api/internal/store"
)

// readerAPI shows a step, practice or item inside Radar. It takes a ref, not
// a URL, so it can only fetch links Radar already knows about.
type readerAPI struct {
	store  *store.Store
	eng    *engine.Engine
	reader *reader.Reader
}

func (a *readerAPI) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/reader", a.view)
}

func (a *readerAPI) view(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("ref")
	parsed, err := engine.ParseRef(ref)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var link, typ string
	if parsed.Kind == "item" {
		link, typ, err = a.store.ItemLink(r.Context(), parsed.Item)
	} else {
		link, typ, err = a.eng.Link(ref)
	}
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, engine.ErrNotFound):
		writeError(w, 404, "nothing to show for "+ref)
		return
	case errors.Is(err, engine.ErrInvalid):
		writeError(w, 400, err.Error())
		return
	case err != nil:
		writeError(w, 500, err.Error())
		return
	}
	v, err := a.reader.View(r.Context(), link, typ)
	if err != nil {
		writeError(w, 502, err.Error())
		return
	}
	// Articles change rarely; let the browser keep them briefly too.
	w.Header().Set("Cache-Control", "private, max-age=300")
	writeJSON(w, 200, v)
}
