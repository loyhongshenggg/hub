// Package importer runs the background jobs: reading uploaded statements
// (OCR → parse → reconcile → resolve merchants → store), refreshing the
// merchant directory, and optional Claude merchant lookups.
package importer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hub.localhost/apps/finance/api/internal/catalog"
	"hub.localhost/apps/finance/api/internal/claude"
	"hub.localhost/apps/finance/api/internal/merchant"
	"hub.localhost/apps/finance/api/internal/ocr"
	"hub.localhost/apps/finance/api/internal/statement"
	"hub.localhost/apps/finance/api/internal/store"
)

// Importer owns the background work.
type Importer struct {
	Store    *store.Store
	OCR      ocr.Engine
	Resolver *merchant.Resolver
	Claude   *claude.Service
	DataDir  string
	Log      *slog.Logger
	HTTP     *http.Client

	queue    chan int64
	mu       sync.Mutex
	queued   map[int64]bool
	dirBusy  map[string]bool
	lookupMu sync.Mutex
}

// New wires an importer.
func New(st *store.Store, eng ocr.Engine, r *merchant.Resolver, cl *claude.Service, dataDir string, log *slog.Logger) *Importer {
	return &Importer{Store: st, OCR: eng, Resolver: r, Claude: cl, DataDir: dataDir, Log: log,
		HTTP:  &http.Client{Timeout: 10 * time.Minute},
		queue: make(chan int64, 256), queued: map[int64]bool{}, dirBusy: map[string]bool{}}
}

// PDFPath is where an uploaded statement is kept.
func (im *Importer) PDFPath(sha string) string {
	return filepath.Join(im.DataDir, "statements", sha+".pdf")
}

// Run processes queued statements until ctx ends. Statements interrupted by
// a restart are queued again.
func (im *Importer) Run(ctx context.Context) {
	if ids, err := im.Store.PendingStatements(ctx); err == nil {
		for _, id := range ids {
			im.Enqueue(id)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-im.queue:
			im.process(ctx, id)
			im.mu.Lock()
			delete(im.queued, id)
			im.mu.Unlock()
		}
	}
}

// Enqueue schedules a statement for reading.
func (im *Importer) Enqueue(id int64) {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.queued[id] {
		return
	}
	im.queued[id] = true
	im.queue <- id
}

func (im *Importer) process(ctx context.Context, id int64) {
	st, err := im.Store.GetStatement(ctx, id)
	if err != nil {
		return
	}
	fail := func(msg string) {
		im.Log.Warn("statement failed", "id", id, "error", msg)
		_ = im.Store.FailStatement(ctx, id, msg)
	}
	if err := im.OCR.Available(); err != nil {
		fail(err.Error())
		return
	}
	_ = im.Store.SetStatementProgress(ctx, id, store.StReading, "Starting…")
	started := time.Now()
	res, err := statement.Read(ctx, im.OCR, im.PDFPath(st.SHA256), func(p string) {
		_ = im.Store.SetStatementProgress(ctx, id, store.StReading, p)
	})
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down; resumes at next start
		}
		fail("Couldn't read this PDF: " + err.Error())
		return
	}
	s := res.Summary
	inst := s.Bank
	if inst == "" {
		inst = "Card"
	}
	name := inst
	if s.Product != "" {
		name = titleProduct(s.Product, inst)
	}
	if s.Last4 != "" {
		name += " ··" + s.Last4
	}
	acct, err := im.Store.EnsureAccount(ctx, store.Account{Name: name, Institution: inst, Last4: s.Last4, Kind: "credit_card", Currency: s.Currency})
	if err != nil {
		fail(err.Error())
		return
	}
	txns := make([]store.Txn, 0, len(res.Txns))
	for _, t := range res.Txns {
		txns = append(txns, im.toTxn(t))
	}
	status := store.StDone
	if res.Recon.Checked && !res.Recon.OK {
		status = store.StReview
	}
	pages := 0
	for _, t := range res.Txns {
		pages = max(pages, t.Page)
	}
	date := func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	}
	n, err := im.Store.FinishStatement(ctx, id, store.StatementResult{
		AccountID: acct, Status: status, PeriodStart: date(s.PeriodStart), PeriodEnd: date(s.PeriodEnd),
		Summary: s, Recon: res.Recon, Warnings: res.Warnings, Passes: res.Passes, Pages: pages,
	}, txns)
	if err != nil {
		fail(err.Error())
		return
	}
	im.Log.Info("statement read", "id", id, "rows", n, "reconciled", res.Recon.OK, "passes", len(res.Passes), "took", time.Since(started).Round(time.Second))
	go im.LookupUnknown(context.WithoutCancel(ctx))
}

func titleProduct(product, bank string) string {
	words := strings.Fields(product)
	for i, w := range words {
		if strings.EqualFold(w, bank) {
			words[i] = bank
			continue
		}
		words[i] = merchant.Title(w)
	}
	out := strings.Join(words, " ")
	// "CITI PREMIERMILES …" already names Citibank; "VISA REVOLUTION" doesn't name HSBC.
	if len(words) > 0 && !strings.Contains(strings.ToUpper(out), strings.ToUpper(bank)) &&
		!strings.HasPrefix(strings.ToUpper(bank), strings.ToUpper(words[0])) {
		out = bank + " " + out
	}
	return out
}

