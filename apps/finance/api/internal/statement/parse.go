// Package statement parses credit-card and bank-account statements (their
// text layer, or OCR) into a summary and transactions, and checks them
// against the statement's own totals.
package statement

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// Summary is what the statement says about itself. Amounts are cents;
// nil means not found.
type Summary struct {
	// Kind is "bank" for a bank account (money in raises the balance);
	// empty for a credit card (purchases raise what you owe).
	Kind        string    `json:"kind,omitempty"`
	Holder      string    `json:"holder,omitempty"` // account holder's name, when printed
	Bank        string    `json:"bank"`
	Product     string    `json:"product"`
	Last4       string    `json:"last4"`
	Currency    string    `json:"currency"`
	PeriodStart time.Time `json:"periodStart"`
	PeriodEnd   time.Time `json:"periodEnd"`
	DueDate     time.Time `json:"dueDate"`
	PrevBalance *int64    `json:"prevBalance"`
	Credits     *int64    `json:"credits"` // payments & credits
	Debits      *int64    `json:"debits"`  // purchases & debits
	Total       *int64    `json:"total"`   // total account balance
}

// Txn is one statement line. Amount is positive cents; Credit marks CR.
type Txn struct {
	PostDate    time.Time `json:"postDate"`
	TxnDate     time.Time `json:"txnDate"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	Credit      bool      `json:"credit"`
	Page        int       `json:"page"`
	Y           float64   `json:"y"` // top of the row as a fraction of page height
	Recovered   bool      `json:"recovered,omitempty"`
	// Foreign is the original currency amount when printed ("MALAYSIAN
	// RINGGIT 19.76").
	Foreign string `json:"foreign,omitempty"`
	// Conf is Tesseract's mean confidence (0-100) for the description words;
	// used to pick the best reading of a row across passes.
	Conf float64 `json:"-"`
	// Bank-account rows (see classify): Kind is the transaction kind when
	// the statement says it ("transfer", "income", "payment", "refund"),
	// Counterparty who the money went to or came from, Note a message
	// worth keeping, Check asks you to confirm.
	Kind         string `json:"kind,omitempty"`
	Counterparty string `json:"counterparty,omitempty"`
	Note         string `json:"note,omitempty"`
	Check        bool   `json:"check,omitempty"`
}

// Parsed is the result of reading one OCR pass.
type Parsed struct {
	Summary Summary
	Txns    []Txn
	Pages   int // pages in the PDF
	// orphans are lines in the transaction column that end in an amount
	// but have no readable dates (used only to close a reconciliation gap).
	orphans []Txn

	rows, orphanRows []row
}

var (
	// 1,838.31 · 942.00CR · (292.00) · -5.00 · $17.99 — parentheses, a minus
	// or CR mark a credit.
	// A leading + marks money in on bank statements (Trust: "+300.00").
	// OCR reads "S$" as "$$" or "5$".
	reAmount = regexp.MustCompile(`^([-(+])?(?:[S5$]?\$)?([0-9][0-9,.]*\.[0-9]{2})\)?(CR)?$`)
	// "From 07 SEP 2026 to 05 OCT 2026", "1 SEP 2026 TO 30 SEP 2026",
	// "1 Sep 2026 - 30 Sep 2026".
	rePeriod = regexp.MustCompile(`(?i)(?:from\s+)?\b(\d{1,2})\s*([a-z0-9]{3,9})\s*(\d{4})\s*(?:to|-|–)\s*(\d{1,2})\s*([a-z0-9]{3,9})\s*(\d{4})\b`)
	reAsAt   = regexp.MustCompile(`(?i)\bas\s+(?:at|of)\s+(\d{1,2})\s+([a-z]{3,9})\s+(\d{4})\b`)
	reAcctNo = regexp.MustCompile(`(?i)\baccount\b(?:\s+(?:no\.?|number|statement))?\s*:?\s*(\d[\d -]{4,}\d)\b`)
	reFXLine = regexp.MustCompile(`^1\s+[A-Z]{3}\s*=\s*[0-9.]+\s+[A-Z]{3}\b`)
	reCCY    = regexp.MustCompile(`^[A-Z]{3}$`)
	reCard   = regexp.MustCompile(`(?i)\b\d{4}[- ]?x{4}[- ]?x{4}[- ]?(\d{4})\b`)
	reCardSp = regexp.MustCompile(`\b\d{4} \d{4} \d{4} (\d{4})\b`)
	reMasked = regexp.MustCompile(`(?i)^x{4}-x{4}-x{4}-\d{4}$`)
	reUSDate = regexp.MustCompile(`(?i)\b(jan|feb|mar|apr|may|jun|jul|aug|sep|oct|nov|dec)[a-z]*\.?\s+(\d{1,2}),?\s+(20\d{2})\b`)
	reStmt   = regexp.MustCompile(`(?i)statement\s+date:?\s+(.+)$`)
	reDate   = regexp.MustCompile(`(?i)^(\d{1,2})\s*([a-z0-9]{3,4})\s*(\d{4})$`)
	reDayMon = regexp.MustCompile(`^(\d{1,2})([A-Za-z0-9]{3,4})$`)
	reDigits = regexp.MustCompile(`^\d{1,2}$`)
)

var months = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}

// month reads an OCR'd month token ("Sep", "0ct", "C0ct", "5ep").
func month(tok string) (time.Month, bool) {
	t := strings.NewReplacer("0", "o", "1", "l", "5", "s", "8", "b", "6", "b").Replace(strings.ToLower(tok))
	cands := []string{t}
	if len(t) == 4 {
		cands = append(cands, t[1:], t[:3])
	}
	for _, c := range cands {
		for i, m := range months {
			if c == m {
				return time.Month(i + 1), true
			}
		}
	}
	// One wrong letter in a 3-letter token, if unambiguous.
	if len(t) == 3 {
		found := -1
		for i, m := range months {
			diff := 0
			for k := range 3 {
				if t[k] != m[k] {
					diff++
				}
			}
			if diff == 1 {
				if found >= 0 {
					return 0, false
				}
				found = i
			}
		}
		if found >= 0 {
			return time.Month(found + 1), true
		}
	}
	return 0, false
}

type dayMonth struct {
	day   int
	month time.Month
	year  int // 0 when the statement prints no year
}

var (
	reNumDate = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})[/.-](\d{4}|\d{2})$`)            // 31/10/2026, 31-10-26
	reISODate = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)                          // 2026-10-31
	reDashMon = regexp.MustCompile(`^(\d{1,2})[-/ ]([A-Za-z]{3,9})(?:[-/ ](\d{4}|\d{2}))?$`) // 31-Oct-2026
	reYear    = regexp.MustCompile(`^(\d{4}),?$`)
)

