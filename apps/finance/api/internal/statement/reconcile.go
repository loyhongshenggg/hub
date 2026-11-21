package statement

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// Recon compares parsed rows with the statement's own totals.
type Recon struct {
	Checked    bool  `json:"checked"` // the statement printed totals we could read
	OK         bool  `json:"ok"`
	Debits     int64 `json:"debits"`     // sum of parsed debit rows
	Credits    int64 `json:"credits"`    // sum of parsed credit rows
	DebitsGap  int64 `json:"debitsGap"`  // statement − parsed
	CreditsGap int64 `json:"creditsGap"` // statement − parsed
	// SummaryOK: previous + debits − credits = total (the summary itself
	// was read correctly).
	SummaryOK bool `json:"summaryOk"`
}

// Reconcile checks txns against s.
func Reconcile(s Summary, txns []Txn) Recon {
	var r Recon
	for _, t := range txns {
		if t.Credit {
			r.Credits += t.Amount
		} else {
			r.Debits += t.Amount
		}
	}
	if s.PrevBalance != nil && s.Debits != nil && s.Credits != nil && s.Total != nil {
		bal := *s.PrevBalance + *s.Debits - *s.Credits
		// A balance in credit prints "Total Due 0.00".
		r.SummaryOK = bal == *s.Total || *s.Total == 0 && bal <= 0
	}
	if s.Debits == nil || s.Credits == nil {
		return r
	}
	r.Checked = true
	r.DebitsGap = *s.Debits - r.Debits
	r.CreditsGap = *s.Credits - r.Credits
	r.OK = r.DebitsGap == 0 && r.CreditsGap == 0
	return r
}

func (r Recon) gap() int64 { return abs64(r.DebitsGap) + abs64(r.CreditsGap) }

// Result is the final reading of a statement.
type Result struct {
	Summary  Summary  `json:"summary"`
	Txns     []Txn    `json:"txns"`
	Recon    Recon    `json:"recon"`
	Passes   []string `json:"passes"` // OCR passes that ran
	Warnings []string `json:"warnings,omitempty"`
}

// TextLayerReader reads a PDF's own text (no OCR), if it has any.
type TextLayerReader interface {
	TextLayer(ctx context.Context, pdfPath string) ([]ocr.Page, error)
}

// Pass is one way of reading the pages.
type Pass struct {
	Name string
	Opt  ocr.Options
}

// Passes run in order until one reconciles. Later passes read the
// transaction column at a higher resolution or with other segmentation;
// their rows are also used to fill gaps in the best pass.
var Passes = []Pass{
	{"300dpi", ocr.Options{DPI: 300, PSM: 6}},
	{"400dpi-column", ocr.Options{DPI: 400, PSM: 6, Crop: &ocr.Rect{X0: 0, Y0: 0, X1: 0.67, Y1: 1}}},
	{"300dpi-columns", ocr.Options{DPI: 300, PSM: 4}},
	{"400dpi-sparse", ocr.Options{DPI: 400, PSM: 11}},
}

// Read OCRs a statement PDF and returns the best reconciled reading.
func Read(ctx context.Context, eng ocr.Engine, pdfPath string, progress func(string)) (*Result, error) {
	if progress == nil {
		progress = func(string) {}
	}
	var reads []Parsed
	var names []string
	// PDFs with a text layer (Citibank) are read exactly; OCR is the
	// fallback for image-only ones (HSBC) or when the text doesn't add up.
	if tl, ok := eng.(TextLayerReader); ok {
		progress("Reading the PDF's text…")
		if pages, err := tl.TextLayer(ctx, pdfPath); err == nil && countWords(pages) > 50 {
			reads = append(reads, Parse(pages))
			names = append(names, "text layer")
			if res := Combine(reads); res.Recon.OK || !res.Recon.Checked && len(res.Txns) > 0 {
				res.Passes = names
				return res, nil
			}
		}
	}
	for i, ps := range Passes {
		progress(fmt.Sprintf("Reading pages (pass %d of %d)…", i+1, len(Passes)))
		pages, err := eng.Read(ctx, pdfPath, ps.Opt)
		if err != nil {
			if i == 0 && len(reads) == 0 {
				return nil, err
			}
			continue
		}
		if i == 0 && countWords(pages) == 0 && len(reads) == 0 {
			return nil, errors.New("no text found on any page")
		}
		reads = append(reads, Parse(pages))
		names = append(names, ps.Name)
		if res := Combine(reads); res.Recon.OK || !res.Recon.Checked && i == 0 && len(res.Txns) > 0 {
			res.Passes = names
			return res, nil
		}
	}
	if len(reads) == 0 {
		return nil, errors.New("could not read the statement")
	}
	res := Combine(reads)
	res.Passes = names
	return res, nil
}

