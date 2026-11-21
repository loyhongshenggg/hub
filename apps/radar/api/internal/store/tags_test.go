package store

import (
	"context"
	"slices"
	"testing"

	"hub.localhost/apps/radar/api/content"
)

func mentorSeed(t *testing.T, s *Store) (newsID, oldMentorID int64) {
	t.Helper()
	ctx := context.Background()
	if err := s.SyncSources(ctx, append(slices.Clone(srcs), content.Source{
		ID: "rehberger-posts", Name: "Johann Rehberger · posts", Category: "security", URL: "https://etr.example/index.xml",
		Mentor: "rehberger", Kind: "post", Topics: []string{"prompt-injection"},
	})); err != nil {
		t.Fatal(err)
	}
	now := t0.AddDate(0, 6, 0)
	s.AddItems(ctx, "hn", []NewItem{{GUID: "n", URL: "https://hn/n", Title: "News", Published: t0}}, now)
	s.AddItems(ctx, "rehberger-posts", []NewItem{
		{GUID: "old", URL: "https://etr/old", Title: "ASCII smuggling", Published: t0.AddDate(-2, 0, 0)},
		{GUID: "new", URL: "https://etr/new", Title: "Agent hijacking", Published: now},
	}, now)
	items, _, _ := s.Items(ctx, Filter{})
	for _, it := range items {
		switch it.Title {
		case "News":
			newsID = it.ID
		case "ASCII smuggling":
			oldMentorID = it.ID
		}
	}
	return newsID, oldMentorID
}

func TestTagJobsAndSetTags(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	newsID, mentorID := mentorSeed(t, s)

	jobs, err := s.TagJobs(ctx, "v1", 10)
	if err != nil || len(jobs) != 3 {
		t.Fatalf("jobs: %v %+v", err, jobs)
	}
	for _, j := range jobs {
		if j.ID == mentorID && (j.Kind != "post" || !slices.Equal(j.Defaults, []string{"prompt-injection"})) {
			t.Errorf("mentor job hints: %+v", j)
		}
	}
	all := map[int64]Tags{}
	for _, j := range jobs {
		all[j.ID] = Tags{Topics: []string{"vulnerabilities"}, Difficulty: 3, Type: "read", Minutes: 10}
	}
	all[mentorID] = Tags{Topics: []string{"prompt-injection", "data-exfiltration"}, Difficulty: 3, Type: "read", Minutes: 10}
	if err := s.SetTags(ctx, all, TaggedByRules, "v1"); err != nil {
		t.Fatal(err)
	}
	if jobs, _ := s.TagJobs(ctx, "v1", 10); len(jobs) != 0 {
		t.Errorf("same version: %d jobs left", len(jobs))
	}
	if jobs, _ := s.TagJobs(ctx, "v2", 10); len(jobs) != 3 {
		t.Errorf("new version: %d jobs, want 3", len(jobs))
	}

	// A hand fix sticks: the rules tagger can't overwrite it.
	if err := s.SetTags(ctx, map[int64]Tags{newsID: {Topics: []string{"evals"}}}, TaggedByUser, ""); err != nil {
		t.Fatal(err)
	}
	s.SetTags(ctx, map[int64]Tags{newsID: {Topics: []string{"linux"}}}, TaggedByRules, "v2")
	if tags, by, _ := s.ItemTags(ctx, newsID); by != TaggedByUser || !slices.Equal(tags.Topics, []string{"evals"}) {
		t.Errorf("user tags overwritten: %+v by %s", tags, by)
	}
	if jobs, _ := s.TagJobs(ctx, "v2", 10); len(jobs) != 2 {
		t.Errorf("user-tagged item still queued: %d jobs", len(jobs))
	}
	if err := s.SetTags(ctx, map[int64]Tags{999: {}}, TaggedByUser, ""); err != ErrNotFound {
		t.Errorf("missing item: %v", err)
	}

	if tagged, _, _ := s.Items(ctx, Filter{Tagged: true}); len(tagged) != 3 {
		t.Errorf("tagged filter: %d items", len(tagged))
	}

	// Topic order survives, and filters see the tags.
	items, _, _ := s.Items(ctx, Filter{Topic: "data-exfiltration"})
	if len(items) != 1 || !slices.Equal(items[0].Topics, []string{"prompt-injection", "data-exfiltration"}) ||
		items[0].Source.Mentor != "rehberger" || items[0].TaggedBy != TaggedByRules {
		t.Errorf("topic filter: %+v", items)
	}
}

func TestMentorItemsWindowAndPrune(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	mentorSeed(t, s)
	since := t0.AddDate(0, -1, 0)

	news, _, _ := s.Items(ctx, Filter{Since: since.Unix()})
	if len(news) != 2 { // the 2-year-old mentor post is outside the News window
		t.Errorf("window: %d items", len(news))
	}
	c, _ := s.Counts(ctx, since)
	if c.Unread != 2 {
		t.Errorf("counts in window: %+v", c)
	}
	if m, _, _ := s.Items(ctx, Filter{MentorsOnly: true}); len(m) != 2 {
		t.Errorf("mentor items: %d", len(m))
	}
	if m, _, _ := s.Items(ctx, Filter{Mentor: "carlini"}); len(m) != 0 {
		t.Errorf("other mentor: %d", len(m))
	}

	if err := s.SetTags(ctx, map[int64]Tags{}, TaggedByRules, "v"); err != nil {
		t.Fatal(err)
	}
	n, err := s.Prune(ctx, t0.AddDate(0, 1, 0))
	if err != nil || n != 1 {
		t.Errorf("prune: %d %v (want only the news item)", n, err)
	}
	if m, _, _ := s.Items(ctx, Filter{MentorsOnly: true}); len(m) != 2 {
		t.Errorf("mentor items pruned: %d left", len(m))
	}
}

func TestSourcesKeepMentorFields(t *testing.T) {
	s := open(t)
	mentorSeed(t, s)
	got, _ := s.Sources(context.Background(), true)
	var found bool
	for _, src := range got {
		if src.ID == "rehberger-posts" {
			found = src.Mentor == "rehberger" && src.Kind == "post"
		} else if src.Kind != "post" || src.Mentor != "" {
			t.Errorf("news source: %+v", src)
		}
	}
	if !found {
		t.Errorf("mentor source fields lost: %+v", got)
	}
}
