package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Avatar is a cached picture.
type Avatar struct {
	URL         string
	ContentType string
	Data        []byte
	FetchedAt   time.Time
}

func (s *Store) Avatar(ctx context.Context, id string) (*Avatar, error) {
	var a Avatar
	var at int64
	err := s.db.QueryRowContext(ctx, "SELECT url, content_type, data, fetched_at FROM avatars WHERE id = ?", id).
		Scan(&a.URL, &a.ContentType, &a.Data, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	a.FetchedAt = time.Unix(at, 0)
	return &a, err
}

func (s *Store) PutAvatar(ctx context.Context, id string, a Avatar) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO avatars (id, url, content_type, data, fetched_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET url = excluded.url, content_type = excluded.content_type,
		data = excluded.data, fetched_at = excluded.fetched_at`, id, a.URL, a.ContentType, a.Data, a.FetchedAt.Unix())
	return err
}
