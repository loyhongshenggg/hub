package store

import (
	"context"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/internal/reader"
)

var _ reader.Cache = (*Store)(nil)

func TestReaderCache(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	at := time.Unix(1_800_000_000, 0)
	if _, _, _, found, err := s.Article(ctx, "https://x/a"); found || err != nil {
		t.Fatalf("empty cache: %v %v", found, err)
	}
	s.PutArticle(ctx, "https://x/a", &reader.Article{Title: "A", HTML: "<p>a</p>", Words: 300}, "", at)
	s.PutArticle(ctx, "https://x/b", nil, "No article found", at)
	if a, _, got, found, _ := s.Article(ctx, "https://x/a"); !found || a == nil || a.Title != "A" || !got.Equal(at) {
		t.Errorf("article: %+v %v", a, found)
	}
	if a, failure, _, found, _ := s.Article(ctx, "https://x/b"); !found || a != nil || failure != "No article found" {
		t.Errorf("failure: %+v %q", a, failure)
	}
	s.PutFrame(ctx, "pwn.college", reader.FrameOK, at)
	s.PutFrame(ctx, "pwn.college", reader.FrameNoSession, at.Add(time.Hour))
	if f, got, found, _ := s.Frame(ctx, "pwn.college"); !found || f != reader.FrameNoSession || !got.Equal(at.Add(time.Hour)) {
		t.Errorf("frame: %v %v %v", f, got, found)
	}
	if _, _, err := s.ItemLink(ctx, 999); err != ErrNotFound {
		t.Errorf("missing item: %v", err)
	}
}
