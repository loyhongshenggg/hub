// Package store keeps Radar's state in SQLite: sources with fetch status,
// news items, and what the user read or saved.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"hub.localhost/apps/radar/api/content"
)

var migrations = []string{
	`CREATE TABLE sources (
		id            TEXT PRIMARY KEY,
		name          TEXT NOT NULL,
		url           TEXT NOT NULL,
		category      TEXT NOT NULL,
		active        INTEGER NOT NULL DEFAULT 1,
		etag          TEXT NOT NULL DEFAULT '',
		last_modified TEXT NOT NULL DEFAULT '',
		last_fetch_at INTEGER,
		last_ok_at    INTEGER,
		last_error    TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE items (
		id           INTEGER PRIMARY KEY,
		source_id    TEXT NOT NULL REFERENCES sources(id),
		guid         TEXT NOT NULL,
		url          TEXT NOT NULL,
		title        TEXT NOT NULL,
		summary      TEXT NOT NULL DEFAULT '',
		published_at INTEGER NOT NULL,
		fetched_at   INTEGER NOT NULL,
		read_at      INTEGER,
		saved_at     INTEGER,
		UNIQUE (source_id, guid)
	);
	CREATE INDEX items_published ON items (published_at DESC, id DESC);
	CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);`,
}

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
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
	if v > len(migrations) {
		return fmt.Errorf("database version %d is newer than this app (%d)", v, len(migrations))
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

// SyncSources makes the table match sources.yaml. Sources no longer listed
// become inactive (not fetched); their items stay, so saved ones survive.
func (s *Store) SyncSources(ctx context.Context, srcs []content.Source) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "UPDATE sources SET active = 0"); err != nil {
		return err
	}
	for _, src := range srcs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO sources (id, name, url, category, active) VALUES (?, ?, ?, ?, 1)
			ON CONFLICT (id) DO UPDATE SET
				name = excluded.name, category = excluded.category, active = 1,
				url = excluded.url,
				-- a new URL invalidates the cache validators
				etag = CASE WHEN sources.url = excluded.url THEN sources.etag ELSE '' END,
				last_modified = CASE WHEN sources.url = excluded.url THEN sources.last_modified ELSE '' END`,
			src.ID, src.Name, src.URL, src.Category); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Source is a feed with its fetch status.
type Source struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	Category     string     `json:"category"`
	Active       bool       `json:"active"`
	ETag         string     `json:"-"`
	LastModified string     `json:"-"`
	LastFetchAt  *time.Time `json:"lastFetchAt"`
	LastOKAt     *time.Time `json:"lastOkAt"`
	LastError    string     `json:"lastError,omitempty"`
	Items        int        `json:"items"`
}

func unixPtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.Unix(v.Int64, 0).UTC()
	return &t
}

func (s *Store) Sources(ctx context.Context, activeOnly bool) ([]Source, error) {
	q := `SELECT s.id, s.name, s.url, s.category, s.active, s.etag, s.last_modified,
		s.last_fetch_at, s.last_ok_at, s.last_error,
		(SELECT COUNT(*) FROM items i WHERE i.source_id = s.id)
		FROM sources s`
	if activeOnly {
		q += " WHERE s.active = 1"
	}
	q += " ORDER BY s.category, s.name"
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var src Source
		var fetch, ok sql.NullInt64
		if err := rows.Scan(&src.ID, &src.Name, &src.URL, &src.Category, &src.Active, &src.ETag, &src.LastModified,
			&fetch, &ok, &src.LastError, &src.Items); err != nil {
			return nil, err
		}
		src.LastFetchAt, src.LastOKAt = unixPtr(fetch), unixPtr(ok)
		out = append(out, src)
	}
	return out, rows.Err()
}

// FetchResult records how fetching one source went.
type FetchResult struct {
	At           time.Time
	Err          error
	ETag         string // empty: keep the previous one
	LastModified string
}

func (s *Store) RecordFetch(ctx context.Context, sourceID string, r FetchResult) error {
	if r.Err != nil {
		_, err := s.db.ExecContext(ctx, "UPDATE sources SET last_fetch_at = ?, last_error = ? WHERE id = ?",
			r.At.Unix(), r.Err.Error(), sourceID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET last_fetch_at = ?, last_ok_at = ?, last_error = '',
		etag = CASE WHEN ? = '' THEN etag ELSE ? END,
		last_modified = CASE WHEN ? = '' THEN last_modified ELSE ? END
		WHERE id = ?`, r.At.Unix(), r.At.Unix(), r.ETag, r.ETag, r.LastModified, r.LastModified, sourceID)
	return err
}

// NewItem is a parsed feed entry.
type NewItem struct {
	GUID      string
	URL       string
	Title     string
	Summary   string
	Published time.Time
}

// AddItems inserts items not seen before; it returns how many were new.
func (s *Store) AddItems(ctx context.Context, sourceID string, items []NewItem, now time.Time) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	added := 0
	for _, it := range items {
		res, err := tx.ExecContext(ctx, `INSERT INTO items (source_id, guid, url, title, summary, published_at, fetched_at)
			VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (source_id, guid) DO NOTHING`,
			sourceID, it.GUID, it.URL, it.Title, it.Summary, it.Published.Unix(), now.Unix())
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
		}
	}
	return added, tx.Commit()
}

// Item is a news item as the UI sees it.
type Item struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Summary     string    `json:"summary"`
	PublishedAt time.Time `json:"publishedAt"`
	Read        bool      `json:"read"`
	Saved       bool      `json:"saved"`
	Source      struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Category string `json:"category"`
	} `json:"source"`
}

