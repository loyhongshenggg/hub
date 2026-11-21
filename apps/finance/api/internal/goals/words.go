package goals

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/store"
)

// Reading is what FromWords understood in a paragraph: goal drafts (not
// saved) plus notes for the user to check.
type Reading struct {
	Goals       []store.Goal
	Reasoning   []string // one per goal
	Assumptions []string
	Questions   []string
}

// FromWords turns a paragraph into goal drafts without any external service.
// It looks for amounts (S$18k, $1,700, 800 a month), their role (target,
// what you have now, per month), dates (by July 2029, end of 2027, in 2
// years, 30/09/2029), and intent words (save, invest, pay off, keep under).
// averages (category → cents per month) only add context to notes.
func FromWords(text string, today time.Time, averages map[string]int64) Reading {
	var r Reading
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	var last *store.Goal // the previous target goal, for "I have S$6k now" follow-ups
	lastIdx := -1
	for _, s := range sentences(text) {
		c := readClause(s, today)
		if c.empty() {
			continue
		}
		g, why, ok := c.goal(s, today)
		if !ok {
			// Only "have now" / "per month" amounts: they belong to the
			// previous savings goal ("I have S$6k now and can save S$700").
			if last != nil && c.target == 0 && (c.current > 0 || c.monthly > 0) && c.intent != "limit" {
				if c.current > 0 && last.StartAmount == 0 {
					last.StartAmount = c.current
				}
				if c.monthly > 0 && last.MonthlyPlan == 0 {
					last.MonthlyPlan = c.monthly
				}
				last.Why = joinWhy(last.Why, s)
				r.Reasoning[lastIdx] = describe(*last)
			}
			continue
		}
		g.Why = why
		if g.Kind == Limit && averages != nil {
			for _, cat := range g.Filter.Categories {
				if avg := averages[cat]; avg > 0 {
					cc, _ := catalog.Get(cat)
					r.Assumptions = append(r.Assumptions, fmt.Sprintf("Your recent average on %s is %s a month.", cc.Name, money(avg)))
				}
			}
		}
		r.Assumptions = append(r.Assumptions, c.notes...)
		r.Goals = append(r.Goals, g)
		r.Reasoning = append(r.Reasoning, describe(g))
		if g.Kind == Target || g.Kind == Payoff {
			last, lastIdx = &r.Goals[len(r.Goals)-1], len(r.Goals)-1
		} else {
			last, lastIdx = nil, -1
		}
	}
	for i := range r.Goals {
		g := &r.Goals[i]
		switch {
		case (g.Kind == Target || g.Kind == Payoff) && g.TargetDate == "":
			r.Questions = append(r.Questions, fmt.Sprintf("When do you want “%s” done? A date gives you a forecast and a monthly amount.", g.Title))
		case g.Kind == Limit && len(g.Filter.Categories) == 0 && len(g.Filter.Merchants) == 0 && len(g.Filter.Keywords) == 0:
			r.Questions = append(r.Questions, fmt.Sprintf("“%s” counts all spending — pick categories if you meant something narrower.", g.Title))
		}
		if g.Kind == Target && g.Target > g.StartAmount && g.StartAmount > 0 {
			half := (g.Target + g.StartAmount) / 2
			g.Milestones = append(g.Milestones, store.Milestone{Label: "Half way", Amount: &half})
		}
	}
	if len(r.Goals) == 0 {
		r.Questions = append(r.Questions, "No amount found. Try e.g. “save S$20k for a wedding by June 2028” or “keep food delivery under S$200 a month”.")
	}
	r.Assumptions = dedupe(r.Assumptions)
	return r
}

// --- clauses

var reSentence = regexp.MustCompile(`(?m)[^.!?\n•]+(?:[.!?]+(?:\s|$)|$|\n)`)

