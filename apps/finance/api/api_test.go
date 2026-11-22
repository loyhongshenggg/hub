package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/finance/api/internal/claude"
	"hub.localhost/apps/finance/api/internal/importer"
	"hub.localhost/apps/finance/api/internal/merchant"
	"hub.localhost/apps/finance/api/internal/ocr"
	"hub.localhost/apps/finance/api/internal/store"
)

// fakeOCR returns one prepared statement page.
type fakeOCR struct{}

func (fakeOCR) Available() error { return nil }
func (fakeOCR) Read(context.Context, string, ocr.Options) ([]ocr.Page, error) {
	// "text|amount": amounts sit in a right-aligned column, like a statement.
	lines := []string{
		"HSBC VISA REVOLUTION",
		"From 01 OCT 2026 to 31 OCT 2026",
		"1234-XXXX-XXXX-9999",
		"DATE DATE DESCRIPTION|AMOUNT(SGD)",
		"Previous Statement Balance|0.00",
		"Payments & Credits|5.00CR",
		"Purchases & Debits|37.40",
		"Total Account Balance|32.40",
		"02 Oct 01 Oct fp*Food Panda Singapore SG|12.40",
		"03 Oct 02 Oct SMP*MYSTERY KITCHEN SINGAPORE SG|25.00",
		"04 Oct 03 Oct SHOPEE SG MP SINGAPORE SG|5.00CR",
	}
	p := ocr.Page{Number: 1, Width: 2000, Height: 2000}
	for i, l := range lines {
		parts := strings.SplitN(l, "|", 2)
		x := 0
		for _, w := range strings.Fields(parts[0]) {
			p.Words = append(p.Words, ocr.Word{Text: w, X0: x, X1: x + 10*len(w), Y0: 40 * (i + 1), Y1: 40*(i+1) + 20, Conf: 90})
			x += 10 * (len(w) + 1)
		}
		if len(parts) == 2 {
			p.Words = append(p.Words, ocr.Word{Text: parts[1], X0: 1100, X1: 1100 + 10*len(parts[1]), Y0: 40 * (i + 1), Y1: 40*(i+1) + 20, Conf: 90})
		}
	}
	return []ocr.Page{p}, nil
}

