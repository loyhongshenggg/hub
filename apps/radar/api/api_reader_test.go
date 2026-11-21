package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/engine"
	"hub.localhost/apps/radar/api/internal/reader"
	"hub.localhost/apps/radar/api/internal/store"
)

func TestReaderAPI(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, "<html><head><title>T</title></head><body><article><h1>T</h1><p>%s</p></article></body></html>",
			strings.Repeat("Words about prompt injection and exfiltration. ", 60))
	}))
	defer site.Close()
	_, _, st := setup(t)
	ctx := context.Background()
	st.AddItems(ctx, "hn", []store.NewItem{{GUID: "r", URL: site.URL + "/post", Title: "Readable", Published: time.Now()}}, time.Now())
	items, _, _ := st.Items(ctx, store.Filter{Query: "Readable"})
	cur, _ := content.LoadCurriculum()
	mux := http.NewServeMux()
	(&readerAPI{store: st, eng: engine.New(cur), reader: &reader.Reader{Cache: st, Client: site.Client()}}).routes(mux)
	rs := httptest.NewServer(mux)
	defer rs.Close()

	var v reader.View
	if code := call(t, "GET", rs.URL+"/api/reader?ref=item:"+itoa(items[0].ID), "", &v); code != 200 || v.Mode != reader.ModeReader || v.Article == nil {
		t.Fatalf("item: %d %+v", code, v)
	}
	if code := call(t, "GET", rs.URL+"/api/reader?ref=step:agentic-engineering/agents-as-teammates/all-code-is-slop-talk", "", &v); code != 200 || v.Mode != reader.ModeVideo {
		t.Errorf("youtube step: %d %+v", code, v)
	}
	var e map[string]string
	for path, code := range map[string]int{
		"/api/reader?ref=nope":                    400,
		"/api/reader?ref=step:x/y/z":              404,
		"/api/reader?ref=item:99999":              404,
		"/api/reader?ref=capstone:ai-red-teaming": 404,
	} {
		if got := call(t, "GET", rs.URL+path, "", &e); got != code {
			t.Errorf("%s: %d, want %d (%v)", path, got, code, e)
		}
	}
}
