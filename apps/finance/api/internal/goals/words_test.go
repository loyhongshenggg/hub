package goals

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

var day0 = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)

func TestFromWordsMixed(t *testing.T) {
	r := FromWords(`I want a 6-month emergency fund of about S$18k by end of 2027 — I have S$6k now and can put aside S$700 a month.
I also order too much food delivery; I'd like to keep it under S$200 a month.
I want to invest $800 every month.
Pay off my S$5,000 credit card debt by June 2027.
At most 8 Grab rides a month.`, day0, map[string]int64{"delivery": 31000})
	if len(r.Goals) != 5 {
		for _, g := range r.Goals {
			t.Logf("%+v", g)
		}
		t.Fatalf("got %d goals", len(r.Goals))
	}
	ef := r.Goals[0]
	if ef.Kind != Target || ef.Title != "Emergency fund" || ef.Target != 1_800_000 || ef.StartAmount != 600_000 || ef.MonthlyPlan != 70_000 || ef.TargetDate != "2027-12-31" || ef.Area != "safety" {
		t.Errorf("emergency fund: %+v", ef)
	}
	if len(ef.Milestones) != 1 || *ef.Milestones[0].Amount != 1_200_000 {
		t.Errorf("milestones %+v", ef.Milestones)
	}
	fd := r.Goals[1]
	if fd.Kind != Limit || fd.Target != 20_000 || fd.Period != "month" || len(fd.Filter.Categories) != 1 || fd.Filter.Categories[0] != "delivery" || fd.Title != "Food delivery under S$200" {
		t.Errorf("delivery: %+v", fd)
	}
	inv := r.Goals[2]
	if inv.Kind != Habit || inv.Target != 80_000 || inv.Area != "investing" || inv.Title != "Invest S$800 a month" {
		t.Errorf("invest: %+v", inv)
	}
	debt := r.Goals[3]
	if debt.Kind != Payoff || debt.StartAmount != 500_000 || debt.TargetDate != "2027-06-30" || debt.Title != "Pay off card debt" {
		t.Errorf("debt: %+v", debt)
	}
	rides := r.Goals[4]
	if rides.Kind != Limit || rides.Metric != "count" || rides.Target != 8 || rides.Filter.Categories[0] != "rides" {
		t.Errorf("rides: %+v", rides)
	}
	if !strings.Contains(strings.Join(r.Assumptions, " "), "Food delivery is S$310 a month") {
		t.Errorf("assumptions %v", r.Assumptions)
	}
	if len(r.Reasoning) != 5 || !strings.HasPrefix(r.Reasoning[0], "Read on this computer") {
		t.Errorf("reasoning %v", r.Reasoning)
	}
}

func TestFromWordsDatesAndAmounts(t *testing.T) {
	cases := []struct {
		text, date string
		target     int64
		title      string
	}{
		{"Save 160,000 sgd for the wedding and renovation by 07/2029", "2029-07-31", 16_000_000, "Wedding & renovation fund"},
		{"We need $155,025 for the BTO downpayment by 30/09/2029", "2029-09-30", 15_502_500, "Home fund"},
		{"Build a 1.5k travel pot in 6 months", "2027-05-21", 150_000, "Travel fund"},
		{"Car fund of $40k by 2030", "2030-12-31", 4_000_000, "Car fund"},
		{"Save S$20k by mid 2028", "2028-06-30", 2_000_000, "Savings goal"},
	}
	for _, c := range cases {
		r := FromWords(c.text, day0, nil)
		if len(r.Goals) != 1 {
			t.Errorf("%q: %d goals", c.text, len(r.Goals))
			continue
		}
		g := r.Goals[0]
		if g.TargetDate != c.date || g.Target != c.target || g.Title != c.title {
			t.Errorf("%q → date %s target %d title %q", c.text, g.TargetDate, g.Target, g.Title)
		}
	}
}

func TestFromWordsLivingCostsYearAndQuestions(t *testing.T) {
	r := FromWords("Keep my living costs under $1,500 a month. Insurance no more than $6,000 a year. Save for a house.", day0, nil)
	if len(r.Goals) != 2 {
		t.Fatalf("%d goals: %+v", len(r.Goals), r.Goals)
	}
	if g := r.Goals[0]; g.Title != "Living costs under S$1,500" || len(g.Filter.Categories) < 10 {
		t.Errorf("living: %q %v", g.Title, g.Filter.Categories)
	}
	if g := r.Goals[1]; g.Period != "year" || g.Target != 600_000 || g.Filter.Categories[0] != "insurance" || g.Area != "safety" {
		t.Errorf("insurance: %+v", g)
	}
	// "Save for a house" has no amount: not a goal, no crash.
	none := FromWords("I'd like to be better with money.", day0, nil)
	if len(none.Goals) != 0 || len(none.Questions) != 1 {
		t.Errorf("none: %+v", none)
	}
}

func TestFromWordsIgnoresYearsAndPercent(t *testing.T) {
	r := FromWords("By 2029 I want 25% of the flat paid, about $172k.", day0, nil)
	if len(r.Goals) != 1 || r.Goals[0].Target != 17_200_000 || r.Goals[0].TargetDate != "2029-12-31" {
		t.Errorf("%+v", r.Goals)
	}
}

func TestFromWordsJoinedGoals(t *testing.T) {
	titles := func(text string) []string {
		var out []string
		for _, g := range FromWords(text, today, nil).Goals {
			out = append(out, fmt.Sprintf("%s=%d", g.Kind, g.Target+g.StartAmount*boolInt(g.Kind == Payoff)))
		}
		return out
	}
	for text, want := range map[string][]string{
		// Two goals in one sentence.
		"I want to save 20k for an emergency fund by end 2027, and keep food delivery under S$250 a month.": {"target=2000000", "limit=25000"},
		"Save S$30k for the wedding by June 2028 and invest S$500 a month":                                  {"target=3000000", "habit=50000"},
		"Pay off my S$8k car loan by 2027; keep dining under S$400 a month":                                 {"payoff=800000", "limit=40000"},
		// One goal with details: stays one goal.
		"Save S$20k for emergencies by 2027, and I have S$6k now": {"target=2000000"},
	} {
		if got := titles(text); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("%q\n got %v\nwant %v", text, got, want)
		}
	}
	// The follow-up still lands on the savings goal.
	r := FromWords("Save S$20k for emergencies by 2027, and I have S$6k now and can save S$700 a month.", today, nil)
	if len(r.Goals) != 1 || r.Goals[0].StartAmount != 6_000_00 || r.Goals[0].MonthlyPlan != 700_00 {
		t.Errorf("follow-up: %+v", r.Goals)
	}
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
