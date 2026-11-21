// Package store keeps hub's own small state in SQLite: per-app preferences
// (pins, order, autostart override) and opaque shell UI state (open tabs…).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// migrations run in order; PRAGMA user_version records how many ran.
var migrations = []string{
	`CREATE TABLE app_prefs (
		app_id     TEXT PRIMARY KEY,
		pinned     INTEGER NOT NULL DEFAULT 0,
		position   INTEGER,
		auto_start INTEGER -- NULL: use the manifest's autoStart
	);
	CREATE TABLE ui_state (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL -- JSON
	);`,
	// 2: apps running when a session (desktop launcher) ended, restarted next time.
	`CREATE TABLE session_apps (
		app_id TEXT PRIMARY KEY,
		mode   TEXT NOT NULL -- run | dev
	);`,
}

type Store struct{ db *sql.DB }

// Open opens (creating if needed) the database at path and migrates it.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database is version %d, newer than this hub (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
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

// Prefs are the user's per-app settings.
type Prefs struct {
	Pinned    bool  `json:"pinned"`
	Position  *int  `json:"position"`  // order in the dock; nil = after positioned apps
	AutoStart *bool `json:"autoStart"` // nil = manifest default
}

// AllPrefs returns prefs keyed by app id (apps without a row are absent).
func (s *Store) AllPrefs(ctx context.Context) (map[string]Prefs, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT app_id, pinned, position, auto_start FROM app_prefs")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Prefs{}
	for rows.Next() {
		var id string
		var p Prefs
		var pos sql.NullInt64
		var auto sql.NullBool
		if err := rows.Scan(&id, &p.Pinned, &pos, &auto); err != nil {
			return nil, err
		}
		if pos.Valid {
			v := int(pos.Int64)
			p.Position = &v
		}
		if auto.Valid {
			p.AutoStart = &auto.Bool
		}
		out[id] = p
	}
	return out, rows.Err()
}

func (s *Store) Prefs(ctx context.Context, appID string) (Prefs, error) {
	all, err := s.AllPrefs(ctx)
	return all[appID], err
}

func (s *Store) SetPrefs(ctx context.Context, appID string, p Prefs) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO app_prefs (app_id, pinned, position, auto_start) VALUES (?, ?, ?, ?)
		ON CONFLICT (app_id) DO UPDATE SET pinned = excluded.pinned, position = excluded.position, auto_start = excluded.auto_start`,
		appID, p.Pinned, p.Position, p.AutoStart)
	return err
}

var ErrNotFound = errors.New("not found")

// UIState returns the JSON stored under key, or ErrNotFound.
func (s *Store) UIState(ctx context.Context, key string) (json.RawMessage, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM ui_state WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return json.RawMessage(v), err
}

func (s *Store) SetUIState(ctx context.Context, key string, value json.RawMessage) error {
	if !json.Valid(value) {
		return errors.New("value must be valid JSON")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ui_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(value))
	return err
}

// SaveSession records which apps (id → mode) were running when a session
// ended, replacing any earlier record.
func (s *Store) SaveSession(ctx context.Context, apps map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_apps"); err != nil {
		return err
	}
	for id, mode := range apps {
		if _, err := tx.ExecContext(ctx, "INSERT INTO session_apps (app_id, mode) VALUES (?, ?)", id, mode); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TakeSession returns and clears the apps recorded by SaveSession.
func (s *Store) TakeSession(ctx context.Context) (map[string]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT app_id, mode FROM session_apps")
	if err != nil {
		return nil, err
	}
	apps := map[string]string{}
	for rows.Next() {
		var id, mode string
		if err := rows.Scan(&id, &mode); err != nil {
			rows.Close()
			return nil, err
		}
		apps[id] = mode
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, "DELETE FROM session_apps"); err != nil {
		return nil, err
	}
	return apps, tx.Commit()
}
