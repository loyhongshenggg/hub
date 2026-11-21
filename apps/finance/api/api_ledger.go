package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/insights"
	"hub.localhost/apps/finance/api/internal/merchant"
	"hub.localhost/apps/finance/api/internal/store"
)

var reMonth = regexp.MustCompile(`^\d{4}-\d{2}$`)
var reDate = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func (a *api) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := a.store.Txns(ctx, store.TxnFilter{})
	if err != nil {
		fail(w, err)
		return
	}
	m := r.URL.Query().Get("month")
	if m == "" {
		m = defaultMonth(all, a.now())
	}
	if !reMonth.MatchString(m) {
		writeError(w, 400, "month must be YYYY-MM")
		return
	}
	ov := insights.Build(all, m, a.now(), r.URL.Query().Get("spread") == "1")
	writeJSON(w, 200, ov)
}

// defaultMonth is the latest month with data, or the one before it when the
// latest has under 10 days of data (statements arrive a month at a time).
func defaultMonth(all []store.Txn, now time.Time) string {
	if len(all) == 0 {
		return now.Format("2006-01")
	}
	last := all[0].Date // newest first
	m := last[:7]
	if day, _ := strconv.Atoi(last[8:10]); day < 10 {
		prev := m
		if t, err := time.Parse("2006-01", m); err == nil {
			prev = t.AddDate(0, -1, 0).Format("2006-01")
		}
		for _, t := range all {
			if t.Date[:7] == prev {
				return prev
			}
		}
	}
	return m
}

func (a *api) listTxns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.TxnFilter{From: q.Get("from"), To: q.Get("to"), Category: q.Get("category"), Kind: q.Get("kind"),
		MerchantKey: q.Get("merchantKey"), Merchant: q.Get("merchant"), Query: q.Get("q"), Review: q.Get("review") == "1", Split: q.Get("split") == "1",
		Spending: q.Get("spending") == "1"}
	if m := q.Get("month"); reMonth.MatchString(m) {
		f.From, f.To = m+"-01", m+"-31"
	}
	if v := q.Get("statement"); v != "" {
		f.StatementID, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("limit"); v != "" {
		f.Limit, _ = strconv.Atoi(v)
	}
	txns, err := a.store.Txns(r.Context(), f)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"transactions": txns})
}

func (a *api) addTxn(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Date      string `json:"date"`
		Amount    int64  `json:"amount"` // cents, positive
		Kind      string `json:"kind"`
		Category  string `json:"category"`
		Merchant  string `json:"merchant"`
		Note      string `json:"note"`
		AccountID *int64 `json:"accountId"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if !reDate.MatchString(in.Date) || in.Amount <= 0 || !catalog.ValidKind(in.Kind) || in.Merchant == "" {
		writeError(w, 400, "date (YYYY-MM-DD), a positive amount, kind and merchant are required")
		return
	}
	if in.Category != "" && !catalog.Valid(in.Category) {
		writeError(w, 400, "unknown category")
		return
	}
	amount := in.Amount
	if in.Kind == catalog.Refund || in.Kind == catalog.Income || in.Kind == catalog.Payment {
		amount = -amount
	}
	if catalog.Spending(in.Kind) && in.Category == "" {
		in.Category = "other"
	}
	id, err := a.store.AddTxn(r.Context(), store.Txn{Date: in.Date, Posted: in.Date, Raw: in.Merchant, Amount: amount, Kind: in.Kind,
		Category: in.Category, Merchant: in.Merchant, MerchantKey: merchant.Clean(in.Merchant).Key, MatchSource: "user",
		Confidence: 1, Note: in.Note, AccountID: in.AccountID})
	if err != nil {
		fail(w, err)
		return
	}
	t, _ := a.store.GetTxn(r.Context(), id)
	writeJSON(w, 201, t)
}

func (a *api) patchTxn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	var in struct {
		store.TxnPatch
		// ApplyToSimilar saves the merchant/category as a rule for every
		// transaction with the same descriptor (now and in future imports).
		ApplyToSimilar bool `json:"applyToSimilar"`
	}
	if err := decode(r, &in); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	p := in.TxnPatch
	if p.Category != nil && *p.Category != "" && !catalog.Valid(*p.Category) {
		writeError(w, 400, "unknown category")
		return
	}
	if p.Kind != nil && !catalog.ValidKind(*p.Kind) {
		writeError(w, 400, "unknown kind")
		return
	}
	if p.Date != nil && !reDate.MatchString(*p.Date) {
		writeError(w, 400, "date must be YYYY-MM-DD")
		return
	}
	ctx := r.Context()
	t, err := a.store.GetTxn(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if p.MyCost != nil && !p.ClearMyCost {
		// Typed as a positive amount; stored with the transaction's sign.
		v := abs(*p.MyCost)
		total := abs(t.Amount)
		if p.Amount != nil {
			total = abs(*p.Amount)
		}
		if v > total {
			writeError(w, 400, "your cost can't be more than the amount charged")
			return
		}
		if t.Others != 0 {
			writeError(w, 400, "this cost is split with others — edit or remove the split instead")
			return
		}
		if t.Amount < 0 {
			v = -v
		}
		p.MyCost = &v
	}
	if p.Kind != nil && *p.Kind != t.Kind && p.Amount == nil {
		// Keep the sign consistent with the kind.
		amt := abs(t.Amount)
		if *p.Kind == catalog.Refund || *p.Kind == catalog.Income || *p.Kind == catalog.Payment {
			amt = -amt
		}
		p.Amount = &amt
	}
	if err := a.store.UpdateTxn(ctx, id, p); err != nil {
		fail(w, err)
		return
	}
	applied := int64(0)
	if in.ApplyToSimilar && t.MerchantKey != "" && (p.Merchant != nil || p.Category != nil || p.Kind != nil) {
		t2, _ := a.store.GetTxn(ctx, id)
		rule := store.Rule{Key: t2.MerchantKey, Merchant: t2.Merchant, Category: t2.Category}
		if p.Kind != nil && (*p.Kind == catalog.Transfer || *p.Kind == catalog.Income) {
			rule.Kind = *p.Kind
		}
		if applied, err = a.store.SetRule(ctx, rule); err != nil {
			fail(w, err)
			return
		}
	}
	t, _ = a.store.GetTxn(ctx, id)
	writeJSON(w, 200, map[string]any{"transaction": t, "applied": applied})
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func (a *api) deleteTxn(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteTxn(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *api) listRules(w http.ResponseWriter, r *http.Request) {
	rules, err := a.store.Rules(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"rules": rules})
}

func (a *api) deleteRule(w http.ResponseWriter, r *http.Request) {
	if err := a.store.DeleteRule(r.Context(), r.PathValue("key")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *api) exportCSV(w http.ResponseWriter, r *http.Request) {
	txns, err := a.store.Txns(r.Context(), store.TxnFilter{})
	if err != nil {
		fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="finance-%s.csv"`, a.now().Format("2006-01-02")))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"date", "posted", "merchant", "category", "kind", "amount_sgd", "my_share_sgd", "description", "note", "excluded", "one_off"})
	for i := len(txns) - 1; i >= 0; i-- {
		t := txns[i]
		_ = cw.Write([]string{t.Date, t.Posted, t.Merchant, t.Category, t.Kind, fmt.Sprintf("%.2f", float64(t.Amount)/100), fmt.Sprintf("%.2f", float64(t.Mine())/100),
			t.Raw, t.Note, strconv.FormatBool(t.Excluded), strconv.FormatBool(t.OneOff)})
	}
	cw.Flush()
}

