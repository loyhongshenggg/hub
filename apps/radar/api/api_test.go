package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

type fakeFetcher struct{ triggered int }

func (f *fakeFetcher) Trigger()                  { f.triggered++ }
func (f *fakeFetcher) Status() (bool, time.Time) { return false, time.Unix(1_800_000_000, 0) }

func setup(t *testing.T) (*httptest.Server, *fakeFetcher, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	st.SyncSources(ctx, []content.Source{
		{ID: "hn", Name: "HN", Category: "swe", URL: "https://hn/rss"},
		{ID: "krebs", Name: "Krebs", Category: "security", URL: "https://k/feed"},
	})
	now := time.Now()
	st.AddItems(ctx, "hn", []store.NewItem{{GUID: "1", URL: "https://hn/1", Title: "Go 1.28", Published: now.Add(-time.Hour)}}, now)
	st.AddItems(ctx, "krebs", []store.NewItem{{GUID: "2", URL: "https://k/2", Title: "Patch now", Published: now}}, now)
	f := &fakeFetcher{}
	mux := http.NewServeMux()
	(&api{store: st, fetcher: f, categories: []content.Category{{ID: "swe", Name: "SWE"}}}).routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, f, st
}

func call(t *testing.T, method, url, body string, out any) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: decode: %v", method, url, err)
		}
	}
	return resp.StatusCode
}

type newsResp struct {
	Items      []store.Item
	NextCursor string
}

func TestNewsFlow(t *testing.T) {
	srv, f, _ := setup(t)
	var news newsResp
	if code := call(t, "GET", srv.URL+"/api/news?category=all", "", &news); code != 200 || len(news.Items) != 2 || news.Items[0].Title != "Patch now" {
		t.Fatalf("news: %d %+v", code, news)
	}
	id := news.Items[0].ID
	if code := call(t, "POST", srv.URL+"/api/items/"+itoa(id)+"/saved", `{"saved":true}`, nil); code != 204 {
		t.Fatalf("save: %d", code)
	}
	if code := call(t, "POST", srv.URL+"/api/items/"+itoa(id)+"/read", `{"read":true}`, nil); code != 204 {
		t.Fatalf("read: %d", code)
	}
	call(t, "GET", srv.URL+"/api/news?saved=1", "", &news)
	if len(news.Items) != 1 || !news.Items[0].Saved || !news.Items[0].Read {
		t.Fatalf("saved filter: %+v", news.Items)
	}
	call(t, "GET", srv.URL+"/api/news?unread=1&category=swe", "", &news)
	if len(news.Items) != 1 || news.Items[0].Title != "Go 1.28" {
		t.Fatalf("unread swe: %+v", news.Items)
	}

	var sum struct {
		Counts store.Counts
		Fetch  struct{ LastRound *time.Time }
	}
	call(t, "GET", srv.URL+"/api/summary", "", &sum)
	if sum.Counts.Unread != 1 || sum.Counts.Saved != 1 || sum.Fetch.LastRound == nil {
		t.Errorf("summary: %+v", sum)
	}
	var marked struct{ Marked int }
	call(t, "POST", srv.URL+"/api/news/read-all?category=swe", "", &marked)
	if marked.Marked != 1 {
		t.Errorf("read-all: %+v", marked)
	}
	if code := call(t, "POST", srv.URL+"/api/refresh", "", nil); code != 202 || f.triggered != 1 {
		t.Errorf("refresh: %d triggered=%d", code, f.triggered)
	}
	if code := call(t, "POST", srv.URL+"/api/visit", "", nil); code != 204 {
		t.Errorf("visit: %d", code)
	}
}

func TestNewsErrors(t *testing.T) {
	srv, _, _ := setup(t)
	var e map[string]string
	for _, c := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/news?cursor=x", "", 400},
		{"GET", "/api/news?limit=0", "", 400},
		{"POST", "/api/items/abc/read", `{"read":true}`, 400},
		{"POST", "/api/items/1/read", `{"saved":true}`, 400},
		{"POST", "/api/items/999/read", `{"read":true}`, 404},
		{"GET", "/api/nope", "", 404},
	} {
		if code := call(t, c.method, srv.URL+c.path, c.body, &e); code != c.code {
			t.Errorf("%s %s: %d, want %d (%v)", c.method, c.path, code, c.code, e)
		}
	}
}

func TestSources(t *testing.T) {
	srv, _, _ := setup(t)
	var s struct {
		Categories []content.Category
		Sources    []store.Source
	}
	call(t, "GET", srv.URL+"/api/sources", "", &s)
	if len(s.Sources) != 2 || len(s.Categories) != 1 {
		t.Errorf("sources: %+v", s)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
