package statement

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// TestRealStatements reads real statement PDFs when FINANCE_TEST_PDFS points
// at a folder of them (never committed: they hold personal data). It needs
// pdftoppm and tesseract on PATH.
func TestRealStatements(t *testing.T) {
	dir := os.Getenv("FINANCE_TEST_PDFS")
	if dir == "" {
		t.Skip("set FINANCE_TEST_PDFS to a folder of statement PDFs")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.pdf"))
	eng := &ocr.Tesseract{}
	if err := eng.Available(); err != nil {
		t.Skip(err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			res, err := Read(context.Background(), eng, f, func(s string) { t.Log(s) })
			if err != nil {
				t.Fatal(err)
			}
			s := res.Summary
			t.Logf("%s %s ··%s %s→%s due %s passes %v", s.Bank, s.Product, s.Last4,
				s.PeriodStart.Format("2006-01-02"), s.PeriodEnd.Format("2006-01-02"), s.DueDate.Format("2006-01-02"), res.Passes)
			t.Logf("summary prev=%v credits=%v debits=%v total=%v", deref(s.PrevBalance), deref(s.Credits), deref(s.Debits), deref(s.Total))
			for _, tx := range res.Txns {
				cr := ""
				if tx.Credit {
					cr = "CR"
				}
				rec := ""
				if tx.Recovered {
					rec = " (recovered)"
				}
				t.Logf("  %s %s %-45q %8.2f%s%s", tx.PostDate.Format("01-02"), tx.TxnDate.Format("01-02"), tx.Description, float64(tx.Amount)/100, cr, rec)
			}
			t.Logf("recon %+v warnings %v", res.Recon, res.Warnings)
			if !res.Recon.OK {
				t.Errorf("does not reconcile: %+v", res.Recon)
			}
		})
	}
}

func deref(p *int64) any {
	if p == nil {
		return nil
	}
	return float64(*p) / 100
}