func fullYear(s string) int {
	y, _ := strconv.Atoi(s)
	if y < 100 {
		y += 2000
	}
	return y
}

// numericDate reads one-token dates: 31/10/2026, 31-10-26, 2026-10-31,
// 31-Oct-2026. Day/month order is Singapore's (day first) unless the
// numbers only fit month first.
func numericDate(tok string) (dayMonth, bool) {
	tok = strings.TrimRight(tok, ",")
	var d, m, y int
	switch {
	case reISODate.MatchString(tok):
		g := reISODate.FindStringSubmatch(tok)
		y, _ = strconv.Atoi(g[1])
		m, _ = strconv.Atoi(g[2])
		d, _ = strconv.Atoi(g[3])
	case reNumDate.MatchString(tok):
		g := reNumDate.FindStringSubmatch(tok)
		d, _ = strconv.Atoi(g[1])
		m, _ = strconv.Atoi(g[2])
		y = fullYear(g[3])
		if m > 12 && d <= 12 {
			d, m = m, d
		}
	case reDashMon.MatchString(tok):
		g := reDashMon.FindStringSubmatch(tok)
		mo, ok := month(g[2])
		if !ok {
			return dayMonth{}, false
		}
		m = int(mo)
		d, _ = strconv.Atoi(g[1])
		if g[3] != "" {
			y = fullYear(g[3])
		}
	default:
		return dayMonth{}, false
	}
	if d < 1 || d > 31 || m < 1 || m > 12 || y != 0 && (y < 1990 || y > 2100) {
		return dayMonth{}, false
	}
	return dayMonth{d, time.Month(m), y}, true
}

// readDate reads "07 Sep" (two tokens) or "07Sep" (one) at toks[i].
func readDate(toks []string, i int) (dayMonth, int, bool) {
	dm, j, ok := readDayMonth(toks, i)
	if !ok {
		// "Oct 31, 2026" (month first).
		if i+1 < len(toks) {
			if mo, mok := month(toks[i]); mok && len(toks[i]) >= 3 && letters(toks[i]) == len(strings.TrimRight(toks[i], ".")) {
				if d, err := strconv.Atoi(strings.TrimRight(toks[i+1], ",")); err == nil && d >= 1 && d <= 31 {
					dm, j, ok = dayMonth{d, mo, 0}, i+2, true
				}
			}
		}
		if !ok {
			return dm, j, false
		}
	}
	// A year after the day and month ("07 Sep 2026").
	if dm.year == 0 && j < len(toks) {
		if m := reYear.FindStringSubmatch(toks[j]); m != nil {
			if y, _ := strconv.Atoi(m[1]); y >= 1990 && y <= 2100 {
				dm.year, j = y, j+1
			}
		}
	}
	return dm, j, true
}

