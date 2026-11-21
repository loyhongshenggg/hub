// Package reader shows resources inside Radar instead of sending the
// learner away. Each URL gets the best mode for it:
//
//   - video: YouTube, as a youtube-nocookie.com embed;
//   - reader: articles, fetched once, extracted with go-readability,
//     sanitised with bluemonday and cached;
//   - live: pages that aren't articles, on sites that allow framing;
//   - outside: everything else opens in a new tab. Labs whose sign-in
//     cookie can't work inside another site (SameSite=Lax/Strict) open
//     outside too, with an optional in-app preview.
//
// Framing is checked per host from the site's own headers and remembered.
package reader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html/charset"
)

type Mode string

const (
	ModeReader  Mode = "reader"
	ModeVideo   Mode = "video"
	ModeLive    Mode = "live"
	ModeOutside Mode = "outside"
)

// Framing is what a host allows inside another site's iframe.
type Framing int

const (
	FrameBlocked   Framing = iota // X-Frame-Options / frame-ancestors
	FrameOK                       // frameable, and its cookies work there
	FrameNoSession                // frameable, but sign-in won't stick (SameSite cookies)
)

// View is how the UI should show a resource.
type View struct {
	Mode    Mode     `json:"mode"`
	URL     string   `json:"url"`             // the original link ("open outside")
	Embed   string   `json:"embed,omitempty"` // iframe src for video and live; a preview for outside
	Article *Article `json:"article,omitempty"`
	Note    string   `json:"note,omitempty"` // why this mode, when it isn't obvious
}

// Article is extracted, sanitised reading content.
type Article struct {
	Title     string     `json:"title"`
	Byline    string     `json:"byline,omitempty"`
	Site      string     `json:"site,omitempty"`
	Published *time.Time `json:"published,omitempty"`
	Image     string     `json:"image,omitempty"`
	HTML      string     `json:"html"`
	Words     int        `json:"words"`
	Minutes   int        `json:"minutes"`
}

// Cache keeps extracted articles and per-host framing answers.
type Cache interface {
	Article(ctx context.Context, url string) (a *Article, failure string, at time.Time, found bool, err error)
	PutArticle(ctx context.Context, url string, a *Article, failure string, at time.Time) error
	Frame(ctx context.Context, host string) (f Framing, at time.Time, found bool, err error)
	PutFrame(ctx context.Context, host string, f Framing, at time.Time) error
}

const (
	articleTTL = 30 * 24 * time.Hour
	failureTTL = 24 * time.Hour
	frameTTL   = 14 * 24 * time.Hour
	maxBody    = 5 << 20
	minWords   = 80 // less than this isn't an article (JS app shells, login walls)
	minVideo   = 20 // ...unless it carries a video (a talk page)
	userAgent  = "Mozilla/5.0 (compatible; RadarReader/1.0; personal reading app)"
)

type Reader struct {
	Cache  Cache
	Client *http.Client // use SafeClient() outside tests
	Log    *slog.Logger
	Now    func() time.Time

	mu       sync.Mutex
	inflight map[string]*call
}

type call struct {
	done chan struct{}
	page *page
	err  error
}

func (r *Reader) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// View decides how to show rawURL. typ is the content type from the
// curriculum or the tagger: read | watch | lab | tool | walkthrough.
func (r *Reader) View(ctx context.Context, rawURL, typ string) (View, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return View{}, fmt.Errorf("not a web link: %q", rawURL)
	}
	v := View{URL: u.String()}
	if embed, ok := YouTubeEmbed(u); ok {
		v.Mode, v.Embed = ModeVideo, embed
		return v, nil
	}
	if typ == "lab" || typ == "tool" {
		return r.liveOrOutside(ctx, v, u, "", true)
	}
	a, failure, err := r.article(ctx, readableURL(u))
	if err != nil {
		return View{}, err
	}
	if a != nil {
		v.Mode, v.Article = ModeReader, a
		return v, nil
	}
	return r.liveOrOutside(ctx, v, u, failure, false)
}

// liveOrOutside embeds the page if its site allows it. An interactive page
// (a lab) also needs its sign-in to work embedded.
func (r *Reader) liveOrOutside(ctx context.Context, v View, u *url.URL, failure string, interactive bool) (View, error) {
	f, err := r.framing(ctx, u)
	if err != nil {
		return View{}, err
	}
	host := u.Hostname()
	switch {
	case f == FrameOK || (f == FrameNoSession && !interactive):
		v.Mode, v.Embed = ModeLive, u.String()
		if failure != "" {
			v.Note = "Shown live: " + failure
		}
	case f == FrameNoSession:
		v.Mode, v.Embed = ModeOutside, u.String()
		v.Note = host + " needs you signed in, and that only works in its own tab. Preview it here, or open it to do the lab."
	default:
		v.Mode = ModeOutside
		v.Note = host + " doesn't allow being shown inside other apps, so it opens in a new tab."
		if failure != "" {
			v.Note = failure + " — and " + v.Note
		}
	}
	return v, nil
}

