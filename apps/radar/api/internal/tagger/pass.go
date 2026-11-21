package tagger

import (
	"context"

	"hub.localhost/apps/radar/api/internal/store"
)

// Store is the part of store.Store a tagging pass uses.
type Store interface {
	TagJobs(ctx context.Context, version string, limit int) ([]store.TagJob, error)
	SetTags(ctx context.Context, tags map[int64]store.Tags, by, version string) error
}

// Pass tags every item that is untagged or was tagged by older rules, in
// batches. It returns how many items it tagged.
func (t *Tagger) Pass(ctx context.Context, st Store) (int, error) {
	const batch = 200
	total := 0
	for {
		jobs, err := st.TagJobs(ctx, t.Version, batch)
		if err != nil || len(jobs) == 0 {
			return total, err
		}
		tags := make(map[int64]store.Tags, len(jobs))
		for _, j := range jobs {
			tags[j.ID] = t.Tag(Item{Title: j.Title, Summary: j.Summary, URL: j.URL, Kind: j.Kind, Defaults: j.Defaults})
		}
		if err := st.SetTags(ctx, tags, store.TaggedByRules, t.Version); err != nil {
			return total, err
		}
		total += len(jobs)
		if len(jobs) < batch {
			return total, nil
		}
	}
}