// --- statements

func (a *api) listStatements(w http.ResponseWriter, r *http.Request) {
	sts, err := a.store.Statements(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"statements": sts})
}

const maxPDF = 25 << 20

func (a *api) upload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 10*maxPDF)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, 400, "expected a multipart upload of PDF files (max 25 MB each)")
		return
	}
	type result struct {
		ID        int64  `json:"id"`
		FileName  string `json:"fileName"`
		Duplicate bool   `json:"duplicate"`
		Error     string `json:"error,omitempty"`
	}
	var out []result
	for _, fh := range r.MultipartForm.File["files"] {
		name := filepath.Base(fh.Filename)
		res := result{FileName: name}
		f, err := fh.Open()
		if err != nil {
			res.Error = err.Error()
			out = append(out, res)
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, maxPDF+1))
		f.Close()
		switch {
		case err != nil:
			res.Error = err.Error()
		case len(data) > maxPDF:
			res.Error = "file is larger than 25 MB"
		case !bytes.HasPrefix(data, []byte("%PDF-")):
			res.Error = "not a PDF"
		}
		if res.Error != "" {
			out = append(out, res)
			continue
		}
		sum := sha256.Sum256(data)
		sha := hex.EncodeToString(sum[:])
		id, exists, err := a.store.CreateStatement(r.Context(), name, sha)
		if err != nil {
			res.Error = err.Error()
			out = append(out, res)
			continue
		}
		res.ID, res.Duplicate = id, exists
		if !exists {
			if err := a.imp.SaveUpload(sha, data); err != nil {
				_ = a.store.DeleteStatement(r.Context(), id)
				res.Error = err.Error()
				out = append(out, res)
				continue
			}
			a.imp.Enqueue(id)
		}
		out = append(out, res)
	}
	if len(out) == 0 {
		writeError(w, 400, "no files")
		return
	}
	writeJSON(w, 201, map[string]any{"uploads": out})
}

func (a *api) getStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	st, err := a.store.GetStatement(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	txns, err := a.store.Txns(r.Context(), store.TxnFilter{StatementID: id})
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"statement": st, "transactions": txns})
}

func (a *api) deleteStatement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if err := a.store.DeleteStatement(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *api) reread(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeError(w, 400, "bad id")
		return
	}
	if _, err := a.store.GetStatement(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	if err := a.imp.Reread(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 202, map[string]string{"status": "queued"})
}

// pageRenderer is implemented by the OCR engine (poppler).
type pageRenderer interface {
	RenderPage(ctx context.Context, pdf string, page, dpi int) ([]byte, error)
}

// pageImage renders a statement page for the review screen.
func (a *api) pageImage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	page, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("page"), ".png"))
	if !ok || err != nil || page < 1 || page > 200 {
		writeError(w, 400, "bad id or page")
		return
	}
	st, err := a.store.GetStatement(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	rp, ok := a.ocr.(pageRenderer)
	if !ok {
		writeError(w, 501, "page rendering unavailable")
		return
	}
	img, err := rp.RenderPage(r.Context(), a.imp.PDFPath(st.SHA256), page, 110)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(img)
}
