package main

import (
	"net/http"
	"strings"

	"hub.localhost/apps/finance/api/internal/split"
	"hub.localhost/apps/finance/api/internal/store"
)

// Splits: a cost you paid that's shared with others. The card amount stays
// as charged (statements must reconcile); each other person's share is
// money owed to you, and only your share counts as your spending.

func (a *api) splitRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/transactions/{id}/split", a.getSplit)
	mux.HandleFunc("PUT /api/transactions/{id}/split", a.putSplit)
	mux.HandleFunc("DELETE /api/transactions/{id}/split", a.deleteSplit)
	mux.HandleFunc("GET /api/people", a.listPeople)
	mux.HandleFunc("GET /api/people/{id}/shares", a.personShares)
	mux.HandleFunc("PATCH /api/people/{id}", a.renamePerson)
	mux.HandleFunc("POST /api/people/{id}/settle", a.settlePerson)
	mux.HandleFunc("POST /api/shares/{id}/settle", a.settleShare)
}

type splitView struct {
	Transaction store.Txn     `json:"transaction"`
	Shares      []store.Share `json:"shares"`
}

func (a *api) writeSplit(w http.ResponseWriter, r *http.Request, id int64) {
	t, err := a.store.GetTxn(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	shares, err := a.store.TxnShares(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, splitView{Transaction: t, Shares: shares})
}

func (a *api) getSplit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	a.writeSplit(w, r, id)
}

func (a *api) putSplit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	var in struct {
		Method   string       `json:"method"`   // equal | shares | exact
		MyWeight float64      `json:"myWeight"` // shares
		Parts    []split.Part `json:"parts"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	t, err := a.store.GetTxn(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if t.Kind == "payment" || t.Kind == "transfer" {
		writeError(w, 400, "card payments and transfers can't be split")
		return
	}
	if in.Method == split.Shares && in.MyWeight < 0 {
		writeError(w, 400, "your share can't be negative")
		return
	}
	res, err := split.Compute(t.Amount, in.Method, in.MyWeight, in.Parts)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	shares := make([]store.NewShare, 0, len(in.Parts))
	for _, p := range in.Parts {
		wgt := p.Weight
		if in.Method != split.Shares {
			wgt = 1
		}
		shares = append(shares, store.NewShare{Name: strings.TrimSpace(p.Name), Amount: res.Others[strings.TrimSpace(p.Name)], Weight: wgt})
	}
	myWeight := in.MyWeight
	if in.Method != split.Shares {
		myWeight = 1
	}
	if err := a.store.SetSplit(ctx, id, in.Method, myWeight, shares); err != nil {
		fail(w, err)
		return
	}
	a.writeSplit(w, r, id)
}

func (a *api) deleteSplit(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.ClearSplit(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	a.writeSplit(w, r, id)
}

func (a *api) listPeople(w http.ResponseWriter, r *http.Request) {
	people, err := a.store.People(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	owed, err := a.store.OwedToYou(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"people": people, "owedToYou": owed})
}

func (a *api) personShares(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	shares, err := a.store.PersonShares(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"shares": shares})
}

func (a *api) renamePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	var in struct {
		Name string `json:"name"`
	}
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeError(w, 400, "name required")
		return
	}
	if err := a.store.RenamePerson(r.Context(), id, in.Name); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeError(w, 409, "someone already has that name")
			return
		}
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *api) settlePerson(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	n, err := a.store.SettlePerson(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]int64{"settled": n})
}

func (a *api) settleShare(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	var in struct {
		Settled bool `json:"settled"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err := a.store.SettleShare(r.Context(), id, in.Settled); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
