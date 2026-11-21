package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

func TestAvatars(t *testing.T) {
	var hits atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if strings.Contains(r.URL.Path, "html") {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<script>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("\x89PNG fake"))
	}))
	defer up.Close()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cur, err := content.ParseCurriculum(testCurriculum(t, up.URL))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	(&avatars{store: st, cur: cur, client: up.Client()}).routes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for range 2 {
		resp, err := http.Get(srv.URL + "/api/avatars/m1")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" || resp.Header.Get("Cache-Control") == "" {
			t.Errorf("avatar: %d %v", resp.StatusCode, resp.Header)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("fetched %d times, want 1 (cached)", hits.Load())
	}
	for path, code := range map[string]int{"/api/avatars/m2": 502, "/api/avatars/nobody": 404} {
		resp, _ := http.Get(srv.URL + path)
		resp.Body.Close()
		if resp.StatusCode != code {
			t.Errorf("%s: %d, want %d", path, resp.StatusCode, code)
		}
	}
	if cur.Mentors[0].Face != "/api/avatars/m1" {
		t.Errorf("face: %q", cur.Mentors[0].Face)
	}
}

// testCurriculum is a minimal curriculum whose mentors' pictures live on up.
func testCurriculum(t *testing.T, up string) fstest.MapFS {
	t.Helper()
	return fstest.MapFS{
		"skills.yaml": {Data: []byte("areas: [{id: a, name: A}]\ntopics:\n  - {id: x, name: X, area: a, keywords: [x]}\n")},
		"mentors.yaml": {Data: []byte("mentors:\n" +
			"  - {id: m1, name: M1, role: r, focus: f, area: a, links: [], feeds: [], avatar: '" + up + "/m1.png'}\n" +
			"  - {id: m2, name: M2, role: r, focus: f, area: a, links: [], feeds: [], avatar: '" + up + "/html'}\n")},
		"tracks/t.yaml": {Data: []byte("id: t\ntitle: T\ntagline: tag\narea: a\nlevel: intro\nmentors: [m1]\nstatus: outline\n" +
			"chapters:\n  - {id: c1, title: C1, difficulty: 2, topics: [x], outcomes: [do it]}\n")},
	}
}
