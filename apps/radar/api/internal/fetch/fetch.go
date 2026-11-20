// Package fetch pulls RSS/Atom feeds into the store: conditional GETs
// (ETag / Last-Modified), at most a few sources at once, on a timer and on
// demand.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mmcdole/gofeed"

	"hub.localhost/apps/radar/api/internal/store"
)

type Fetcher struct {
	Store    *store.Store
	Client   *http.Client
	Log      *slog.Logger
	Interval time.Duration // between full rounds; default 30m
	// MaxPerFetch caps items taken from one feed per fetch (newest first),
	// so a first import of a 1,000-item feed doesn't flood the list.
	MaxPerFetch int
	// Retention drops unsaved items older than this. Default 120 days.
	Retention time.Duration
	// OnNew is called after a round that added items.
	OnNew func(added int)

	mu      sync.Mutex
	running bool
	last    time.Time
	trigger chan struct{}
}

const userAgent = "hub-radar/0.1 (personal feed reader)"

func (f *Fetcher) defaults() {
	if f.Client == nil {
		f.Client = &http.Client{Timeout: 20 * time.Second}
	}
	if f.Log == nil {
		f.Log = slog.Default()
	}
	if f.Interval == 0 {
		f.Interval = 30 * time.Minute
	}
	if f.MaxPerFetch == 0 {
		f.MaxPerFetch = 50
	}
	if f.Retention == 0 {
		f.Retention = 120 * 24 * time.Hour
	}
	if f.trigger == nil {
		f.trigger = make(chan struct{}, 1)
	}
}

// Run fetches now, then every Interval and whenever Trigger is called.
func (f *Fetcher) Run(ctx context.Context) {
	f.mu.Lock()
	f.defaults()
	f.mu.Unlock()
	t := time.NewTicker(f.Interval)
	defer t.Stop()
	for {
		f.Round(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-f.trigger:
		}
	}
}

// Trigger asks Run for a round now (no-op if one is queued).
func (f *Fetcher) Trigger() {
	f.mu.Lock()
	f.defaults()
	f.mu.Unlock()
	select {
	case f.trigger <- struct{}{}:
	default:
	}
}

// Status reports whether a round is running and when the last one ended.
func (f *Fetcher) Status() (running bool, last time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running, f.last
}

// Round fetches every active source once (4 at a time) and prunes.
func (f *Fetcher) Round(ctx context.Context) {
	f.mu.Lock()
	f.defaults()
	if f.running {
		f.mu.Unlock()
		return
	}
	f.running = true
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.running, f.last = false, time.Now()
		f.mu.Unlock()
	}()

	srcs, err := f.Store.Sources(ctx, true)
	if err != nil {
		f.Log.Error("list sources", "error", err)
		return
	}
	var (
		wg    sync.WaitGroup
		sem   = make(chan struct{}, 4)
		mu    sync.Mutex
		added int
	)
	for _, src := range srcs {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			n, err := f.fetchOne(ctx, src)
			res := store.FetchResult{At: time.Now(), Err: err}
			if err != nil {
				f.Log.Warn("fetch", "source", src.ID, "error", err)
			}
			if rerr := f.Store.RecordFetch(ctx, src.ID, res); rerr != nil {
				f.Log.Error("record fetch", "source", src.ID, "error", rerr)
			}
			mu.Lock()
			added += n
			mu.Unlock()
		})
	}
	wg.Wait()
	if n, err := f.Store.Prune(ctx, time.Now().Add(-f.Retention)); err == nil && n > 0 {
		f.Log.Info("pruned old items", "count", n)
	}
	f.Log.Info("fetch round done", "sources", len(srcs), "new", added)
	if added > 0 && f.OnNew != nil {
		f.OnNew(added)
	}
}

