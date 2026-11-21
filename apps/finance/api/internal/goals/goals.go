// Package goals computes progress, status and forecasts for goals
// (docs/finance/design.md §5).
package goals

import (
	"fmt"
	"math"
	"strings"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/store"
)

// Kinds and statuses.
const (
	Target = "target"
	Payoff = "payoff"
	Limit  = "limit"
	Habit  = "habit"
)

var Kinds = []string{Target, Payoff, Limit, Habit}

// Areas are life areas a goal belongs to (long-run grouping).
var Areas = []string{"safety", "home", "retirement", "investing", "debt", "lifestyle", "education", "other"}

// Point is one value on a progress chart.
type Point struct {
	Date  string `json:"date"`  // YYYY-MM-DD (target/payoff) or period start (limit/habit)
	Label string `json:"label"` // display label
	Value int64  `json:"value"`
	Met   *bool  `json:"met,omitempty"` // limit/habit periods
}

// MilestoneState is a milestone with whether it's reached.
type MilestoneState struct {
	store.Milestone
	Reached bool `json:"reached"`
}

// Progress is a goal's computed state.
type Progress struct {
	Current        int64            `json:"current"` // target: saved; payoff: remaining; limit: spent (or count) this period; habit: put in this period
	Percent        float64          `json:"percent"` // 0..1 (limit: share of cap used; may exceed 1)
	Remaining      int64            `json:"remaining"`
	Status         string           `json:"status"` // achieved | ahead | on_track | at_risk | behind | over | no_data | paused
	StatusText     string           `json:"statusText"`
	NeededPerMonth int64            `json:"neededPerMonth,omitempty"`
	ProjectedDate  string           `json:"projectedDate,omitempty"`
	Projected      int64            `json:"projected,omitempty"` // limit: end-of-period spend at current pace
	PeriodLabel    string           `json:"periodLabel,omitempty"`
	PeriodStart    string           `json:"periodStart,omitempty"`
	PeriodEnd      string           `json:"periodEnd,omitempty"`
	History        []Point          `json:"history"`
	Streak         int              `json:"streak"` // consecutive met periods (before the current one)
	Milestones     []MilestoneState `json:"milestones"`
	LastCheckIn    string           `json:"lastCheckIn,omitempty"`
}

const day = 24 * time.Hour

func parse(d string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", d)
	return t, err == nil
}

func fmtDate(t time.Time) string { return t.Format("2006-01-02") }

// Compute returns the progress of g today. txns are spending transactions
// (only used by limit goals); dataEnd is the date of the latest imported
// transaction ("" if none).
func Compute(g store.Goal, entries []store.GoalEntry, txns []store.Txn, today time.Time, dataEnd string) Progress {
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	var p Progress
	switch g.Kind {
	case Target:
		p = balanceGoal(g, entries, today, false)
	case Payoff:
		p = balanceGoal(g, entries, today, true)
	case Limit:
		p = limitGoal(g, txns, today, dataEnd)
	case Habit:
		p = habitGoal(g, entries, today)
	}
	if p.History == nil {
		p.History = []Point{}
	}
	p.Milestones = milestones(g, p)
	switch g.Status {
	case "achieved":
		p.Status, p.StatusText = "achieved", "Achieved"
		if g.AchievedAt != nil {
			p.StatusText = "Achieved " + g.AchievedAt.Format("Jan 2006")
		}
	case "paused":
		p.Status, p.StatusText = "paused", "Paused"
	}
	if len(entries) > 0 {
		p.LastCheckIn = entries[len(entries)-1].Date
	}
	return p
}

