package goals

import (
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/store"
)

var today = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

func TestTargetForecast(t *testing.T) {
	g := store.Goal{Kind: Target, Target: 20_000_00, StartAmount: 5_000_00, StartDate: "2026-05-01", TargetDate: "2027-12-31", Status: "active",
		Milestones: []store.Milestone{{Label: "Half way", Amount: ptr(10_000_00)}, {Label: "Three months of expenses", Amount: ptr(9_000_00)}}}
	// +1,000/month since May.
	entries := []store.GoalEntry{{Date: "2026-08-01", Amount: 8_000_00}, {Date: "2026-11-01", Amount: 11_000_00}}
	p := Compute(g, entries, nil, today, "")
	if p.Current != 11_000_00 || p.Remaining != 9_000_00 {
		t.Fatalf("current %d remaining %d", p.Current, p.Remaining)
	}
	if p.Percent < 0.54 || p.Percent > 0.56 {
		t.Errorf("percent %.2f", p.Percent)
	}
	// 9,000 at ~1,000/month ≈ Sep 2027 → ahead of Dec 2027.
	if p.Status != "ahead" || p.ProjectedDate[:4] != "2027" {
		t.Errorf("status %s projected %s (%s)", p.Status, p.ProjectedDate, p.StatusText)
	}
	// 9,000 over ~13.3 months.
	if p.NeededPerMonth < 66_000 || p.NeededPerMonth > 69_000 {
		t.Errorf("needed/month %d", p.NeededPerMonth)
	}
	if !p.Milestones[0].Reached || !p.Milestones[1].Reached {
		t.Errorf("milestones %+v", p.Milestones)
	}
	// Tight deadline → behind.
	g.TargetDate = "2027-02-28"
	if p := Compute(g, entries, nil, today, ""); p.Status != "behind" {
		t.Errorf("tight deadline: %s", p.Status)
	}
	// Reached.
	entries = append(entries, store.GoalEntry{Date: "2026-11-20", Amount: 20_500_00})
	if p := Compute(g, entries, nil, today, ""); p.Status != "achieved" || p.Percent != 1 {
		t.Errorf("reached: %s %.2f", p.Status, p.Percent)
	}
}

func TestTargetPlanForecast(t *testing.T) {
	// No check-ins yet: the monthly plan drives the forecast.
	g := store.Goal{Kind: Target, Target: 160_000_00, StartAmount: 75_000_00, StartDate: "2026-11-21", TargetDate: "2029-07-01", MonthlyPlan: 2_600_00, Status: "active"}
	p := Compute(g, nil, nil, today, "")
	// 85,000 at 2,600/month ≈ 32.7 months → Aug 2029: just after the date.
	if p.Status != "at_risk" || p.ProjectedDate[:7] != "2029-08" || !strings.HasPrefix(p.StatusText, "At S$2,600/month: reached Aug 2029") {
		t.Errorf("%s %s %q", p.Status, p.ProjectedDate, p.StatusText)
	}
	g.MonthlyPlan = 3_000_00
	if p := Compute(g, nil, nil, today, ""); p.Status != "ahead" && p.Status != "on_track" {
		t.Errorf("bigger plan: %s %q", p.Status, p.StatusText)
	}
}

func TestTargetNoData(t *testing.T) {
	g := store.Goal{Kind: Target, Target: 10_000_00, StartDate: "2026-11-01", Status: "active"}
	if p := Compute(g, nil, nil, today, ""); p.Status != "no_data" {
		t.Errorf("status %s", p.Status)
	}
}

func TestPayoff(t *testing.T) {
	g := store.Goal{Kind: Payoff, StartAmount: 6_000_00, StartDate: "2026-06-01", TargetDate: "2027-09-01", Status: "active"}
	entries := []store.GoalEntry{{Date: "2026-09-01", Amount: 4_500_00}, {Date: "2026-11-01", Amount: 3_500_00}}
	p := Compute(g, entries, nil, today, "")
	if p.Remaining != 3_500_00 || p.Percent < 0.41 || p.Percent > 0.42 {
		t.Errorf("remaining %d percent %.3f", p.Remaining, p.Percent)
	}
	if p.Status != "ahead" && p.Status != "on_track" {
		t.Errorf("status %s (%s)", p.Status, p.StatusText)
	}
	entries = append(entries, store.GoalEntry{Date: "2026-11-15", Amount: 0})
	if p := Compute(g, entries, nil, today, ""); p.Status != "achieved" {
		t.Errorf("paid off: %s", p.Status)
	}
}