// readDayMonth reads "07 Sep" (two tokens), "07Sep" or a numeric date at toks[i].
func readDayMonth(toks []string, i int) (dayMonth, int, bool) {
	if i >= len(toks) {
		return dayMonth{}, i, false
	}
	if dm, ok := numericDate(toks[i]); ok {
		return dm, i + 1, true
	}
	if m := reDayMon.FindStringSubmatch(toks[i]); m != nil && strings.ContainsAny(strings.ToLower(m[2]), "abcdefghijklmnopqrstuvwxyz") {
		if mo, ok := month(m[2]); ok {
			d, _ := strconv.Atoi(m[1])
			if d >= 1 && d <= 31 {
				return dayMonth{d, mo, 0}, i + 1, true
			}
		}
	}
	if reDigits.MatchString(toks[i]) && i+1 < len(toks) {
		d, _ := strconv.Atoi(toks[i])
		if d < 1 || d > 31 {
			return dayMonth{}, i, false
		}
		if mo, ok := month(toks[i+1]); ok {
			return dayMonth{d, mo, 0}, i + 2, true
		}
		// OCR split the month: "08 Se p".
		if i+2 < len(toks) && len(toks[i+1])+len(toks[i+2]) == 3 {
			if mo, ok := month(toks[i+1] + toks[i+2]); ok {
				return dayMonth{d, mo, 0}, i + 3, true
			}
		}
	}
	return dayMonth{}, i, false
}

// parseAmount reads "1,838.31", "942.00CR". OCR sometimes turns a thousands
// comma into a dot; only the last dot is the decimal point.
func parseAmount(s string) (int64, bool, bool) {
	m := reAmount.FindStringSubmatch(s)
	if m == nil {
		return 0, false, false
	}
	num := m[2]
	if m[1] == "(" && !strings.HasSuffix(s, ")") {
		return 0, false, false
	}
	dot := strings.LastIndexByte(num, '.')
	intPart := strings.NewReplacer(",", "", ".", "").Replace(num[:dot])
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, false, false
	}
	frac, _ := strconv.ParseInt(num[dot+1:], 10, 64)
	return whole*100 + frac, m[1] != "" || m[3] != "", true
}

