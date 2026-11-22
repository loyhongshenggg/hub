package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Account is a card or bank account.
type Account struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Institution string `json:"institution"`
	Kind        string `json:"kind"`
	Last4       string `json:"last4"`
	Currency    string `json:"currency"`
}

// EnsureAccount finds or creates the account for an institution + last 4.
func (s *Store) EnsureAccount(ctx context.Context, a Account) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, "SELECT id FROM accounts WHERE institution = ? AND last4 = ?", a.Institution, a.Last4).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if a.Kind == "" {
		a.Kind = "credit_card"
	}
	if a.Currency == "" {
		a.Currency = "SGD"
	}
	res, err := s.db.ExecContext(ctx, "INSERT INTO accounts (name, institution, kind, last4, currency, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		a.Name, a.Institution, a.Kind, a.Last4, a.Currency, unix(s.now()))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Accounts lists accounts.
func (s *Store) Accounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, institution, kind, last4, currency FROM accounts ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Account{}
	for rows.Next() {
		var a Account
		if err := rows.Scan(&a.ID, &a.Name, &a.Institution, &a.Kind, &a.Last4, &a.Currency); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RenameAccount sets an account's display name.
func (s *Store) RenameAccount(ctx context.Context, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE accounts SET name = ? WHERE id = ?", name, id)
	return notFound(res, err)
}

// --- statements

// Statement is an imported file and what reading it found.
type Statement struct {
	ID          int64           `json:"id"`
	AccountID   *int64          `json:"accountId"`
	FileName    string          `json:"fileName"`
	SHA256      string          `json:"-"`
	Status      string          `json:"status"` // queued | reading | done | review | failed
	Progress    string          `json:"progress"`
	Error       string          `json:"error,omitempty"`
	PeriodStart string          `json:"periodStart"`
	PeriodEnd   string          `json:"periodEnd"`
	Summary     json.RawMessage `json:"summary"`
	Recon       json.RawMessage `json:"recon"`
	Warnings    json.RawMessage `json:"warnings"`
	Passes      json.RawMessage `json:"passes"`
	Pages       int             `json:"pages"`
	ImportedAt  time.Time       `json:"importedAt"`
	FinishedAt  *time.Time      `json:"finishedAt"`
	Txns        int             `json:"txns"`
}

// Statement statuses.
const (
	StQueued  = "queued"
	StReading = "reading"
	StDone    = "done"   // reconciled (or no totals to check)
	StReview  = "review" // rows don't match the totals
	StFailed  = "failed"
)

// CreateStatement registers an upload. If the same file was imported before
// it returns that statement and exists = true.
func (s *Store) CreateStatement(ctx context.Context, fileName, sha string) (id int64, exists bool, err error) {
	err = s.db.QueryRowContext(ctx, "SELECT id FROM statements WHERE sha256 = ?", sha).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}
	res, err := s.db.ExecContext(ctx, "INSERT INTO statements (file_name, sha256, status, imported_at) VALUES (?, ?, ?, ?)",
		fileName, sha, StQueued, unix(s.now()))
	if err != nil {
		return 0, false, err
	}
	id, err = res.LastInsertId()
	return id, false, err
}

// SetStatementProgress records status + a progress line.
func (s *Store) SetStatementProgress(ctx context.Context, id int64, status, progress string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE statements SET status = ?, progress = ?, error = '' WHERE id = ?", status, progress, id)
	return err
}

// FailStatement marks a statement failed.
func (s *Store) FailStatement(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE statements SET status = ?, error = ?, progress = '', finished_at = ? WHERE id = ?",
		StFailed, msg, unix(s.now()), id)
	return err
}

// StatementResult is what an import writes back.
type StatementResult struct {
	AccountID   int64
	Status      string
	PeriodStart string
	PeriodEnd   string
	Summary     any
	Recon       any
	Warnings    []string
	Passes      []string
	Pages       int
}

