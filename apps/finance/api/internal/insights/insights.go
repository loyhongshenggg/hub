// Package insights computes the Overview: this month's spending against
// your usual, category and merchant breakdowns, a 12-month trend, recurring
// charges, and plain-language insight cards. Everything is local.
package insights

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/store"
)

// CategoryStat is one category in a month.
type CategoryStat struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Group  string  `json:"group"`
	Amount int64   `json:"amount"` // net of refunds
	Count  int     `json:"count"`
	Prev   int64   `json:"prev"` // previous month
	Avg    int64   `json:"avg"`  // average of up to 3 earlier months with data
	Share  float64 `json:"share"`
}

// MerchantStat is one merchant in a month.
type MerchantStat struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
	Count    int    `json:"count"`
}

// MonthStat is one month of the trend.
type MonthStat struct {
	Month   string           `json:"month"` // YYYY-MM
	Total   int64            `json:"total"`
	ByGroup map[string]int64 `json:"byGroup"`
	HasData bool             `json:"hasData"`
}

// DayPoint is cumulative spend by day of month.
type DayPoint struct {
	Day   int   `json:"day"`
	Value int64 `json:"value"`
}

// Recurring is a charge that repeats.
type Recurring struct {
	Name     string `json:"name"`
	Key      string `json:"key"`
	Category string `json:"category"`
	Amount   int64  `json:"amount"` // typical charge
	Every    string `json:"every"`  // month | year
	Last     string `json:"last"`   // last date
	Monthly  int64  `json:"monthly"`
}

// Insight is a card on the Overview.
type Insight struct {
	ID       string `json:"id"`
	Tone     string `json:"tone"` // good | info | warn
	Title    string `json:"title"`
	Body     string `json:"body"`
	Category string `json:"category,omitempty"` // click-through
	Merchant string `json:"merchant,omitempty"`
	TxnID    int64  `json:"txnId,omitempty"`
}

// Overview is everything the Overview screen shows for one month.
type Overview struct {
	Month       string         `json:"month"`
	Months      []string       `json:"months"` // months with data, newest first
	Partial     bool           `json:"partial"`
	DataEnd     string         `json:"dataEnd"`
	Spent       int64          `json:"spent"`
	Count       int            `json:"count"`
	PrevSpent   int64          `json:"prevSpent"`
	AvgSpent    int64          `json:"avgSpent"`
	AvgMonths   int            `json:"avgMonths"`
	Refunds     int64          `json:"refunds"`
	Income      int64          `json:"income"`
	Categories  []CategoryStat `json:"categories"`
	Merchants   []MerchantStat `json:"merchants"`
	Trend       []MonthStat    `json:"trend"`
	Pace        []DayPoint     `json:"pace"`
	AvgPace     []DayPoint     `json:"avgPace"`
	Recurring   []Recurring    `json:"recurring"`
	RecurringPM int64          `json:"recurringPerMonth"`
	Insights    []Insight      `json:"insights"`
}

func month(date string) string {
	if len(date) >= 7 {
		return date[:7]
	}
	return ""
}