// balanceGoal: target (saved grows to target) and payoff (balance falls to 0).
func balanceGoal(g store.Goal, entries []store.GoalEntry, today time.Time, payoff bool) Progress {
	var p Progress
	start, ok := parse(g.StartDate)
	if !ok {
		start = today
	}
	pts := []Point{{Date: fmtDate(start), Label: start.Format("2 Jan 2006"), Value: g.StartAmount}}
	for _, e := range entries {
		pts = append(pts, Point{Date: e.Date, Label: e.Date, Value: e.Amount})
	}
	for i := range pts {
		if t, ok := parse(pts[i].Date); ok {
			pts[i].Label = t.Format("2 Jan 2006")
		}
	}
	p.History = pts
	cur := pts[len(pts)-1].Value
	p.Current = cur
	if payoff {
		owed := max(g.StartAmount, 1)
		p.Remaining = max(cur, 0)
		p.Percent = clamp01(float64(g.StartAmount-cur) / float64(owed))
		if cur <= 0 {
			p.Status, p.StatusText = "achieved", "Paid off"
			return p
		}
	} else {
		p.Remaining = max(g.Target-cur, 0)
		if g.Target > 0 {
			p.Percent = clamp01(float64(cur) / float64(g.Target))
		}
		if g.Target > 0 && cur >= g.Target {
			p.Status, p.StatusText = "achieved", "Target reached"
			return p
		}
	}

	// Trend: least squares over check-ins (value per day). Until check-ins
	// show a trend, the planned monthly amount stands in for it.
	slope, ok := trend(pts)
	if payoff {
		slope = -slope
	}
	plan := ""
	if (!ok || slope <= 0) && g.MonthlyPlan > 0 {
		slope, ok = float64(g.MonthlyPlan)/30.44, true
		plan = "At " + money(g.MonthlyPlan) + "/month: "
	}
	if ok && slope > 0 {
		days := float64(p.Remaining) / slope
		if days < 365*60 {
			p.ProjectedDate = fmtDate(today.Add(time.Duration(days) * day))
		}
	}
	targetDate, hasDate := parse(g.TargetDate)
	if hasDate {
		months := targetDate.Sub(today).Hours() / 24 / 30.44
		if months > 0 {
			p.NeededPerMonth = int64(math.Ceil(float64(p.Remaining) / months))
		} else {
			p.NeededPerMonth = p.Remaining
		}
	}
	switch {
	case len(entries) == 0 && g.MonthlyPlan == 0:
		p.Status, p.StatusText = "no_data", "Add a check-in to start tracking"
	case !ok || slope <= 0:
		if hasDate {
			p.Status, p.StatusText = "at_risk", "No progress yet — "+money(p.NeededPerMonth)+"/month needed"
		} else {
			p.Status, p.StatusText = "no_data", "Add another check-in to see a forecast"
		}
	case !hasDate:
		p.Status, p.StatusText = "on_track", plan+"done by "+monthYear(p.ProjectedDate)
	default:
		proj, _ := parse(p.ProjectedDate)
		when := targetDate.Format("Jan 2006")
		switch {
		case p.ProjectedDate == "" || proj.After(targetDate.Add(60*day)):
			p.Status = "behind"
			if plan != "" {
				p.StatusText = fmt.Sprintf("%sreached %s — %s/month needed for %s", plan, monthYear(p.ProjectedDate), money(p.NeededPerMonth), when)
			} else {
				p.StatusText = fmt.Sprintf("Behind — %s/month needed to make %s", money(p.NeededPerMonth), when)
			}
		case proj.After(targetDate):
			p.Status = "at_risk"
			if plan != "" {
				p.StatusText = fmt.Sprintf("%sreached %s — %s/month makes %s", plan, monthYear(p.ProjectedDate), money(p.NeededPerMonth), when)
			} else {
				p.StatusText = fmt.Sprintf("Slightly behind — %s/month keeps you on track", money(p.NeededPerMonth))
			}
		case proj.Before(targetDate.Add(-60 * day)):
			p.Status, p.StatusText = "ahead", plan+"ahead — on pace for "+monthYear(p.ProjectedDate)
		default:
			p.Status, p.StatusText = "on_track", plan+"on track for "+when
		}
		if plan == "" && (p.Status == "ahead" || p.Status == "on_track") {
			p.StatusText = strings.ToUpper(p.StatusText[:1]) + p.StatusText[1:]
		}
	}
	return p
}