func sentences(text string) []string {
	// Protect decimals ("1.5k") and abbreviations from sentence splitting.
	t := regexp.MustCompile(`(\d)\.(\d)`).ReplaceAllString(text, "$1\x00$2")
	t = strings.NewReplacer("e.g.", "eg", "i.e.", "ie", "approx.", "approx", "S.P.A.", "SPA").Replace(t)
	var out []string
	for _, s := range reSentence.FindAllString(t, -1) {
		s = strings.TrimSpace(strings.ReplaceAll(s, "\x00", "."))
		s = strings.TrimLeft(s, "-*• ")
		if len(s) > 2 {
			out = append(out, s)
		}
	}
	return out
}

type amount struct {
	cents int64
	pos   int
	role  string // target | current | monthly | count
	per   string // week | month | year (monthly role)
	noun  string // count: what is counted
}

type clause struct {
	amounts []amount
	date    string
	intent  string // target | payoff | limit | habit | ""
	period  string
	target  int64
	current int64
	monthly int64
	count   int64
	noun    string
	notes   []string
	low     string
}

func (c clause) empty() bool { return len(c.amounts) == 0 }

var (
	// $1,700 · S$18k · SGD 5,000 · 160000 sgd · 1.2m · 800 (bare, decided by context)
	reMoney = regexp.MustCompile(`(?i)(s\$|sgd\s?|\$)?\s?(\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?)\s?(k\b|m\b|mil\b|million\b|thousand\b)?(\s?(?:sgd|dollars?|bucks)\b)?`)
	rePer   = regexp.MustCompile(`(?i)^\s*(?:a|per|every|each|/)\s?(week|wk|month|mth|mo|year|yr)\b|^\s*(weekly|monthly|yearly|annually|a year|per annum)\b`)
	reCount = regexp.MustCompile(`(?i)^\s*(?:\w+\s)?(rides?|times?|orders?|trips?|meals?|deliveries|purchases|visits?|cups?|drinks?)\b`)
	reYear  = regexp.MustCompile(`^(19|20)\d{2}$`)
)

var currentWords = []string{"have", "got", "already", "currently", "now", "today", "so far", "saved up", "as of", "existing", "owe", "balance", "left on"}

