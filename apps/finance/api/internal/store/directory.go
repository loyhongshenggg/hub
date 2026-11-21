package store

import (
	"context"
	"database/sql"
	"time"

	"hub.localhost/apps/finance/api/internal/merchant"
)

// DirectorySource is the state of one merchant dataset.
type DirectorySource struct {
	Source      string     `json:"source"`
	Count       int        `json:"count"`
	RefreshedAt *time.Time `json:"refreshedAt"`
	Status      string     `json:"status"` // "" | running | ok | failed
	Error       string     `json:"error,omitempty"`
}

// ReplaceDirectory swaps all entries of one source.
func (s *Store) ReplaceDirectory(ctx context.Context, source string, entries []merchant.Entry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM directory WHERE source = ?", source); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, "INSERT INTO directory (source, ref, name, branch, category, address, detail) VALUES (?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, e := range entries {
		if _, err := stmt.ExecContext(ctx, source, e.Ref, e.Name, e.Branch, e.Category, e.Address, e.Detail); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO directory_sources (source, count, refreshed_at, status, error) VALUES (?, ?, ?, 'ok', '')
		ON CONFLICT (source) DO UPDATE SET count = excluded.count, refreshed_at = excluded.refreshed_at, status = 'ok', error = ''`,
		source, len(entries), unix(s.now())); err != nil {
		return err
	}
	return tx.Commit()
}

// SetDirectoryStatus records a running / failed refresh.
func (s *Store) SetDirectoryStatus(ctx context.Context, source, status, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO directory_sources (source, status, error) VALUES (?, ?, ?)
		ON CONFLICT (source) DO UPDATE SET status = excluded.status, error = excluded.error`, source, status, errMsg)
	return err
}

// DirectorySources lists dataset states.
func (s *Store) DirectorySources(ctx context.Context) ([]DirectorySource, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT source, count, refreshed_at, status, error FROM directory_sources ORDER BY source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DirectorySource{}
	for rows.Next() {
		var d DirectorySource
		var at sql.NullInt64
		if err := rows.Scan(&d.Source, &d.Count, &at, &d.Status, &d.Error); err != nil {
			return nil, err
		}
		if at.Valid {
			t := time.Unix(at.Int64, 0)
			d.RefreshedAt = &t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// LoadDirectory reads every directory entry.
func (s *Store) LoadDirectory(ctx context.Context) ([]merchant.Entry, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT source, ref, name, branch, category, address, detail FROM directory")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []merchant.Entry
	for rows.Next() {
		var e merchant.Entry
		if err := rows.Scan(&e.Source, &e.Ref, &e.Name, &e.Branch, &e.Category, &e.Address, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