// isNoise: a short token with no letters or digits (OCR of dotted rules).
func isNoise(tok string) bool {
	if utf8.RuneCountInString(tok) > 2 {
		return false
	}
	for _, r := range tok {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// trailingAmount finds the amount at the end of toks (ignoring trailing
// punctuation noise) and returns it with the index where it starts.
func trailingAmount(toks []string) (amount int64, credit bool, at int, ok bool) {
	end := len(toks)
	for end > 0 && isNoise(toks[end-1]) {
		end--
	}
	if end > 0 && strings.EqualFold(toks[end-1], "CR") {
		credit = true
		end--
	}
	if end == 0 {
		return 0, false, 0, false
	}
	a, cr, ok := parseAmount(toks[end-1])
	if !ok && !credit && end >= 2 && utf8.RuneCountInString(toks[end-1]) <= 3 {
		// One short junk word after the amount ("6.29 SS": the edge of a
		// summary panel). Callers still require dates at the start.
		end--
		a, cr, ok = parseAmount(toks[end-1])
	}
	if !ok {
		return 0, false, 0, false
	}
	return a, credit || cr, end - 1, true
}

// row is a transaction row before year inference.
type row struct {
	post, txn dayMonth
	desc      string
	amount    int64
	credit    bool
	page      int
	y         float64
	dated     bool
	conf      float64
	foreign   string
	refund    bool // marked REFUND on the line below (Trust)
}

// parseRow reads "DD Mon DD Mon description amount[CR]". A row with one
// date (some banks print only the transaction date) uses it for both.
func parseRow(l line) (row, bool) {
	toks := strings.Fields(l.text())
	a, credit, at, ok := trailingAmount(toks)
	if !ok {
		return row{}, false
	}
	r := row{amount: a, credit: credit, page: l.page}
	// A foreign-currency column before the SGD amount ("45.49 CNY 8.68").
	if at >= 2 && reCCY.MatchString(toks[at-1]) {
		if _, _, ok := parseAmount(toks[at-2]); ok {
			r.foreign = toks[at-2] + " " + toks[at-1]
			at -= 2
		}
	}
	d1, i, ok1 := readDate(toks, 0)
	if !ok1 {
		r.desc = strings.Join(toks[:at], " ")
		return r, false
	}
	r.dated = true
	r.post, r.txn = d1, d1
	if d2, j, ok2 := readDate(toks, i); ok2 {
		r.txn, i = d2, j
	}
	for i < at && isNoise(toks[i]) {
		i++
	}
	if i >= at {
		return row{}, false
	}
	r.desc = strings.Join(toks[i:at], " ")
	if len(toks) == len(l.words) {
		var sum float64
		for _, w := range l.words[i:at] {
			sum += w.Conf
		}
		r.conf = sum / float64(at-i)
	}
	return r, true
}

// labels that end a statement's transaction list or are not transactions.
var stopLines = []string{"continuedonnextpage", "totaldue", "previousstatementbalance", "balancepreviousstatement",
	"endofstatement", "subtotal", "grandtotal", "totalforthecard", "previousbalance", "openingbalance", "closingbalance"}

func isStop(s string) bool {
	n := norm(s)
	for _, st := range stopLines {
		if strings.HasPrefix(n, st) {
			return true
		}
	}
	return false
}

func letterRatio(s string) float64 {
	var letters, total int
	for _, r := range s {
		if r == ' ' {
			continue
		}
		total++
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			letters++
		}
	}
	if total == 0 {
		return 0
	}
	return float64(letters) / float64(total)
}

// Parse reads OCR'd pages. It works for HSBC Singapore and any statement
// laid out as "post date, transaction date, description, amount[CR]".
func Parse(pages []ocr.Page) Parsed {
	if isLedger(pages) {
		return parseLedger(pages)
	}
	var p Parsed
	p.Pages = len(pages)
	var all []line
	for _, pg := range pages {
		lines := groupLines(pg.Number, pg.Words)
		all = append(all, lines...)
		p.parsePage(pg, lines)
	}
	p.Summary = parseSummary(all, pages)
	year := yearResolver(p.Summary)
	fix := func(ts []row) []Txn {
		out := make([]Txn, 0, len(ts))
		for _, r := range ts {
			t := Txn{
				PostDate: year(r.post), TxnDate: year(r.txn), Description: r.desc,
				Amount: r.amount, Credit: r.credit, Page: r.page, Y: r.y, Conf: r.conf, Foreign: r.foreign,
			}
			if p.Summary.Kind == "bank" {
				// A bank account's card-style rows (Trust): own transfers,
				// interest, refunds and people are told apart from purchases.
				t.Kind, t.Counterparty, t.Note, t.Check = classify([]string{r.desc}, r.credit, p.Summary.Holder, r.amount)
				if r.refund {
					t.Kind = "refund"
				}
			}
			out = append(out, t)
		}
		return out
	}
	p.Txns = fix(p.rows)
	p.orphans = fix(p.orphanRows)
	return p
}

// parsePage finds the transaction column (from the DESCRIPTION / AMOUNT
// header, so a summary panel on the right doesn't bleed into rows) and reads
// the rows below it.
func (p *Parsed) parsePage(pg ocr.Page, lines []line) {
	x0, x1, top := 0, pg.Width, 0
	descX := -1
	for _, l := range lines {
		hdr := -1
		for i, w := range l.words {
			if norm(w.Text) == "description" {
				hdr = i
				break
			}
		}
		if hdr < 0 {
			continue
		}
		descX = l.words[hdr].X0
		// AMOUNT(SGD) may be split by OCR ("AMOU NT(SGD)"); the header row
		// can also carry a summary panel's own header to the right.
		// With two amount columns ("Amount in FCY … Amount in SGD", Trust)
		// the last one is the account's own currency.
		amountR := -1
		rest := l.words[hdr+1:]
		for i := len(rest) - 1; i >= 0; i-- {
			if !strings.Contains(norm(rest[i].Text), "amou") {
				continue
			}
			amountR = rest[i].X1
			for _, nx := range rest[i+1:] {
				if nx.X0-amountR > pg.Width/60 {
					break
				}
				amountR = nx.X1
			}
			break
		}
		if amountR > 0 {
			x1 = amountR + pg.Width*6/100
			for _, w := range l.words[hdr+1:] {
				if w.X0 > amountR+pg.Width/100 {
					x1 = min(x1, w.X0-pg.Width/200)
					break
				}
			}
		}
		top = l.y1
		break
	}
	var last *row
	var prev *row // last dated row on this page (dates for orphans)
	lastY1 := 0
	for _, full := range lines {
		if full.y0 < top {
			continue
		}
		l := full.column(x0, x1)
		if len(l.words) == 0 {
			continue
		}
		txt := l.text()
		if isStop(txt) {
			last = nil
			continue
		}
		// A repeated column header ("Date  Name  Amount") ends the row above.
		if n := norm(txt); len(l.words) <= 8 && strings.Contains(n, "date") &&
			(strings.Contains(n, "amount") || strings.Contains(n, "description") || strings.Contains(n, "name")) {
			last = nil
			continue
		}
		// "XXXX-XXXX-XXXX-0142": which card was used, not part of the name.
		if reMasked.MatchString(strings.TrimSpace(txt)) {
			continue
		}
		// "1 CNY = 0.1908 SGD TRUST FX CHARGE = 0% [REFUND]" under a Trust row.
		if reFXLine.MatchString(strings.TrimSpace(txt)) {
			if last != nil && strings.Contains(strings.ToUpper(txt), "REFUND") {
				last.refund = true
			}
			lastY1 = l.y1
			continue
		}
		// "FOREIGN AMOUNT MALAYSIAN RINGGIT 19.76" under an overseas row.
		if up := strings.ToUpper(txt); last != nil && (strings.HasPrefix(up, "FOREIGN AMOUNT") || strings.HasPrefix(up, "FOREIGN CURRENCY")) {
			last.foreign = strings.TrimSpace(txt[len("FOREIGN AMOUNT"):])
			lastY1 = l.y1
			continue
		}
		r, ok := parseRow(l)
		r.y = float64(l.y0) / float64(max(1, pg.Height))
		if ok && isStop(r.desc) {
			// "01 Sep Previous balance 912.25": a balance, not a transaction.
			last = nil
			continue
		}
		if ok {
			p.rows = append(p.rows, r)
			last = &p.rows[len(p.rows)-1]
			cp := r
			prev = &cp
			lastY1 = l.y1
			continue
		}
		if r.amount > 0 && !r.dated && prev != nil && letterRatio(r.desc) > 0.6 && len(r.desc) >= 3 {
			// Unreadable dates: assume the row above's (rows are in date order).
			r.page, r.post, r.txn = pg.Number, prev.post, prev.txn
			p.orphanRows = append(p.orphanRows, r)
			last = nil
			continue
		}
		// Continuation of the previous row's description ("SINGAPORE SG").
		if last != nil && l.y0-lastY1 < 2*l.height() && letterRatio(txt) >= 0.7 &&
			(descX < 0 || l.x0() >= descX-pg.Width/50) && len(txt) <= 60 {
			last.desc += " " + txt
			lastY1 = l.y1
			continue
		}
		last = nil
	}
}

// parseSummary reads the statement header and account summary.
func parseSummary(lines []line, pages []ocr.Page) Summary {
	s := Summary{Currency: "SGD"}
	width := func(page int) int {
		for _, p := range pages {
			if p.Number == page {
				return p.Width
			}
		}
		return 1
	}
	var stmtDate time.Time
	var spacedLast4 string
	for _, l := range lines {
		t := l.text()
		u := strings.ToUpper(t)
		if s.Bank == "" && l.page == 1 {
			for _, b := range banks {
				if strings.Contains(u, b.key) {
					s.Bank = b.name
					break
				}
			}
		}
		if s.Product == "" && l.page == 1 {
			for _, k := range []string{"VISA", "MASTER", "AMEX"} {
				if strings.Contains(u, k) && len(l.words) <= 12 {
					s.Product = headline(l, width(l.page))
					break
				}
			}
		}
		if s.PeriodStart.IsZero() {
			if m := rePeriod.FindStringSubmatch(t); m != nil {
				s.PeriodStart = mkDate(m[1], m[2], m[3])
				s.PeriodEnd = mkDate(m[4], m[5], m[6])
			}
		}
		if stmtDate.IsZero() {
			if m := reStmt.FindStringSubmatch(t); m != nil {
				stmtDate = anyDate(m[1])
			}
		}
		if s.Last4 == "" {
			if m := reCard.FindStringSubmatch(t); m != nil {
				s.Last4 = m[1]
			}
		}
		if spacedLast4 == "" {
			if m := reCardSp.FindStringSubmatch(t); m != nil {
				spacedLast4 = m[1]
			}
		}
	}
	if s.Last4 == "" {
		s.Last4 = spacedLast4
	}
	// Statements with only a statement date (Citibank): the period is the
	// month up to it.
	if s.PeriodStart.IsZero() && !stmtDate.IsZero() {
		s.PeriodEnd = stmtDate
		s.PeriodStart = stmtDate.AddDate(0, -1, 1)
	}
	// Monthly statements "as at 31 Oct 2026" (DBS): that calendar month.
	if s.PeriodStart.IsZero() {
		for _, l := range lines {
			if m := reAsAt.FindStringSubmatch(l.text()); m != nil {
				if end := mkDate(m[1], m[2], m[3]); !end.IsZero() {
					s.PeriodEnd = end
					s.PeriodStart = time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC)
					break
				}
			}
		}
	}
	if box, ok := boxSummary(lines); ok {
		v := func(k string) *int64 {
			x, ok := box[k]
			if !ok {
				return nil
			}
			return &x
		}
		s.PrevBalance, s.Credits, s.Total = v("prev"), v("credits"), v("total")
		if d, ok := box["debits"]; ok {
			// Rows include interest and fee lines, so they count as debits.
			d += box["interest"] + box["fees"]
			s.Debits = &d
		}
	}
	// amount finds a labelled figure. For balances (signed), "CR" means the
	// bank owes you: a negative balance ("0.52CR"). For "Payments & Credits"
	// CR just labels the column.
	amount := func(signed bool, labels ...string) *int64 {
		for _, lab := range labels {
			for _, b := range findLabel(lines, lab) {
				if v, cr, ok := valueNear(lines, b, width(b.page)); ok {
					if signed && cr {
						v = -v
					}
					return &v
				}
			}
		}
		return nil
	}
	if s.PrevBalance == nil {
		s.PrevBalance = amount(true, "Previous Statement Balance", "Previous Balance")
	}
	if s.Credits == nil {
		s.Credits = amount(false, "Payments & Credits", "Payments and Credits", "Total Credits")
	}
	if s.Debits == nil {
		s.Debits = amount(false, "Purchases & Debits", "Purchases and Debits", "Total Debits")
	}
	if s.Total == nil {
		s.Total = amount(true, "Total Account Balance", "Total Due", "Statement Balance", "Current Balance")
	}
	s.Holder = holderName(lines)
	// Trust-style summary: Opening balance + Total incoming − Total outgoing
	// + Interest = Closing balance.
	if trustSummary(&s, lines) {
		s.Kind = "bank"
		bankHeader(&s, lines, width)
	}
	for _, b := range findLabel(lines, "Payment Due Date") {
		if d, ok := dateRight(lines, b); ok {
			s.DueDate = d
			break
		}
		if d, ok := dateBelow(lines, b, width(b.page)); ok {
			s.DueDate = d
			break
		}
	}
	return s
}