// toTxn resolves a statement row into a stored transaction.
func (im *Importer) toTxn(t statement.Txn) store.Txn {
	d, m, kind := im.Resolver.Resolve(t.Description, t.Credit)
	amount := t.Amount
	if t.Credit {
		amount = -amount
	}
	review := false
	switch kind {
	case catalog.Purchase, catalog.Refund:
		review = m.Check || m.Confidence < merchant.AutoConfidence
	}
	note := ""
	if t.Foreign != "" {
		note = "Paid in " + strings.ToLower(t.Foreign)
	}
	return store.Txn{
		Date: t.TxnDate.Format("2006-01-02"), Posted: t.PostDate.Format("2006-01-02"), Raw: t.Description,
		Amount: amount, Kind: kind, Category: m.Category, Merchant: m.Name, MerchantKey: d.Key,
		MatchSource: m.Source, Confidence: m.Confidence, Detail: m.Detail, Note: note,
		NeedsReview: review || t.Recovered, Foreign: d.Foreign || t.Foreign != "", Page: t.Page, Y: t.Y, Recovered: t.Recovered,
	}
}

// Reread queues a statement to be read again (keeps your rules; row edits
// are replaced).
func (im *Importer) Reread(ctx context.Context, id int64) error {
	if err := im.Store.SetStatementProgress(ctx, id, store.StQueued, "Queued"); err != nil {
		return err
	}
	im.Enqueue(id)
	return nil
}

// --- merchant directory

// Sources that can be refreshed.
var Sources = []string{"osm", "acra"}

// LoadDirectory builds the in-memory index from the database.
func (im *Importer) LoadDirectory(ctx context.Context) error {
	entries, err := im.Store.LoadDirectory(ctx)
	if err != nil {
		return err
	}
	im.Resolver.SetDirectory(merchant.NewDirectory(entries))
	return nil
}

// RefreshDirectory downloads one source in the background.
func (im *Importer) RefreshDirectory(source string) error {
	if source != "osm" && source != "acra" {
		return fmt.Errorf("unknown source %q", source)
	}
	im.mu.Lock()
	if im.dirBusy[source] {
		im.mu.Unlock()
		return errors.New("already refreshing")
	}
	im.dirBusy[source] = true
	im.mu.Unlock()
	go func() {
		defer func() {
			im.mu.Lock()
			delete(im.dirBusy, source)
			im.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = im.Store.SetDirectoryStatus(ctx, source, "running", "")
		var entries []merchant.Entry
		var err error
		switch source {
		case "osm":
			entries, err = merchant.FetchOSM(ctx, im.HTTP)
		case "acra":
			entries, err = merchant.FetchACRA(ctx, im.HTTP, func(done, total int) {
				_ = im.Store.SetDirectoryStatus(ctx, source, "running", fmt.Sprintf("%d of %d files", done, total))
			})
		}
		if err == nil && len(entries) == 0 {
			err = errors.New("no entries returned")
		}
		if err == nil {
			err = im.Store.ReplaceDirectory(ctx, source, entries)
		}
		if err != nil {
			im.Log.Warn("directory refresh failed", "source", source, "error", err)
			_ = im.Store.SetDirectoryStatus(ctx, source, "failed", err.Error())
			return
		}
		if err := im.LoadDirectory(ctx); err != nil {
			im.Log.Warn("directory load", "error", err)
			return
		}
		n, _ := im.Rematch(ctx)
		im.Log.Info("directory refreshed", "source", source, "entries", len(entries), "rematched", n)
	}()
	return nil
}

// Rematch re-resolves automatically matched merchants (after a directory
// refresh). Your edits and rules are never touched.
func (im *Importer) Rematch(ctx context.Context) (int, error) {
	keys, err := im.Store.AutoMatched(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for key, raw := range keys {
		_, m, _ := im.Resolver.Resolve(raw, false)
		if m.Source == "rule" || m.Source == "special" {
			continue
		}
		review := m.Check || m.Confidence < merchant.AutoConfidence
		c, err := im.Store.SetTxnMatch(ctx, key, m.Name, m.Category, m.Source, m.Detail, m.Confidence, review)
		if err != nil {
			return n, err
		}
		n += int(c)
	}
	return n, nil
}

// LookupUnknown asks Claude about merchants still unknown (when switched on).
// Answers are cached per descriptor key.
func (im *Importer) LookupUnknown(ctx context.Context) (int, error) {
	if !im.lookupMu.TryLock() {
		return 0, nil
	}
	defer im.lookupMu.Unlock()
	cfg, err := im.Claude.Settings(ctx)
	if err != nil || !cfg.Merchants {
		return 0, err
	}
	if key, _ := im.Claude.Key(cfg); key == "" {
		return 0, nil
	}
	keys, err := im.Store.UnresolvedKeys(ctx, 25)
	if err != nil {
		return 0, err
	}
	n := 0
	for key, raw := range keys {
		var a claude.MerchantAnswer
		found, err := im.Store.Lookup(ctx, "claude|"+key, &a)
		if err != nil {
			return n, err
		}
		if !found {
			ans, err := im.Claude.LookupMerchant(ctx, merchant.Clean(raw).Raw)
			if errors.Is(err, claude.ErrCap) || errors.Is(err, claude.ErrOff) || errors.Is(err, claude.ErrNoKey) {
				return n, err
			}
			if err != nil {
				im.Log.Warn("claude merchant lookup", "error", err)
				continue
			}
			a = *ans
			_ = im.Store.SaveLookup(ctx, "claude|"+key, a)
		}
		if a.Name == "" || a.Confidence < 0.5 {
			continue
		}
		c, err := im.Store.SetTxnMatch(ctx, key, a.Name, a.Category, "claude", a.Evidence, a.Confidence, a.Confidence < merchant.AutoConfidence)
		if err != nil {
			return n, err
		}
		n += int(c)
	}
	return n, nil
}

// SaveUpload stores an uploaded PDF under its hash.
func (im *Importer) SaveUpload(sha string, data []byte) error {
	dir := filepath.Join(im.DataDir, "statements")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(im.PDFPath(sha), data, 0o600)
}