// FinishStatement stores the reading and replaces the statement's rows.
func (s *Store) FinishStatement(ctx context.Context, id int64, r StatementResult, txns []Txn) (inserted int, err error) {
	j := func(v any) string {
		b, _ := json.Marshal(v)
		return string(b)
	}
	if r.Warnings == nil {
		r.Warnings = []string{}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE statements SET account_id = ?, status = ?, progress = '', error = '', period_start = ?, period_end = ?,
		summary = ?, recon = ?, warnings = ?, passes = ?, pages = ?, finished_at = ? WHERE id = ?`,
		r.AccountID, r.Status, r.PeriodStart, r.PeriodEnd, j(r.Summary), j(r.Recon), j(r.Warnings), j(r.Passes), r.Pages, unix(s.now()), id); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM transactions WHERE statement_id = ?", id); err != nil {
		return 0, err
	}
	for _, t := range txns {
		// Skip a row another statement already holds (overlapping periods).
		var dup int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE account_id = ? AND statement_id != ? AND date = ? AND posted = ? AND amount = ? AND raw = ?`,
			r.AccountID, id, t.Date, t.Posted, t.Amount, t.Raw).Scan(&dup); err != nil {
			return 0, err
		}
		if dup > 0 {
			continue
		}
		t.AccountID, t.StatementID, t.Source = &r.AccountID, &id, "statement"
		if _, err := insertTxn(ctx, tx, t, s.now()); err != nil {
			return 0, err
		}
		inserted++
	}
	return inserted, tx.Commit()
}

const stmtCols = `s.id, s.account_id, s.file_name, s.sha256, s.status, s.progress, s.error, s.period_start, s.period_end,
	s.summary, s.recon, s.warnings, s.passes, s.pages, s.imported_at, s.finished_at,
	(SELECT COUNT(*) FROM transactions t WHERE t.statement_id = s.id)`

func scanStatement(sc interface{ Scan(...any) error }) (Statement, error) {
	var st Statement
	var acct sql.NullInt64
	var imported int64
	var finished sql.NullInt64
	var summary, recon, warnings, passes string
	err := sc.Scan(&st.ID, &acct, &st.FileName, &st.SHA256, &st.Status, &st.Progress, &st.Error, &st.PeriodStart, &st.PeriodEnd,
		&summary, &recon, &warnings, &passes, &st.Pages, &imported, &finished, &st.Txns)
	if err != nil {
		return st, err
	}
	if acct.Valid {
		st.AccountID = &acct.Int64
	}
	st.Summary, st.Recon, st.Warnings, st.Passes = json.RawMessage(summary), json.RawMessage(recon), json.RawMessage(warnings), json.RawMessage(passes)
	st.ImportedAt = time.Unix(imported, 0)
	if finished.Valid {
		t := time.Unix(finished.Int64, 0)
		st.FinishedAt = &t
	}
	return st, nil
}

// Statements lists imports, newest period first.
// SetStatementPages records how many pages a statement's PDF has.
func (s *Store) SetStatementPages(ctx context.Context, id int64, pages int) error {
	_, err := s.db.ExecContext(ctx, "UPDATE statements SET pages = ? WHERE id = ?", pages, id)
	return err
}