// Banks recognised on page 1 (first match wins).
var banks = []struct{ key, name string }{
	{"HSBC", "HSBC"}, {"CITIBANK", "Citibank"}, {"DBS BANK", "DBS"}, {"POSB", "POSB"}, {"OCBC", "OCBC"},
	{"UNITED OVERSEAS BANK", "UOB"}, {"UOB ", "UOB"}, {"STANDARD CHARTERED", "Standard Chartered"},
	{"MAYBANK", "Maybank"}, {"AMERICAN EXPRESS", "American Express"}, {"TRUST BANK", "Trust"},
	// DBS prints its logo as an image; its account names carry the brand.
	{"DBS SAVINGS", "DBS"}, {"DBS MULTIPLIER", "DBS"}, {"DBS CO. REG", "DBS"}, {"POSB ", "POSB"},
}

// anyDate reads "October 19, 2026" or "19 Oct 2026".
func anyDate(s string) time.Time {
	if m := reUSDate.FindStringSubmatch(s); m != nil {
		return mkDate(m[2], m[1], m[3])
	}
	if m := reDate.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		return mkDate(m[1], m[2], m[3])
	}
	return time.Time{}
}

// dateRight reads a date printed after a label on the same row
// ("Payment Due Date November 13, 2026").
func dateRight(lines []line, b box) (time.Time, bool) {
	for _, l := range lines {
		if l.page != b.page || l.y1 < b.y0 || l.y0 > b.y1 {
			continue
		}
		var toks []string
		for _, w := range l.words {
			if w.X0 > b.x1 {
				toks = append(toks, w.Text)
			}
		}
		if d := anyDate(strings.Join(toks, " ")); !d.IsZero() {
			return d, true
		}
	}
	return time.Time{}, false
}

