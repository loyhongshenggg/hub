package statement

import (
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// page lays out text lines as OCR words: each line is "x:text" segments
// separated by "|", where x is the left edge in characters (10 px each).
func page(n int, lines ...string) ocr.Page {
	p := ocr.Page{Number: n, Width: 1000, Height: 30 * (len(lines) + 2), DPI: 300}
	for i, l := range lines {
		y := 30 + i*30
		for _, seg := range strings.Split(l, "|") {
			var x int
			text := seg
			if k := strings.IndexByte(seg, ':'); k > 0 && k < 4 {
				for _, c := range seg[:k] {
					x = x*10 + int(c-'0')
				}
				text = seg[k+1:]
			}
			cx := x * 10
			for _, w := range strings.Fields(text) {
				p.Words = append(p.Words, ocr.Word{Text: w, X0: cx, X1: cx + 10*len(w), Y0: y, Y1: y + 20, Conf: 90})
				cx += 10 * (len(w) + 1)
			}
		}
	}
	return p
}

func statementPages() []ocr.Page {
	return []ocr.Page{
		page(1,
			"0:HSBC VISA REVOLUTION",
			"0:Statement period|30:Total Due",
			"0:From 07 SEP 2026 to 05 OCT 2026|30:473.00",
			"0:4321-XXXX-XXXX-4321",
			"0:POST TRAN|70:ACCOUNT SUMMARY SGD",
			"0:DATE DATE DESCRIPTION|45:AMOUNT(SGD)|70:Previous Statement Balance|99:600.00",
			"70:Payments & Credits|99:1,500.00CR",
			"70:Purchases & Debits|99:1,373.00",
			"70:Total Account Balance|99:473.00",
			"18:Previous Statement Balance|52:600.00",
			"0:07 Sep|7:02 Sep|14:BUS/MRT 900000001|53:2.10|70:Credit Limit (SGD)|99:10,000.00",
			"14:SINGAPORE SG",
			"0:= ~ - - - - - - - - - - - - -",
			"0:07 Sep|7:05Sep|14:SMP*DAILY FUSION CUISI|52:18.40|70:Purchase|99:27.80%",
			"0:08 Se p|7:04 Sep|14:BUS/MRT 900000002|53:3.30|58:SS",
			"14:Continued on next page",
		),
		page(2,
			"0:DATE DATE DESCRIPTION|45:AMOUNT(SGD)",
			"0:07 Sep|7:07 Sep|14:PYMT @ AXS -THANK YOU|51:1,500.00CR",
			"0:19 Sep|7:17 Sep|14:Prudential Singapore SINGAPORE|50:1,250.00",
			"14:SG",
			"0:05 Oct|7:040ct|14:_ fp*Food Panda Singapore SG|52:99.20",
			"14:Total Due|50:473.00",
		),
	}
}

func TestParse(t *testing.T) {
	p := Parse(statementPages())
	s := p.Summary
	if s.Bank != "HSBC" || s.Last4 != "4321" || s.Product != "HSBC VISA REVOLUTION" {
		t.Errorf("header: %+v", s)
	}
	if !s.PeriodStart.Equal(time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)) || !s.PeriodEnd.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("period %v – %v", s.PeriodStart, s.PeriodEnd)
	}
	want := map[string]int64{"prev": 60000, "credits": 150000, "debits": 137300, "total": 47300}
	got := map[string]*int64{"prev": s.PrevBalance, "credits": s.Credits, "debits": s.Debits, "total": s.Total}
	for k, v := range want {
		if got[k] == nil || *got[k] != v {
			t.Errorf("%s = %v, want %d", k, got[k], v)
		}
	}
	type row struct {
		post, txn, desc string
		amount          int64
		credit          bool
	}
	wantRows := []row{
		{"09-07", "09-02", "BUS/MRT 900000001 SINGAPORE SG", 210, false},
		{"09-07", "09-05", "SMP*DAILY FUSION CUISI", 1840, false},
		{"09-08", "09-04", "BUS/MRT 900000002", 330, false}, // split month, junk after amount
		{"09-07", "09-07", "PYMT @ AXS -THANK YOU", 150000, true},
		{"09-19", "09-17", "Prudential Singapore SINGAPORE SG", 125000, false},
		{"10-05", "10-04", "fp*Food Panda Singapore SG", 9920, false}, // "040ct", leading noise
	}
	if len(p.Txns) != len(wantRows) {
		for _, tx := range p.Txns {
			t.Logf("%+v", tx)
		}
		t.Fatalf("got %d rows, want %d", len(p.Txns), len(wantRows))
	}
	for i, w := range wantRows {
		g := p.Txns[i]
		if g.PostDate.Format("01-02") != w.post || g.TxnDate.Format("01-02") != w.txn || g.Description != w.desc || g.Amount != w.amount || g.Credit != w.credit {
			t.Errorf("row %d = %s %s %q %d %v, want %+v", i, g.PostDate.Format("01-02"), g.TxnDate.Format("01-02"), g.Description, g.Amount, g.Credit, w)
		}
		if g.PostDate.Year() != 2026 {
			t.Errorf("row %d year %d", i, g.PostDate.Year())
		}
	}
	if r := Reconcile(s, p.Txns); !r.OK || !r.SummaryOK {
		t.Errorf("reconcile: %+v", r)
	}
}