func countWords(pages []ocr.Page) int {
	n := 0
	for _, p := range pages {
		n += len(p.Words)
	}
	return n
}

// Combine picks the best summary and rows from several OCR passes and, if
// the rows still don't add up, tries to close the gap with rows that only
// other passes saw (or that one pass read with a different amount).
func Combine(reads []Parsed) *Result {
	res := &Result{}
	// Summary: the first one that is internally consistent, else the first
	// with the most fields.
	res.Summary = reads[0].Summary
	for _, r := range reads {
		rc := Reconcile(r.Summary, nil)
		if rc.SummaryOK {
			res.Summary = r.Summary
			break
		}
	}
	fillSummary(&res.Summary, reads)

	best := -1
	var bestRecon Recon
	for i, r := range reads {
		rc := Reconcile(res.Summary, r.Txns)
		if best < 0 || rc.gap() < bestRecon.gap() || rc.gap() == bestRecon.gap() && len(r.Txns) > len(reads[best].Txns) {
			best, bestRecon = i, rc
		}
	}
	res.Txns = append([]Txn(nil), reads[best].Txns...)
	res.Recon = bestRecon
	if !res.Recon.Checked {
		res.Warnings = append(res.Warnings, "The statement's totals could not be read, so the rows were not checked.")
		return res
	}
	if !res.Recon.OK {
		res.Txns = closeGap(res.Txns, res.Recon, reads, best)
		res.Recon = Reconcile(res.Summary, res.Txns)
	}
	bestDescriptions(res.Txns, reads)
	if !res.Recon.SummaryOK {
		res.Warnings = append(res.Warnings, "The account summary doesn't add up (previous + purchases − payments ≠ total); a figure may be misread.")
	}
	if !res.Recon.OK {
		res.Warnings = append(res.Warnings, gapText(res.Recon))
	}
	return res
}

// bestDescriptions replaces each row's description with the reading the
// OCR was most confident about across passes (same dates and amount).
func bestDescriptions(txns []Txn, reads []Parsed) {
	type alt struct {
		desc string
		conf float64
	}
	best := map[string]alt{}
	for _, r := range reads {
		seen := map[string]int{}
		for _, t := range r.Txns {
			k := key(t)
			seen[k]++
			k = fmt.Sprintf("%s#%d", k, seen[k])
			if a, ok := best[k]; !ok || t.Conf > a.conf {
				best[k] = alt{t.Description, t.Conf}
			}
		}
	}
	seen := map[string]int{}
	for i, t := range txns {
		k := key(t)
		seen[k]++
		k = fmt.Sprintf("%s#%d", k, seen[k])
		if a, ok := best[k]; ok && a.conf > t.Conf+2 {
			txns[i].Description, txns[i].Conf = a.desc, a.conf
		}
	}
}

func fillSummary(s *Summary, reads []Parsed) {
	for _, r := range reads {
		o := r.Summary
		if s.Bank == "" {
			s.Bank = o.Bank
		}
		if s.Product == "" {
			s.Product = o.Product
		}
		if s.Last4 == "" {
			s.Last4 = o.Last4
		}
		if s.PeriodStart.IsZero() {
			s.PeriodStart, s.PeriodEnd = o.PeriodStart, o.PeriodEnd
		}
		if s.DueDate.IsZero() {
			s.DueDate = o.DueDate
		}
		for _, p := range []struct{ dst, src **int64 }{{&s.PrevBalance, &o.PrevBalance}, {&s.Credits, &o.Credits}, {&s.Debits, &o.Debits}, {&s.Total, &o.Total}} {
			if *p.dst == nil {
				*p.dst = *p.src
			}
		}
	}
}

func gapText(r Recon) string {
	var parts []string
	if r.DebitsGap != 0 {
		parts = append(parts, fmt.Sprintf("purchases off by %s", money(r.DebitsGap)))
	}
	if r.CreditsGap != 0 {
		parts = append(parts, fmt.Sprintf("payments & credits off by %s", money(r.CreditsGap)))
	}
	return "Rows don't match the statement totals: " + strings.Join(parts, ", ") + ". Check the highlighted pages."
}

