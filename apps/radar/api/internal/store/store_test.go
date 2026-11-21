package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/content"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "radar.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

var srcs = []content.Source{
	{ID: "hn", Name: "HN", Category: "swe", URL: "https://hn.example/rss"},
	{ID: "krebs", Name: "Krebs", Category: "security", URL: "https://krebs.example/feed"},
}

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

func seed(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	if err := s.SyncSources(ctx, srcs); err != nil {
		t.Fatal(err)
	}
	var hn, kr []NewItem
	for i := range 5 {
		hn = append(hn, NewItem{GUID: fmt.Sprint("hn", i), URL: fmt.Sprint("https://hn/", i), Title: fmt.Sprint("Go release ", i), Published: t0.Add(time.Duration(i) * time.Hour)})
	}
	kr = append(kr, NewItem{GUID: "k1", URL: "https://k/1", Title: "Ransomware 100% up", Summary: "a_b", Published: t0.Add(30 * time.Minute)})
	if n, err := s.AddItems(ctx, "hn", hn, t0); err != nil || n != 5 {
		t.Fatalf("add hn: %d %v", n, err)
	}
	if n, err := s.AddItems(ctx, "krebs", kr, t0); err != nil || n != 1 {
		t.Fatalf("add krebs: %d %v", n, err)
	}
}

func TestAddItemsDedupes(t *testing.T) {
	s := open(t)
	seed(t, s)
	n, err := s.AddItems(context.Background(), "hn", []NewItem{{GUID: "hn1", URL: "x", Title: "dup", Published: t0}}, t0)
	if err != nil || n != 0 {
		t.Fatalf("duplicate added: %d %v", n, err)
	}
}

func TestItemsFiltersAndPaging(t *testing.T) {
	s := open(t)
	seed(t, s)
	ctx := context.Background()

	all, cursor, err := s.Items(ctx, Filter{Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || all[0].Title != "Go release 4" || cursor == "" {
		t.Fatalf("page 1: %d items, first %q, cursor %q", len(all), all[0].Title, cursor)
	}
	var bt, bid int64
	fmt.Sscanf(cursor, "%d.%d", &bt, &bid)
	rest, cursor2, _ := s.Items(ctx, Filter{Limit: 4, BeforeTime: bt, BeforeID: bid})
	if len(rest) != 2 || cursor2 != "" || rest[0].Title != "Ransomware 100% up" || rest[1].Title != "Go release 0" {
		t.Fatalf("page 2: %+v cursor %q", rest, cursor2)
	}

	sec, _, _ := s.Items(ctx, Filter{Category: "security"})
	if len(sec) != 1 || sec[0].Source.Name != "Krebs" {
		t.Errorf("category filter: %+v", sec)
	}
	// LIKE wildcards in the query are literal.
	if got, _, _ := s.Items(ctx, Filter{Query: "100%"}); len(got) != 1 {
		t.Errorf("query 100%%: %d", len(got))
	}
	if got, _, _ := s.Items(ctx, Filter{Query: "a_b"}); len(got) != 1 {
		t.Errorf("query in summary: %d", len(got))
	}
	if got, _, _ := s.Items(ctx, Filter{Query: "%"}); len(got) != 1 {
		t.Errorf("query %% should match only the literal: %d", len(got))
	}
}

func TestReadSaveCountsPrune(t *testing.T) {
	s := open(t)
	seed(t, s)
	ctx := context.Background()
	items, _, _ := s.Items(ctx, Filter{})
	if err := s.SetRead(ctx, items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSaved(ctx, items[5].ID, true); err != nil { // oldest: "Go release 0"
		t.Fatal(err)
	}
	if err := s.SetRead(ctx, 9999, true); err != ErrNotFound {
		t.Errorf("missing item: %v", err)
	}
	c, err := s.Counts(ctx, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if c.Unread != 5 || c.UnreadByCategory["swe"] != 4 || c.UnreadByCategory["security"] != 1 || c.Saved != 1 || c.NewSinceVisit != 5 {
		t.Errorf("counts = %+v", c)
	}
	if unread, _, _ := s.Items(ctx, Filter{Unread: true}); len(unread) != 5 {
		t.Errorf("unread filter: %d", len(unread))
	}
	if n, _ := s.MarkAllRead(ctx, Filter{Category: "swe"}); n != 4 {
		t.Errorf("mark all swe read: %d", n)
	}
	if err := s.SetLastVisit(ctx, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	c, _ = s.Counts(ctx, time.Unix(0, 0))
	if c.Unread != 1 || c.NewSinceVisit != 0 {
		t.Errorf("after mark-all + visit: %+v", c)
	}

	// Prune keeps saved items even when old.
	if n, _ := s.Prune(ctx, t0.Add(10*time.Hour)); n != 5 {
		t.Errorf("pruned %d, want 5", n)
	}
	left, _, _ := s.Items(ctx, Filter{})
	if len(left) != 1 || !left[0].Saved {
		t.Errorf("after prune: %+v", left)
	}
}

func TestSyncSourcesDeactivatesAndKeepsValidators(t *testing.T) {
	s := open(t)
	seed(t, s)
	ctx := context.Background()
	if err := s.RecordFetch(ctx, "hn", FetchResult{At: t0, ETag: `"v1"`, LastModified: "Sun"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordFetch(ctx, "krebs", FetchResult{At: t0, Err: fmt.Errorf("boom")}); err != nil {
		t.Fatal(err)
	}
	// krebs removed; hn renamed (same URL keeps validators).
	if err := s.SyncSources(ctx, []content.Source{{ID: "hn", Name: "Hacker News", Category: "swe", URL: "https://hn.example/rss"}}); err != nil {
		t.Fatal(err)
	}
	active, _ := s.Sources(ctx, true)
	if len(active) != 1 || active[0].Name != "Hacker News" || active[0].ETag != `"v1"` || active[0].LastOKAt == nil {
		t.Fatalf("active = %+v", active)
	}
	all, _ := s.Sources(ctx, false)
	if len(all) != 2 || all[0].ID != "krebs" || all[0].Active || all[0].LastError != "boom" || all[0].Items != 1 {
		t.Fatalf("all = %+v", all)
	}
	// A new URL drops the validators.
	s.SyncSources(ctx, []content.Source{{ID: "hn", Name: "HN", Category: "swe", URL: "https://hn.example/new"}})
	active, _ = s.Sources(ctx, true)
	if active[0].ETag != "" {
		t.Errorf("etag kept after URL change: %q", active[0].ETag)
	}
}
