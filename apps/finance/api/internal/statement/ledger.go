package statement

import (
	"strings"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// Bank-account statements (OCBC, DBS/POSB, UOB…) print a ledger:
//
//	Date  [Value date]  Description            Withdrawal  Deposit  Balance
//	      BALANCE B/F                                                656.92
//	04 SEP  04 SEP  PAYMENT/TRANSFER                        38.70    695.62
//	                via PayNow-DBSS
//	                from A PERSON
//
// The columns come from the header row; a row starts with a date and runs
// until the next one. Every printed balance is checked against the one
// before it, so a misread amount shows up instead of passing silently.

// ledgerCols are a page's column positions (word centres, page pixels).
type ledgerCols struct {
	top             int // y of the header row's bottom
	descX           int // left edge of the description column
	wd, dp, bal     int // centres of Withdrawal, Deposit, Balance
	half            int // half the narrowest gap between those columns
	hasWd, hasDp    bool
	hasBal, dateHdr bool
}

// findLedgerHeader looks for a "… Withdrawal … Deposit … Balance" row.
func findLedgerHeader(lines []line) (ledgerCols, bool) {
	for _, l := range lines {
		var c ledgerCols
		for _, w := range l.words {
			n := norm(w.Text)
			mid := (w.X0 + w.X1) / 2
			switch {
			case strings.HasPrefix(n, "withdraw") || n == "debit" || n == "debits":
				c.wd, c.hasWd = mid, true
			case strings.HasPrefix(n, "deposit") || n == "credit" || n == "credits":
				c.dp, c.hasDp = mid, true
			case n == "balance":
				c.bal, c.hasBal = mid, true
			case n == "description" || n == "particulars" || n == "details":
				c.descX = w.X0
			case n == "date":
				c.dateHdr = true
			}
		}
		// A real header is a short row with a date column and the money
		// columns in order (prose like "Debit Balances … Credit Balances"
		// in the small print is not).
		if !c.hasWd || !c.hasDp || !c.hasBal || !c.dateHdr || len(l.words) > 14 ||
			!(c.wd < c.bal && c.dp < c.bal) {
			continue
		}
		c.top = l.y1
		gap := min(abs(c.dp-c.wd), abs(c.bal-c.dp))
		c.half = max(gap/2, 1)
		if c.descX == 0 {
			c.descX = l.words[0].X0
		}
		return c, true
	}
	return ledgerCols{}, false
}

// column says which money column an amount word sits in ("" if none).
func (c ledgerCols) column(w ocr.Word) string {
	mid := (w.X0 + w.X1) / 2
	best, dist := "", c.half+1
	for _, k := range []struct {
		name string
		x    int
	}{{"wd", c.wd}, {"dp", c.dp}, {"bal", c.bal}} {
		if d := abs(mid - k.x); d <= c.half && d < dist {
			best, dist = k.name, d
		}
	}
	return best
}

// ledgerRow is one dated row and the description lines under it.
type ledgerRow struct {
	row
	lines   []string
	balance int64
	hasBal  bool
	wd, dp  int64
}

// isLedger reports whether pages hold a bank-account ledger.
func isLedger(pages []ocr.Page) bool {
	for _, pg := range pages {
		if _, ok := findLedgerHeader(groupLines(pg.Number, pg.Words)); ok {
			return true
		}
	}
	return false
}

var (
	openingLabels = []string{"balancebf", "balancebroughtforward", "openingbalance", "balanceforward", "previousbalance"}
	closingLabels = []string{"balancecf", "balancecarriedforward", "closingbalance"}
	ledgerStops   = []string{"depositinsurancescheme", "checkyourstatement", "totalwithdrawals", "totaldebits", "totalinterest", "averagebalance"}
)

func hasPrefixAny(n string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// parseLedger reads a bank-account statement.
func parseLedger(pages []ocr.Page) Parsed {
	var p Parsed
	var all []line
	var rows []*ledgerRow
	var opening, closing *int64
	var totalWd, totalDp *int64
	for _, pg := range pages {
		lines := groupLines(pg.Number, pg.Words)
		all = append(all, lines...)
		cols, ok := findLedgerHeader(lines)
		// Totals can sit below the last page's table, or on a page without one.
		for _, l := range lines {
			if n := norm(l.text()); strings.HasPrefix(n, "totalwithdrawals") || strings.HasPrefix(n, "totaldebits") {
				var amts []int64
				for _, w := range l.words {
					if a, _, ok := parseAmount(w.Text); ok {
						amts = append(amts, a)
					}
				}
				if len(amts) >= 2 {
					totalWd, totalDp = &amts[0], &amts[1]
				}
			}
		}
		if !ok {
			continue
		}
		var cur *ledgerRow
		lastY1 := cols.top
		for _, l := range lines {
			if l.y0 < cols.top {
				continue
			}
			n := norm(l.text())
			if hasPrefixAny(n, ledgerStops) {
				break
			}
			// Amounts in the money columns, and the words left of them.
			var wd, dp, bal *int64
			var text []ocr.Word
			for _, w := range l.words {
				if a, _, ok := parseAmount(w.Text); ok {
					switch cols.column(w) {
					case "wd":
						wd = &a
						continue
					case "dp":
						dp = &a
						continue
					case "bal":
						bal = &a
						continue
					}
				}
				text = append(text, w)
			}
			toks := make([]string, len(text))
			for i, w := range text {
				toks[i] = w.Text
			}
			// Opening / closing balance rows (no date).
			if hasPrefixAny(n, openingLabels) || hasPrefixAny(norm(strings.Join(toks, "")), openingLabels) {
				if bal != nil && opening == nil {
					v := *bal
					opening = &v
				}
				cur = nil
				continue
			}
			if hasPrefixAny(n, closingLabels) || hasPrefixAny(norm(strings.Join(toks, "")), closingLabels) {
				if bal != nil {
					v := *bal
					closing = &v
				}
				cur = nil
				continue
			}
			d1, i, dated := readDate(toks, 0)
			if dated && (len(text) == 0 || text[0].X0 < cols.descX) {
				r := &ledgerRow{row: row{post: d1, txn: d1, page: pg.Number, dated: true, y: float64(l.y0) / float64(max(1, pg.Height))}}
				// A second date is the value date; the first is when it happened.
				if _, j, ok := readDate(toks, i); ok && j <= len(text) && (j == len(text) || text[j-1].X0 < cols.descX) {
					i = j
				}
				if desc := strings.TrimSpace(strings.Join(toks[i:], " ")); desc != "" {
					r.lines = append(r.lines, desc)
				}
				if wd != nil {
					r.wd = *wd
				}
				if dp != nil {
					r.dp = *dp
				}
				if bal != nil {
					r.balance, r.hasBal = *bal, true
				}
				rows = append(rows, r)
				cur, lastY1 = r, l.y1
				continue
			}
			// A description line under the current row.
			if cur != nil && len(toks) > 0 && l.y0-lastY1 < 3*l.height() && text[0].X0 >= cols.descX-pg.Width/50 {
				cur.lines = append(cur.lines, strings.Join(toks, " "))
				if wd != nil && cur.wd == 0 {
					cur.wd = *wd
				}
				if dp != nil && cur.dp == 0 {
					cur.dp = *dp
				}
				if bal != nil && !cur.hasBal {
					cur.balance, cur.hasBal = *bal, true
				}
				lastY1 = l.y1
				continue
			}
			cur = nil
		}
	}

	s := parseSummary(all, pages)
	s.Kind = "bank"
	bankHeader(&s, all, func(n int) int {
		for _, pg := range pages {
			if pg.Number == n {
				return pg.Width
			}
		}
		return 1
	})
	s.PrevBalance, s.Total = opening, closing
	year := yearResolver(s)
	holder := s.Holder

	// Amounts: from the columns, else from the change in the running
	// balance; every printed balance is checked against the one before.
	bal, known := int64(0), false
	if opening != nil {
		bal, known = *opening, true
	}
	chainOK := known
	var sumWd, sumDp int64
	for _, r := range rows {
		amount, credit := r.wd, false
		if r.dp != 0 && r.wd == 0 {
			amount, credit = r.dp, true
		}
		if amount == 0 && r.hasBal && known {
			amount, credit = abs64(r.balance-bal), r.balance > bal
		}
		if amount == 0 {
			continue
		}
		if known && r.hasBal {
			want := bal - amount
			if credit {
				want = bal + amount
			}
			if want != r.balance {
				// The column guess was wrong if the other sign fits.
				if alt := bal + amount; !credit && alt == r.balance {
					credit = true
				} else if alt := bal - amount; credit && alt == r.balance {
					credit = false
				} else {
					chainOK = false
				}
			}
		}
		if r.hasBal {
			bal, known = r.balance, true
		} else if known {
			if credit {
				bal += amount
			} else {
				bal -= amount
			}
		}
		if credit {
			sumDp += amount
		} else {
			sumWd += amount
		}
		desc := strings.Join(r.lines, " ")
		kind, party, note, check := classify(r.lines, credit, holder)
		p.Txns = append(p.Txns, Txn{
			PostDate: year(r.post), TxnDate: year(r.txn), Description: desc, Amount: amount, Credit: credit,
			Page: r.page, Y: r.y, Conf: 100, Kind: kind, Counterparty: party, Note: note, Check: check,
		})
	}
	if closing == nil && known && len(rows) > 0 {
		v := bal
		s.Total = &v
	}
	switch {
	case totalWd != nil:
		s.Debits, s.Credits = totalWd, totalDp
	case chainOK && s.Total != nil && *s.Total == bal:
		// No printed totals, but every balance checked out: the rows are right.
		s.Debits, s.Credits = &sumWd, &sumDp
	}
	p.Summary = s
	p.Pages = len(pages)
	return p
}