func readClause(s string, today time.Time) clause {
	c := clause{low: strings.ToLower(s), period: "month"}
	for _, m := range reMoney.FindAllStringSubmatchIndex(s, -1) {
		cur, num, suf, word := part(s, m, 2), part(s, m, 4), part(s, m, 6), part(s, m, 8)
		start, end := m[0], m[1]
		// Skip numbers inside dates (30/09/2029, 07/2029) and percentages.
		if start > 0 && strings.ContainsAny(s[start-1:start], "/-") || end < len(s) && strings.ContainsAny(s[end:min(end+1, len(s))], "/%") {
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
		if err != nil || v <= 0 {
			continue
		}
		after := s[end:]
		marked := cur != "" || suf != "" || word != ""
		if !marked && reYear.MatchString(num) {
			continue // a year
		}
		a := amount{pos: start}
		if cm := reCount.FindStringSubmatch(after); cm != nil && !marked && v < 1000 {
			a.role, a.cents, a.noun = "count", int64(v), strings.ToLower(cm[1])
			c.amounts = append(c.amounts, a)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(suf)) {
		case "k", "thousand":
			v *= 1000
		case "m", "mil", "million":
			v *= 1_000_000
		}
		if !marked {
			// Bare numbers count as money only beside money words.
			ctx := strings.ToLower(s[max(0, start-24):start])
			if !rePer.MatchString(after) && !containsAny(ctx, "save", "spend", "invest", "budget", "under", "below", "than", "max", "limit", "put", "set aside", "pay", "owe", "give", "reach", "have", "of") {
				continue
			}
			if v < 10 {
				continue
			}
		}
		a.cents = int64(math.Round(v * 100))
		if pm := rePer.FindStringSubmatch(after); pm != nil {
			a.role = "monthly"
			a.per = normPer(pm[1] + pm[2])
		} else {
			before := strings.ToLower(s[max(0, start-28):start])
			aft := strings.ToLower(after[:min(len(after), 22)])
			if containsAny(before, currentWords...) || containsAny(aft, " now", " already", " today", " so far", " saved", " currently", " as of") {
				a.role = "current"
			} else {
				a.role = "target"
			}
		}
		c.amounts = append(c.amounts, a)
	}
	for _, a := range c.amounts {
		switch a.role {
		case "target":
			if a.cents > c.target {
				c.target = a.cents
			}
		case "current":
			if c.current == 0 {
				c.current = a.cents
			}
		case "monthly":
			if c.monthly == 0 {
				c.monthly, c.period = a.cents, a.per
			}
		case "count":
			c.count, c.noun = a.cents, a.noun
		}
	}
	low := c.low
	switch {
	case containsAny(low, "pay off", "payoff", "pay down", "clear my", "clear the", "repay", "get rid of", "debt free", "debt-free"):
		c.intent = "payoff"
	case containsAny(low, "under", "below", "at most", "no more than", "not more than", "less than", "max ", "maximum", "cap ", "limit", "cut down", "cut back", "spend less", "keep it", "keep my"):
		c.intent = "limit"
	case c.target == 0 && c.monthly > 0 && containsAny(low, "invest", "save", "put aside", "set aside", "put away", "contribute", "dca", "top up", "give", "send", "support"):
		c.intent = "habit"
	case c.target > 0:
		c.intent = "target"
	}
	if c.intent == "limit" && c.target > 0 && c.monthly == 0 {
		// "keep food delivery under S$200" without a period: monthly.
		c.monthly, c.period = c.target, "month"
		if p := rePer.FindStringSubmatch(low[strings.Index(low, "under")+5:]); p != nil {
			c.period = normPer(p[1] + p[2])
		}
	}
	c.date = findDate(s, today, &c.notes)
	return c
}

func part(s string, m []int, i int) string {
	if m[i] < 0 {
		return ""
	}
	return s[m[i]:m[i+1]]
}

func normPer(p string) string {
	p = strings.ToLower(p)
	switch {
	case strings.HasPrefix(p, "w"):
		return "week"
	case strings.HasPrefix(p, "y"), strings.HasPrefix(p, "ann"), strings.Contains(p, "year"):
		return "year"
	}
	return "month"
}

func containsAny(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// goal builds a draft from a clause. ok is false when the clause only adds
// detail to the previous goal.
func (c clause) goal(s string, today time.Time) (store.Goal, string, bool) {
	g := store.Goal{Status: "active", StartDate: today.Format("2006-01-02"), Period: "month", Metric: "amount",
		Source: "manual", Milestones: []store.Milestone{}, Filter: store.GoalFilter{Categories: []string{}, Merchants: []string{}, Keywords: []string{}}}
	why := strings.TrimSpace(s)
	if len(why) > 240 {
		why = why[:237] + "…"
	}
	switch c.intent {
	case "payoff":
		g.Kind, g.Area = Payoff, "debt"
		g.StartAmount = max(c.target, c.current)
		if g.StartAmount == 0 {
			return g, "", false
		}
		g.MonthlyPlan, g.TargetDate = c.monthly, c.date
		g.Title = "Pay off " + debtNoun(c.low)
	case "limit":
		g.Kind, g.Area = Limit, "lifestyle"
		cats, merchants := spendWhat(c.low)
		g.Filter.Categories, g.Filter.Merchants = cats, merchants
		if c.count > 0 {
			g.Metric, g.Target = "count", c.count
			if strings.Contains(c.low, "week") {
				g.Period = "week"
			}
			g.Title = fmt.Sprintf("At most %d %s a %s", c.count, c.noun, g.Period)
		} else {
			if c.monthly == 0 {
				return g, "", false
			}
			g.Target, g.Period = c.monthly, c.period
			g.Title = fmt.Sprintf("%s under %s", limitNoun(cats, merchants), money(g.Target))
		}
		if len(cats) == 1 && cats[0] == "insurance" {
			g.Area = "safety"
		}
	case "habit":
		g.Kind, g.Target, g.Period = Habit, c.monthly, c.period
		switch {
		case containsAny(c.low, "invest", "dca", "etf", "stocks", "portfolio"):
			g.Area, g.Title = "investing", fmt.Sprintf("Invest %s a %s", money(g.Target), g.Period)
		case containsAny(c.low, "parent", "mum", "mom", "dad", "family"):
			g.Area, g.Title = "other", fmt.Sprintf("Give parents %s a %s", money(g.Target), g.Period)
		case containsAny(c.low, "cpf", "srs", "retire"):
			g.Area, g.Title = "retirement", fmt.Sprintf("Top up %s a %s", strings.ToUpper(firstOf(c.low, "cpf", "srs", "retirement")), g.Period)
		default:
			g.Area, g.Title = areaOf(c.low), fmt.Sprintf("Save %s a %s", money(g.Target), g.Period)
		}
	case "target":
		g.Kind, g.Target, g.StartAmount, g.MonthlyPlan, g.TargetDate = Target, c.target, c.current, c.monthly, c.date
		g.Area = areaOf(c.low)
		g.Title = fundName(c.low)
	default:
		return g, "", false
	}
	return g, why, true
}

func firstOf(s string, opts ...string) string {
	for _, o := range opts {
		if strings.Contains(s, o) {
			return o
		}
	}
	return opts[0]
}

// --- vocabulary

var fundWords = []struct {
	words []string
	name  string
}{
	{[]string{"emergenc", "rainy day", "safety net"}, "Emergency fund"},
	{[]string{"wedding", "proposal", "rom "}, "Wedding"},
	{[]string{"renovation", "reno ", "furniture"}, "Renovation"},
	{[]string{"bto", "hdb", "downpayment", "down payment", "house", "flat", "condo", "home", "key collection"}, "Home"},
	{[]string{"car"}, "Car"},
	{[]string{"travel", "holiday", "trip", "vacation"}, "Travel"},
	{[]string{"retire", "retirement"}, "Retirement"},
	{[]string{"invest", "portfolio", "etf"}, "Investments"},
	{[]string{"course", "degree", "masters", "school", "education", "study"}, "Education"},
	{[]string{"baby", "child", "kid"}, "Family"},
}

func fundName(low string) string {
	var hits []string
	for _, f := range fundWords {
		if containsAny(" "+low+" ", f.words...) {
			if f.name == "Emergency fund" {
				return f.name
			}
			hits = append(hits, f.name)
		}
		if len(hits) == 2 {
			break
		}
	}
	switch len(hits) {
	case 0:
		return "Savings goal"
	case 1:
		return hits[0] + " fund"
	}
	return hits[0] + " & " + strings.ToLower(hits[1]) + " fund"
}

func areaOf(low string) string {
	switch {
	case containsAny(low, "emergenc", "rainy day", "safety net", "insurance"):
		return "safety"
	case containsAny(low, "bto", "hdb", "house", "flat", "condo", "home", "renovation", "downpayment", "down payment", "key collection"):
		return "home"
	case containsAny(low, "retire", "cpf sa", "srs"):
		return "retirement"
	case containsAny(low, "invest", "portfolio", "etf", "stocks"):
		return "investing"
	case containsAny(low, "debt", "loan"):
		return "debt"
	case containsAny(low, "course", "degree", "masters", "school", "education"):
		return "education"
	case containsAny(low, "wedding", "travel", "holiday", "trip", "car", "proposal"):
		return "lifestyle"
	}
	return "other"
}

func debtNoun(low string) string {
	switch {
	case containsAny(low, "credit card", "card debt", "card balance"):
		return "card debt"
	case containsAny(low, "car loan"):
		return "car loan"
	case containsAny(low, "study loan", "student loan", "tuition loan"):
		return "study loan"
	case containsAny(low, "loan"):
		return "loan"
	}
	return "debt"
}

// spendWords map phrases to categories (order: most specific first).
var spendWords = []struct {
	words []string
	cat   string
}{
	{[]string{"food delivery", "delivery", "foodpanda", "deliveroo", "grabfood"}, "delivery"},
	{[]string{"grab ride", "grab", "taxi", "ride-hailing", "ride hailing", "gojek", "cab", "rides"}, "rides"},
	{[]string{"eating out", "dining", "restaurant", "hawker", "meals out", "coffee", "bubble tea", "cafe"}, "dining"},
	{[]string{"groceries", "grocery", "supermarket"}, "groceries"},
	{[]string{"public transport", "mrt", "bus "}, "transport"},
	{[]string{"petrol", "parking", "car "}, "car"},
	{[]string{"online shopping", "shopping", "shopee", "lazada", "taobao", "clothes", "amazon"}, "shopping"},
	{[]string{"subscription", "netflix", "spotify", "streaming"}, "subscriptions"},
	{[]string{"game", "gaming", "steam", "entertainment"}, "entertainment"},
	{[]string{"travel", "holiday", "flights"}, "travel"},
	{[]string{"insurance", "premium"}, "insurance"},
	{[]string{"facial", "beauty", "salon", "hair", "nails", "personal care"}, "personal"},
	{[]string{"gym", "fitness", "sports"}, "leisure"},
	{[]string{"phone bill", "bills", "utilities", "telco"}, "bills"},
}

func spendWhat(low string) ([]string, []string) {
	cats := []string{}
	seen := map[string]bool{}
	for _, sw := range spendWords {
		if containsAny(" "+low+" ", sw.words...) && !seen[sw.cat] {
			// "food delivery" shouldn't also count as rides via "grab".
			if sw.cat == "rides" && seen["delivery"] && !containsAny(low, "ride", "taxi", "cab") {
				continue
			}
			seen[sw.cat] = true
			cats = append(cats, sw.cat)
		}
	}
	if len(cats) == 0 && containsAny(low, "living cost", "living expense", "daily expense", "day-to-day", "everyday") {
		for _, c := range catalog.Categories {
			if c.ID != "insurance" {
				cats = append(cats, c.ID)
			}
		}
	}
	return cats, []string{}
}

func limitNoun(cats, merchants []string) string {
	switch {
	case len(cats) == 0:
		return "Spending"
	case len(cats) > 6:
		return "Living costs"
	case len(cats) == 1:
		c, _ := catalog.Get(cats[0])
		return c.Name
	}
	names := make([]string, 0, len(cats))
	for _, id := range cats {
		c, _ := catalog.Get(id)
		names = append(names, strings.ToLower(c.Name))
	}
	return strings.ToUpper(names[0][:1]) + names[0][1:] + " & " + strings.Join(names[1:], " & ")
}

// --- dates

var months = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}

var (
	reDMY      = regexp.MustCompile(`\b(\d{1,2})/(\d{1,2})/(20\d{2})\b`)
	reMY       = regexp.MustCompile(`\b(\d{1,2})/(20\d{2})\b`)
	reISO      = regexp.MustCompile(`\b(20\d{2})-(\d{2})(?:-(\d{2}))?\b`)
	reMonthYr  = regexp.MustCompile(`(?i)\b(?:(\d{1,2})\s)?(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?,?\s(20\d{2})\b`)
	reEndYr    = regexp.MustCompile(`(?i)\b(end|mid|middle|start|beginning|early|late)(?:\s?of|-)?\s?(20\d{2})\b`)
	reByYr     = regexp.MustCompile(`(?i)\b(?:by|in|before|until|around|within)\s(20\d{2})\b`)
	reIn       = regexp.MustCompile(`(?i)\b(?:in|within|over|next)\s(\d+|one|two|three|four|five|six|ten|a)\s(years?|months?)\b`)
	reNextYear = regexp.MustCompile(`(?i)\b(?:by\s)?(?:the\s)?end of (?:this|next) year\b|\bnext year\b|\bthis year\b`)
)

var smallNums = map[string]int{"one": 1, "a": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "ten": 10}

func lastDay(y int, m time.Month) time.Time {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)
}