// boxSummary reads a summary printed as labelled boxes with the figures on
// one row underneath (Citibank): PREVIOUS BALANCE − PAYMENTS & CREDITS +
// PURCHASES & ADVANCES + INTEREST CHARGES + FEES & CHARGES = CURRENT BALANCE.
// Each figure belongs to the nearest heading above it.
func boxSummary(lines []line) (map[string]int64, bool) {
	cx := func(w ocr.Word) int { return (w.X0 + w.X1) / 2 }
	for i, l := range lines {
		var vals []ocr.Word
		only := true
		for _, w := range l.words {
			if _, _, ok := parseAmount(w.Text); ok {
				vals = append(vals, w)
			} else if !isNoise(w.Text) {
				only = false
				break
			}
		}
		if !only || len(vals) < 4 {
			continue
		}
		heads := map[string]int{}
		h := l.height()
		for j := i - 1; j >= 0 && lines[j].page == l.page && l.y0-lines[j].y1 < 8*h; j-- {
			for _, w := range lines[j].words {
				var k string
				switch norm(w.Text) {
				case "previous":
					k = "prev"
				case "payments":
					k = "credits"
				case "purchases":
					k = "debits"
				case "interest":
					k = "interest"
				case "fees":
					k = "fees"
				case "current", "new":
					k = "total"
				}
				if _, seen := heads[k]; k != "" && !seen {
					heads[k] = cx(w)
				}
			}
		}
		if _, ok := heads["prev"]; !ok {
			continue
		}
		if _, ok := heads["total"]; !ok {
			continue
		}
		out := map[string]int64{}
		for _, v := range vals {
			best, bestD := "", 1<<30
			for k, x := range heads {
				if d := abs(cx(v) - x); d < bestD {
					best, bestD = k, d
				}
			}
			a, _, _ := parseAmount(v.Text)
			out[best] = a
		}
		return out, true
	}
	return nil, false
}