// Filter selects items. Zero values mean "any".
type Filter struct {
	Category string
	Source   string
	Query    string
	Unread   bool
	Saved    bool
	// Before is a keyset cursor: items strictly older than (time, id).
	BeforeTime int64
	BeforeID   int64
	Limit      int
}

func (f Filter) where() (string, []any) {
	var conds []string
	var args []any
	if f.Category != "" {
		conds = append(conds, "s.category = ?")
		args = append(args, f.Category)
	}
	if f.Source != "" {
		conds = append(conds, "i.source_id = ?")
		args = append(args, f.Source)
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		conds = append(conds, "(i.title LIKE ? ESCAPE '\\' OR i.summary LIKE ? ESCAPE '\\')")
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(q) + "%"
		args = append(args, like, like)
	}
	if f.Unread {
		conds = append(conds, "i.read_at IS NULL")
	}
	if f.Saved {
		conds = append(conds, "i.saved_at IS NOT NULL")
	}
	if f.BeforeID > 0 {
		conds = append(conds, "(i.published_at < ? OR (i.published_at = ? AND i.id < ?))")
		args = append(args, f.BeforeTime, f.BeforeTime, f.BeforeID)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// Items returns matching items, newest first, and a cursor for the next page
// ("" when there is none).
func (s *Store) Items(ctx context.Context, f Filter) ([]Item, string, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	where, args := f.where()
	rows, err := s.db.QueryContext(ctx, `SELECT i.id, i.title, i.url, i.summary, i.published_at,
		i.read_at IS NOT NULL, i.saved_at IS NOT NULL, s.id, s.name, s.category
		FROM items i JOIN sources s ON s.id = i.source_id`+where+`
		ORDER BY i.published_at DESC, i.id DESC LIMIT ?`, append(args, f.Limit+1)...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Item{}
	var lastPub int64
	for rows.Next() {
		var it Item
		var pub int64
		if err := rows.Scan(&it.ID, &it.Title, &it.URL, &it.Summary, &pub, &it.Read, &it.Saved,
			&it.Source.ID, &it.Source.Name, &it.Source.Category); err != nil {
			return nil, "", err
		}
		it.PublishedAt = time.Unix(pub, 0).UTC()
		if len(out) == f.Limit {
			return out, fmt.Sprintf("%d.%d", lastPub, out[len(out)-1].ID), rows.Err()
		}
		lastPub = pub
		out = append(out, it)
	}
	return out, "", rows.Err()
}

var ErrNotFound = errors.New("not found")

func (s *Store) setFlag(ctx context.Context, col string, id int64, on bool, now time.Time) error {
	var v any
	if on {
		v = now.Unix()
	}
	res, err := s.db.ExecContext(ctx, "UPDATE items SET "+col+" = ? WHERE id = ?", v, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetRead(ctx context.Context, id int64, read bool) error {
	return s.setFlag(ctx, "read_at", id, read, time.Now())
}

func (s *Store) SetSaved(ctx context.Context, id int64, saved bool) error {
	return s.setFlag(ctx, "saved_at", id, saved, time.Now())
}

// MarkAllRead marks every unread item matching f (cursor and limit ignored).
func (s *Store) MarkAllRead(ctx context.Context, f Filter) (int64, error) {
	f.BeforeID, f.Limit, f.Unread = 0, 0, false
	where, args := f.where()
	cond := "read_at IS NULL"
	if where != "" {
		cond += " AND id IN (SELECT i.id FROM items i JOIN sources s ON s.id = i.source_id" + where + ")"
	}
	res, err := s.db.ExecContext(ctx, "UPDATE items SET read_at = ? WHERE "+cond, append([]any{time.Now().Unix()}, args...)...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Counts summarises unread items for badges and filters.
type Counts struct {
	Unread           int            `json:"unread"`
	UnreadByCategory map[string]int `json:"unreadByCategory"`
	Saved            int            `json:"saved"`
	// NewSinceVisit counts items fetched since the user last opened Radar.
	NewSinceVisit int `json:"newSinceVisit"`
}

func (s *Store) Counts(ctx context.Context) (Counts, error) {
	c := Counts{UnreadByCategory: map[string]int{}}
	rows, err := s.db.QueryContext(ctx, `SELECT s.category, COUNT(*) FROM items i JOIN sources s ON s.id = i.source_id
		WHERE i.read_at IS NULL GROUP BY s.category`)
	if err != nil {
		return c, err
	}
	for rows.Next() {
		var cat string
		var n int
		if err := rows.Scan(&cat, &n); err != nil {
			rows.Close()
			return c, err
		}
		c.UnreadByCategory[cat] = n
		c.Unread += n
	}
	rows.Close()
	visit, err := s.LastVisit(ctx)
	if err != nil {
		return c, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM items WHERE saved_at IS NOT NULL),
		(SELECT COUNT(*) FROM items WHERE read_at IS NULL AND fetched_at > ?)`, visit.Unix()).Scan(&c.Saved, &c.NewSinceVisit)
	return c, err
}

// LastVisit is when the user last opened Radar (zero time if never).
func (s *Store) LastVisit(ctx context.Context) (time.Time, error) {
	var v string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = 'last_visit'").Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Unix(0, 0), nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, v)
	return t, err
}

func (s *Store) SetLastVisit(ctx context.Context, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES ('last_visit', ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, t.UTC().Format(time.RFC3339))
	return err
}

// Prune deletes unsaved items published before cutoff.
func (s *Store) Prune(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM items WHERE saved_at IS NULL AND published_at < ?", cutoff.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
