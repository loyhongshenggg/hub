package importer

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/merchant"
	"hub.localhost/apps/finance/api/internal/statement"
	"hub.localhost/apps/finance/api/internal/store"
)

func newImporter(t *testing.T) *Importer {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	brands, err := merchant.LoadBrands()
	if err != nil {
		t.Fatal(err)
	}
	return New(st, nil, &merchant.Resolver{Brands: brands}, nil, t.TempDir(), slog.Default())
}

// Bank-account rows are matched on their payee and typed by what the bank
// printed; "match again" must not re-read their raw line.
func TestBankRowsKeepTheirMatch(t *testing.T) {
	im := newImporter(t)
	ctx := context.Background()
	day := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	rows := []statement.Txn{
		{TxnDate: day, PostDate: day, Description: "FUND TRANSFER via PayNow-QR Code to PINDUODUONEW ShangHai CN OTHR - OTHR", Amount: 2000,
			Counterparty: "PINDUODUONEW ShangHai CN"},
		{TxnDate: day, PostDate: day, Description: "FAST PAYMENT 0193491628 to TAN AH KOW OTHR-Other", Amount: 30000,
			Kind: "transfer", Counterparty: "Own account", Note: "Between your own accounts"},
		{TxnDate: day, PostDate: day, Description: "NETS QR 88968901 NETS QR PURCHASE AXS PTE LTD", Amount: 9400,
			Kind: "payment", Counterparty: "AXS", Note: "Paid at AXS", Check: true},
	}
	var txns []store.Txn
	for _, r := range rows {
		txns = append(txns, im.toTxn(r))
	}
	if g := txns[0]; g.Merchant != "Pinduoduo" || g.Category != "shopping" || g.Kind != "purchase" || g.Raw != rows[0].Description {
		t.Errorf("payee match: %+v", g)
	}
	if g := txns[1]; g.Kind != "transfer" || g.Merchant != "Own account" || g.NeedsReview || g.Note != "Between your own accounts" {
		t.Errorf("own transfer: %+v", g)
	}
	if g := txns[2]; g.Kind != "payment" || !g.NeedsReview {
		t.Errorf("AXS: %+v", g)
	}

	id, _, err := im.Store.CreateStatement(ctx, "bank.pdf", "abc")
	if err != nil {
		t.Fatal(err)
	}
	acct, _ := im.Store.EnsureAccount(ctx, store.Account{Name: "Bank ··1234", Institution: "Bank", Last4: "1234", Kind: "bank"})
	if _, err := im.Store.FinishStatement(ctx, id, store.StatementResult{AccountID: acct, Status: store.StDone}, txns); err != nil {
		t.Fatal(err)
	}
	if _, err := im.Rematch(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := im.Store.Txns(ctx, store.TxnFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range got {
		if g.Amount == 2000 && (g.Merchant != "Pinduoduo" || g.Category != "shopping") {
			t.Errorf("match again replaced the payee match: %+v", g)
		}
	}
}