func txn(date, cat, merchant string, cents int64) store.Txn {
	return store.Txn{Date: date, Category: cat, Merchant: merchant, Raw: merchant, Amount: cents, Kind: "purchase"}
}

func TestLimit(t *testing.T) {
	g := store.Goal{Kind: Limit, Target: 250_00, Period: "month", Metric: "amount", StartDate: "2026-08-01", Status: "active",
		Filter: store.GoalFilter{Categories: []string{"delivery"}}}
	txns := []store.Txn{
		txn("2026-09-10", "delivery", "foodpanda", 200_00),
		txn("2026-10-03", "delivery", "foodpanda", 120_00),
		txn("2026-10-20", "delivery", "Deliveroo", 150_00),
		txn("2026-10-21", "dining", "Kopitiam", 900_00), // not counted
		txn("2026-11-02", "delivery", "foodpanda", 90_00),
		txn("2026-11-04", "delivery", "foodpanda", 50_00),
	}
	// Data runs to 4 Nov: 140 in 4 days → on pace for ~1,050.
	p := Compute(g, nil, txns, today, "2026-11-04")
	if p.Current != 140_00 || p.Status != "at_risk" || p.Projected < 1000_00 {
		t.Errorf("current %d status %s projected %d", p.Current, p.Status, p.Projected)
	}
	if len(p.History) != 4 || p.History[0].Label != "August 2026" {
		t.Fatalf("history %+v", p.History)
	}
	if *p.History[1].Met != true || *p.History[2].Met != false {
		t.Errorf("met: %v %v", *p.History[1].Met, *p.History[2].Met)
	}
	if p.Streak != 0 { // October was over
		t.Errorf("streak %d", p.Streak)
	}
	// Count metric by keyword.
	g2 := store.Goal{Kind: Limit, Target: 1, Period: "month", Metric: "count", StartDate: "2026-11-01", Status: "active",
		Filter: store.GoalFilter{Keywords: []string{"panda"}}}
	if p := Compute(g2, nil, txns, today, "2026-11-30"); p.Current != 2 || p.Status != "over" {
		t.Errorf("count: %d %s", p.Current, p.Status)
	}
	// No data this month.
	if p := Compute(g, nil, txns, time.Date(2026, 12, 5, 0, 0, 0, 0, time.UTC), "2026-11-04"); p.Status != "no_data" {
		t.Errorf("no data: %s", p.Status)
	}
}

func TestHabit(t *testing.T) {
	g := store.Goal{Kind: Habit, Target: 800_00, Period: "month", StartDate: "2026-08-01", Status: "active"}
	entries := []store.GoalEntry{
		{Date: "2026-08-05", Amount: 800_00}, {Date: "2026-09-05", Amount: 800_00}, {Date: "2026-10-05", Amount: 500_00},
		{Date: "2026-10-25", Amount: 300_00}, {Date: "2026-11-05", Amount: 200_00},
	}
	p := Compute(g, entries, nil, time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC), "")
	if p.Current != 200_00 || p.Status != "at_risk" || p.Streak != 3 {
		t.Errorf("current %d status %s streak %d", p.Current, p.Status, p.Streak)
	}
}

func TestPausedAndAchievedLabels(t *testing.T) {
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	g := store.Goal{Kind: Target, Target: 100, StartDate: "2026-01-01", Status: "achieved", AchievedAt: &at}
	if p := Compute(g, nil, nil, today, ""); p.Status != "achieved" || p.StatusText != "Achieved Mar 2026" {
		t.Errorf("%s %q", p.Status, p.StatusText)
	}
}

func ptr(v int64) *int64 { return &v }