func money(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%sS$%d.%02d", sign, c/100, c%100)
}

// key identifies a row across passes.
func key(t Txn) string {
	return fmt.Sprintf("%s|%s|%d|%t", t.PostDate.Format("0102"), t.TxnDate.Format("0102"), t.Amount, t.Credit)
}

// closeGap adds rows seen only by other passes, or swaps in another pass's
// amount for a row, when that makes the totals match exactly.
func closeGap(txns []Txn, rc Recon, reads []Parsed, best int) []Txn {
	have := map[string]int{}
	for _, t := range txns {
		have[key(t)]++
	}
	// 1. Missing rows: rows other passes read that the best pass doesn't have.
	var extra []Txn
	seen := map[string]int{}
	for i, r := range reads {
		if i == best {
			continue
		}
		cnt := map[string]int{}
		for _, t := range append(append([]Txn(nil), r.Txns...), r.orphans...) {
			k := key(t)
			cnt[k]++
			if cnt[k] > have[k] && cnt[k] > seen[k] {
				seen[k] = cnt[k]
				t.Recovered = true
				extra = append(extra, t)
			}
		}
	}
	for _, t := range reads[best].orphans {
		t.Recovered = true
		extra = append(extra, t)
	}
	if pick := subsetForGap(extra, rc.DebitsGap, rc.CreditsGap); pick != nil {
		return sortTxns(append(txns, pick...))
	}
	// 2. Misread amounts: the same row (dates + description) read with a
	// different amount by another pass.
	for i, t := range txns {
		for j, r := range reads {
			if j == best {
				continue
			}
			for _, o := range r.Txns {
				if o.Credit != t.Credit || o.Amount == t.Amount || !o.PostDate.Equal(t.PostDate) || !o.TxnDate.Equal(t.TxnDate) || !similar(o.Description, t.Description) {
					continue
				}
				d := o.Amount - t.Amount
				if !t.Credit && d == rc.DebitsGap && rc.CreditsGap == 0 || t.Credit && d == rc.CreditsGap && rc.DebitsGap == 0 {
					fixed := append([]Txn(nil), txns...)
					fixed[i].Amount = o.Amount
					fixed[i].Recovered = true
					return fixed
				}
			}
		}
	}
	return txns
}

// subsetForGap finds up to three extra rows whose debits and credits close
// both gaps exactly.
func subsetForGap(extra []Txn, debitGap, creditGap int64) []Txn {
	if len(extra) == 0 || debitGap < 0 || creditGap < 0 {
		return nil
	}
	sum := func(ts []Txn) (d, c int64) {
		for _, t := range ts {
			if t.Credit {
				c += t.Amount
			} else {
				d += t.Amount
			}
		}
		return
	}
	n := len(extra)
	if n > 40 {
		extra, n = extra[:40], 40
	}
	try := func(ts ...Txn) []Txn {
		if d, c := sum(ts); d == debitGap && c == creditGap {
			return ts
		}
		return nil
	}
	for a := range n {
		if r := try(extra[a]); r != nil {
			return r
		}
	}
	for a := range n {
		for b := a + 1; b < n; b++ {
			if r := try(extra[a], extra[b]); r != nil {
				return r
			}
		}
	}
	for a := range n {
		for b := a + 1; b < n; b++ {
			for c := b + 1; c < n; c++ {
				if r := try(extra[a], extra[b], extra[c]); r != nil {
					return r
				}
			}
		}
	}
	return nil
}

func similar(a, b string) bool {
	na, nb := norm(a), norm(b)
	if na == "" || nb == "" {
		return false
	}
	n := min(len(na), len(nb), 8)
	return na[:n] == nb[:n]
}

func sortTxns(ts []Txn) []Txn {
	// Stable by page then position, like the statement.
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && less(ts[j], ts[j-1]); j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
	return ts
}

func less(a, b Txn) bool {
	if a.Page != b.Page {
		return a.Page < b.Page
	}
	if a.Y != b.Y {
		return a.Y < b.Y
	}
	return a.PostDate.Before(b.PostDate)
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// Year of a period, for labels.
func (s Summary) Month() time.Time {
	if s.PeriodEnd.IsZero() {
		return time.Time{}
	}
	return time.Date(s.PeriodEnd.Year(), s.PeriodEnd.Month(), 1, 0, 0, 0, 0, time.UTC)
}