func TestYearAcrossNewYear(t *testing.T) {
	y := yearResolver(Summary{PeriodStart: time.Date(2026, 12, 7, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2027, 1, 5, 0, 0, 0, 0, time.UTC)})
	if got := y(dayMonth{30, time.November}); got.Year() != 2026 {
		t.Errorf("30 Nov → %d", got.Year())
	}
	if got := y(dayMonth{3, time.January}); got.Year() != 2027 {
		t.Errorf("3 Jan → %d", got.Year())
	}
}

func TestMonthOCR(t *testing.T) {
	for tok, want := range map[string]time.Month{"Sep": 9, "0ct": 10, "C0ct": 10, "5ep": 9, "Nov": 11, "N0v": 11, "Dec": 12} {
		if m, ok := month(tok); !ok || m != want {
			t.Errorf("month(%q) = %v %v", tok, m, ok)
		}
	}
	if _, ok := month("SG"); ok {
		t.Error("SG is not a month")
	}
}

func TestParseAmount(t *testing.T) {
	for in, want := range map[string]int64{"1.75": 175, "1,250.40": 125040, "1.250.40": 125040, "942.00CR": 94200} {
		if got, _, ok := parseAmount(in); !ok || got != want {
			t.Errorf("parseAmount(%q) = %d %v", in, got, ok)
		}
	}
	if _, cr, _ := parseAmount("942.00CR"); !cr {
		t.Error("CR not detected")
	}
	if _, _, ok := parseAmount("27.80%"); ok {
		t.Error("percent parsed as amount")
	}
}

func TestCombineRecoversMissingRow(t *testing.T) {
	full := Parse(statementPages())
	// Pass 1 missed the Prudential row; pass 2 read it.
	missing := full
	missing.Txns = append([]Txn(nil), full.Txns[:4]...)
	missing.Txns = append(missing.Txns, full.Txns[5])
	res := Combine([]Parsed{missing, full})
	if !res.Recon.OK || len(res.Txns) != 6 {
		t.Fatalf("recon %+v rows %d", res.Recon, len(res.Txns))
	}
	// Only one pass, row missing: gap reported.
	res = Combine([]Parsed{missing})
	if res.Recon.OK || res.Recon.DebitsGap != 125000 || len(res.Warnings) == 0 {
		t.Errorf("gap: %+v %v", res.Recon, res.Warnings)
	}
}

func TestCombineFixesMisreadAmount(t *testing.T) {
	full := Parse(statementPages())
	bad := full
	bad.Txns = append([]Txn(nil), full.Txns...)
	bad.Txns[1].Amount = 1640 // 18.40 misread as 16.40
	res := Combine([]Parsed{bad, full})
	if !res.Recon.OK {
		t.Fatalf("not fixed: %+v", res.Recon)
	}
	// Combine prefers the pass that reconciles outright.
	res = Combine([]Parsed{bad})
	if res.Recon.OK {
		t.Error("single bad pass should not reconcile")
	}
}

func TestBestDescription(t *testing.T) {
	a := Parse(statementPages())
	b := a
	b.Txns = append([]Txn(nil), a.Txns...)
	a.Txns = append([]Txn(nil), a.Txns...)
	a.Txns[1].Description, a.Txns[1].Conf = "SMP*DA1LY FUS10N", 40
	b.Txns[1].Conf = 95
	res := Combine([]Parsed{a, b})
	if res.Txns[1].Description != "SMP*DAILY FUSION CUISI" {
		t.Errorf("got %q", res.Txns[1].Description)
	}
}

func TestParseTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"4\t1\t1\t1\t1\t0\t10\t20\t300\t30\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t50\t30\t96.5\tHSBC\n" +
		"5\t1\t1\t1\t1\t2\t70\t20\t40\t30\t91\t \n"
	words, err := ocr.ParseTSV([]byte(tsv))
	if err != nil || len(words) != 1 || words[0].Text != "HSBC" || words[0].X1 != 60 || words[0].Conf != 96.5 {
		t.Fatalf("%+v %v", words, err)
	}
}

func TestCreditBalance(t *testing.T) {
	p := Parse([]ocr.Page{page(1,
		"0:HSBC VISA REVOLUTION",
		"0:From 06 JUL 2026 to 05 AUG 2026",
		"0:DATE DATE DESCRIPTION|45:AMOUNT(SGD)|70:Previous Statement Balance|99:0.52CR",
		"70:Payments & Credits|99:0.00",
		"70:Purchases & Debits|99:0.00",
		"70:Total Account Balance|99:0.52CR",
	)})
	s := p.Summary
	if s.PrevBalance == nil || *s.PrevBalance != -52 || s.Total == nil || *s.Total != -52 {
		t.Fatalf("prev %v total %v", s.PrevBalance, s.Total)
	}
	if r := Reconcile(s, p.Txns); !r.OK || !r.SummaryOK || len(p.Txns) != 0 {
		t.Errorf("%+v rows %d", r, len(p.Txns))
	}
}