func findDate(s string, today time.Time, notes *[]string) string {
	f := func(t time.Time, raw string) string {
		d := t.Format("2006-01-02")
		*notes = append(*notes, fmt.Sprintf("“%s” read as %s.", strings.TrimSpace(raw), t.Format("2 Jan 2006")))
		return d
	}
	if m := reDMY.FindStringSubmatch(s); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		y, _ := strconv.Atoi(m[3])
		if mo >= 1 && mo <= 12 && d >= 1 && d <= 31 {
			return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		}
	}
	if m := reMonthYr.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[3])
		mo := months[strings.ToLower(m[2][:3])]
		if m[1] != "" {
			d, _ := strconv.Atoi(m[1])
			return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		}
		return f(lastDay(y, mo), m[0]) // "by June 2027" = end of June
	}
	if m := reMY.FindStringSubmatch(s); m != nil {
		mo, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		if mo >= 1 && mo <= 12 {
			return f(lastDay(y, time.Month(mo)), m[0])
		}
	}
	if m := reISO.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d := 1
		if m[3] != "" {
			d, _ = strconv.Atoi(m[3])
		}
		if mo >= 1 && mo <= 12 {
			return time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		}
	}
	if m := reEndYr.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[2])
		switch strings.ToLower(m[1]) {
		case "mid", "middle":
			return f(time.Date(y, 6, 30, 0, 0, 0, 0, time.UTC), m[0])
		case "start", "beginning", "early":
			return f(time.Date(y, 3, 31, 0, 0, 0, 0, time.UTC), m[0])
		}
		return f(lastDay(y, 12), m[0])
	}
	if m := reByYr.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		return f(lastDay(y, 12), m[0])
	}
	if m := reIn.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = smallNums[strings.ToLower(m[1])]
		}
		if n > 0 {
			t := today.AddDate(n, 0, 0)
			if strings.HasPrefix(strings.ToLower(m[2]), "month") {
				t = today.AddDate(0, n, 0)
			}
			return f(t, m[0])
		}
	}
	if m := reNextYear.FindString(s); m != "" {
		y := today.Year()
		if strings.Contains(strings.ToLower(m), "next") {
			y++
		}
		return f(lastDay(y, 12), m)
	}
	return ""
}

