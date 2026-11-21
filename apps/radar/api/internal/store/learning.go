package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"hub.localhost/apps/radar/api/internal/engine"
)

func nullTime(v sql.NullInt64) time.Time {
	if !v.Valid {
		return time.Time{}
	}
	return time.Unix(v.Int64, 0).UTC()
}

func timeOrNull(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.Unix()
}

func boolPtr(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 == 1
	return &b
}

func boolOrNull(b *bool) any {
	if b == nil {
		return nil
	}
	if *b {
		return 1
	}
	return 0
}

func (s *Store) meta(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, key string) (string, error) {
	var v string
	err := q.QueryRowContext(ctx, "SELECT value FROM meta WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// LoadLearning reads the whole learning state (it is small: one row per
// thing done, chapter touched and topic seen).
func (s *Store) LoadLearning(ctx context.Context) (*engine.State, error) {
	st := engine.NewState()
	rows, err := s.db.QueryContext(ctx, "SELECT ref, status, at, minutes, rating, worth FROM progress")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p engine.Progress
		var at int64
		var worth sql.NullInt64
		var rating string
		if err := rows.Scan(&p.Ref, &p.Status, &at, &p.Minutes, &rating, &worth); err != nil {
			rows.Close()
			return nil, err
		}
		p.At, p.Rating, p.Worth = time.Unix(at, 0).UTC(), engine.Rating(rating), boolPtr(worth)
		st.Progress[p.Ref] = p
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, "SELECT chapter, started_at, completed_at, outcomes, rating, worth, snoozed_at FROM chapter_state")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key, outcomes, rating string
		var started, completed, worth sql.NullInt64
		var c engine.ChapterState
		if err := rows.Scan(&key, &started, &completed, &outcomes, &rating, &worth, &c.SnoozedAt); err != nil {
			rows.Close()
			return nil, err
		}
		c.Started, c.Completed, c.Rating, c.Worth = nullTime(started), nullTime(completed), engine.Rating(rating), boolPtr(worth)
		if outcomes != "[]" {
			_ = json.Unmarshal([]byte(outcomes), &c.Outcomes)
		}
		st.Chapters[key] = c
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, "SELECT topic, theta, n, assessed, alpha, beta, obs FROM topic_state")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, obs string
		var t engine.TopicState
		if err := rows.Scan(&id, &t.Theta, &t.N, &t.Assessed, &t.Alpha, &t.Beta, &obs); err != nil {
			rows.Close()
			return nil, err
		}
		if obs != "[]" {
			_ = json.Unmarshal([]byte(obs), &t.Obs)
		}
		st.Topics[id] = t
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, "SELECT track, alpha, beta FROM track_interest")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var b engine.Beta
		if err := rows.Scan(&id, &b.Alpha, &b.Beta); err != nil {
			rows.Close()
			return nil, err
		}
		st.TrackInterest[id] = b
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, "SELECT chapter, ref, after, reason, at FROM extras ORDER BY at, ref")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var x engine.Extra
		var key string
		var at int64
		if err := rows.Scan(&key, &x.Ref, &x.After, &x.Reason, &at); err != nil {
			rows.Close()
			return nil, err
		}
		x.Track, x.Chapter, _ = strings.Cut(key, "/")
		x.At = time.Unix(at, 0).UTC()
		st.Extras = append(st.Extras, x)
	}
	rows.Close()

	if st.LastTrack, err = s.meta(ctx, s.db, "last_track"); err != nil {
		return nil, err
	}
	a, err := s.meta(ctx, s.db, "assessed")
	if err != nil {
		return nil, err
	}
	st.Assessed = a == "1"
	if v, err := s.meta(ctx, s.db, "learning_settings"); err != nil {
		return nil, err
	} else if v != "" {
		_ = json.Unmarshal([]byte(v), &st.Settings)
		if st.Settings.Priorities == nil {
			st.Settings.Priorities = map[string]float64{}
		}
	}
	return st, rows.Err()
}

// SaveLearning writes the whole learning state in one transaction.
func (s *Store) SaveLearning(ctx context.Context, st *engine.State) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"progress", "chapter_state", "topic_state", "track_interest", "extras"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+t); err != nil {
			return err
		}
	}
	for _, p := range st.Progress {
		if _, err := tx.ExecContext(ctx, "INSERT INTO progress (ref, status, at, minutes, rating, worth) VALUES (?, ?, ?, ?, ?, ?)",
			p.Ref, p.Status, p.At.Unix(), p.Minutes, string(p.Rating), boolOrNull(p.Worth)); err != nil {
			return err
		}
	}
	for key, c := range st.Chapters {
		outcomes, _ := json.Marshal(c.Outcomes)
		if c.Outcomes == nil {
			outcomes = []byte("[]")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO chapter_state (chapter, started_at, completed_at, outcomes, rating, worth, snoozed_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, key, timeOrNull(c.Started), timeOrNull(c.Completed), string(outcomes),
			string(c.Rating), boolOrNull(c.Worth), c.SnoozedAt); err != nil {
			return err
		}
	}
	for id, t := range st.Topics {
		obs, _ := json.Marshal(t.Obs)
		if t.Obs == nil {
			obs = []byte("[]")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO topic_state (topic, theta, n, assessed, alpha, beta, obs) VALUES (?, ?, ?, ?, ?, ?, ?)",
			id, t.Theta, t.N, t.Assessed, t.Alpha, t.Beta, string(obs)); err != nil {
			return err
		}
	}
	for id, b := range st.TrackInterest {
		if _, err := tx.ExecContext(ctx, "INSERT INTO track_interest (track, alpha, beta) VALUES (?, ?, ?)", id, b.Alpha, b.Beta); err != nil {
			return err
		}
	}
	for _, x := range st.Extras {
		if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO extras (chapter, ref, after, reason, at) VALUES (?, ?, ?, ?, ?)",
			x.Track+"/"+x.Chapter, x.Ref, x.After, x.Reason, x.At.Unix()); err != nil {
			return err
		}
	}
	settings, _ := json.Marshal(st.Settings)
	assessed := "0"
	if st.Assessed {
		assessed = "1"
	}
	for k, v := range map[string]string{"last_track": st.LastTrack, "assessed": assessed, "learning_settings": string(settings)} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta (key, value) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MentorPool lists every mentor item with its tags, for fresh and
// corrective steps.
func (s *Store) MentorPool(ctx context.Context) ([]engine.Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id, i.title, i.url, i.type, i.minutes, i.difficulty, i.published_at, s.mentor,
		(SELECT group_concat(topic, ',') FROM (SELECT topic FROM item_topics t WHERE t.item_id = i.id ORDER BY rank))
		FROM items i JOIN sources s ON s.id = i.source_id
		WHERE s.mentor != '' ORDER BY i.published_at DESC, i.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []engine.Item
	for rows.Next() {
		var it engine.Item
		var pub int64
		var topics sql.NullString
		if err := rows.Scan(&it.ID, &it.Title, &it.URL, &it.Type, &it.Minutes, &it.Difficulty, &pub, &it.Mentor, &topics); err != nil {
			return nil, err
		}
		it.Published = time.Unix(pub, 0).UTC()
		it.Topics = splitList(topics.String)
		if it.Type == "" {
			it.Type = "read"
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
