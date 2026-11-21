package main

import (
	"context"
	"errors"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/claude"
	"hub.localhost/apps/finance/api/internal/goals"
	"hub.localhost/apps/finance/api/internal/store"
)

// goalView is a goal with its computed progress.
type goalView struct {
	store.Goal
	Progress goals.Progress    `json:"progress"`
	Entries  []store.GoalEntry `json:"entries,omitempty"`
}

func (a *api) view(ctx context.Context, g store.Goal, txns []store.Txn, dataEnd string, withEntries bool) (goalView, error) {
	entries, err := a.store.GoalEntries(ctx, g.ID)
	if err != nil {
		return goalView{}, err
	}
	v := goalView{Goal: g, Progress: goals.Compute(g, entries, txns, a.now(), dataEnd)}
	if withEntries {
		v.Entries = entries
	}
	return v, nil
}

func (a *api) spendingTxns(ctx context.Context) ([]store.Txn, string, error) {
	txns, err := a.store.Txns(ctx, store.TxnFilter{Spending: true, From: a.now().AddDate(-2, 0, 0).Format("2006-01-02")})
	if err != nil {
		return nil, "", err
	}
	_, last, err := a.store.DataRange(ctx)
	return txns, last, err
}

func (a *api) listGoals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gs, err := a.store.Goals(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	txns, last, err := a.spendingTxns(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]goalView, 0, len(gs))
	for _, g := range gs {
		v, err := a.view(ctx, g, txns, last, false)
		if err != nil {
			fail(w, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, 200, map[string]any{"goals": out})
}

func (a *api) getGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	ctx := r.Context()
	g, err := a.store.GetGoal(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	txns, last, err := a.spendingTxns(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	v, err := a.view(ctx, g, txns, last, true)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, v)
}

// validateGoal normalises and checks a goal from the UI.
func validateGoal(g *store.Goal, today time.Time) error {
	g.Title = strings.TrimSpace(g.Title)
	if g.Title == "" {
		return errors.New("a title is required")
	}
	if !slices.Contains(goals.Kinds, g.Kind) {
		return errors.New("kind must be target, payoff, limit or habit")
	}
	if g.Area == "" {
		g.Area = "other"
	}
	if !slices.Contains(goals.Areas, g.Area) {
		return errors.New("unknown area")
	}
	switch g.Status {
	case "":
		g.Status = "active"
	case "active", "paused", "achieved", "archived":
	default:
		return errors.New("unknown status")
	}
	if g.StartDate == "" {
		g.StartDate = today.Format("2006-01-02")
	}
	if !reDate.MatchString(g.StartDate) || g.TargetDate != "" && !reDate.MatchString(g.TargetDate) {
		return errors.New("dates must be YYYY-MM-DD")
	}
	if g.Period == "" {
		g.Period = "month"
	}
	if !slices.Contains([]string{"week", "month", "year"}, g.Period) {
		return errors.New("period must be week, month or year")
	}
	if g.Metric == "" {
		g.Metric = "amount"
	}
	if g.Metric != "amount" && g.Metric != "count" {
		return errors.New("metric must be amount or count")
	}
	switch g.Kind {
	case goals.Target, goals.Limit, goals.Habit:
		if g.Target <= 0 {
			return errors.New("set a target amount")
		}
	case goals.Payoff:
		if g.StartAmount <= 0 {
			return errors.New("set the balance you're paying off")
		}
	}
	for _, c := range g.Filter.Categories {
		if !catalog.Valid(c) {
			return errors.New("unknown category " + c)
		}
	}
	if g.Source == "" {
		g.Source = "manual"
	}
	if g.Status == "achieved" && g.AchievedAt == nil {
		t := today
		g.AchievedAt = &t
	}
	if g.Status != "achieved" {
		g.AchievedAt = nil
	}
	return nil
}

func (a *api) saveGoal(w http.ResponseWriter, r *http.Request) {
	var g store.Goal
	if err := decode(r, &g); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	ctx := r.Context()
	if r.Method == http.MethodPut {
		id, ok := pathID(r, "id")
		if !ok {
			writeError(w, 400, "bad id")
			return
		}
		old, err := a.store.GetGoal(ctx, id)
		if err != nil {
			fail(w, err)
			return
		}
		g.ID, g.Source, g.Prompt, g.CreatedAt = id, old.Source, old.Prompt, old.CreatedAt
		if g.Status == "achieved" && old.AchievedAt != nil {
			g.AchievedAt = old.AchievedAt
		}
	} else {
		g.ID = 0
	}
	if err := validateGoal(&g, a.now()); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	id, err := a.store.SaveGoal(ctx, g)
	if err != nil {
		fail(w, err)
		return
	}
	g, _ = a.store.GetGoal(ctx, id)
	txns, last, _ := a.spendingTxns(ctx)
	v, err := a.view(ctx, g, txns, last, true)
	if err != nil {
		fail(w, err)
		return
	}
	code := 200
	if r.Method == http.MethodPost {
		code = 201
	}
	writeJSON(w, code, v)
}

func (a *api) deleteGoal(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteGoal(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *api) addEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	var e store.GoalEntry
	if err := decode(r, &e); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if e.Date == "" {
		e.Date = a.now().Format("2006-01-02")
	}
	if !reDate.MatchString(e.Date) {
		writeError(w, 400, "date must be YYYY-MM-DD")
		return
	}
	ctx := r.Context()
	g, err := a.store.GetGoal(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if g.Kind == goals.Limit {
		writeError(w, 400, "spending-limit goals track themselves from transactions")
		return
	}
	e.GoalID = id
	if _, err := a.store.AddGoalEntry(ctx, e); err != nil {
		fail(w, err)
		return
	}
	a.getGoal(w, r)
}

func (a *api) deleteEntry(w http.ResponseWriter, r *http.Request) {
	id, ok1 := pathID(r, "id")
	eid, ok2 := pathID(r, "entry")
	if !ok1 || !ok2 {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteGoalEntry(r.Context(), id, eid); err != nil {
		fail(w, err)
		return
	}
	a.getGoal(w, r)
}

// draftGoals turns a paragraph into goal drafts (not saved) for the user to
// review. It reads the paragraph on this computer (goals.FromWords); when the
// Claude switch is on and a key is set, Claude reads it instead, falling back
// to the local reader on any error. No feature depends on the key.
func (a *api) draftGoals(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paragraph     string `json:"paragraph"`
		ShareAverages *bool  `json:"shareAverages"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	in.Paragraph = strings.TrimSpace(in.Paragraph)
	if len(in.Paragraph) < 10 || len(in.Paragraph) > 6000 {
		writeError(w, 400, "write a few sentences (up to 6,000 characters)")
		return
	}
	ctx := r.Context()
	today := a.now()
	averages, monthly := a.categoryAverages(ctx, today)

	cfg, _ := a.claude.Settings(ctx)
	key, _ := a.claude.Key(cfg)
	var fallbackNote string
	if cfg.Goals && key != "" {
		share := cfg.ShareAverages
		if in.ShareAverages != nil {
			share = *in.ShareAverages
		}
		gc := claude.GoalContext{Today: today.Format("2006-01-02")}
		if share {
			gc.Averages, gc.MonthlySpend = averages, monthly
		}
		if gs, err := a.store.Goals(ctx); err == nil {
			for _, g := range gs {
				if g.Status == "active" {
					gc.Existing = append(gc.Existing, g.Title)
				}
			}
		}
		d, err := a.claude.DraftGoals(ctx, in.Paragraph, gc)
		if err == nil {
			a.writeClaudeDrafts(w, d, in.Paragraph, today, len(gc.Averages) > 0)
			return
		}
		fallbackNote = "Claude wasn't available (" + err.Error() + "), so this was read on this computer."
	}
	rd := goals.FromWords(in.Paragraph, today, averages)
	for i := range rd.Goals {
		rd.Goals[i].Prompt = in.Paragraph
	}
	assumptions := rd.Assumptions
	if fallbackNote != "" {
		assumptions = append([]string{fallbackNote}, assumptions...)
	}
	writeJSON(w, 200, map[string]any{"drafts": rd.Goals, "reasoning": nonNil(rd.Reasoning), "assumptions": nonNil(assumptions),
		"questions": nonNil(rd.Questions), "sharedAverages": false, "engine": "local"})
}

// categoryAverages is the average monthly spend per category over the last
// months with data (local only; sent to Claude only if the user allows).
func (a *api) categoryAverages(ctx context.Context, today time.Time) (map[string]int64, int64) {
	all, err := a.store.Txns(ctx, store.TxnFilter{Spending: true, From: today.AddDate(0, -7, 0).Format("2006-01-02")})
	if err != nil || len(all) == 0 {
		return nil, 0
	}
	avg := map[string]int64{}
	months := map[string]bool{}
	for _, t := range all {
		if t.Excluded {
			continue
		}
		months[t.Date[:7]] = true
		avg[catOrOther(t)] += t.Mine()
	}
	var total int64
	for k := range avg {
		avg[k] /= int64(max(1, len(months)))
		total += avg[k]
	}
	return avg, total
}

func (a *api) writeClaudeDrafts(w http.ResponseWriter, d *claude.Drafts, paragraph string, today time.Time, shared bool) {
	drafts := make([]store.Goal, 0, len(d.Goals))
	reasons := make([]string, 0, len(d.Goals))
	for _, dg := range d.Goals {
		g := store.Goal{Kind: dg.Kind, Title: dg.Title, Why: dg.Why, Area: dg.Area, Status: "active",
			Target: cents(dg.TargetAmount), StartAmount: cents(dg.StartAmount), StartDate: today.Format("2006-01-02"),
			TargetDate: dg.TargetDate, Period: dg.Period, Metric: dg.Metric, MonthlyPlan: cents(dg.MonthlyPlan),
			Filter: store.GoalFilter{Categories: validCats(dg.Categories), Merchants: dg.Merchants, Keywords: dg.Keywords},
			Source: "claude", Prompt: paragraph, Milestones: []store.Milestone{}}
		if g.Metric == "count" {
			g.Target = int64(math.Round(dg.TargetAmount))
		}
		if g.TargetDate != "" && !reDate.MatchString(g.TargetDate) {
			g.TargetDate = ""
		}
		for _, m := range dg.Milestones {
			ms := store.Milestone{Label: m.Label}
			if m.Amount > 0 {
				v := cents(m.Amount)
				ms.Amount = &v
			}
			if reDate.MatchString(m.Date) {
				ms.Date = m.Date
			}
			g.Milestones = append(g.Milestones, ms)
		}
		drafts = append(drafts, g)
		reasons = append(reasons, dg.Reasoning)
	}
	writeJSON(w, 200, map[string]any{"drafts": drafts, "reasoning": reasons, "assumptions": nonNil(d.Assumptions),
		"questions": nonNil(d.Questions), "sharedAverages": shared, "engine": "claude"})
}

func catOrOther(t store.Txn) string {
	if t.Category == "" {
		return "other"
	}
	return t.Category
}

func cents(dollars float64) int64 { return int64(math.Round(dollars * 100)) }

func validCats(cs []string) []string {
	out := []string{}
	for _, c := range cs {
		if catalog.Valid(c) {
			out = append(out, c)
		}
	}
	return out
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
