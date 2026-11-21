package statement

import (
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// A Citibank-style statement (synthetic): one date column, credits in
// parentheses, a boxed summary, foreign-amount and card-number lines.
func citiPages() []ocr.Page {
	return []ocr.Page{
		page(1,
			"0:Citibank Singapore Ltd",
			"0:Statement Date|30:March 18, 2027",
			"0:Payment Due Date|30:April 12, 2027",
			"0:CITI PREMIERMILES WORLD MASTER|40:58.20|55:0.00|70:58.20",
		),
		page(2,
			"0:CITI PREMIERMILES WORLD MASTER 1111 2222 3333 4444|60:Payment Due Date: April 12, 2027",
			"0:PREVIOUS|20:PAYMENTS|40:PURCHASES|55:INTEREST|70:FEES|85:CURRENT",
			"0:BALANCE|20:&|40:&|55:CHARGES|70:&|85:BALANCE",
			"20:CREDITS|40:ADVANCES|70:CHARGES",
			"0:100.00|20:150.00|40:108.00|55:0.00|70:0.20|85:58.20",
			"0:DATE|8:DESCRIPTION|80:AMOUNT (SGD)",
			"8:BALANCE PREVIOUS STATEMENT|85:100.00",
			"0:01 MAR|8:PAYMENT - THANK YOU SG|84:(100.00)",
			"8:SUB-TOTAL:|85:0.00",
			"0:CITI PREMIERMILES WORLD MASTER 1111 2222 3333 4444 - A CARDHOLDER",
			"0:20 FEB|8:GRAB-EC|25:PETALING JAYAMY|86:10.00",
			"8:FOREIGN AMOUNT MALAYSIAN RINGGIT 31.00",
			"0:22 FEB|8:AMAZON MKTPLC SG|30:SINGAPORE SG|84:(50.00)",
			"0:23 FEB|8:clippers.com.sg|30:SINGAPORE SG|86:15.00",
			"8:XXXX-XXXX-XXXX-9999",
			"0:25 FEB|8:CCY CONVERSION FEE|30:SGD|35:10.00|87:0.20",
			"0:10 MAR|8:SHOPEE SG MP|30:SINGAPORE SG|86:83.00",
			"8:SUB-TOTAL:|85:58.20",
			"8:GRAND TOTAL|85:58.20",
		),
	}
}

func TestParseCiti(t *testing.T) {
	p := Parse(citiPages())
	s := p.Summary
	if s.Bank != "Citibank" || s.Product != "CITI PREMIERMILES WORLD MASTER" || s.Last4 != "4444" {
		t.Errorf("header: %q %q %q", s.Bank, s.Product, s.Last4)
	}
	if !s.PeriodEnd.Equal(time.Date(2027, 3, 18, 0, 0, 0, 0, time.UTC)) || !s.PeriodStart.Equal(time.Date(2027, 2, 19, 0, 0, 0, 0, time.UTC)) ||
		!s.DueDate.Equal(time.Date(2027, 4, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("dates: %v %v due %v", s.PeriodStart, s.PeriodEnd, s.DueDate)
	}
	// Debits = purchases & advances + interest + fees.
	if s.PrevBalance == nil || *s.PrevBalance != 10000 || *s.Credits != 15000 || *s.Debits != 10820 || *s.Total != 5820 {
		t.Fatalf("box summary: %v %v %v %v", s.PrevBalance, s.Credits, s.Debits, s.Total)
	}
	want := []struct {
		date, desc string
		amount     int64
		credit     bool
		foreign    string
	}{
		{"03-01", "PAYMENT - THANK YOU SG", 10000, true, ""},
		{"02-20", "GRAB-EC PETALING JAYAMY", 1000, false, "MALAYSIAN RINGGIT 31.00"},
		{"02-22", "AMAZON MKTPLC SG SINGAPORE SG", 5000, true, ""},
		{"02-23", "clippers.com.sg SINGAPORE SG", 1500, false, ""},
		{"02-25", "CCY CONVERSION FEE SGD 10.00", 20, false, ""},
		{"03-10", "SHOPEE SG MP SINGAPORE SG", 8300, false, ""},
	}
	if len(p.Txns) != len(want) {
		for _, tx := range p.Txns {
			t.Logf("%+v", tx)
		}
		t.Fatalf("got %d rows", len(p.Txns))
	}
	for i, w := range want {
		g := p.Txns[i]
		if g.TxnDate.Format("01-02") != w.date || g.TxnDate.Year() != 2027 || g.Description != w.desc || g.Amount != w.amount || g.Credit != w.credit || g.Foreign != w.foreign {
			t.Errorf("row %d = %s %q %d %v %q", i, g.TxnDate.Format("2006-01-02"), g.Description, g.Amount, g.Credit, g.Foreign)
		}
	}
	if r := Reconcile(s, p.Txns); !r.OK || !r.SummaryOK {
		t.Errorf("reconcile %+v", r)
	}
}

func TestParseBBoxDropsVerticalText(t *testing.T) {
	x := `<doc><page width="595.2" height="841.9">
<word xMin="70.0" yMin="100.0" xMax="90.0" yMax="107.0">12 SEP</word>
<word xMin="542.1" yMin="712.8" xMax="566.0" yMax="720.2">306.58</word>
<word xMin="572.9" yMin="770.9" xMax="578.4" yMax="798.9">EPSTCSX</word>
<word xMin="572.9" yMin="737.0" xMax="578.4" yMax="740.4">3</word>
<word xMin="572.9" yMin="727.1" xMax="578.4" yMax="735.3">OF</word>
<word xMin="100.0" yMin="100.0" xMax="140.0" yMax="107.0">A&amp;W</word>
</page></doc>`
	pages := ocr.ParseBBox([]byte(x), 300)
	if len(pages) != 1 || pages[0].Width != 2480 {
		t.Fatalf("%+v", pages)
	}
	var got []string
	for _, w := range pages[0].Words {
		got = append(got, w.Text)
	}
	if len(got) != 3 || got[2] != "A&W" {
		t.Errorf("words %v", got)
	}
}
