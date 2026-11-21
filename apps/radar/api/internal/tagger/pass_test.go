package tagger

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

func TestPass(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SyncSources(ctx, []content.Source{
		{ID: "hc-videos", Name: "Hammond · videos", Category: "security", URL: "https://yt/feed", Mentor: "hammond", Kind: "video", Topics: []string{"malware-analysis"}},
	})
	now := time.Now()
	st.AddItems(ctx, "hc-videos", []store.NewItem{
		{GUID: "1", URL: "https://www.youtube.com/watch?v=a", Title: "Reverse engineering a ransomware loader", Published: now},
		{GUID: "2", URL: "https://www.youtube.com/watch?v=b", Title: "I tried a new keyboard", Published: now},
	}, now)

	tg := newTagger(t)
	if n, err := tg.Pass(ctx, st); err != nil || n != 2 {
		t.Fatalf("pass: %d %v", n, err)
	}
	if n, _ := tg.Pass(ctx, st); n != 0 {
		t.Errorf("second pass retagged %d", n)
	}
	items, _, _ := st.Items(ctx, store.Filter{})
	for _, it := range items {
		if it.Type != "watch" || it.Minutes != 20 || it.TaggedBy != store.TaggedByRules || len(it.Topics) == 0 {
			t.Errorf("%q: %+v", it.Title, it)
		}
		if it.Title == "I tried a new keyboard" && !slices.Equal(it.Topics, []string{"malware-analysis"}) {
			t.Errorf("source default not used: %v", it.Topics)
		}
	}

	// Changing the rules (here: the version) retags.
	tg.Version += "x"
	if n, _ := tg.Pass(ctx, st); n != 2 {
		t.Errorf("version change retagged %d, want 2", n)
	}
}