func addMonths(m string, n int) string {
	t, err := time.Parse("2006-01", m)
	if err != nil {
		return m
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}

func daysIn(m string) int {
	t, _ := time.Parse("2006-01", m)
	return t.AddDate(0, 1, -1).Day()
}

// Build computes the overview of month m from all transactions. spread
// divides one-off charges over 12 months in the trend.
func Build(all []store.Txn, m string, today time.Time, spread bool) Overview {
	ov := Overview{Month: m, Categories: []CategoryStat{}, Merchants: []MerchantStat{}, Insights: []Insight{}, Recurring: []Recurring{}}
	var spend []store.Txn
	monthsSet := map[string]bool{}
	for _, t := range all {
		if t.Date > ov.DataEnd {
			ov.DataEnd = t.Date
		}
		monthsSet[month(t.Date)] = true
		if t.Kind == catalog.Income && !t.Excluded && month(t.Date) == m {
			ov.Income += -t.Mine()
		}
		if catalog.Spending(t.Kind) && !t.Excluded {
			spend = append(spend, t)
		}
	}
	for k := range monthsSet {
		ov.Months = append(ov.Months, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ov.Months)))
	if ov.Months == nil {
		ov.Months = []string{}
	}
	ov.Partial = m == today.Format("2006-01") || month(ov.DataEnd) == m && ov.DataEnd < fmt.Sprintf("%s-%02d", m, daysIn(m))

	// Totals per month / category.
	byMonth := map[string]int64{}
	byMonthCat := map[string]map[string]int64{}
	for _, t := range spend {
		mm := month(t.Date)
		byMonth[mm] += t.Mine()
		if byMonthCat[mm] == nil {
			byMonthCat[mm] = map[string]int64{}
		}
		byMonthCat[mm][catOf(t)] += t.Mine()
	}
	ov.Spent = byMonth[m]
	ov.PrevSpent = byMonth[addMonths(m, -1)]
	var avgMonths []string
	for i := 1; i <= 12 && len(avgMonths) < 3; i++ {
		if pm := addMonths(m, -i); monthsSet[pm] {
			avgMonths = append(avgMonths, pm)
		}
	}
	ov.AvgMonths = len(avgMonths)
	for _, pm := range avgMonths {
		ov.AvgSpent += byMonth[pm]
	}
	if len(avgMonths) > 0 {
		ov.AvgSpent /= int64(len(avgMonths))
	}

	// Categories and merchants this month.
	cats := map[string]*CategoryStat{}
	merch := map[string]*MerchantStat{}
	for _, t := range spend {
		if month(t.Date) != m {
			continue
		}
		if t.Mine() < 0 {
			ov.Refunds += -t.Mine()
		} else {
			ov.Count++
		}
		c := catOf(t)
		cs := cats[c]
		if cs == nil {
			cat, _ := catalog.Get(c)
			cs = &CategoryStat{ID: c, Name: cat.Name, Group: cat.Group}
			cats[c] = cs
		}
		cs.Amount += t.Mine()
		if t.Mine() > 0 {
			cs.Count++
		}
		mk := t.Merchant
		if mk == "" {
			mk = t.Raw
		}
		ms := merch[mk]
		if ms == nil {
			ms = &MerchantStat{Name: mk, Key: t.MerchantKey, Category: c}
			merch[mk] = ms
		}
		ms.Amount += t.Mine()
		if t.Mine() > 0 {
			ms.Count++
		}
	}
	for _, cs := range cats {
		cs.Prev = byMonthCat[addMonths(m, -1)][cs.ID]
		for _, pm := range avgMonths {
			cs.Avg += byMonthCat[pm][cs.ID]
		}
		if len(avgMonths) > 0 {
			cs.Avg /= int64(len(avgMonths))
		}
		if ov.Spent > 0 {
			cs.Share = float64(cs.Amount) / float64(ov.Spent)
		}
		ov.Categories = append(ov.Categories, *cs)
	}
	sort.Slice(ov.Categories, func(i, j int) bool { return ov.Categories[i].Amount > ov.Categories[j].Amount })
	for _, ms := range merch {
		ov.Merchants = append(ov.Merchants, *ms)
	}
	sort.Slice(ov.Merchants, func(i, j int) bool { return ov.Merchants[i].Amount > ov.Merchants[j].Amount })
	if len(ov.Merchants) > 10 {
		ov.Merchants = ov.Merchants[:10]
	}

	ov.Trend = trend(spend, m, spread, monthsSet)
	ov.Pace = pace(spend, []string{m}, m)
	if len(avgMonths) > 0 {
		ov.AvgPace = pace(spend, avgMonths, m)
	}
	ov.Recurring, ov.RecurringPM = recurring(spend)
	ov.Insights = cards(ov, spend, today)
	return ov
}

func catOf(t store.Txn) string {
	if t.Category == "" {
		if t.Kind == catalog.Fee {
			return "fees"
		}
		return "other"
	}
	return t.Category
}

func trend(spend []store.Txn, m string, spread bool, has map[string]bool) []MonthStat {
	out := make([]MonthStat, 12)
	idx := map[string]int{}
	for i := range 12 {
		mm := addMonths(m, i-11)
		out[i] = MonthStat{Month: mm, ByGroup: map[string]int64{}, HasData: has[mm]}
		idx[mm] = i
	}
	add := func(mm, cat string, v int64) {
		i, ok := idx[mm]
		if !ok {
			return
		}
		c, _ := catalog.Get(cat)
		g := c.Group
		if g == "" {
			g = "other"
		}
		out[i].ByGroup[g] += v
		out[i].Total += v
	}
	for _, t := range spend {
		if spread && t.OneOff && t.Mine() > 0 {
			part := t.Mine() / 12
			for k := range 12 {
				v := part
				if k == 0 {
					v += t.Mine() - part*12
				}
				add(addMonths(month(t.Date), k), catOf(t), v)
			}
			continue
		}
		add(month(t.Date), catOf(t), t.Mine())
	}
	return out
}

