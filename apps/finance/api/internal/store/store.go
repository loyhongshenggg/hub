// Package store keeps Finance's data in SQLite: accounts, imported
// statements, transactions, your merchant rules, goals and their check-ins,
// the merchant directory, settings and Claude usage.
//
// Money is integer cents. Transaction amounts are signed from your point of
// view: positive = money out (purchases, fees), negative = money in
// (refunds, card payments, income).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var migrations = []string{
	`CREATE TABLE accounts (
		id          INTEGER PRIMARY KEY,
		name        TEXT NOT NULL,
		institution TEXT NOT NULL DEFAULT '',
		kind        TEXT NOT NULL DEFAULT 'credit_card',
		last4       TEXT NOT NULL DEFAULT '',
		currency    TEXT NOT NULL DEFAULT 'SGD',
		created_at  INTEGER NOT NULL,
		UNIQUE (institution, last4)
	);
	CREATE TABLE statements (
		id           INTEGER PRIMARY KEY,
		account_id   INTEGER REFERENCES accounts(id),
		file_name    TEXT NOT NULL,
		sha256       TEXT NOT NULL UNIQUE,
		status       TEXT NOT NULL,
		progress     TEXT NOT NULL DEFAULT '',
		error        TEXT NOT NULL DEFAULT '',
		period_start TEXT NOT NULL DEFAULT '',
		period_end   TEXT NOT NULL DEFAULT '',
		summary      TEXT NOT NULL DEFAULT '{}',
		recon        TEXT NOT NULL DEFAULT '{}',
		warnings     TEXT NOT NULL DEFAULT '[]',
		passes       TEXT NOT NULL DEFAULT '[]',
		pages        INTEGER NOT NULL DEFAULT 0,
		imported_at  INTEGER NOT NULL,
		finished_at  INTEGER
	);
	CREATE TABLE transactions (
		id           INTEGER PRIMARY KEY,
		account_id   INTEGER REFERENCES accounts(id),
		statement_id INTEGER REFERENCES statements(id) ON DELETE CASCADE,
		date         TEXT NOT NULL,
		posted       TEXT NOT NULL DEFAULT '',
		raw          TEXT NOT NULL,
		amount       INTEGER NOT NULL,
		kind         TEXT NOT NULL,
		category     TEXT NOT NULL DEFAULT '',
		merchant     TEXT NOT NULL DEFAULT '',
		merchant_key TEXT NOT NULL DEFAULT '',
		match_source TEXT NOT NULL DEFAULT '',
		confidence   REAL NOT NULL DEFAULT 0,
		detail       TEXT NOT NULL DEFAULT '',
		needs_review INTEGER NOT NULL DEFAULT 0,
		foreign_txn  INTEGER NOT NULL DEFAULT 0,
		excluded     INTEGER NOT NULL DEFAULT 0,
		one_off      INTEGER NOT NULL DEFAULT 0,
		note         TEXT NOT NULL DEFAULT '',
		source       TEXT NOT NULL DEFAULT 'statement',
		page         INTEGER NOT NULL DEFAULT 0,
		y            REAL NOT NULL DEFAULT 0,
		recovered    INTEGER NOT NULL DEFAULT 0,
		created_at   INTEGER NOT NULL
	);
	CREATE INDEX tx_date ON transactions (date);
	CREATE INDEX tx_category ON transactions (category, date);
	CREATE INDEX tx_key ON transactions (merchant_key);
	CREATE INDEX tx_statement ON transactions (statement_id);
	CREATE TABLE rules (
		key        TEXT PRIMARY KEY,
		merchant   TEXT NOT NULL,
		category   TEXT NOT NULL,
		kind       TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);
	CREATE TABLE lookups (
		key    TEXT PRIMARY KEY,
		result TEXT NOT NULL,
		at     INTEGER NOT NULL
	);
	CREATE TABLE directory (
		id       INTEGER PRIMARY KEY,
		source   TEXT NOT NULL,
		ref      TEXT NOT NULL,
		name     TEXT NOT NULL,
		branch   TEXT NOT NULL DEFAULT '',
		category TEXT NOT NULL,
		address  TEXT NOT NULL DEFAULT '',
		detail   TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX directory_source ON directory (source);
	CREATE TABLE directory_sources (
		source       TEXT PRIMARY KEY,
		count        INTEGER NOT NULL DEFAULT 0,
		refreshed_at INTEGER,
		status       TEXT NOT NULL DEFAULT '',
		error        TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE goals (
		id           INTEGER PRIMARY KEY,
		kind         TEXT NOT NULL,
		title        TEXT NOT NULL,
		why          TEXT NOT NULL DEFAULT '',
		area         TEXT NOT NULL DEFAULT 'other',
		status       TEXT NOT NULL DEFAULT 'active',
		target       INTEGER NOT NULL DEFAULT 0,
		start_amount INTEGER NOT NULL DEFAULT 0,
		start_date   TEXT NOT NULL,
		target_date  TEXT NOT NULL DEFAULT '',
		period       TEXT NOT NULL DEFAULT 'month',
		metric       TEXT NOT NULL DEFAULT 'amount',
		filter       TEXT NOT NULL DEFAULT '{}',
		monthly_plan INTEGER NOT NULL DEFAULT 0,
		milestones   TEXT NOT NULL DEFAULT '[]',
		source       TEXT NOT NULL DEFAULT 'manual',
		prompt       TEXT NOT NULL DEFAULT '',
		sort         INTEGER NOT NULL DEFAULT 0,
		created_at   INTEGER NOT NULL,
		updated_at   INTEGER NOT NULL,
		achieved_at  INTEGER
	);
	CREATE TABLE goal_entries (
		id         INTEGER PRIMARY KEY,
		goal_id    INTEGER NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
		date       TEXT NOT NULL,
		amount     INTEGER NOT NULL,
		note       TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	);
	CREATE INDEX goal_entries_goal ON goal_entries (goal_id, date);
	CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
	CREATE TABLE claude_usage (
		month         TEXT PRIMARY KEY,
		calls         INTEGER NOT NULL DEFAULT 0,
		input_tokens  INTEGER NOT NULL DEFAULT 0,
		output_tokens INTEGER NOT NULL DEFAULT 0,
		searches      INTEGER NOT NULL DEFAULT 0,
		cost_micros   INTEGER NOT NULL DEFAULT 0
	);`,
	// 2: splits. A transaction you paid can be shared with others (Splitwise
	// model): each other person owes a share; your share is the rest. Shares
	// carry the transaction's sign; settled_at marks repayment.
	`CREATE TABLE people (
		id         INTEGER PRIMARY KEY,
		name       TEXT NOT NULL UNIQUE COLLATE NOCASE,
		created_at INTEGER NOT NULL
	);
	CREATE TABLE txn_shares (
		id         INTEGER PRIMARY KEY,
		txn_id     INTEGER NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
		person_id  INTEGER NOT NULL REFERENCES people(id),
		amount     INTEGER NOT NULL,
		weight     REAL NOT NULL DEFAULT 1,
		settled_at INTEGER,
		UNIQUE (txn_id, person_id)
	);
	CREATE INDEX txn_shares_person ON txn_shares (person_id, settled_at);
	ALTER TABLE transactions ADD COLUMN split_method TEXT NOT NULL DEFAULT '';
	ALTER TABLE transactions ADD COLUMN my_weight REAL NOT NULL DEFAULT 1;`,
	// 3: your cost typed by hand (NULL = the charged amount, minus any split).
	// Same sign as amount. The charged amount itself never changes.
	`ALTER TABLE transactions ADD COLUMN my_cost INTEGER;`,
}

