package reader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type memCache struct {
	articles map[string]memArticle
	frames   map[string]memFrame
}
type memArticle struct {
	a       *Article
	failure string
	at      time.Time
}
type memFrame struct {
	f  Framing
	at time.Time
}

func newCache() *memCache {
	return &memCache{articles: map[string]memArticle{}, frames: map[string]memFrame{}}
}
func (c *memCache) Article(_ context.Context, u string) (*Article, string, time.Time, bool, error) {
	a, ok := c.articles[u]
	return a.a, a.failure, a.at, ok, nil
}
func (c *memCache) PutArticle(_ context.Context, u string, a *Article, f string, at time.Time) error {
	c.articles[u] = memArticle{a, f, at}
	return nil
}
func (c *memCache) Frame(_ context.Context, h string) (Framing, time.Time, bool, error) {
	f, ok := c.frames[h]
	return f.f, f.at, ok, nil
}
func (c *memCache) PutFrame(_ context.Context, h string, f Framing, at time.Time) error {
	c.frames[h] = memFrame{f, at}
	return nil
}

var para = strings.Repeat("Prompt injection lets untrusted text steer a model's actions, and data exfiltration follows when the model can render links. ", 12)

func site(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-Frame-Options", "DENY")
		fmt.Fprintf(w, `<html><head><title>Exfil 101</title><meta name="author" content="J. R."></head><body>
			<nav>menu menu menu</nav>
			<article><h1>Exfil 101</h1>
			<p onclick="steal()">%s</p><script>alert(1)</script>
			<p><a href="/next">next post</a> <img src="/img.png" alt="diagram"></p>
			<iframe src="https://www.youtube.com/embed/dQw4w9WgXcQ"></iframe>
			<iframe src="https://evil.example/x"></iframe>
			<p>%s</p></article></body></html>`, para, para)
	})
	mux.HandleFunc("/app", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><div id="root"></div><script src="app.js"></script></body></html>`)
	})
	mux.HandleFunc("/lab", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'self'")
		fmt.Fprint(w, `<html><body>lab</body></html>`)
	})
	mux.HandleFunc("/dojo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Add("Set-Cookie", "session=x; HttpOnly; Path=/; SameSite=Lax")
		fmt.Fprint(w, `<html><body>challenges</body></html>`)
	})
	mux.HandleFunc("/talk", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><head><title>Forget About Memo</title></head><body><article><h1>Forget About Memo</h1>
			<p>Lauren Tan explains how the React Compiler memoises automatically, and why the Rules of React make it safe to do so.</p>
			<iframe src="https://www.youtube.com/embed/O_zcc6Xl12g"></iframe></article></body></html>`)
	})
	mux.HandleFunc("/paper.pdf", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		fmt.Fprint(w, "%PDF-1.7")
	})
	mux.HandleFunc("/gone", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func newReader(srv *httptest.Server) (*Reader, *memCache, *time.Time) {
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	c := newCache()
	return &Reader{Cache: c, Client: srv.Client(), Now: func() time.Time { return now }}, c, &now
}

func TestArticleReaderMode(t *testing.T) {
	srv, hits := site(t)
	r, c, now := newReader(srv)
	ctx := context.Background()
	v, err := r.View(ctx, srv.URL+"/post", "read")
	if err != nil {
		t.Fatal(err)
	}
	if v.Mode != ModeReader || v.Article == nil {
		t.Fatalf("mode %s, note %q", v.Mode, v.Note)
	}
	a := v.Article
	if unquote(`"Embrace The Red"`) != "Embrace The Red" || unquote("“x”") != "x" || unquote(`"a" and "b"`) != `a" and "b` {
		t.Error("unquote")
	}
	if a.Title != "Exfil 101" || a.Words < minWords || a.Minutes < 1 {
		t.Errorf("article meta: %+v", a)
	}
	for _, bad := range []string{"<script", "onclick", "alert(1)", "<iframe", "evil.example/x\"", "menu menu"} {
		if strings.Contains(a.HTML, bad) {
			t.Errorf("sanitised HTML still has %q", bad)
		}
	}
	for _, good := range []string{`data-youtube="dQw4w9WgXcQ"`, `href="` + srv.URL + `/next"`, `src="` + srv.URL + `/img.png"`,
		`target="_blank"`, `noreferrer`} {
		if !strings.Contains(a.HTML, good) {
			t.Errorf("sanitised HTML lacks %q:\n%s", good, a.HTML)
		}
	}
	// Cached: no second fetch; the framing answer came along for free.
	r.View(ctx, srv.URL+"/post", "read")
	if hits.Load() != 1 {
		t.Errorf("fetched %d times", hits.Load())
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	host = host[:strings.LastIndex(host, ":")]
	if f, ok := c.frames[host]; !ok || f.f != FrameBlocked {
		t.Errorf("frame policy not recorded as blocked: %+v %v", f, ok)
	}
	// After the TTL it refetches.
	*now = now.Add(articleTTL + time.Hour)
	r.View(ctx, srv.URL+"/post", "read")
	if hits.Load() != 2 {
		t.Errorf("stale article not refetched (%d)", hits.Load())
	}
}

func TestModes(t *testing.T) {
	srv, _ := site(t)
	ctx := context.Background()
	cases := []struct {
		url, typ string
		mode     Mode
		note     string
	}{
		{"https://www.youtube.com/watch?v=O_zcc6Xl12g&t=1m30s", "watch", ModeVideo, ""},
		{srv.URL + "/lab", "lab", ModeOutside, "new tab"},
		{srv.URL + "/dojo", "lab", ModeOutside, "signed in"},
		{srv.URL + "/dojo", "read", ModeLive, "Shown live"}, // reading needs no sign-in
		{srv.URL + "/talk", "watch", ModeReader, ""},
		{srv.URL + "/app", "read", ModeLive, "Shown live"},
		{srv.URL + "/paper.pdf", "read", ModeLive, "Not an article"},
		{srv.URL + "/gone", "read", ModeOutside, "HTTP 404"},
	}
	for _, c := range cases {
		r, _, _ := newReader(srv)
		v, err := r.View(ctx, c.url, c.typ)
		if err != nil {
			t.Fatalf("%s: %v", c.url, err)
		}
		if v.Mode != c.mode || !strings.Contains(v.Note, c.note) {
			t.Errorf("%s (%s): mode %s note %q, want %s with %q", c.url, c.typ, v.Mode, v.Note, c.mode, c.note)
		}
	}
	r, _, _ := newReader(srv)
	if v, _ := r.View(ctx, srv.URL+"/dojo", "lab"); v.Embed == "" {
		t.Error("frameable lab without embedded sign-in should still offer a preview")
	}
	if v, _ := r.View(ctx, srv.URL+"/talk", "watch"); v.Article == nil || !strings.Contains(v.Article.HTML, "data-youtube") {
		t.Errorf("talk page should read with its video: %+v", v)
	}
	if v, _ := r.View(ctx, "https://youtu.be/O_zcc6Xl12g", "read"); v.Embed != "https://www.youtube-nocookie.com/embed/O_zcc6Xl12g" {
		t.Errorf("youtu.be embed: %q", v.Embed)
	}
	if _, err := r.View(ctx, "javascript:alert(1)", "read"); err == nil {
		t.Error("javascript: link accepted")
	}
}

func TestRules(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.youtube.com/watch?v=O_zcc6Xl12g&t=90": "https://www.youtube-nocookie.com/embed/O_zcc6Xl12g?start=90",
		"https://youtube.com/shorts/O_zcc6Xl12g":           "https://www.youtube-nocookie.com/embed/O_zcc6Xl12g",
		"https://m.youtube.com/watch?v=O_zcc6Xl12g&t=1h2m": "https://www.youtube-nocookie.com/embed/O_zcc6Xl12g?start=3720",
		"https://www.youtube.com/@embracethered":           "",
		"https://www.youtube.com/watch?v=short":            "",
	} {
		u, _ := url.Parse(in)
		got, _ := YouTubeEmbed(u)
		if got != want {
			t.Errorf("%s → %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"https://arxiv.org/abs/2302.12173":       "https://arxiv.org/html/2302.12173",
		"https://arxiv.org/pdf/2302.12173v2.pdf": "https://arxiv.org/html/2302.12173v2",
		"https://example.com/abs/2302.12173":     "https://example.com/abs/2302.12173",
	} {
		u, _ := url.Parse(in)
		if got := readableURL(u); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
	h := http.Header{}
	if !Frameable(h) {
		t.Error("no headers should be frameable")
	}
	h.Set("Content-Security-Policy", "frame-ancestors *")
	if !Frameable(h) {
		t.Error("frame-ancestors * should be frameable")
	}
	h.Set("Content-Security-Policy", "script-src 'self'; frame-ancestors 'none'")
	if Frameable(h) {
		t.Error("frame-ancestors 'none' should block")
	}
	h = http.Header{"X-Frame-Options": {"SAMEORIGIN"}}
	if Frameable(h) {
		t.Error("X-Frame-Options should block")
	}
	if !SessionEmbeddable(http.Header{}) || !SessionEmbeddable(http.Header{"Set-Cookie": {"a=1; Secure; SameSite=None"}}) {
		t.Error("no cookies / SameSite=None should embed")
	}
	if SessionEmbeddable(http.Header{"Set-Cookie": {"a=1; SameSite=None; Secure", "session=x; SameSite=Lax"}}) {
		t.Error("a Lax session cookie shouldn't embed")
	}
}

func TestSafeClientRefusesLocal(t *testing.T) {
	srv, _ := site(t)
	_, err := SafeClient().Get(srv.URL + "/post") // 127.0.0.1
	if err == nil || !strings.Contains(err.Error(), "private or local") {
		t.Errorf("loopback fetch: %v", err)
	}
	for _, addr := range []string{"http://10.1.2.3/", "http://[::1]/", "http://169.254.169.254/latest/meta-data"} {
		c := SafeClient()
		c.Timeout = 2 * time.Second
		if _, err := c.Get(addr); err == nil || !strings.Contains(err.Error(), "private or local") {
			t.Errorf("%s: %v", addr, err)
		}
	}
}