// pace is the cumulative spend by day of month, averaged over months.
func pace(spend []store.Txn, months []string, m string) []DayPoint {
	n := daysIn(m)
	daily := make([]float64, n+1)
	set := map[string]bool{}
	for _, mm := range months {
		set[mm] = true
	}
	for _, t := range spend {
		if !set[month(t.Date)] || len(t.Date) < 10 {
			continue
		}
		var d int
		fmt.Sscanf(t.Date[8:10], "%d", &d)
		if d > n {
			d = n
		}
		daily[d] += float64(t.Mine())
	}
	out := make([]DayPoint, 0, n)
	var cum float64
	for d := 1; d <= n; d++ {
		cum += daily[d]
		out = append(out, DayPoint{Day: d, Value: int64(math.Round(cum / float64(len(months))))})
	}
	return out
}

// recurring finds charges from the same merchant at a steady amount, about
// monthly or yearly.
func recurring(spend []store.Txn) ([]Recurring, int64) {
	byKey := map[string][]store.Txn{}
	for _, t := range spend {
		if t.Kind != catalog.Purchase || t.MerchantKey == "" || t.MerchantKey == "transit" || t.Mine() < 200 {
			continue
		}
		byKey[t.MerchantKey] = append(byKey[t.MerchantKey], t)
	}
	var out []Recurring
	var perMonth int64
	for key, ts := range byKey {
		sort.Slice(ts, func(i, j int) bool { return ts[i].Date < ts[j].Date })
		// Group charges of a similar amount.
		amounts := map[int64][]store.Txn{}
		for _, t := range ts {
			placed := false
			for a := range amounts {
				if math.Abs(float64(t.Mine()-a)) <= 0.1*float64(a) {
					amounts[a] = append(amounts[a], t)
					placed = true
					break
				}
			}
			if !placed {
				amounts[t.Mine()] = []store.Txn{t}
			}
		}
		for a, group := range amounts {
			if len(group) < 2 {
				continue
			}
			monthly, yearly := 0, 0
			for i := 1; i < len(group); i++ {
				t1, _ := time.Parse("2006-01-02", group[i-1].Date)
				t2, _ := time.Parse("2006-01-02", group[i].Date)
				d := t2.Sub(t1).Hours() / 24
				switch {
				case d >= 25 && d <= 35:
					monthly++
				case d >= 330 && d <= 400:
					yearly++
				}
			}
			last := group[len(group)-1]
			r := Recurring{Name: last.Merchant, Key: key, Category: catOf(last), Amount: a, Last: last.Date}
			switch {
			case monthly >= 1 && monthly >= (len(group)-1)/2:
				r.Every, r.Monthly = "month", a
			case yearly >= 1:
				r.Every, r.Monthly = "year", a/12
			default:
				continue
			}
			perMonth += r.Monthly
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Monthly > out[j].Monthly })
	if out == nil {
		out = []Recurring{}
	}
	return out, perMonth
}