func (f *Fetcher) fetchOne(ctx context.Context, src store.Source) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	if src.ETag != "" {
		req.Header.Set("If-None-Match", src.ETag)
	}
	if src.LastModified != "" {
		req.Header.Set("If-Modified-Since", src.LastModified)
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return 0, f.Store.RecordFetch(ctx, src.ID, store.FetchResult{At: time.Now()})
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %s", resp.Status)
	}
	feed, err := gofeed.NewParser().Parse(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return 0, fmt.Errorf("parse feed: %w", err)
	}
	items := Normalize(feed, src.URL, time.Now())
	if len(items) > f.MaxPerFetch {
		items = items[:f.MaxPerFetch]
	}
	n, err := f.Store.AddItems(ctx, src.ID, items, time.Now())
	if err != nil {
		return 0, err
	}
	// Save validators only after the items are stored.
	return n, f.Store.RecordFetch(ctx, src.ID, store.FetchResult{
		At: time.Now(), ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified"),
	})
}

var (
	tagRe   = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRe = regexp.MustCompile(`[\s\p{Z}]+`) // \p{Z} covers &nbsp; (U+00A0)
)

// Clean turns feed HTML into one line of plain text, cut to max runes.
func Clean(s string, max int) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)[:max]
		s = strings.TrimRight(string(r), " .,;:") + "…"
	}
	return s
}

var (
	hnMeta     = regexp.MustCompile(`^Article URL: \S+ Comments URL: \S+ Points: (\d+) # Comments: (\d+)$`)
	wpFooter   = regexp.MustCompile(`\s*The post .{1,300} appeared first on .{1,120}\.$`)
	ellipsisRe = regexp.MustCompile(`\s*\[(…|\.\.\.)\]$`)
)

// Tidy removes feed boilerplate from a cleaned summary: Hacker News
// metadata becomes "333 points · 213 comments", bare "Comments" links,
// WordPress "appeared first on" footers and trailing "[…]" go.
func Tidy(s string, max int) string {
	if m := hnMeta.FindStringSubmatch(s); m != nil {
		return m[1] + " points · " + m[2] + " comments"
	}
	if s == "Comments" {
		return ""
	}
	s = wpFooter.ReplaceAllString(s, "")
	if t := ellipsisRe.ReplaceAllString(s, ""); t != s {
		s = t
		if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "!") && !strings.HasSuffix(s, "?") {
			s += "…"
		}
	}
	return Clean(s, max)
}

// Normalize converts parsed feed items, newest first. Items without a usable
// link are dropped; relative links resolve against the feed URL.
func Normalize(feed *gofeed.Feed, feedURL string, now time.Time) []store.NewItem {
	base, _ := url.Parse(feedURL)
	var out []store.NewItem
	for _, it := range feed.Items {
		link := strings.TrimSpace(it.Link)
		if link == "" && len(it.Links) > 0 {
			link = it.Links[0]
		}
		if link == "" && strings.HasPrefix(it.GUID, "http") {
			link = it.GUID
		}
		u, err := url.Parse(link)
		if link == "" || err != nil {
			continue
		}
		if base != nil {
			u = base.ResolveReference(u)
		}
		if u.Scheme != "https" && u.Scheme != "http" {
			continue // no javascript: or data: links in the UI
		}
		guid := strings.TrimSpace(it.GUID)
		if guid == "" {
			guid = u.String()
		}
		title := Clean(it.Title, 300)
		if title == "" {
			title = u.Host
		}
		summary := it.Description
		if summary == "" {
			summary = it.Content
		}
		pub := now
		switch {
		case it.PublishedParsed != nil:
			pub = *it.PublishedParsed
		case it.UpdatedParsed != nil:
			pub = *it.UpdatedParsed
		}
		if pub.After(now) {
			pub = now
		}
		out = append(out, store.NewItem{GUID: guid, URL: u.String(), Title: title, Summary: Tidy(Clean(summary, 400), 280), Published: pub})
	}
	slices.SortStableFunc(out, func(a, b store.NewItem) int { return b.Published.Compare(a.Published) })
	return out
}
