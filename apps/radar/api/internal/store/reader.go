package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"hub.localhost/apps/radar/api/internal/reader"
)

// The reader cache: Store implements reader.Cache.

func (s *Store) Article(ctx context.Context, url string) (*reader.Article, string, time.Time, bool, error) {
	var raw sql.NullString
	var failure string
	var at int64
	err := s.db.QueryRowContext(ctx, "SELECT article, failure, fetched_at FROM reader_cache WHERE url = ?", url).
		Scan(&raw, &failure, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", time.Time{}, false, nil
	}
	if err != nil {
		return nil, "", time.Time{}, false, err
	}
	var a *reader.Article
	if raw.Valid {
		a = &reader.Article{}
		if err := json.Unmarshal([]byte(raw.String), a); err != nil {
			return nil, "", time.Time{}, false, nil // unreadable entry: refetch
		}
	}
	return a, failure, time.Unix(at, 0), true, nil
}

func (s *Store) PutArticle(ctx context.Context, url string, a *reader.Article, failure string, at time.Time) error {
	var raw any
	if a != nil {
		b, err := json.Marshal(a)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO reader_cache (url, article, failure, fetched_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (url) DO UPDATE SET article = excluded.article, failure = excluded.failure, fetched_at = excluded.fetched_at`,
		url, raw, failure, at.Unix())
	return err
}

func (s *Store) Frame(ctx context.Context, host string) (reader.Framing, time.Time, bool, error) {
	var f reader.Framing
	var at int64
	err := s.db.QueryRowContext(ctx, "SELECT framing, checked_at FROM frame_policy WHERE host = ?", host).Scan(&f, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return reader.FrameBlocked, time.Time{}, false, nil
	}
	return f, time.Unix(at, 0), err == nil, err
}

func (s *Store) PutFrame(ctx context.Context, host string, f reader.Framing, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO frame_policy (host, framing, checked_at) VALUES (?, ?, ?)
		ON CONFLICT (host) DO UPDATE SET framing = excluded.framing, checked_at = excluded.checked_at`,
		host, f, at.Unix())
	return err
}

// ItemLink returns an item's URL and content type (read | watch | lab).
func (s *Store) ItemLink(ctx context.Context, id int64) (string, string, error) {
	var url, typ string
	err := s.db.QueryRowContext(ctx, "SELECT url, type FROM items WHERE id = ?", id).Scan(&url, &typ)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if typ == "" {
		typ = "read"
	}
	return url, typ, err
}
