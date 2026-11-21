package insights

import (
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/store"
)

func tx(id int64, date, kind, cat, merchant string, cents int64) store.Txn {
	return store.Txn{ID: id, Date: date, Kind: kind, Category: cat, Merchant: merchant, MerchantKey: strings.ToLower(merchant), Raw: merchant, Amount: cents}
}

func sample() []store.Txn {
	return []store.Txn{
		tx(1, "2026-08-22", "purchase", "subscriptions", "Netflix", 1998),
		tx(2, "2026-09-22", "purchase", "subscriptions", "Netflix", 1998),
		tx(3, "2026-10-22", "purchase", "subscriptions", "Netflix", 1998),
		tx(4, "2026-08-10", "purchase", "delivery", "foodpanda", 4000),
		tx(5, "2026-09-10", "purchase", "delivery", "foodpanda", 2500),
		tx(6, "2026-08-15", "purchase", "dining", "Kopitiam", 30000),
		tx(7, "2026-09-15", "purchase", "dining", "Kopitiam", 21000),
		tx(8, "2026-10-15", "purchase", "dining", "Kopitiam", 35000),
		tx(9, "2026-10-01", "purchase", "delivery", "foodpanda", 2000),
		tx(10, "2026-10-02", "purchase", "delivery", "foodpanda", 2000),
		tx(11, "2026-10-03", "purchase", "delivery", "foodpanda", 2000),
		tx(12, "2026-10-04", "purchase", "delivery", "foodpanda", 2000),
		tx(13, "2026-10-05", "purchase", "delivery", "foodpanda", 2000),
		tx(14, "2026-10-06", "purchase", "delivery", "foodpanda", 2000),
		tx(15, "2026-10-07", "refund", "shopping", "Shopee", -5000),
		tx(16, "2026-10-08", "payment", "", "Card payment", -100000),
		tx(17, "2026-10-09", "fee", "fees", "DCC Fee", 378),
		tx(18, "2026-10-11", "purchase", "insurance", "Prudential", 183831),
		tx(19, "2026-10-12", "income", "", "Salary", -500000),
		tx(20, "2026-10-13", "purchase", "shopping", "Shopee", 9000),
	}
}

func TestBuild(t *testing.T) {
	sample := sample()
	sample[16].Raw = "DCC FEE"
	ov := Build(sample, "2026-10", time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), false)
	// Spend = 1998 + 35000 + 6×2000 − 5000 + 378 + 183831 + 9000 (payments and income excluded).
	if want := int64(1998 + 35000 + 12000 - 5000 + 378 + 183831 + 9000); ov.Spent != want {
		t.Errorf("spent %d, want %d", ov.Spent, want)
	}
	if ov.Income != 500000 || ov.Refunds != 5000 {
		t.Errorf("income %d refunds %d", ov.Income, ov.Refunds)
	}
	// Data ends 22 Oct and today is November: October may still be incomplete.
	if ov.AvgMonths != 2 || !ov.Partial {
		t.Errorf("avg months %d partial %v", ov.AvgMonths, ov.Partial)
	}
	if ov.Categories[0].ID != "insurance" {
		t.Errorf("top category %s", ov.Categories[0].ID)
	}
	var shop CategoryStat
	for _, c := range ov.Categories {
		if c.ID == "shopping" {
			shop = c
		}
	}
	if shop.Amount != 4000 || shop.Count != 1 {
		t.Errorf("shopping net of refund: %+v", shop)
	}
	if len(ov.Recurring) != 1 || ov.Recurring[0].Name != "Netflix" || ov.Recurring[0].Every != "month" {
		t.Errorf("recurring %+v", ov.Recurring)
	}
	ids := map[string]bool{}
	for _, i := range ov.Insights {
		ids[i.ID] = true
	}
	for _, want := range []string{"cat-delivery", "big-18", "freq-delivery", "dcc", "recurring"} {
		if !ids[want] {
			t.Errorf("missing insight %s (have %v)", want, ids)
		}
	}
	if len(ov.Trend) != 12 || ov.Trend[11].Month != "2026-10" || ov.Trend[11].ByGroup["food"] != 47000 {
		t.Errorf("trend %+v", ov.Trend[11])
	}
	if last := ov.Pace[len(ov.Pace)-1]; last.Value != ov.Spent {
		t.Errorf("pace end %d vs spent %d", last.Value, ov.Spent)
	}
}

func TestSpreadOneOff(t *testing.T) {
	s := sample()
	s[17].OneOff = true
	ov := Build(s, "2026-10", time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC), true)
	oct := ov.Trend[11]
	if oct.ByGroup["health"] > 20000 {
		t.Errorf("one-off not spread: %d", oct.ByGroup["health"])
	}
}

func TestEmpty(t *testing.T) {
	ov := Build(nil, "2026-11", time.Now(), false)
	if ov.Spent != 0 || len(ov.Categories) != 0 || ov.Insights == nil || ov.Months == nil {
		t.Errorf("%+v", ov)
	}
}