// trend fits value = a + b·days and returns b (per day). Needs two points
// at least a day apart.
func trend(pts []Point) (float64, bool) {
	if len(pts) < 2 {
		return 0, false
	}
	t0, _ := parse(pts[0].Date)
	var n, sx, sy, sxx, sxy float64
	for _, p := range pts {
		t, ok := parse(p.Date)
		if !ok {
			continue
		}
		x := t.Sub(t0).Hours() / 24
		y := float64(p.Value)
		n++
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
	}
	den := n*sxx - sx*sx
	if n < 2 || den < 1e-9 {
		return 0, false
	}
	return (n*sxy - sx*sy) / den, true
}

// periodOf returns the period containing t.
func periodOf(period string, t time.Time) (start, end time.Time, label string) {
	switch period {
	case "week":
		wd := (int(t.Weekday()) + 6) % 7 // Monday start
		start = t.Add(-time.Duration(wd) * day)
		end = start.Add(6 * day)
		label = "Week of " + start.Format("2 Jan")
	case "year":
		start = time.Date(t.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(t.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
		label = start.Format("2006")
	default:
		start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(0, 1, -1)
		label = start.Format("January 2006")
	}
	return
}

func prevPeriod(period string, start time.Time) time.Time {
	switch period {
	case "week":
		return start.Add(-7 * day)
	case "year":
		return start.AddDate(-1, 0, 0)
	}
	return start.AddDate(0, -1, 0)
}

// Matches reports whether a spending transaction counts for a limit goal.
func Matches(f store.GoalFilter, t store.Txn) bool {
	if len(f.Categories) == 0 && len(f.Merchants) == 0 && len(f.Keywords) == 0 {
		return true
	}
	for _, c := range f.Categories {
		if c == t.Category {
			return true
		}
	}
	for _, m := range f.Merchants {
		if strings.EqualFold(strings.TrimSpace(m), t.Merchant) {
			return true
		}
	}
	hay := strings.ToLower(t.Merchant + " " + t.Raw)
	for _, k := range f.Keywords {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" && strings.Contains(hay, k) {
			return true
		}
	}
	return false
}

func sumPeriod(g store.Goal, txns []store.Txn, from, to string) int64 {
	var v int64
	for _, t := range txns {
		if t.Excluded || t.Date < from || t.Date > to || !catalog.Spending(t.Kind) || !Matches(g.Filter, t) {
			continue
		}
		if g.Metric == "count" {
			if t.Kind == catalog.Purchase {
				v++
			}
		} else {
			v += t.Mine()
		}
	}
	return v
}

func limitGoal(g store.Goal, txns []store.Txn, today time.Time, dataEnd string) Progress {
	var p Progress
	start, end, label := periodOf(g.Period, today)
	p.PeriodLabel, p.PeriodStart, p.PeriodEnd = label, fmtDate(start), fmtDate(end)
	spent := sumPeriod(g, txns, p.PeriodStart, p.PeriodEnd)
	p.Current = spent
	p.Remaining = max(g.Target-spent, 0)
	if g.Target > 0 {
		p.Percent = float64(spent) / float64(g.Target)
	}
	// Pace: data runs until the last imported transaction, not today.
	last := today
	if d, ok := parse(dataEnd); ok && d.Before(today) {
		last = d
	}
	total := end.Sub(start).Hours()/24 + 1
	elapsed := last.Sub(start).Hours()/24 + 1
	hasData := dataEnd != "" && dataEnd >= p.PeriodStart
	if hasData && elapsed > 0 && elapsed < total {
		p.Projected = int64(float64(spent) * total / elapsed)
	} else {
		p.Projected = spent
	}
	fmtv := money
	if g.Metric == "count" {
		fmtv = func(v int64) string { return fmt.Sprintf("%d", v) }
	}
	switch {
	case !hasData:
		p.Status, p.StatusText = "no_data", "No transactions yet for "+label+" — import your statement"
	case spent > g.Target:
		p.Status, p.StatusText = "over", fmt.Sprintf("Over by %s", fmtv(spent-g.Target))
	case p.Projected > g.Target && elapsed < total:
		p.Status, p.StatusText = "at_risk", fmt.Sprintf("On pace for %s of %s", fmtv(p.Projected), fmtv(g.Target))
	default:
		p.Status, p.StatusText = "on_track", fmt.Sprintf("%s left", fmtv(g.Target-spent))
	}
	// Last 6 periods (oldest first), only from the goal's start.
	goalStart, _ := parse(g.StartDate)
	s := start
	var hist []Point
	for i := 0; i < 6; i++ {
		ps, pe, pl := periodOf(g.Period, s)
		if i > 0 && !goalStart.IsZero() && pe.Before(goalStart) {
			break
		}
		v := sumPeriod(g, txns, fmtDate(ps), fmtDate(pe))
		met := v <= g.Target
		hist = append([]Point{{Date: fmtDate(ps), Label: pl, Value: v, Met: &met}}, hist...)
		s = prevPeriod(g.Period, ps)
	}
	p.History = hist
	// Streak: consecutive past periods under the cap (with data).
	for i := len(hist) - 2; i >= 0; i-- {
		if hist[i].Date < firstDataPeriod(txns) || !*hist[i].Met {
			break
		}
		p.Streak++
	}
	return p
}

func firstDataPeriod(txns []store.Txn) string {
	first := ""
	for _, t := range txns {
		if first == "" || t.Date < first {
			first = t.Date
		}
	}
	if len(first) >= 7 {
		return first[:7] + "-01"
	}
	return "9999"
}

func habitGoal(g store.Goal, entries []store.GoalEntry, today time.Time) Progress {
	var p Progress
	start, end, label := periodOf(g.Period, today)
	p.PeriodLabel, p.PeriodStart, p.PeriodEnd = label, fmtDate(start), fmtDate(end)
	sum := func(from, to string) int64 {
		var v int64
		for _, e := range entries {
			if e.Date >= from && e.Date <= to {
				v += e.Amount
			}
		}
		return v
	}
	cur := sum(p.PeriodStart, p.PeriodEnd)
	p.Current = cur
	p.Remaining = max(g.Target-cur, 0)
	if g.Target > 0 {
		p.Percent = clamp01(float64(cur) / float64(g.Target))
	}
	elapsed := (today.Sub(start).Hours()/24 + 1) / (end.Sub(start).Hours()/24 + 1)
	switch {
	case cur >= g.Target:
		p.Status, p.StatusText = "on_track", "Done for "+label
	case elapsed > 0.7:
		p.Status, p.StatusText = "at_risk", money(g.Target-cur)+" to go before "+end.Format("2 Jan")
	default:
		p.Status, p.StatusText = "on_track", money(g.Target-cur)+" to go this period"
	}
	goalStart, _ := parse(g.StartDate)
	s := start
	var hist []Point
	for i := 0; i < 6; i++ {
		ps, pe, pl := periodOf(g.Period, s)
		if i > 0 && !goalStart.IsZero() && pe.Before(goalStart) {
			break
		}
		v := sum(fmtDate(ps), fmtDate(pe))
		met := v >= g.Target
		hist = append([]Point{{Date: fmtDate(ps), Label: pl, Value: v, Met: &met}}, hist...)
		s = prevPeriod(g.Period, ps)
	}
	p.History = hist
	for i := len(hist) - 2; i >= 0 && *hist[i].Met; i-- {
		p.Streak++
	}
	return p
}

func milestones(g store.Goal, p Progress) []MilestoneState {
	out := make([]MilestoneState, 0, len(g.Milestones))
	for _, m := range g.Milestones {
		ms := MilestoneState{Milestone: m, Reached: m.DoneAt != ""}
		if m.Amount != nil && !ms.Reached {
			switch g.Kind {
			case Target:
				ms.Reached = p.Current >= *m.Amount
			case Payoff:
				ms.Reached = g.StartAmount-p.Current >= *m.Amount
			}
		}
		out = append(out, ms)
	}
	return out
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func money(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "−", -c
	}
	d := c / 100
	s := fmt.Sprintf("%d", d)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if c%100 != 0 && d < 1000 {
		s += fmt.Sprintf(".%02d", c%100)
	}
	return sign + "S$" + s
}

func monthYear(d string) string {
	if t, ok := parse(d); ok {
		return t.Format("Jan 2006")
	}
	return "—"
}
