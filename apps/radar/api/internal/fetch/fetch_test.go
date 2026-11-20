package fetch

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mmcdole/gofeed"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

const rss = `<?xml version="1.0"?>
<rss version="2.0"><channel><title>T</title>
<item><title>Old &amp; gold</title><link>https://ex.com/old</link><guid>g-old</guid>
  <pubDate>Mon, 02 Nov 2026 10:00:00 GMT</pubDate><description>&lt;p&gt;Hello &lt;b&gt;world&lt;/b&gt;&lt;/p&gt;</description></item>
<item><title>New</title><link>/relative/new</link><pubDate>Tue, 03 Nov 2026 10:00:00 GMT</pubDate></item>
<item><title>Evil</title><link>javascript:alert(1)</link></item>
<item><title>Future</title><link>https://ex.com/future</link><pubDate>Fri, 01 Jan 2100 00:00:00 GMT</pubDate></item>
</channel></rss>`

const atom = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom"><title>A</title>
<entry><title>Atom entry</title><link href="https://ex.org/a1"/><id>tag:ex.org,2026:a1</id>
<updated>2026-11-04T09:00:00Z</updated><summary>Short</summary></entry>
</feed>`

func TestNormalize(t *testing.T) {
	feed, err := gofeed.NewParser().ParseString(rss)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	items := Normalize(feed, "https://ex.com/feed.xml", now)
	if len(items) != 3 {
		t.Fatalf("got %d items (javascript: link must be dropped): %+v", len(items), items)
	}
	// Newest first; the future date is clamped to now.
	if items[0].URL != "https://ex.com/future" || !items[0].Published.Equal(now) {
		t.Errorf("first = %+v", items[0])
	}
	if items[1].URL != "https://ex.com/relative/new" || items[1].GUID != "https://ex.com/relative/new" {
		t.Errorf("relative link / guid fallback: %+v", items[1])
	}
	if items[2].Title != "Old & gold" || items[2].Summary != "Hello world" || items[2].GUID != "g-old" {
		t.Errorf("cleaning: %+v", items[2])
	}
}

func TestClean(t *testing.T) {
	if got := Clean("<p>a\n\n  b</p>&nbsp;c", 100); got != "a b c" {
		t.Errorf("Clean = %q", got)
	}
	if got := Clean(strings.Repeat("word ", 100), 12); got != "word word wo…" {
		t.Errorf("truncate = %q", got)
	}
}

func TestTidy(t *testing.T) {
	for in, want := range map[string]string{
		"Article URL: https://a.b/x Comments URL: https://news.ycombinator.com/item?id=1 Points: 333 # Comments: 213": "333 points · 213 comments",
		"Comments":                             "",
		"Breach at a vendor exposed data. […]": "Breach at a vendor exposed data.",
		"Big news here. [...]":                 "Big news here.",
		"Copilot gets agents. The post Copilot gets agents appeared first on The GitHub Blog.": "Copilot gets agents.",
		"Plain summary": "Plain summary",
	} {
		if got := Tidy(in, 280); got != want {
			t.Errorf("Tidy(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRoundConditionalGetAndErrors(t *testing.T) {
	var rssHits, cond atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/rss", func(w http.ResponseWriter, r *http.Request) {
		rssHits.Add(1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			cond.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if !strings.HasPrefix(r.UserAgent(), "hub-radar/") {
			t.Errorf("user agent %q", r.UserAgent())
		}
		w.Header().Set("ETag", `"v1"`)
		io.WriteString(w, rss)
	})
	mux.HandleFunc("/atom", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, atom) })
	mux.HandleFunc("/broken", func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, "<html>not a feed") })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.SyncSources(ctx, []content.Source{
		{ID: "rss", Name: "RSS", Category: "swe", URL: srv.URL + "/rss"},
		{ID: "atom", Name: "Atom", Category: "ai", URL: srv.URL + "/atom"},
		{ID: "broken", Name: "Broken", Category: "security", URL: srv.URL + "/broken"},
		{ID: "gone", Name: "Gone", Category: "security", URL: "http://127.0.0.1:1/feed"},
	})
	var news []int
	f := &Fetcher{Store: st, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), OnNew: func(n int) { news = append(news, n) }}

	f.Round(ctx)
	items, _, _ := st.Items(ctx, store.Filter{})
	if len(items) != 4 {
		t.Fatalf("items after round 1: %d", len(items))
	}
	srcs, _ := st.Sources(ctx, true)
	status := map[string]store.Source{}
	for _, s := range srcs {
		status[s.ID] = s
	}
	if status["rss"].LastError != "" || status["rss"].LastOKAt == nil || status["atom"].Items != 1 {
		t.Errorf("rss/atom status: %+v %+v", status["rss"], status["atom"])
	}
	if !strings.Contains(status["broken"].LastError, "parse feed") || status["gone"].LastError == "" {
		t.Errorf("error status: broken=%q gone=%q", status["broken"].LastError, status["gone"].LastError)
	}

	f.Round(ctx) // second round: 304 for rss, no new items
	if cond.Load() != 1 || rssHits.Load() != 2 {
		t.Errorf("conditional GET: hits=%d conditional=%d", rssHits.Load(), cond.Load())
	}
	if len(news) != 1 || news[0] != 4 {
		t.Errorf("OnNew calls = %v", news)
	}
	if running, last := f.Status(); running || last.IsZero() {
		t.Errorf("status running=%v last=%v", running, last)
	}
}
