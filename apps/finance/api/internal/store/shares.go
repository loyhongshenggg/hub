package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Person is someone you split costs with.
type Person struct {
	ID      int64   `json:"id"`
	Name    string  `json:"name"`
	Owed    int64   `json:"owed"`    // unsettled: what they still owe you (cents)
	Settled int64   `json:"settled"` // paid back so far
	Count   int     `json:"count"`   // shared transactions
	Last    *string `json:"last"`    // date of the latest shared transaction
}

// Share is one person's part of a transaction.
type Share struct {
	ID        int64      `json:"id"`
	TxnID     int64      `json:"txnId"`
	PersonID  int64      `json:"personId"`
	Name      string     `json:"name"`
	Amount    int64      `json:"amount"`
	Weight    float64    `json:"weight"`
	SettledAt *time.Time `json:"settledAt"`
	// Transaction context (for a person's list).
	Date     string `json:"date,omitempty"`
	Merchant string `json:"merchant,omitempty"`
	Total    int64  `json:"total,omitempty"`
}

// NewShare is one person's computed share when saving a split.
type NewShare struct {
	Name   string
	Amount int64
	Weight float64
}

// SetSplit replaces a transaction's split. People are created by name;
// a share keeps its "settled" mark if that person's amount didn't change.
func (s *Store) SetSplit(ctx context.Context, txnID int64, method string, myWeight float64, shares []NewShare) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	old := map[string]struct {
		amount  int64
		settled sql.NullInt64
	}{}
	rows, err := tx.QueryContext(ctx, `SELECT lower(p.name), s.amount, s.settled_at FROM txn_shares s JOIN people p ON p.id = s.person_id WHERE s.txn_id = ?`, txnID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		var o struct {
			amount  int64
			settled sql.NullInt64
		}
		if err := rows.Scan(&name, &o.amount, &o.settled); err != nil {
			rows.Close()
			return err
		}
		old[name] = o
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, "DELETE FROM txn_shares WHERE txn_id = ?", txnID); err != nil {
		return err
	}
	for _, sh := range shares {
		pid, err := ensurePerson(ctx, tx, sh.Name, s.now())
		if err != nil {
			return err
		}
		var settled any
		if o, ok := old[strings.ToLower(sh.Name)]; ok && o.amount == sh.Amount && o.settled.Valid {
			settled = o.settled.Int64
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO txn_shares (txn_id, person_id, amount, weight, settled_at) VALUES (?, ?, ?, ?, ?)",
			txnID, pid, sh.Amount, sh.Weight, settled); err != nil {
			return err
		}
	}
	// A split decides your share, replacing any cost typed by hand.
	res, err := tx.ExecContext(ctx, "UPDATE transactions SET split_method = ?, my_weight = ?, my_cost = NULL WHERE id = ?", method, myWeight, txnID)
	if err := notFound(res, err); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearSplit makes a transaction wholly yours again.
func (s *Store) ClearSplit(ctx context.Context, txnID int64) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM txn_shares WHERE txn_id = ?", txnID); err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, "UPDATE transactions SET split_method = '', my_weight = 1 WHERE id = ?", txnID)
	return notFound(res, err)
}

func ensurePerson(ctx context.Context, tx *sql.Tx, name string, now time.Time) (int64, error) {
	name = strings.TrimSpace(name)
	var id int64
	err := tx.QueryRowContext(ctx, "SELECT id FROM people WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	res, err := tx.ExecContext(ctx, "INSERT INTO people (name, created_at) VALUES (?, ?)", name, unix(now))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const shareCols = `s.id, s.txn_id, s.person_id, p.name, s.amount, s.weight, s.settled_at, t.date, t.merchant, t.amount`

func scanShares(rows *sql.Rows) ([]Share, error) {
	defer rows.Close()
	out := []Share{}
	for rows.Next() {
		var sh Share
		var settled sql.NullInt64
		if err := rows.Scan(&sh.ID, &sh.TxnID, &sh.PersonID, &sh.Name, &sh.Amount, &sh.Weight, &settled, &sh.Date, &sh.Merchant, &sh.Total); err != nil {
			return nil, err
		}
		if settled.Valid {
			t := time.Unix(settled.Int64, 0)
			sh.SettledAt = &t
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// TxnShares lists a transaction's shares.
func (s *Store) TxnShares(ctx context.Context, txnID int64) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+shareCols+` FROM txn_shares s JOIN people p ON p.id = s.person_id
		JOIN transactions t ON t.id = s.txn_id WHERE s.txn_id = ? ORDER BY p.name`, txnID)
	if err != nil {
		return nil, err
	}
	return scanShares(rows)
}

// PersonShares lists a person's shares, unsettled first, newest first.
func (s *Store) PersonShares(ctx context.Context, personID int64) ([]Share, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+shareCols+` FROM txn_shares s JOIN people p ON p.id = s.person_id
		JOIN transactions t ON t.id = s.txn_id WHERE s.person_id = ? ORDER BY s.settled_at IS NOT NULL, t.date DESC`, personID)
	if err != nil {
		return nil, err
	}
	return scanShares(rows)
}

// People lists everyone you've split with, most owed first.
func (s *Store) People(ctx context.Context) ([]Person, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id, p.name,
			COALESCE(SUM(CASE WHEN s.settled_at IS NULL THEN s.amount END), 0),
			COALESCE(SUM(CASE WHEN s.settled_at IS NOT NULL THEN s.amount END), 0),
			COUNT(s.id), MAX(t.date)
		FROM people p LEFT JOIN txn_shares s ON s.person_id = p.id LEFT JOIN transactions t ON t.id = s.txn_id
		GROUP BY p.id ORDER BY 3 DESC, p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Person{}
	for rows.Next() {
		var p Person
		var last sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Owed, &p.Settled, &p.Count, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			p.Last = &last.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SettleShare marks one share as paid back (or not).
func (s *Store) SettleShare(ctx context.Context, id int64, settled bool) error {
	var at any
	if settled {
		at = unix(s.now())
	}
	res, err := s.db.ExecContext(ctx, "UPDATE txn_shares SET settled_at = ? WHERE id = ?", at, id)
	return notFound(res, err)
}

// SettlePerson marks everything a person owes as paid back.
func (s *Store) SettlePerson(ctx context.Context, personID int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE txn_shares SET settled_at = ? WHERE person_id = ? AND settled_at IS NULL", unix(s.now()), personID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// RenamePerson renames someone (names are unique, case-insensitive).
func (s *Store) RenamePerson(ctx context.Context, id int64, name string) error {
	res, err := s.db.ExecContext(ctx, "UPDATE people SET name = ? WHERE id = ?", strings.TrimSpace(name), id)
	return notFound(res, err)
}

// OwedToYou is the total of unsettled shares.
func (s *Store) OwedToYou(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM txn_shares WHERE settled_at IS NULL").Scan(&n)
	return n, err
}
