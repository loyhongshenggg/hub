package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/claude"
	"hub.localhost/apps/finance/api/internal/goals"
	"hub.localhost/apps/finance/api/internal/importer"
	"hub.localhost/apps/finance/api/internal/ocr"
	"hub.localhost/apps/finance/api/internal/store"
)

type api struct {
	store  *store.Store
	imp    *importer.Importer
	claude *claude.Service
	ocr    ocr.Engine
	now    func() time.Time
}

func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", a.status)
	mux.HandleFunc("GET /api/catalog", a.catalog)
	mux.HandleFunc("GET /api/settings/claude", a.getClaude)
	mux.HandleFunc("PUT /api/settings/claude", a.putClaude)
	mux.HandleFunc("POST /api/directory/{source}/refresh", a.refreshDirectory)
	mux.HandleFunc("POST /api/merchants/lookup", a.lookupMerchants)
	mux.HandleFunc("PATCH /api/accounts/{id}", a.renameAccount)

	mux.HandleFunc("GET /api/overview", a.overview)
	mux.HandleFunc("GET /api/transactions", a.listTxns)
	mux.HandleFunc("POST /api/transactions", a.addTxn)
	mux.HandleFunc("PATCH /api/transactions/{id}", a.patchTxn)
	mux.HandleFunc("DELETE /api/transactions/{id}", a.deleteTxn)
	mux.HandleFunc("GET /api/rules", a.listRules)
	mux.HandleFunc("DELETE /api/rules/{key}", a.deleteRule)
	mux.HandleFunc("GET /api/export.csv", a.exportCSV)

	mux.HandleFunc("GET /api/statements", a.listStatements)
	mux.HandleFunc("POST /api/statements", a.upload)
	mux.HandleFunc("GET /api/statements/{id}", a.getStatement)
	mux.HandleFunc("DELETE /api/statements/{id}", a.deleteStatement)
	mux.HandleFunc("POST /api/statements/{id}/reread", a.reread)
	mux.HandleFunc("GET /api/statements/{id}/pages/{page}", a.pageImage)

	mux.HandleFunc("GET /api/goals", a.listGoals)
	mux.HandleFunc("POST /api/goals", a.saveGoal)
	mux.HandleFunc("GET /api/goals/{id}", a.getGoal)
	mux.HandleFunc("PUT /api/goals/{id}", a.saveGoal)
	mux.HandleFunc("DELETE /api/goals/{id}", a.deleteGoal)
	mux.HandleFunc("POST /api/goals/{id}/entries", a.addEntry)
	mux.HandleFunc("DELETE /api/goals/{id}/entries/{entry}", a.deleteEntry)
	mux.HandleFunc("POST /api/goals/draft", a.draftGoals)
	mux.HandleFunc("PUT /api/goals/order", a.reorderGoals)
	a.splitRoutes(mux)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) { writeError(w, 404, "no such endpoint") })
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// fail maps store errors to HTTP codes.
func fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, 404, "not found")
	case errors.Is(err, context.Canceled):
		writeError(w, 499, "cancelled")
	default:
		writeError(w, 500, err.Error())
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.New("bad JSON: " + err.Error())
	}
	return nil
}

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil && id > 0
}

func (a *api) status(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ocrErr := ""
	if err := a.ocr.Available(); err != nil {
		ocrErr = err.Error()
	}
	cfg, err := a.claude.Settings(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	_, keySource := a.claude.Key(cfg)
	usage, _ := a.store.UsageFor(ctx, a.now().Format("2006-01"))
	dir, _ := a.store.DirectorySources(ctx)
	review, _ := a.store.ReviewCount(ctx)
	first, last, _ := a.store.DataRange(ctx)
	accounts, _ := a.store.Accounts(ctx)
	owed, _ := a.store.OwedToYou(ctx)
	writeJSON(w, 200, map[string]any{
		"ocr":       map[string]any{"ok": ocrErr == "", "error": ocrErr},
		"claude":    map[string]any{"keySource": keySource, "goals": cfg.Goals, "merchants": cfg.Merchants, "capCents": cfg.CapCents, "usage": usage},
		"directory": dir,
		"review":    review,
		"dataRange": map[string]string{"first": first, "last": last},
		"accounts":  accounts,
		"owedToYou": owed,
		"today":     a.now().Format("2006-01-02"),
	})
}

func (a *api) catalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, 200, map[string]any{
		"categories": catalog.Categories,
		"groups":     catalog.Groups,
		"kinds":      catalog.Kinds,
		"goalKinds":  goals.Kinds,
		"areas":      goals.Areas,
	})
}