// headline is the text of a line up to the first wide gap.
func headline(l line, pageW int) string {
	var parts []string
	for i, w := range l.words {
		if i > 0 && w.X0-l.words[i-1].X1 > pageW/25 {
			break
		}
		parts = append(parts, w.Text)
	}
	return strings.Join(parts, " ")
}

// valueNear finds the amount belonging to a label: to its right on the same
// row, or just below it (header/value tables).
func valueNear(lines []line, b box, pageW int) (int64, bool, bool) {
	h := max(1, b.y1-b.y0)
	type cand struct {
		v     int64
		cr    bool
		score int
	}
	best := cand{score: 1 << 30}
	for _, l := range lines {
		if l.page != b.page {
			continue
		}
		for i, w := range l.words {
			cy := (w.Y0 + w.Y1) / 2
			text := w.Text
			if i+1 < len(l.words) && strings.EqualFold(l.words[i+1].Text, "CR") {
				text += "CR"
			}
			v, cr, ok := parseAmount(text)
			if !ok {
				continue
			}
			var score int
			switch {
			case w.X0 > b.x1 && w.X0-b.x1 < pageW*45/100 && cy >= b.y0-h/2 && cy <= b.y1+h*3/2:
				score = (w.X0 - b.x1) + 10*abs(cy-(b.y0+b.y1)/2)
			case cy > b.y1 && cy < b.y1+h*3 && w.X0 >= b.x0-pageW/50 && w.X0 <= b.x1+pageW/10:
				score = pageW + 10*(cy-b.y1)
			default:
				continue
			}
			if score < best.score {
				best = cand{v, cr, score}
			}
		}
	}
	return best.v, best.cr, best.score < 1<<30
}

func dateBelow(lines []line, b box, pageW int) (time.Time, bool) {
	h := max(1, b.y1-b.y0)
	for _, l := range lines {
		if l.page != b.page || l.y0 <= b.y1 || l.y0 > b.y1+h*3 {
			continue
		}
		var toks []string
		for _, w := range l.words {
			if w.X0 >= b.x0-pageW/50 && w.X0 <= b.x1+pageW/10 {
				toks = append(toks, w.Text)
			}
		}
		if m := reDate.FindStringSubmatch(strings.Join(toks, " ")); m != nil {
			if d := mkDate(m[1], m[2], m[3]); !d.IsZero() {
				return d, true
			}
		}
	}
	return time.Time{}, false
}