// --- text

func describe(g store.Goal) string {
	var parts []string
	switch g.Kind {
	case Target:
		parts = append(parts, "save up to "+money(g.Target))
		if g.StartAmount > 0 {
			parts = append(parts, money(g.StartAmount)+" already")
		}
		if g.MonthlyPlan > 0 {
			parts = append(parts, money(g.MonthlyPlan)+" a month planned")
		}
	case Payoff:
		parts = append(parts, "pay off "+money(g.StartAmount))
		if g.MonthlyPlan > 0 {
			parts = append(parts, money(g.MonthlyPlan)+" a month")
		}
	case Limit:
		if g.Metric == "count" {
			parts = append(parts, fmt.Sprintf("at most %d per %s", g.Target, g.Period))
		} else {
			parts = append(parts, fmt.Sprintf("spend at most %s per %s", money(g.Target), g.Period))
		}
		if n := len(g.Filter.Categories); n > 0 && n <= 6 {
			names := make([]string, 0, n)
			for _, c := range g.Filter.Categories {
				cc, _ := catalog.Get(c)
				names = append(names, cc.Name)
			}
			parts = append(parts, "on "+strings.Join(names, ", "))
		}
	case Habit:
		parts = append(parts, fmt.Sprintf("%s every %s", money(g.Target), g.Period))
	}
	if g.TargetDate != "" {
		if t, err := time.Parse("2006-01-02", g.TargetDate); err == nil {
			parts = append(parts, "by "+t.Format("2 Jan 2006"))
		}
	}
	return "Read on this computer: " + strings.Join(parts, ", ") + "."
}

func joinWhy(a, b string) string {
	if a == "" {
		return b
	}
	s := a + " " + b
	if len(s) > 240 {
		s = s[:237] + "…"
	}
	return s
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
