package content

import "testing"

func TestFeeds(t *testing.T) {
	s, err := LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	feeds, err := Feeds(s, c)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Source{}
	for _, f := range feeds {
		byID[f.ID] = f
	}
	if len(feeds) != len(s.Sources)+5 {
		t.Errorf("got %d feeds, want %d news + 5 mentor feeds", len(feeds), len(s.Sources))
	}
	r := byID["rehberger-videos"]
	if r.Mentor != "rehberger" || r.Kind != "video" || r.Category != "security" || len(r.Topics) == 0 {
		t.Errorf("rehberger-videos: %+v", r)
	}
	if byID["hn"].Kind != "post" || byID["hn"].Mentor != "" {
		t.Errorf("hn: %+v", byID["hn"])
	}

	s.Sources = append(s.Sources, Source{ID: "x", Name: "X", Category: "ai", URL: "https://x.dev/feed", Topics: []string{"nope"}})
	if _, err := Feeds(s, c); err == nil {
		t.Error("unknown topic accepted")
	}
}