// ErrNotFound is returned for unknown ids.
var ErrNotFound = errors.New("not found")

type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (and migrates) the database at path.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, now: time.Now}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

func (s *Store) migrate(ctx context.Context) error {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// --- settings

// Setting reads a JSON setting into v; ok is false when unset.
func (s *Store) Setting(ctx context.Context, key string, v any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key = ?", key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// SetSetting stores v as JSON.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value", key, string(b))
	return err
}

// --- Claude usage

// Usage is one month's Claude spend.
type Usage struct {
	Month        string `json:"month"`
	Calls        int64  `json:"calls"`
	InputTokens  int64  `json:"inputTokens"`
	OutputTokens int64  `json:"outputTokens"`
	Searches     int64  `json:"searches"`
	CostMicros   int64  `json:"costMicros"` // US$ millionths
}

// AddUsage adds to a month's totals.
func (s *Store) AddUsage(ctx context.Context, u Usage) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO claude_usage (month, calls, input_tokens, output_tokens, searches, cost_micros)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (month) DO UPDATE SET calls = calls + excluded.calls, input_tokens = input_tokens + excluded.input_tokens,
			output_tokens = output_tokens + excluded.output_tokens, searches = searches + excluded.searches,
			cost_micros = cost_micros + excluded.cost_micros`,
		u.Month, u.Calls, u.InputTokens, u.OutputTokens, u.Searches, u.CostMicros)
	return err
}

// UsageFor returns a month's totals (zero when none).
func (s *Store) UsageFor(ctx context.Context, month string) (Usage, error) {
	u := Usage{Month: month}
	err := s.db.QueryRowContext(ctx, `SELECT calls, input_tokens, output_tokens, searches, cost_micros FROM claude_usage WHERE month = ?`, month).
		Scan(&u.Calls, &u.InputTokens, &u.OutputTokens, &u.Searches, &u.CostMicros)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return u, err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func unix(t time.Time) int64 { return t.Unix() }
