package store

import (
	"context"
	"database/sql"
	"strings"
)

// Tags is what a tagger (rules, Claude or the user) says about an item.
type Tags struct {
	Topics     []string `json:"topics"` // best first
	Difficulty int      `json:"difficulty"`
	Type       string   `json:"type"` // read | watch | lab
	Minutes    int      `json:"minutes"`
}

// Who set an item's tags. A user's tags are never overwritten.
const (
	TaggedByRules  = "rules"
	TaggedByClaude = "claude"
	TaggedByUser   = "user"
)

// TagJob is an item waiting for (re)tagging, with its source's hints.
type TagJob struct {
	ID       int64
	Title    string
	Summary  string
	URL      string
	Kind     string   // source kind: post | video
	Defaults []string // source's fallback topics
}

// TagJobs lists items never tagged, or tagged by the rules tagger at a
// different version than version.
func (s *Store) TagJobs(ctx context.Context, version string, limit int) ([]TagJob, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.id, i.title, i.summary, i.url, s.kind, s.topics
		FROM items i JOIN sources s ON s.id = i.source_id
		WHERE i.tagged_by = '' OR (i.tagged_by = ? AND i.tag_version != ?)
		ORDER BY i.id LIMIT ?`, TaggedByRules, version, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagJob
	for rows.Next() {
		var j TagJob
		var defaults string
		if err := rows.Scan(&j.ID, &j.Title, &j.Summary, &j.URL, &j.Kind, &defaults); err != nil {
			return nil, err
		}
		j.Defaults = splitList(defaults)
		out = append(out, j)
	}
	return out, rows.Err()
}

// SetTags replaces the tags of the given items in one transaction. Items
// the user tagged are skipped unless by is TaggedByUser.
func (s *Store) SetTags(ctx context.Context, tags map[int64]Tags, by, version string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for id, t := range tags {
		res, err := tx.ExecContext(ctx, `UPDATE items SET difficulty = ?, type = ?, minutes = ?, tagged_by = ?, tag_version = ?
			WHERE id = ? AND (tagged_by != ? OR ? = ?)`,
			t.Difficulty, t.Type, t.Minutes, by, version, id, TaggedByUser, by, TaggedByUser)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if len(tags) == 1 && by == TaggedByUser {
				return ErrNotFound
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM item_topics WHERE item_id = ?", id); err != nil {
			return err
		}
		for rank, topic := range t.Topics {
			if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO item_topics (item_id, topic, rank) VALUES (?, ?, ?)",
				id, topic, rank); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ItemTags returns an item's current tags.
func (s *Store) ItemTags(ctx context.Context, id int64) (Tags, string, error) {
	var t Tags
	var by string
	var topics sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT difficulty, type, minutes, tagged_by,
		(SELECT group_concat(topic, ',') FROM (SELECT topic FROM item_topics WHERE item_id = items.id ORDER BY rank))
		FROM items WHERE id = ?`, id).Scan(&t.Difficulty, &t.Type, &t.Minutes, &by, &topics)
	if err == sql.ErrNoRows {
		return t, "", ErrNotFound
	}
	t.Topics = splitList(topics.String)
	return t, by, err
}

func splitList(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}