func (s *Store) Statements(ctx context.Context) ([]Statement, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+stmtCols+" FROM statements s ORDER BY s.period_end DESC, s.id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Statement{}
	for rows.Next() {
		st, err := scanStatement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// GetStatement returns one statement.
func (s *Store) GetStatement(ctx context.Context, id int64) (Statement, error) {
	st, err := scanStatement(s.db.QueryRowContext(ctx, "SELECT "+stmtCols+" FROM statements s WHERE s.id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	return st, err
}

// PendingStatements are queued or interrupted reads (to resume at start).
func (s *Store) PendingStatements(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM statements WHERE status IN (?, ?) ORDER BY id", StQueued, StReading)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// DeleteStatement removes a statement and its rows.
func (s *Store) DeleteStatement(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM statements WHERE id = ?", id)
	return notFound(res, err)
}

// --- transactions

// Txn is a transaction row.
type Txn struct {
	ID          int64   `json:"id"`
	AccountID   *int64  `json:"accountId"`
	StatementID *int64  `json:"statementId"`
	Date        string  `json:"date"`   // transaction date, YYYY-MM-DD
	Posted      string  `json:"posted"` // post date
	Raw         string  `json:"raw"`
	Amount      int64   `json:"amount"` // cents; + out, − in
	Kind        string  `json:"kind"`
	Category    string  `json:"category"`
	Merchant    string  `json:"merchant"`
	MerchantKey string  `json:"merchantKey"`
	MatchSource string  `json:"matchSource"`
	Confidence  float64 `json:"confidence"`
	Detail      string  `json:"detail,omitempty"`
	NeedsReview bool    `json:"needsReview"`
	Foreign     bool    `json:"foreign"`
	Excluded    bool    `json:"excluded"`
	OneOff      bool    `json:"oneOff"`
	Note        string  `json:"note"`
	Source      string  `json:"source"` // statement | manual
	Page        int     `json:"page"`
	Y           float64 `json:"y"`
	Recovered   bool    `json:"recovered"`
	// Split: the others' shares (same sign as Amount) and what's left as yours.
	SplitMethod string  `json:"splitMethod,omitempty"`
	MyWeight    float64 `json:"myWeight,omitempty"`
	Others      int64   `json:"othersShare"`
	// MyCost is your cost typed by hand; nil = not set.
	MyCost   *int64 `json:"myCost"`
	MyAmount int64  `json:"myAmount"`
}

// Mine is your part of the transaction: your cost if you typed one, else the
// amount minus others' shares. Insights and goals count this, not the card
// amount.
func (t Txn) Mine() int64 {
	if t.MyCost != nil {
		return *t.MyCost
	}
	return t.Amount - t.Others
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

func insertTxn(ctx context.Context, db execer, t Txn, now time.Time) (int64, error) {
	if t.Source == "" {
		t.Source = "manual"
	}
	res, err := db.ExecContext(ctx, `INSERT INTO transactions (account_id, statement_id, date, posted, raw, amount, kind, category, merchant,
		merchant_key, match_source, confidence, detail, needs_review, foreign_txn, excluded, one_off, note, source, page, y, recovered, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.AccountID, t.StatementID, t.Date, t.Posted, t.Raw, t.Amount, t.Kind, t.Category, t.Merchant,
		t.MerchantKey, t.MatchSource, t.Confidence, t.Detail, boolInt(t.NeedsReview), boolInt(t.Foreign), boolInt(t.Excluded),
		boolInt(t.OneOff), t.Note, t.Source, t.Page, t.Y, boolInt(t.Recovered), unix(now))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// AddTxn inserts a manual transaction.
func (s *Store) AddTxn(ctx context.Context, t Txn) (int64, error) {
	t.Source, t.StatementID = "manual", nil
	return insertTxn(ctx, s.db, t, s.now())
}

// TxnFilter selects transactions.
type TxnFilter struct {
	From, To    string // inclusive dates
	Category    string
	Kind        string
	MerchantKey string
	Merchant    string
	Query       string
	StatementID int64
	Review      bool
	Split       bool // shared with others
	Spending    bool // purchases, refunds and fees only
	Limit       int
}

const txnCols = `id, account_id, statement_id, date, posted, raw, amount, kind, category, merchant, merchant_key, match_source,
	confidence, detail, needs_review, foreign_txn, excluded, one_off, note, source, page, y, recovered, split_method, my_weight,
	COALESCE((SELECT SUM(s.amount) FROM txn_shares s WHERE s.txn_id = transactions.id), 0), my_cost`

// Txns lists transactions, newest first.
func (s *Store) Txns(ctx context.Context, f TxnFilter) ([]Txn, error) {
	var where []string
	var args []any
	add := func(cond string, a ...any) {
		where = append(where, cond)
		args = append(args, a...)
	}
	if f.From != "" {
		add("date >= ?", f.From)
	}
	if f.To != "" {
		add("date <= ?", f.To)
	}
	if f.Category != "" {
		add("category = ?", f.Category)
	}
	if f.Kind != "" {
		add("kind = ?", f.Kind)
	}
	if f.MerchantKey != "" {
		add("merchant_key = ?", f.MerchantKey)
	}
	if f.Merchant != "" {
		add("merchant = ?", f.Merchant)
	}
	if f.Query != "" {
		q := "%" + strings.ToLower(f.Query) + "%"
		add("(lower(merchant) LIKE ? OR lower(raw) LIKE ? OR lower(note) LIKE ?)", q, q, q)
	}
	if f.StatementID != 0 {
		add("statement_id = ?", f.StatementID)
	}
	if f.Review {
		add("needs_review = 1")
	}
	if f.Spending {
		add("kind IN ('purchase', 'refund', 'fee')")
	}
	if f.Split {
		add("EXISTS (SELECT 1 FROM txn_shares s WHERE s.txn_id = transactions.id)")
	}
	q := "SELECT " + txnCols + " FROM transactions"
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	order := " ORDER BY date DESC, id DESC"
	if f.StatementID != 0 {
		order = " ORDER BY page, y, id"
	}
	q += order
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Txn{}
	for rows.Next() {
		t, err := scanTxn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanTxn(sc interface{ Scan(...any) error }) (Txn, error) {
	var t Txn
	var acct, stmt sql.NullInt64
	var review, foreign, excluded, oneOff, recovered int
	var myCost sql.NullInt64
	err := sc.Scan(&t.ID, &acct, &stmt, &t.Date, &t.Posted, &t.Raw, &t.Amount, &t.Kind, &t.Category, &t.Merchant, &t.MerchantKey,
		&t.MatchSource, &t.Confidence, &t.Detail, &review, &foreign, &excluded, &oneOff, &t.Note, &t.Source, &t.Page, &t.Y, &recovered,
		&t.SplitMethod, &t.MyWeight, &t.Others, &myCost)
	if myCost.Valid {
		t.MyCost = &myCost.Int64
	}
	t.MyAmount = t.Mine()
	if acct.Valid {
		t.AccountID = &acct.Int64
	}
	if stmt.Valid {
		t.StatementID = &stmt.Int64
	}
	t.NeedsReview, t.Foreign, t.Excluded, t.OneOff, t.Recovered = review == 1, foreign == 1, excluded == 1, oneOff == 1, recovered == 1
	return t, err
}

// GetTxn returns one transaction.
func (s *Store) GetTxn(ctx context.Context, id int64) (Txn, error) {
	t, err := scanTxn(s.db.QueryRowContext(ctx, "SELECT "+txnCols+" FROM transactions WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// TxnPatch is a partial update; nil fields are unchanged.
type TxnPatch struct {
	Date     *string `json:"date"`
	Amount   *int64  `json:"amount"`
	Kind     *string `json:"kind"`
	Category *string `json:"category"`
	Merchant *string `json:"merchant"`
	Note     *string `json:"note"`
	Excluded *bool   `json:"excluded"`
	OneOff   *bool   `json:"oneOff"`
	Reviewed *bool   `json:"reviewed"`
	// MyCost sets your cost by hand (cents, same sign as the amount);
	// ClearMyCost goes back to the charged amount.
	MyCost      *int64 `json:"myCost"`
	ClearMyCost bool   `json:"clearMyCost"`
}

// UpdateTxn applies a patch. Editing merchant/category/kind marks the row
// reviewed and its source "user".
func (s *Store) UpdateTxn(ctx context.Context, id int64, p TxnPatch) error {
	var sets []string
	var args []any
	set := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Date != nil {
		set("date", *p.Date)
	}
	if p.Amount != nil {
		set("amount", *p.Amount)
	}
	if p.Note != nil {
		set("note", *p.Note)
	}
	if p.Excluded != nil {
		set("excluded", boolInt(*p.Excluded))
	}
	if p.OneOff != nil {
		set("one_off", boolInt(*p.OneOff))
	}
	if p.ClearMyCost {
		set("my_cost", nil)
	} else if p.MyCost != nil {
		set("my_cost", *p.MyCost)
	}
	user := false
	if p.Kind != nil {
		set("kind", *p.Kind)
		user = true
	}
	if p.Category != nil {
		set("category", *p.Category)
		user = true
	}
	if p.Merchant != nil {
		set("merchant", *p.Merchant)
		user = true
	}
	if user {
		set("match_source", "user")
		set("confidence", 1.0)
	}
	if user || p.Reviewed != nil && *p.Reviewed {
		set("needs_review", 0)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	res, err := s.db.ExecContext(ctx, "UPDATE transactions SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return notFound(res, err)
}

// DeleteTxn deletes a transaction.
func (s *Store) DeleteTxn(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM transactions WHERE id = ?", id)
	return notFound(res, err)
}

// SetTxnMatch updates the automatic merchant match of rows with a key that
// the user hasn't edited (used after a directory refresh or a Claude lookup).
func (s *Store) SetTxnMatch(ctx context.Context, key, merchant, category, source, detail string, conf float64, review bool) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE transactions SET merchant = ?, category = ?, match_source = ?, detail = ?, confidence = ?, needs_review = ?
		WHERE merchant_key = ? AND match_source NOT IN ('user', 'rule', 'special') AND kind IN ('purchase', 'refund')`,
		merchant, category, source, detail, conf, boolInt(review), key)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// UnresolvedKeys lists merchant keys still without a confident match, with
// one raw descriptor each.
func (s *Store) UnresolvedKeys(ctx context.Context, limit int) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT merchant_key, MIN(raw) FROM transactions
		WHERE match_source IN ('none', 'keyword') AND kind IN ('purchase', 'refund') AND merchant_key != ''
		GROUP BY merchant_key ORDER BY COUNT(*) DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, raw string
		if err := rows.Scan(&k, &raw); err != nil {
			return nil, err
		}
		out[k] = raw
	}
	return out, rows.Err()
}

// AutoMatched lists distinct (key, raw) of rows matched automatically, for
// re-resolving after a directory refresh.
func (s *Store) AutoMatched(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT merchant_key, MIN(raw) FROM transactions
		WHERE match_source IN ('none', 'keyword', 'osm', 'acra', 'brand') AND kind IN ('purchase', 'refund') AND merchant_key != ''
		GROUP BY merchant_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, raw string
		if err := rows.Scan(&k, &raw); err != nil {
			return nil, err
		}
		out[k] = raw
	}
	return out, rows.Err()
}

// DataRange returns the first and last transaction dates ("" when empty).
func (s *Store) DataRange(ctx context.Context) (first, last string, err error) {
	var f, l sql.NullString
	err = s.db.QueryRowContext(ctx, "SELECT MIN(date), MAX(date) FROM transactions").Scan(&f, &l)
	return f.String, l.String, err
}

// ReviewCount counts rows flagged to check.
func (s *Store) ReviewCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM transactions WHERE needs_review = 1").Scan(&n)
	return n, err
}

// --- rules (your corrections)

// Rule maps a merchant key to your merchant name and category.
type Rule struct {
	Key       string    `json:"key"`
	Merchant  string    `json:"merchant"`
	Category  string    `json:"category"`
	Kind      string    `json:"kind,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// SetRule saves a rule and applies it to every row with that key.
func (s *Store) SetRule(ctx context.Context, r Rule) (applied int64, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO rules (key, merchant, category, kind, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET merchant = excluded.merchant, category = excluded.category, kind = excluded.kind`,
		r.Key, r.Merchant, r.Category, r.Kind, unix(s.now())); err != nil {
		return 0, err
	}
	q := `UPDATE transactions SET merchant = ?, category = ?, match_source = 'rule', confidence = 1, needs_review = 0`
	args := []any{r.Merchant, r.Category}
	if r.Kind != "" {
		q += ", kind = ?"
		args = append(args, r.Kind)
	}
	q += " WHERE merchant_key = ? AND kind NOT IN ('payment')"
	args = append(args, r.Key)
	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	applied, _ = res.RowsAffected()
	return applied, tx.Commit()
}

// GetRule returns the rule for a key.
func (s *Store) GetRule(ctx context.Context, key string) (Rule, bool, error) {
	var r Rule
	var at int64
	err := s.db.QueryRowContext(ctx, "SELECT key, merchant, category, kind, created_at FROM rules WHERE key = ?", key).
		Scan(&r.Key, &r.Merchant, &r.Category, &r.Kind, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	r.CreatedAt = time.Unix(at, 0)
	return r, err == nil, err
}

// Rules lists all rules.
func (s *Store) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, merchant, category, kind, created_at FROM rules ORDER BY merchant")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		var r Rule
		var at int64
		if err := rows.Scan(&r.Key, &r.Merchant, &r.Category, &r.Kind, &at); err != nil {
			return nil, err
		}
		r.CreatedAt = time.Unix(at, 0)
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRule removes a rule (rows keep their current values).
func (s *Store) DeleteRule(ctx context.Context, key string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM rules WHERE key = ?", key)
	return notFound(res, err)
}

// --- lookups (cached Claude merchant answers)

// Lookup reads a cached lookup result.
func (s *Store) Lookup(ctx context.Context, key string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT result FROM lookups WHERE key = ?", key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// SaveLookup caches a lookup result.
func (s *Store) SaveLookup(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO lookups (key, result, at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET result = excluded.result, at = excluded.at",
		key, string(b), unix(s.now()))
	return err
}

func notFound(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