func setup(t *testing.T) (*httptest.Server, *api) {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st, err := store.Open(ctx, filepath.Join(dir, "f.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	brands, err := merchant.LoadBrands()
	if err != nil {
		t.Fatal(err)
	}
	r := &merchant.Resolver{Brands: brands}
	r.Rule = func(key string) (merchant.Match, bool) {
		rule, ok, _ := st.GetRule(context.Background(), key)
		return merchant.Match{Name: rule.Merchant, Category: rule.Category, Kind: rule.Kind}, ok
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cl := claude.New(st)
	imp := importer.New(st, fakeOCR{}, r, cl, dir, log)
	go imp.Run(ctx)
	a := &api{store: st, imp: imp, claude: cl, ocr: fakeOCR{}, now: func() time.Time { return time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC) }}
	mux := http.NewServeMux()
	a.routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, a
}

func call(t *testing.T, srv *httptest.Server, method, path string, body any, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, srv.URL+path, rd)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func TestImportFlow(t *testing.T) {
	srv, _ := setup(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("files", "oct.pdf")
	fw.Write([]byte("%PDF-1.7 fake"))
	fw, _ = mw.CreateFormFile("files", "notes.txt")
	fw.Write([]byte("hello"))
	mw.Close()
	resp, err := http.Post(srv.URL+"/api/statements", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	var up struct {
		Uploads []struct {
			ID    int64
			Error string
		}
	}
	json.NewDecoder(resp.Body).Decode(&up)
	resp.Body.Close()
	if len(up.Uploads) != 2 || up.Uploads[0].ID == 0 || up.Uploads[1].Error != "not a PDF" {
		t.Fatalf("uploads %+v", up.Uploads)
	}
	// Wait for the background read.
	var st struct {
		Statement    store.Statement
		Transactions []store.Txn
	}
	for i := 0; i < 50; i++ {
		call(t, srv, "GET", "/api/statements/1", nil, &st)
		if st.Statement.Status == store.StDone || st.Statement.Status == store.StReview || st.Statement.Status == store.StFailed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st.Statement.Status != store.StDone || len(st.Transactions) != 3 {
		t.Fatalf("status %s error %q rows %d", st.Statement.Status, st.Statement.Error, len(st.Transactions))
	}
	byRaw := map[string]store.Txn{}
	for _, tx := range st.Transactions {
		byRaw[tx.Raw] = tx
	}
	refund := byRaw["SHOPEE SG MP SINGAPORE SG"]
	if refund.Kind != "refund" || refund.Amount != -500 || refund.Category != "shopping" {
		t.Errorf("refund %+v", refund)
	}
	mystery := byRaw["SMP*MYSTERY KITCHEN SINGAPORE SG"]
	if !mystery.NeedsReview || mystery.Category != "dining" {
		t.Errorf("mystery %+v", mystery)
	}

	// Fix the merchant for all similar rows → rule.
	var patched struct {
		Transaction store.Txn
		Applied     int64
	}
	code := call(t, srv, "PATCH", "/api/transactions/"+itoa(mystery.ID), map[string]any{"merchant": "Mum's Kitchen", "category": "dining", "applyToSimilar": true}, &patched)
	if code != 200 || patched.Transaction.Merchant != "Mum's Kitchen" || patched.Transaction.NeedsReview || patched.Applied != 1 {
		t.Fatalf("patch %d %+v", code, patched)
	}
	var rules struct{ Rules []store.Rule }
	call(t, srv, "GET", "/api/rules", nil, &rules)
	if len(rules.Rules) != 1 || rules.Rules[0].Key != "smp|mysterykitchen" {
		t.Errorf("rules %+v", rules.Rules)
	}

	// Re-uploading the same file is a no-op.
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("files", "again.pdf")
	fw.Write([]byte("%PDF-1.7 fake"))
	mw.Close()
	resp, _ = http.Post(srv.URL+"/api/statements", mw.FormDataContentType(), &buf)
	var up2 struct {
		Uploads []struct{ Duplicate bool }
	}
	json.NewDecoder(resp.Body).Decode(&up2)
	resp.Body.Close()
	if !up2.Uploads[0].Duplicate {
		t.Error("duplicate not detected")
	}

	// Overview.
	var ov struct {
		Month string
		Spent int64
	}
	call(t, srv, "GET", "/api/overview?month=2026-10", nil, &ov)
	if ov.Spent != 1240+2500-500 {
		t.Errorf("spent %d", ov.Spent)
	}
}

func TestGoalsAPI(t *testing.T) {
	srv, _ := setup(t)
	var g struct {
		store.Goal
		Progress struct {
			Status  string
			Current int64
		}
	}
	code := call(t, srv, "POST", "/api/goals", map[string]any{"kind": "target", "title": "Emergency fund", "area": "safety",
		"target": 2000000, "startAmount": 500000, "targetDate": "2027-12-31"}, &g)
	if code != 201 || g.ID == 0 || g.Status != "active" || g.StartDate != "2026-11-21" {
		t.Fatalf("create %d %+v", code, g)
	}
	code = call(t, srv, "POST", "/api/goals/"+itoa(g.ID)+"/entries", map[string]any{"date": "2026-11-20", "amount": 600000}, &g)
	if code != 200 || g.Progress.Current != 600000 {
		t.Errorf("check-in %d %+v", code, g.Progress)
	}
	// Validation.
	if code := call(t, srv, "POST", "/api/goals", map[string]any{"kind": "limit", "title": "Delivery", "target": 0}, nil); code != 400 {
		t.Errorf("limit without target: %d", code)
	}
	// Mark achieved; it stays listed with an achieved date.
	g.Status = "achieved"
	if code := call(t, srv, "PUT", "/api/goals/"+itoa(g.ID), g.Goal, &g); code != 200 || g.AchievedAt == nil {
		t.Errorf("achieve %d %+v", code, g.AchievedAt)
	}
	var list struct{ Goals []json.RawMessage }
	call(t, srv, "GET", "/api/goals", nil, &list)
	if len(list.Goals) != 1 {
		t.Errorf("goals %d", len(list.Goals))
	}
	// Drafting works without any API key (read locally).
	var d struct {
		Drafts []store.Goal
		Engine string
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	if code := call(t, srv, "POST", "/api/goals/draft", map[string]any{"paragraph": "I want to save 20k for emergencies by 2027"}, &d); code != 200 ||
		d.Engine != "local" || len(d.Drafts) != 1 || d.Drafts[0].Target != 2_000_000 || d.Drafts[0].Title != "Emergency fund" {
		t.Errorf("local draft: %d %+v", code, d)
	}
}

func TestClaudeSettingsHideKey(t *testing.T) {
	srv, _ := setup(t)
	t.Setenv("ANTHROPIC_API_KEY", "")
	var v map[string]any
	call(t, srv, "PUT", "/api/settings/claude", map[string]any{"apiKey": "sk-ant-test-1234567890", "goals": true, "capCents": 300}, &v)
	if v["keySet"] != true || v["keyHint"] != "…7890" || v["goals"] != true || v["capCents"].(float64) != 300 {
		t.Errorf("%v", v)
	}
	if _, leaked := v["apiKey"]; leaked {
		t.Error("key returned")
	}
}

func itoa(n int64) string { return fmtInt(n) }

func fmtInt(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestSplitFlow(t *testing.T) {
	srv, _ := setup(t)
	var tx store.Txn
	if code := call(t, srv, "POST", "/api/transactions", map[string]any{"date": "2026-10-10", "amount": 10000, "kind": "purchase",
		"category": "dining", "merchant": "Group dinner", "note": ""}, &tx); code != 201 {
		t.Fatalf("add %d", code)
	}
	type view struct {
		Transaction store.Txn
		Shares      []store.Share
	}
	var v view
	// Equal three ways: the extra cent stays with the payer.
	code := call(t, srv, "PUT", "/api/transactions/"+itoa(tx.ID)+"/split", map[string]any{"method": "equal",
		"parts": []map[string]any{{"name": "Alice"}, {"name": "Bob"}}}, &v)
	if code != 200 || v.Transaction.MyAmount != 3334 || v.Transaction.Others != 6666 || len(v.Shares) != 2 {
		t.Fatalf("split %d %+v", code, v)
	}
	var ov struct{ Spent int64 }
	call(t, srv, "GET", "/api/overview?month=2026-10", nil, &ov)
	if ov.Spent != 3334 {
		t.Errorf("overview counts the whole bill: %d", ov.Spent)
	}
	var people struct {
		People    []store.Person
		OwedToYou int64
	}
	call(t, srv, "GET", "/api/people", nil, &people)
	if people.OwedToYou != 6666 || len(people.People) != 2 {
		t.Fatalf("people %+v", people)
	}
	alice := v.Shares[0]
	if code := call(t, srv, "POST", "/api/shares/"+itoa(alice.ID)+"/settle", map[string]any{"settled": true}, nil); code != 204 {
		t.Errorf("settle %d", code)
	}
	call(t, srv, "GET", "/api/people", nil, &people)
	if people.OwedToYou != 3333 {
		t.Errorf("after settling Alice: %d", people.OwedToYou)
	}
	// Re-split: Bob keeps his settled state only if his amount is unchanged.
	call(t, srv, "PUT", "/api/transactions/"+itoa(tx.ID)+"/split", map[string]any{"method": "exact",
		"parts": []map[string]any{{"name": "alice", "amount": 5000}}}, &v)
	if v.Transaction.MyAmount != 5000 || len(v.Shares) != 1 || v.Shares[0].SettledAt != nil || v.Shares[0].Name != "Alice" {
		t.Errorf("exact re-split %+v", v)
	}
	// Over-allocation is refused; card payments can't be split.
	var e struct{ Error string }
	if code := call(t, srv, "PUT", "/api/transactions/"+itoa(tx.ID)+"/split", map[string]any{"method": "exact",
		"parts": []map[string]any{{"name": "Alice", "amount": 20000}}}, &e); code != 400 {
		t.Errorf("over-allocation %d", code)
	}
	call(t, srv, "DELETE", "/api/transactions/"+itoa(tx.ID)+"/split", nil, &v)
	if v.Transaction.MyAmount != 10000 || len(v.Shares) != 0 {
		t.Errorf("cleared %+v", v)
	}
}

func TestMyCost(t *testing.T) {
	srv, _ := setup(t)
	var tx store.Txn
	call(t, srv, "POST", "/api/transactions", map[string]any{"date": "2026-10-10", "amount": 10000, "kind": "purchase",
		"category": "shopping", "merchant": "Gift for the team", "note": ""}, &tx)
	var p struct{ Transaction store.Txn }
	if code := call(t, srv, "PATCH", "/api/transactions/"+itoa(tx.ID), map[string]any{"myCost": 3000}, &p); code != 200 ||
		p.Transaction.MyAmount != 3000 || p.Transaction.Amount != 10000 || p.Transaction.MyCost == nil {
		t.Fatalf("set %d %+v", code, p.Transaction)
	}
	var ov struct{ Spent int64 }
	call(t, srv, "GET", "/api/overview?month=2026-10", nil, &ov)
	if ov.Spent != 3000 {
		t.Errorf("overview %d", ov.Spent)
	}
	if code := call(t, srv, "PATCH", "/api/transactions/"+itoa(tx.ID), map[string]any{"myCost": 20000}, nil); code != 400 {
		t.Errorf("more than charged: %d", code)
	}
	// Splitting replaces the typed cost.
	var v struct{ Transaction store.Txn }
	call(t, srv, "PUT", "/api/transactions/"+itoa(tx.ID)+"/split", map[string]any{"method": "equal", "parts": []map[string]any{{"name": "Sam"}}}, &v)
	if v.Transaction.MyCost != nil || v.Transaction.MyAmount != 5000 {
		t.Errorf("split %+v", v.Transaction)
	}
	call(t, srv, "DELETE", "/api/transactions/"+itoa(tx.ID)+"/split", nil, nil)
	call(t, srv, "PATCH", "/api/transactions/"+itoa(tx.ID), map[string]any{"myCost": 4000}, &p)
	call(t, srv, "PATCH", "/api/transactions/"+itoa(tx.ID), map[string]any{"clearMyCost": true}, &p)
	if p.Transaction.MyCost != nil || p.Transaction.MyAmount != 10000 {
		t.Errorf("clear %+v", p.Transaction)
	}
}

// TestGoalEditRoundTrip edits a goal the way the UI does: GET the goal view
// (with its computed progress and entries), change fields, PUT it back.
func TestGoalEditRoundTrip(t *testing.T) {
	srv, _ := setup(t)
	var created store.Goal
	if code := call(t, srv, "POST", "/api/goals", map[string]any{"kind": "target", "title": "Home", "area": "home",
		"target": 9_360_000, "startAmount": 3_750_000, "monthlyPlan": 170_000, "startDate": "2026-11-21", "targetDate": "2029-07-01",
		"milestones": []map[string]any{{"label": "Emergency fund", "amount": 1_500_000}, {"label": "Wedding", "amount": 5_500_000}}}, &created); code != 201 {
		t.Fatalf("create %d", code)
	}
	call(t, srv, "POST", "/api/goals/"+itoa(created.ID)+"/entries", map[string]any{"date": "2026-11-21", "amount": 3_750_000}, nil)
	path := "/api/goals/" + itoa(created.ID)
	var view map[string]any
	call(t, srv, "GET", path, nil, &view)
	if view["progress"] == nil || view["entries"] == nil {
		t.Fatalf("view has no progress/entries: %v", view)
	}

	// Every field the edit form keys in.
	view["title"] = "  My share: wedding, renovation & keys  "
	view["why"] = "My half of our S$160k cash target."
	view["area"] = "home"
	view["target"] = 8_000_000
	view["startAmount"] = 2_200_000
	view["monthlyPlan"] = 170_000
	view["startDate"] = "2026-11-21"
	view["targetDate"] = "2029-07-01"
	view["milestones"] = []map[string]any{
		{"label": "Emergency fund", "amount": 1_500_000},
		{"label": "", "amount": 0}, // empty row left in the form: dropped
		{"label": "Keys", "amount": 8_000_000, "date": "2029-07-01"},
	}
	var got store.Goal
	if code := call(t, srv, "PUT", path, view, &got); code != 200 {
		t.Fatalf("edit with a full view: %d", code)
	}
	if got.Title != "My share: wedding, renovation & keys" || got.Why != "My half of our S$160k cash target." || got.Target != 8_000_000 ||
		got.StartAmount != 2_200_000 || got.MonthlyPlan != 170_000 || got.TargetDate != "2029-07-01" || len(got.Milestones) != 2 ||
		got.Milestones[1].Label != "Keys" || *got.Milestones[1].Amount != 8_000_000 || got.Milestones[1].Date != "2029-07-01" {
		t.Errorf("saved %+v", got)
	}

	// Status buttons and ticking a milestone send the view back too.
	call(t, srv, "GET", path, nil, &view)
	view["status"] = "paused"
	if code := call(t, srv, "PUT", path, view, &got); code != 200 || got.Status != "paused" {
		t.Errorf("pause %d %s", code, got.Status)
	}
	call(t, srv, "GET", path, nil, &view)
	view["status"] = "active"
	view["milestones"] = []map[string]any{{"label": "Booked the venue", "doneAt": "2026-11-22"}}
	if code := call(t, srv, "PUT", path, view, &got); code != 200 || got.Status != "active" || got.Milestones[0].DoneAt != "2026-11-22" {
		t.Errorf("tick milestone %d %+v", code, got)
	}

	// Bad input is refused with a message, not saved.
	for name, patch := range map[string]map[string]any{
		"no title":            {"title": "   "},
		"zero target":         {"target": 0},
		"negative have":       {"startAmount": -1},
		"negative plan":       {"monthlyPlan": -100},
		"impossible date":     {"targetDate": "2029-02-30"},
		"target before start": {"targetDate": "2026-01-01"},
		"bad area":            {"area": "moon"},
		"unnamed milestone":   {"milestones": []map[string]any{{"label": " ", "amount": 100}}},
		"negative milestone":  {"milestones": []map[string]any{{"label": "x", "amount": -100}}},
		"bad milestone date":  {"milestones": []map[string]any{{"label": "x", "date": "2027-13-01"}}},
		"unknown field":       {"tagret": 1},
	} {
		call(t, srv, "GET", path, nil, &view)
		for k, v := range patch {
			view[k] = v
		}
		var e struct{ Error string }
		if code := call(t, srv, "PUT", path, view, &e); code != 400 || e.Error == "" {
			t.Errorf("%s: %d %q", name, code, e.Error)
		}
	}
	call(t, srv, "GET", path, nil, &got)
	if got.Target != 8_000_000 || got.Title != "My share: wedding, renovation & keys" {
		t.Errorf("a refused edit changed the goal: %+v", got)
	}

	// Check-ins: amount and date are keyed in.
	if code := call(t, srv, "POST", path+"/entries", map[string]any{"date": "2026-02-30", "amount": 100}, nil); code != 400 {
		t.Errorf("impossible check-in date: %d", code)
	}
	if code := call(t, srv, "POST", path+"/entries", map[string]any{"amount": -100}, nil); code != 400 {
		t.Errorf("negative check-in: %d", code)
	}
	var v struct{ Entries []store.GoalEntry }
	if code := call(t, srv, "POST", path+"/entries", map[string]any{"date": "2026-12-01", "amount": 4_000_000, "note": "Dec"}, &v); code != 200 ||
		len(v.Entries) != 2 {
		t.Errorf("check-in %d %+v", code, v.Entries)
	}
}

func TestGoalReorder(t *testing.T) {
	srv, _ := setup(t)
	var ids []int64
	for _, title := range []string{"A", "B", "C"} {
		var g store.Goal
		call(t, srv, "POST", "/api/goals", map[string]any{"kind": "target", "title": title, "target": 100_00}, &g)
		ids = append(ids, g.ID)
	}
	order := func() string {
		var l struct{ Goals []store.Goal }
		call(t, srv, "GET", "/api/goals", nil, &l)
		s := ""
		for _, g := range l.Goals {
			s += g.Title
		}
		return s
	}
	if o := order(); o != "ABC" {
		t.Fatalf("new goals go last: %s", o)
	}
	if code := call(t, srv, "PUT", "/api/goals/order", map[string]any{"ids": []int64{ids[2], ids[0], ids[1]}}, nil); code != 204 || order() != "CAB" {
		t.Errorf("reorder %d: %s", code, order())
	}
	// Editing a goal with a stale position keeps the new order.
	var g map[string]any
	call(t, srv, "GET", "/api/goals/"+itoa(ids[1]), nil, &g)
	g["sort"], g["title"] = 0, "B2"
	call(t, srv, "PUT", "/api/goals/"+itoa(ids[1]), g, nil)
	if o := order(); o != "CAB2" {
		t.Errorf("edit moved the goal: %s", o)
	}
	for name, body := range map[string]any{
		"unknown id":   map[string]any{"ids": []int64{ids[0], 9999}},
		"duplicate id": map[string]any{"ids": []int64{ids[0], ids[0]}},
		"bad body":     map[string]any{"order": []int64{ids[0]}},
	} {
		if code := call(t, srv, "PUT", "/api/goals/order", body, nil); code < 400 {
			t.Errorf("%s: %d", name, code)
		}
	}
	if o := order(); o != "CAB2" {
		t.Errorf("a refused reorder changed the order: %s", o)
	}
}

// TestTxnsForGoal: "See transactions" on a spending-limit goal lists exactly
// what the goal counts, so the list adds up to the goal's figure.
func TestTxnsForGoal(t *testing.T) {
	srv, _ := setup(t)
	add := func(date, cat, kind, merchant string, cents int64) int64 {
		var tx store.Txn
		call(t, srv, "POST", "/api/transactions", map[string]any{"date": date, "amount": cents, "kind": kind, "category": cat, "merchant": merchant}, &tx)
		return tx.ID
	}
	add("2026-11-03", "dining", "purchase", "Kopitiam", 12_50)
	add("2026-11-05", "groceries", "purchase", "FairPrice", 40_00)
	add("2026-11-06", "insurance", "purchase", "AIA", 300_00) // not counted: other category
	add("2026-11-07", "other", "income", "Salary", 6000_00)   // not counted: not spending
	skip := add("2026-11-08", "shopping", "purchase", "Uniqlo", 59_90)
	call(t, srv, "PATCH", "/api/transactions/"+itoa(skip), map[string]any{"excluded": true}, nil) // not counted: excluded
	add("2026-10-30", "dining", "purchase", "Toast Box", 8_00) // other month

	var g struct {
		store.Goal
		Progress struct{ Current int64 }
	}
	call(t, srv, "POST", "/api/goals", map[string]any{"kind": "limit", "title": "Living costs", "target": 1500_00, "period": "month",
		"filter": map[string]any{"categories": []string{"dining", "groceries", "shopping", "other"}}}, &g)

	var l struct{ Transactions []store.Txn }
	if code := call(t, srv, "GET", "/api/transactions?goal="+itoa(g.ID)+"&month=2026-11", nil, &l); code != 200 {
		t.Fatalf("list %d", code)
	}
	var sum int64
	var names []string
	for _, tx := range l.Transactions {
		sum += tx.Mine()
		names = append(names, tx.Merchant)
	}
	if strings.Join(names, ",") != "FairPrice,Kopitiam" || sum != 52_50 {
		t.Errorf("goal list: %v = %d", names, sum)
	}
	call(t, srv, "GET", "/api/transactions?goal="+itoa(g.ID)+"&limit=1", nil, &l)
	if len(l.Transactions) != 1 || l.Transactions[0].Merchant != "FairPrice" {
		t.Errorf("limit applies after the goal filter: %+v", l.Transactions)
	}
	var habit store.Goal
	call(t, srv, "POST", "/api/goals", map[string]any{"kind": "habit", "title": "Invest", "target": 950_00}, &habit)
	for _, q := range []string{"goal=abc", "goal=99999", "goal=" + itoa(habit.ID)} {
		if code := call(t, srv, "GET", "/api/transactions?"+q, nil, nil); code < 400 {
			t.Errorf("%s: %d", q, code)
		}
	}
}