// article returns a cached or freshly extracted article; nil with a reason
// when the page isn't readable.
func (r *Reader) article(ctx context.Context, link string) (*Article, string, error) {
	if a, failure, at, found, err := r.Cache.Article(ctx, link); err != nil {
		return nil, "", err
	} else if found {
		age := r.now().Sub(at)
		if a != nil && age < articleTTL {
			return a, "", nil
		}
		if a == nil && age < failureTTL {
			return nil, failure, nil
		}
	}
	p, err := r.fetch(ctx, link)
	now := r.now()
	if err != nil {
		failure := "Couldn't load the page (" + shortErr(err) + ")"
		return nil, failure, r.Cache.PutArticle(ctx, link, nil, failure, now)
	}
	if u, perr := url.Parse(link); perr == nil {
		_ = r.Cache.PutFrame(ctx, u.Hostname(), p.framing, now)
	}
	a, failure := extract(p)
	return a, failure, r.Cache.PutArticle(ctx, link, a, failure, now)
}

func (r *Reader) framing(ctx context.Context, u *url.URL) (Framing, error) {
	host := u.Hostname()
	if f, at, found, err := r.Cache.Frame(ctx, host); err != nil {
		return FrameBlocked, err
	} else if found && r.now().Sub(at) < frameTTL {
		return f, nil
	}
	p, err := r.fetch(ctx, u.String())
	if err != nil {
		return FrameBlocked, nil // unknown: don't cache, open outside this time
	}
	return p.framing, r.Cache.PutFrame(ctx, host, p.framing, r.now())
}

// page is a fetched document.
type page struct {
	url         *url.URL // after redirects
	contentType string
	body        []byte
	framing     Framing
}

// fetch GETs a page once even if several callers ask at the same time
// (opening a step while it is being prefetched).
func (r *Reader) fetch(ctx context.Context, link string) (*page, error) {
	r.mu.Lock()
	if r.inflight == nil {
		r.inflight = map[string]*call{}
	}
	if c, ok := r.inflight[link]; ok {
		r.mu.Unlock()
		select {
		case <-c.done:
			return c.page, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &call{done: make(chan struct{})}
	r.inflight[link] = c
	r.mu.Unlock()

	c.page, c.err = r.get(ctx, link)
	close(c.done)
	r.mu.Lock()
	delete(r.inflight, link)
	r.mu.Unlock()
	return c.page, c.err
}

func (r *Reader) get(ctx context.Context, link string) (*page, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	resp, err := r.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	p := &page{url: resp.Request.URL, contentType: resp.Header.Get("Content-Type"), framing: framingOf(resp.Header)}
	mt, _, _ := mime.ParseMediaType(p.contentType)
	if mt != "text/html" && mt != "application/xhtml+xml" {
		return p, nil // headers are all we need
	}
	body, err := charset.NewReader(io.LimitReader(resp.Body, maxBody), p.contentType)
	if err != nil {
		return nil, err
	}
	if p.body, err = io.ReadAll(body); err != nil {
		return nil, err
	}
	return p, nil
}

func framingOf(h http.Header) Framing {
	switch {
	case !Frameable(h):
		return FrameBlocked
	case !SessionEmbeddable(h):
		return FrameNoSession
	}
	return FrameOK
}

// SessionEmbeddable says whether the cookies a page sets would work inside
// a cross-site iframe: only SameSite=None cookies are sent there (Lax is the
// browser default). No cookies at all is fine.
func SessionEmbeddable(h http.Header) bool {
	for _, c := range h.Values("Set-Cookie") {
		if !strings.Contains(strings.ToLower(strings.ReplaceAll(c, " ", "")), "samesite=none") {
			return false
		}
	}
	return true
}

// Frameable reads a response's framing policy: X-Frame-Options blocks;
// CSP frame-ancestors blocks unless it allows any origin.
func Frameable(h http.Header) bool {
	if h.Get("X-Frame-Options") != "" {
		return false
	}
	for _, csp := range h.Values("Content-Security-Policy") {
		for _, dir := range strings.Split(csp, ";") {
			f := strings.Fields(dir)
			if len(f) == 0 || !strings.EqualFold(f[0], "frame-ancestors") {
				continue
			}
			for _, src := range f[1:] {
				if src == "*" || src == "https:" || src == "http:" {
					return true
				}
			}
			return false
		}
	}
	return true
}

func shortErr(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		err = uerr.Err
	}
	s := err.Error()
	if len(s) > 80 {
		s = s[:80] + "…"
	}
	return s
}
