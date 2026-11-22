package statement

import (
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// An OCBC-style bank-account ledger (synthetic): Withdrawal / Deposit /
// Balance columns, two dates, multi-line descriptions, B/F and C/F rows,
// and printed totals on the last page.
func ledgerPages() []ocr.Page {
	return []ocr.Page{
		page(1,
			"68:OCBC Bank",
			"8:TAN AH KOW|68:STATEMENT OF ACCOUNT",
			"7:EXAMPLE ACCOUNT|78:1 SEP 2026 TO 30 SEP 2026",
			"7:Account No. 123456789012",
			"7:Transaction|15:Value",
			"7:Date|15:Date|22:Description|39:Cheque|55:Withdrawal|71:Deposit|86:Balance",
			"22:BALANCE B/F|90:100.00",
			"7:04 SEP|15:04 SEP|22:FAST PAYMENT|60:20.00|90:80.00",
			"22:via PayNow-UEN",
			"22:to SINGAPORE POOLS",
			"22:OTHR-DICNSQ183107018",
			"7:05 SEP|15:05 SEP|22:PAYMENT/TRANSFER|75:55.00|90:135.00",
			"22:via PayNow-DBSS",
			"22:from LIM MEI LING",
			"22:OTHR Photoshoot",
			"7:06 SEP|15:07 SEP|22:NETS QR|60:50.00|90:85.00",
			"22:88968901",
			"22:NETS QR PURCHASE",
			"22:AXS PTE LTD",
			"7:29 SEP|15:29 SEP|22:IBG GIRO|74:3,000.00|89:3,085.00",
			"22:SALA",
			"22:ACME PTE LTD",
		),
		page(2,
			"7:Date|15:Date|22:Description|39:Cheque|55:Withdrawal|71:Deposit|86:Balance",
			"7:30 SEP|15:30 SEP|22:FAST PAYMENT|59:1,000.00|90:2,085.00",
			"22:0193491628",
			"22:to TAN AH KOW",
			"7:30 SEP|15:30 SEP|22:FUND TRANSFER|60:300.00|89:1,785.00",
			"22:via PayNow-Mobile",
			"22:to LIX SIXX SUXX",
			"22:OTHR - Happy Deepavali",
			"7:30 SEP|15:30 SEP|22:IBG GIRO|75:60.00|89:1,845.00",
			"22:OTHR",
			"22:ACC 14",
			"7:01 OCT|15:30 SEP|22:INTEREST CREDIT|76:0.31|89:1,845.31",
			"22:BALANCE C/F|90:1,845.31",
			"22:Total Withdrawals/Deposits|59:1,370.00|73:3,115.31",
			"3:Deposit Insurance Scheme",
		),
	}
}

func TestParseLedger(t *testing.T) {
	p := Parse(ledgerPages())
	s := p.Summary
	if s.Kind != "bank" || s.Bank != "OCBC" || s.Product != "EXAMPLE ACCOUNT" || s.Last4 != "9012" || s.Holder != "TAN AH KOW" {
		t.Errorf("header: %+v", s)
	}
	if !s.PeriodStart.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || !s.PeriodEnd.Equal(time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("period %v %v", s.PeriodStart, s.PeriodEnd)
	}
	if deref(s.PrevBalance) != 100.0 || deref(s.Total) != 1845.31 || deref(s.Debits) != 1370.0 || deref(s.Credits) != 3115.31 || p.Pages != 2 {
		t.Errorf("summary prev=%v total=%v debits=%v credits=%v pages=%d", deref(s.PrevBalance), deref(s.Total), deref(s.Debits), deref(s.Credits), p.Pages)
	}
	want := []struct {
		date        string
		amount      int64
		credit      bool
		kind, party string
		note        string
		check       bool
	}{
		{"09-04", 2000, false, "", "SINGAPORE POOLS", "", false}, // a business: the merchant matcher decides
		{"09-05", 5500, true, "transfer", "Lim Mei Ling", "From Lim Mei Ling: Photoshoot", false},
		{"09-06", 5000, false, "payment", "AXS", "", false}, // AXS: a card bill, not spending
		{"09-29", 300000, true, "income", "ACME PTE LTD", "Salary", false},
		{"09-30", 100000, false, "transfer", "Own account", "Between your own accounts", false},
		{"09-30", 30000, false, "transfer", "Lix Sixx Suxx", "To Lix Sixx Suxx: Happy Deepavali", false},
		{"09-30", 6000, true, "transfer", "GIRO transfer", "", true}, // money in that says little: never a refund
		{"10-01", 31, true, "income", "Interest", "", false},
	}
	if len(p.Txns) != len(want) {
		for _, tx := range p.Txns {
			t.Logf("%+v", tx)
		}
		t.Fatalf("got %d rows", len(p.Txns))
	}
	for i, w := range want {
		g := p.Txns[i]
		if g.TxnDate.Format("01-02") != w.date || g.Amount != w.amount || g.Credit != w.credit || g.Kind != w.kind ||
			g.Counterparty != w.party || (w.note != "" && g.Note != w.note) || g.Check != w.check {
			t.Errorf("row %d = %s %d cr=%v kind=%q party=%q note=%q check=%v", i, g.TxnDate.Format("01-02"), g.Amount, g.Credit, g.Kind, g.Counterparty, g.Note, g.Check)
		}
	}
	if r := Reconcile(s, p.Txns); !r.OK || !r.SummaryOK {
		t.Errorf("reconcile %+v", r)
	}
}

// Without printed totals, a ledger still checks out through its running
// balances; a misread amount breaks the chain instead of passing.
func TestLedgerRunningBalance(t *testing.T) {
	pages := func(second string) []ocr.Page {
		return []ocr.Page{page(1,
			"7:Date|22:Description|55:Withdrawal|71:Deposit|86:Balance",
			"22:Balance Brought Forward|90:500.00",
			"7:01/09/2026|22:Coffee shop|60:5.00|90:495.00",
			"7:02/09/2026|22:Refund|"+second+"|90:505.00",
			"22:Balance Carried Forward|90:505.00",
		)}
	}
	p := Parse(pages("75:10.00"))
	if r := Reconcile(p.Summary, p.Txns); !r.Checked || !r.OK || len(p.Txns) != 2 || !p.Txns[1].Credit || p.Txns[0].TxnDate.Year() != 2026 {
		t.Errorf("balances: %+v %+v", r, p.Txns)
	}
	p = Parse(pages("75:11.00")) // misread: 495 + 11 ≠ 505
	if r := Reconcile(p.Summary, p.Txns); r.OK {
		t.Errorf("a broken balance chain must not reconcile: %+v", r)
	}
}

// Small print mentioning debit and credit balances is not a ledger header.
func TestLedgerNotFromProse(t *testing.T) {
	if isLedger(citiPages()) {
		t.Error("Citibank card statement taken for a bank ledger")
	}
	prose := []ocr.Page{page(1, "0:Note: The Current Balance is the sum of Debit Balances only. All Credit Balances are excluded from the date of this statement.")}
	if isLedger(prose) {
		t.Error("prose taken for a ledger header")
	}
}

// A Trust-style savings statement: one date, "+" for money in, a foreign
// amount column, REFUND under a row, and a balance table.
func trustPages() []ocr.Page {
	return []ocr.Page{
		page(1,
			"67:Trust Bank Singapore Limited",
			"8:TAN AH KOW|50:Account Statement|77:01-2345678-9",
			"50:Statement period|73:1 Oct 2026 - 31 Oct 2026",
			"8:Savings Account by Trust",
			"9:Savings Account|23:Opening balance|35:+|38:Total incoming|49:-|51:Total outgoing|62:+|67:Interest*|76:=|78:Closing balance",
			"9:Total|26:S$100.00|39:S$510.00|53:S$155.00|67:S$0.01|80:S$455.01",
		),
		page(2,
			"8:Posting date|24:Description|61:Amount in FCY|79:Amount in SGD",
			"8:01 Oct|24:Previous balance|86:100.00",
			"8:02 Oct|24:Prudential Insurance|86:50.00",
			"8:04 Oct|24:PINDUODUONEW ShangHai CN|61:391.70 CNY|87:75.00",
			"25:1 CNY = 0.1913 SGD TRUST FX CHARGE = 0%",
			"8:05 Oct|24:PINDUODUONEW ShangHai CN|61:52.30 CNY|87:+10.00",
			"25:1 CNY = 0.1912 SGD TRUST FX CHARGE = 0% REFUND",
			"8:06 Oct|24:TAX AH KOX|85:30.00",
			"8:29 Oct|24:TAN AH KOW|84:+500.00",
			"8:31 Oct|24:Interest|87:+0.01",
			"8:31 Oct|24:Closing balance|86:455.01",
		),
	}
}

func TestParseTrust(t *testing.T) {
	p := Parse(trustPages())
	s := p.Summary
	if s.Kind != "bank" || s.Bank != "Trust" || s.Product != "Savings Account by Trust" || s.Last4 != "6789" || s.Holder != "TAN AH KOW" {
		t.Errorf("header: %+v", s)
	}
	type row struct {
		amount      int64
		credit      bool
		kind, party string
		fx          string
	}
	want := []row{
		{5000, false, "", "Prudential Insurance", ""},
		{7500, false, "", "PINDUODUONEW ShangHai CN", "391.70 CNY"},
		{1000, true, "refund", "PINDUODUONEW ShangHai CN", "52.30 CNY"},
		{3000, false, "transfer", "Own account", ""}, // the holder's name, masked
		{50000, true, "transfer", "Own account", ""},
		{1, true, "income", "Interest", ""},
	}
	if len(p.Txns) != len(want) {
		for _, tx := range p.Txns {
			t.Logf("%+v", tx)
		}
		t.Fatalf("got %d rows", len(p.Txns))
	}
	for i, w := range want {
		g := p.Txns[i]
		if g.Amount != w.amount || g.Credit != w.credit || g.Kind != w.kind || g.Counterparty != w.party || g.Foreign != w.fx {
			t.Errorf("row %d = %d cr=%v kind=%q party=%q fx=%q", i, g.Amount, g.Credit, g.Kind, g.Counterparty, g.Foreign)
		}
	}
	if r := Reconcile(s, p.Txns); !r.OK || !r.SummaryOK {
		t.Errorf("reconcile %+v (prev %v in %v out %v closing %v)", r, deref(s.PrevBalance), deref(s.Credits), deref(s.Debits), deref(s.Total))
	}
}

// A plain table someone made: date, name, amount, in common date formats,
// with the header repeated part-way down.
func TestParseSimpleTable(t *testing.T) {
	p := Parse([]ocr.Page{page(1,
		"10:Expenses",
		"12:Date|27:Name|42:Amount",
		"12:03/11/2026|27:Kopitiam|42:12.50",
		"12:2026-11-05|27:NTUC FairPrice|42:S$45.80",
		"12:7 Nov 2026|27:Grab ride|42:$$18.20",
		"12:Date|27:Name|42:Amount",
		"12:12-Nov-26|27:Uniqlo|42:59.90",
		"12:Nov 15, 2026|27:Starbucks|42:7.40",
		"12:20/11/2026|27:Refund Uniqlo|42:-59.90",
	)})
	want := []struct {
		date, desc string
		amount     int64
		credit     bool
	}{
		{"2026-11-03", "Kopitiam", 1250, false},
		{"2026-11-05", "NTUC FairPrice", 4580, false},
		{"2026-11-07", "Grab ride", 1820, false},
		{"2026-11-12", "Uniqlo", 5990, false},
		{"2026-11-15", "Starbucks", 740, false},
		{"2026-11-20", "Refund Uniqlo", 5990, true},
	}
	if len(p.Txns) != len(want) {
		for _, tx := range p.Txns {
			t.Logf("%s %q %d", tx.TxnDate.Format("2006-01-02"), tx.Description, tx.Amount)
		}
		t.Fatalf("got %d rows", len(p.Txns))
	}
	for i, w := range want {
		g := p.Txns[i]
		if g.TxnDate.Format("2006-01-02") != w.date || g.Description != w.desc || g.Amount != w.amount || g.Credit != w.credit {
			t.Errorf("row %d = %s %q %d %v", i, g.TxnDate.Format("2006-01-02"), g.Description, g.Amount, g.Credit)
		}
	}
}

func TestNumericDates(t *testing.T) {
	for in, want := range map[string]string{
		"31/10/2026": "2026-10-31", "31-10-26": "2026-10-31", "2026-10-31": "2026-10-31",
		"10/31/2026":  "2026-10-31", // only fits month first
		"31-Oct-2026": "2026-10-31", "03.11.2026": "2026-11-03",
	} {
		dm, ok := numericDate(in)
		got := time.Date(dm.year, dm.month, dm.day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		if !ok || got != want {
			t.Errorf("%s → %s %v", in, got, ok)
		}
	}
	for _, in := range []string{"12.50", "1,234.00", "99/99/2026", "2026"} {
		if _, ok := numericDate(in); ok {
			t.Errorf("%s read as a date", in)
		}
	}
}

func TestSameName(t *testing.T) {
	for name, want := range map[string]bool{
		"TAN AH KOW": true, "tan ah kow": true, "AH KOW": true, "TAX AH KOX": true, // masked by the bank
		"TAN AH KOK": false, "LOX HOXX TAX": false, "TAN": false, "AH KOW TAN LEE": false,
	} {
		if got := sameName(name, "TAN AH KOW"); got != want {
			t.Errorf("sameName(%q) = %v", name, got)
		}
	}
}

// A DBS-style consolidated statement (synthetic): DD/MM/YYYY dates, "TO:" /
// "FROM:" lines, people's names masked, Balance Brought/Carried Forward on
// every page and a "Total Balance Carried Forward" row with the totals.
func dbsPages() []ocr.Page {
	return []ocr.Page{
		page(1,
			"12:S/N: EN00000000000001|67:Consolidated Statement",
			"12:TAN AH MEI",
			"12:BLK 1 EXAMPLE AVENUE 1",
			"5:Account Summary as at 31 Oct 2026",
			"7:Current and Savings Account|72:Total: SGD Equivalent 40.00",
			"7:Account|45:Account No.|74:Balance|86:Balance",
			"7:DBS Savings Plus Account|45:123-4-567890|66:SGD|76:40.00|88:40.00",
		),
		page(2,
			"7:DBS Savings Plus Account|74:Account No. 123-4-567890",
			"7:Date|19:Description|56:Withdrawal|65:(-)|72:Deposit|78:(+)|82:Balance|89:(SGD)",
			"19:Balance Brought Forward|88:100.00",
			"7:01/10/2026|19:FAST Payment / Receipt|63:6.00|88:94.00",
			"19:PAYNOW TRANSFER 8911289",
			"19:TO: YX GUANGXXXX",
			"19:PAYNOW TRANSFER NOODLES",
			"19:OTHER",
			"7:02/10/2026|19:FAST Payment / Receipt|61:50.00|88:44.00",
			"19:PAYNOW TRANSFER 8002197",
			"19:TO: LOX HOXX SHEXX",
			"19:HL WEDDING ANGPAO",
			"19:OTHER",
			"7:03/10/2026|19:Point-of-Sale Transaction|63:4.00|88:40.00",
			"19:NETS QR PAYMENT 624718052561777",
			"19:TO: BROTHERS CHIN 703",
			"19:Balance Carried Forward|88:40.00",
		),
		page(3,
			"7:Date|19:Description|56:Withdrawal|65:(-)|72:Deposit|78:(+)|82:Balance|89:(SGD)",
			"19:Balance Brought Forward|88:40.00",
			"7:04/10/2026|19:FAST Payment / Receipt|75:2,000.00|88:2,040.00",
			"19:SYFE",
			"19:20261004SCBLSG22BRT0136927",
			"19:INVESTMENT & SECURITIES",
			"7:05/10/2026|19:FAST Collection|59:2,000.00|88:40.00",
			"19:01A109A3DBAC7310A87511929DD54AA9",
			"19:COLLECTION PAYMENT",
			"7:06/10/2026|19:FAST Payment / Receipt|76:20.00|88:60.00",
			"19:INCOMING PAYNOW REF 6561505",
			"19:FROM: ONG KWEE LIAN",
			"19:WINNIE ONG LUNCH",
			"19:OTHER",
			"7:07/10/2026|19:Debit Card Transaction|62:10.00|88:50.00",
			"19:GRAB* AB2734D809BAFED3 SI SGP 28SEP",
			"19:4628-4500-1958-5794",
			"7:13/09/2026|19:FAST Payment / Receipt|75:119.50|88:169.50",
			"19:OTHER",
			"19:20260913OCBCSGSGBRT8904706",
			"19:OTHER",
			"7:31/10/2026|19:Interest Earned|76:0.03|88:169.53",
			"5:4|19:4|54:4|67:4|81:4",
			"19:Total Balance Carried Forward:|59:2,070.00|73:2,139.53|88:169.53",
		),
	}
}

func TestParseDBS(t *testing.T) {
	p := Parse(dbsPages())
	s := p.Summary
	if s.Kind != "bank" || s.Bank != "DBS" || s.Product != "DBS Savings Plus Account" || s.Last4 != "7890" || s.Holder != "TAN AH MEI" {
		t.Errorf("header: kind=%q bank=%q product=%q last4=%q holder=%q", s.Kind, s.Bank, s.Product, s.Last4, s.Holder)
	}
	if s.PeriodStart.Format("2006-01-02") != "2026-10-01" || s.PeriodEnd.Format("2006-01-02") != "2026-10-31" {
		t.Errorf("period %v %v", s.PeriodStart, s.PeriodEnd)
	}
	if deref(s.PrevBalance) != 100.0 || deref(s.Total) != 169.53 || deref(s.Debits) != 2070.0 || deref(s.Credits) != 2139.53 {
		t.Errorf("summary prev=%v total=%v out=%v in=%v", deref(s.PrevBalance), deref(s.Total), deref(s.Debits), deref(s.Credits))
	}
	type row struct {
		amount      int64
		credit      bool
		kind, party string
		check       bool
	}
	want := []row{
		{600, false, "", "Yx Guangxxxx", true},             // a person, ≤ S$30: spending to check
		{5000, false, "transfer", "Lox Hoxx Shexx", false}, // a person, > S$30: transfer
		{400, false, "", "BROTHERS CHIN 703", false},       // NETS: a shop
		{200000, true, "transfer", "Syfe", false},          // from investments
		{200000, false, "transfer", "Direct debit", true},  // a direct debit
		{2000, true, "transfer", "Ong Kwee Lian", false},   // from a person
		{1000, false, "", "GRAB* AB2734D809BAFED3 SI SGP 28SEP", false},
		{11950, true, "transfer", "Money in from OCBC", true}, // says little: never a refund
		{3, true, "income", "Interest", false},
	}
	if len(p.Txns) != len(want) {
		for _, tx := range p.Txns {
			t.Logf("%+v", tx)
		}
		t.Fatalf("got %d rows", len(p.Txns))
	}
	for i, w := range want {
		g := p.Txns[i]
		if g.Amount != w.amount || g.Credit != w.credit || g.Kind != w.kind || g.Counterparty != w.party || g.Check != w.check {
			t.Errorf("row %d = %d cr=%v kind=%q party=%q check=%v note=%q", i, g.Amount, g.Credit, g.Kind, g.Counterparty, g.Check, g.Note)
		}
	}
	if !strings.HasPrefix(p.Txns[0].Note, "Noodles") || !strings.Contains(p.Txns[1].Note, "Hl Wedding Angpao") {
		t.Errorf("messages: %q / %q", p.Txns[0].Note, p.Txns[1].Note)
	}
	if r := Reconcile(s, p.Txns); !r.OK || !r.SummaryOK {
		t.Errorf("reconcile %+v", r)
	}
}