// claudeView is the settings as shown to the UI (the key is never sent back).
type claudeView struct {
	KeySet        bool   `json:"keySet"`
	KeySource     string `json:"keySource"`
	KeyHint       string `json:"keyHint"`
	Goals         bool   `json:"goals"`
	Merchants     bool   `json:"merchants"`
	ShareAverages bool   `json:"shareAverages"`
	CapCents      int64  `json:"capCents"`
	Usage         any    `json:"usage"`
	Model         string `json:"model"`
}

func (a *api) claudeView(ctx context.Context) (claudeView, error) {
	cfg, err := a.claude.Settings(ctx)
	if err != nil {
		return claudeView{}, err
	}
	key, src := a.claude.Key(cfg)
	hint := ""
	if len(key) > 8 {
		hint = "…" + key[len(key)-4:]
	}
	usage, _ := a.store.UsageFor(ctx, a.now().Format("2006-01"))
	return claudeView{KeySet: key != "", KeySource: src, KeyHint: hint, Goals: cfg.Goals, Merchants: cfg.Merchants,
		ShareAverages: cfg.ShareAverages, CapCents: cfg.CapCents, Usage: usage, Model: claude.Model}, nil
}

func (a *api) getClaude(w http.ResponseWriter, r *http.Request) {
	v, err := a.claudeView(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

func (a *api) putClaude(w http.ResponseWriter, r *http.Request) {
	var in struct {
		APIKey        *string `json:"apiKey"` // "" clears
		Goals         *bool   `json:"goals"`
		Merchants     *bool   `json:"merchants"`
		ShareAverages *bool   `json:"shareAverages"`
		CapCents      *int64  `json:"capCents"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	cfg, err := a.claude.Settings(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	if in.APIKey != nil {
		cfg.APIKey = trimKey(*in.APIKey)
	}
	if in.Goals != nil {
		cfg.Goals = *in.Goals
	}
	if in.Merchants != nil {
		cfg.Merchants = *in.Merchants
	}
	if in.ShareAverages != nil {
		cfg.ShareAverages = *in.ShareAverages
	}
	if in.CapCents != nil {
		if *in.CapCents < 0 || *in.CapCents > 100_000 {
			writeError(w, 400, "cap must be between 0 and 1000 dollars")
			return
		}
		cfg.CapCents = *in.CapCents
	}
	if err := a.store.SetSetting(ctx, "claude", cfg); err != nil {
		fail(w, err)
		return
	}
	if cfg.Merchants {
		go a.imp.LookupUnknown(context.WithoutCancel(ctx))
	}
	a.getClaude(w, r)
}

func trimKey(k string) string {
	out := make([]rune, 0, len(k))
	for _, r := range k {
		if r > ' ' && r < 127 {
			out = append(out, r)
		}
	}
	return string(out)
}

func (a *api) refreshDirectory(w http.ResponseWriter, r *http.Request) {
	if err := a.imp.RefreshDirectory(r.PathValue("source")); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	writeJSON(w, 202, map[string]string{"status": "running"})
}

func (a *api) lookupMerchants(w http.ResponseWriter, r *http.Request) {
	n, err := a.imp.LookupUnknown(r.Context())
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]int{"updated": n})
}

func (a *api) renameAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	var in struct {
		Name string `json:"name"`
	}
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if err := decode(r, &in); err != nil || in.Name == "" {
		writeError(w, 400, "name required")
		return
	}
	if err := a.store.RenameAccount(r.Context(), id, in.Name); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}