func mkDate(day, mon, year string) time.Time {
	d, _ := strconv.Atoi(day)
	y, _ := strconv.Atoi(year)
	m, ok := month(mon)
	if !ok || d < 1 || d > 31 || y < 1990 {
		return time.Time{}
	}
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// yearResolver picks the year that puts a day/month closest to the
// statement period (transactions can predate the period by weeks, and a
// period can span December → January).
func yearResolver(s Summary) func(dayMonth) time.Time {
	ref := s.PeriodEnd
	if ref.IsZero() {
		ref = time.Now().UTC()
	}
	return func(dm dayMonth) time.Time {
		if dm.year != 0 {
			return time.Date(dm.year, dm.month, dm.day, 0, 0, 0, 0, time.UTC)
		}
		best := time.Time{}
		bestDist := time.Duration(1<<63 - 1)
		for _, y := range []int{ref.Year() - 1, ref.Year(), ref.Year() + 1} {
			t := time.Date(y, dm.month, dm.day, 0, 0, 0, 0, time.UTC)
			d := ref.Sub(t)
			if d < 0 {
				d = -d * 4 // dates after the period end are unlikely
			}
			if d < bestDist {
				best, bestDist = t, d
			}
		}
		return best
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// holderName reads the account holder printed beside "Statement of Account"
// / "Account Statement" on page 1 ("LOY TAN … STATEMENT OF ACCOUNT").
func holderName(lines []line) string {
	for _, l := range lines {
		if l.page != 1 {
			continue
		}
		n := norm(l.text())
		if !strings.Contains(n, "statementofaccount") && !strings.Contains(n, "accountstatement") {
			continue
		}
		// The name is in capitals, right before the heading.
		var name []string
		heading := false
		for _, w := range l.words {
			u := strings.ToUpper(w.Text)
			if u == "STATEMENT" || u == "ACCOUNT" {
				heading = true
				break
			}
			if letters(w.Text) != len(w.Text) || u != w.Text {
				name = nil
				break
			}
			name = append(name, w.Text)
		}
		if heading && len(name) >= 2 && len(name) <= 5 {
			return strings.Join(name, " ")
		}
	}
	// Otherwise the name printed above the postal address ("LOK AH MEI" /
	// "BLK 216 …", DBS): 2–5 capitalised words over a line starting with a
	// block or house number.
	for i, l := range lines {
		if l.page != 1 || i+1 >= len(lines) || len(l.words) < 2 || len(l.words) > 5 {
			continue
		}
		ok := true
		for _, w := range l.words {
			if letters(w.Text) != len(w.Text) || strings.ToUpper(w.Text) != w.Text {
				ok = false
				break
			}
		}
		next := strings.ToUpper(strings.TrimSpace(lines[i+1].text()))
		if ok && lines[i+1].page == 1 && reAddress.MatchString(next) && abs(lines[i+1].x0()-l.x0()) < 30 {
			return l.text()
		}
	}
	return ""
}

var reAcctDigits = regexp.MustCompile(`^(?:Account\s+No\.?\s*)?\d[\d-]{6,}`)

var reAddress = regexp.MustCompile(`^(?:BLK|BLOCK|APT|#|\d)`)

// bankHeader fills a bank account's number (last 4) and product name.
func bankHeader(s *Summary, lines []line, width func(int) int) {
	s.Product, s.Last4 = "", ""
	// "DBS Savings Plus Account   203-0-002495": a name beside its number
	// beats a heading like "Current and Savings Account".
	for _, l := range lines {
		h := headline(l, width(l.page))
		rest := strings.TrimSpace(strings.TrimPrefix(l.text(), h))
		if strings.Contains(strings.ToLower(h), "account") && letters(strings.ReplaceAll(h, " ", "")) == len(strings.ReplaceAll(h, " ", "")) &&
			reAcctDigits.MatchString(rest) && len(strings.Fields(h)) <= 5 {
			s.Product = h
			break
		}
	}
	for _, l := range lines {
		if l.page != 1 {
			continue
		}
		t := l.text()
		if s.Last4 == "" {
			if m := reAcctNo.FindStringSubmatch(t); m != nil {
				digits := strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return r
					}
					return -1
				}, m[1])
				if len(digits) >= 4 {
					s.Last4 = digits[len(digits)-4:]
				}
			}
		}
		if s.Product == "" {
			h := headline(l, width(l.page))
			low := strings.ToLower(h)
			words := strings.Fields(h)
			if len(words) >= 2 && len(words) <= 5 && strings.Contains(low, "account") && letters(strings.Join(words, "")) == len(strings.Join(words, "")) &&
				!strings.Contains(low, "statement") && !strings.Contains(low, "summary") && !strings.HasPrefix(low, "main ") {
				s.Product = h
			}
		}
	}
}

// trustSummary reads a one-row balance table: opening, money in, money out,
// interest, closing (the last row with five amounts under the header).
func trustSummary(s *Summary, lines []line) bool {
	for i, l := range lines {
		n := norm(l.text())
		if !strings.Contains(n, "openingbalance") || !strings.Contains(n, "closingbalance") {
			continue
		}
		var vals []int64
		for _, nx := range lines[i+1 : min(len(lines), i+5)] {
			var amts []int64
			for _, w := range nx.words {
				if a, _, ok := parseAmount(w.Text); ok {
					amts = append(amts, a)
				}
			}
			if len(amts) == 5 {
				vals = amts
			}
		}
		if vals == nil {
			continue
		}
		// Interest is paid in as a row of its own, so it counts as money in.
		prev, in, out, closing := vals[0], vals[1]+vals[3], vals[2], vals[4]
		s.PrevBalance, s.Credits, s.Debits, s.Total = &prev, &in, &out, &closing
		return true
	}
	return false
}