func cards(ov Overview, spend []store.Txn, today time.Time) []Insight {
	var out []Insight
	m := ov.Month
	// How far through the month the data goes (pro-rate comparisons).
	frac := 1.0
	if ov.Partial && month(ov.DataEnd) == m {
		var d int
		fmt.Sscanf(ov.DataEnd[8:], "%d", &d)
		frac = float64(d) / float64(daysIn(m))
	}

	// 1. Category vs usual. Lumpy categories (yearly premiums) and catch-alls
	// say little month to month.
	basis := fmt.Sprintf("%d-month average", ov.AvgMonths)
	if ov.AvgMonths == 1 {
		basis = "last month"
	}
	if ov.AvgMonths > 0 {
		for _, c := range ov.Categories {
			if c.ID == "insurance" || c.ID == "other" || c.ID == "fees" {
				continue
			}
			usual := int64(float64(c.Avg) * frac)
			diff := c.Amount - usual
			if usual < 3000 && c.Amount < 3000 || abs(diff) < 3000 || usual == 0 {
				continue
			}
			pct := float64(diff) / float64(usual)
			if math.Abs(pct) < 0.25 {
				continue
			}
			tone, word := "warn", "up"
			if diff < 0 {
				tone, word = "good", "down"
			}
			so := ""
			if frac < 1 {
				so = " so far"
			}
			out = append(out, Insight{ID: "cat-" + c.ID, Tone: tone, Category: c.ID,
				Title: fmt.Sprintf("%s %s %.0f%%%s", c.Name, word, math.Abs(pct)*100, so),
				Body:  fmt.Sprintf("%s this month vs %s usual (%s).", money(c.Amount), money(usual), basis)})
		}
	}

	// 2. Large one-off charges.
	for _, t := range spend {
		if month(t.Date) != m || t.Kind != catalog.Purchase || t.OneOff || t.Mine() < 20000 {
			continue
		}
		if t.Mine() >= 100000 || t.Category == "insurance" && t.Mine() >= 20000 {
			body := "Mark it as one-off to spread it over 12 months in the trend."
			if t.Category == "insurance" {
				body = "Looks like a yearly premium. " + body
			}
			out = append(out, Insight{ID: fmt.Sprintf("big-%d", t.ID), Tone: "info", TxnID: t.ID, Category: catOf(t),
				Title: fmt.Sprintf("Large charge: %s %s", t.Merchant, money(t.Mine())), Body: body})
		}
	}

	// 3. Frequency: delivery and rides.
	for _, cat := range []string{"delivery", "rides"} {
		for _, c := range ov.Categories {
			if c.ID != cat || c.Count < 6 {
				continue
			}
			noun := map[string]string{"delivery": "food delivery orders", "rides": "rides"}[cat]
			days := float64(daysIn(m)) * frac
			out = append(out, Insight{ID: "freq-" + cat, Tone: "info", Category: cat,
				Title: fmt.Sprintf("%d %s this month", c.Count, noun),
				Body:  fmt.Sprintf("%s in total, about one every %.1f days.", money(c.Amount), days/float64(c.Count))})
		}
	}

	// 4. DCC fees.
	var dcc int64
	var dccN int
	for _, t := range spend {
		if month(t.Date) == m && t.Kind == catalog.Fee && strings.Contains(strings.ToUpper(t.Raw), "DCC") {
			dcc += t.Mine()
			dccN++
		}
	}
	if dcc > 0 {
		out = append(out, Insight{ID: "dcc", Tone: "warn", Category: "fees",
			Title: fmt.Sprintf("%s in currency-conversion (DCC) fees", money(dcc)),
			Body:  fmt.Sprintf("%d charge(s) were converted to SGD by the merchant. When paying overseas or online in a foreign currency, choose to pay in that currency — not SGD — to avoid these.", dccN)})
	}

	// 5. Recurring.
	if ov.RecurringPM > 0 && len(ov.Recurring) > 0 {
		names := make([]string, 0, 3)
		for i, r := range ov.Recurring {
			if i == 3 {
				break
			}
			names = append(names, r.Name)
		}
		out = append(out, Insight{ID: "recurring", Tone: "info",
			Title: fmt.Sprintf("%d recurring %s ≈ %s a month", len(ov.Recurring), map[bool]string{true: "charge", false: "charges"}[len(ov.Recurring) == 1], money(ov.RecurringPM)),
			Body:  "Including " + strings.Join(names, ", ") + ". Worth a yearly review."})
	}

	// 6. Overall vs usual.
	if ov.AvgMonths > 0 && ov.AvgSpent > 0 {
		usual := int64(float64(ov.AvgSpent) * frac)
		if usual > 0 && ov.Spent < usual*85/100 {
			out = append(out, Insight{ID: "total-down", Tone: "good",
				Title: fmt.Sprintf("Spending %s below your usual", money(usual-ov.Spent)),
				Body:  "Nice — that's room for your goals."})
		}
	}
	if out == nil {
		out = []Insight{}
	}
	return out
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// money formats cents as S$1,234.56 (whole dollars from S$1,000).
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
	if d < 1000 {
		s += fmt.Sprintf(".%02d", c%100)
	}
	return sign + "S$" + s
}
