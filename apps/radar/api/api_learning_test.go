package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/engine"
	"hub.localhost/apps/radar/api/internal/store"
)

func learningSetup(t *testing.T) (*httptest.Server, *content.Curriculum, *time.Time) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	cur, err := content.LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	(&api{store: st, fetcher: &fakeFetcher{}, curriculum: cur}).routes(mux)
	(&learning{store: st, eng: engine.New(cur), now: func() time.Time { return now }}).routes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, cur, &now
}

func TestLearningFlow(t *testing.T) {
	srv, cur, now := learningSetup(t)
	u := srv.URL + "/api/learning"

	var d engine.Dashboard
	if code := call(t, "GET", u, "", &d); code != 200 || !d.NeedsAssessment || d.Continue == nil {
		t.Fatalf("dashboard: %d %+v", code, d)
	}
	if code := call(t, "PUT", u+"/assessment", `{"answers":{"linux":"solid","prompt-injection":"new"}}`, nil); code != 204 {
		t.Fatalf("assess: %d", code)
	}
	var as struct {
		Done   bool
		Topics []engine.AssessmentTopic
	}
	call(t, "GET", u+"/assessment", "", &as)
	if !as.Done || len(as.Topics) != len(cur.Topics) {
		t.Errorf("assessment: %+v", as)
	}
	for _, at := range as.Topics {
		if at.ID == "linux" && at.Level != "solid" {
			t.Errorf("linux level %q", at.Level)
		}
	}

	// Do the first step of the recommended chapter.
	call(t, "GET", u, "", &d)
	rec, firstRef := *d.Continue, d.Continue.Next.Ref // copies: decoding into d again reuses its pointers
	var resp struct{ Chapter *engine.ChapterView }
	body := `{"ref":"` + firstRef + `","rating":"easy","worth":true}`
	if code := call(t, "POST", u+"/done", body, &resp); code != 200 || resp.Chapter == nil || resp.Chapter.StepsDone != 1 {
		t.Fatalf("done: %d %+v", code, resp.Chapter)
	}
	if !resp.Chapter.OfferSkip {
		t.Error("too easy should offer skip")
	}
	ch := "/tracks/" + rec.Track + "/chapters/" + rec.Chapter
	var cv engine.ChapterView
	if code := call(t, "POST", u+ch+"/skip-to-practice", "", &cv); code != 200 || cv.Next == nil {
		t.Fatalf("skip: %d %+v", code, cv)
	}
	if cv.Next.Kind == "practice" {
		call(t, "POST", u+"/done", `{"ref":"`+cv.Next.Ref+`"}`, nil)
	}
	met := "["
	for i := range cv.Outcomes {
		if i > 0 {
			met += ","
		}
		met += "true"
	}
	met += "]"
	var cp struct {
		Result  engine.CheckpointResult
		Chapter engine.ChapterView
	}
	if code := call(t, "POST", u+ch+"/checkpoint", `{"met":`+met+`,"rating":"right","worth":true}`, &cp); code != 200 || !cp.Result.Completed || !cp.Chapter.Completed {
		t.Fatalf("checkpoint: %d %+v", code, cp)
	}

	// Now the dashboard moves on and the week counts the minutes.
	*now = now.Add(time.Hour)
	d = engine.Dashboard{}
	call(t, "GET", u, "", &d)
	if d.Continue == nil || (d.Continue.Track == rec.Track && d.Continue.Chapter == rec.Chapter) {
		t.Errorf("dashboard didn't move on: %+v", d.Continue)
	}
	if d.Week.Minutes == 0 {
		t.Error("week minutes not counted")
	}
	var tv engine.TrackView
	if code := call(t, "GET", u+"/tracks/"+rec.Track, "", &tv); code != 200 || tv.Completed != 1 || tv.Chapters[0].Status != "done" {
		t.Errorf("track: %d completed=%d", code, tv.Completed)
	}
	var hist struct {
		Items      []engine.HistoryEntry
		NextBefore string
	}
	if code := call(t, "GET", u+"/history?limit=1", "", &hist); code != 200 || len(hist.Items) != 1 || hist.NextBefore == "" || hist.Items[0].Title == "" {
		t.Errorf("history: %d %+v", code, hist)
	}

	// Undo takes it back out of history.
	if code := call(t, "POST", u+"/undo", `{"ref":"`+firstRef+`"}`, &resp); code != 200 || resp.Chapter == nil {
		t.Errorf("undo: %d", code)
	}
}

func TestLearningSettingsAndErrors(t *testing.T) {
	srv, _, _ := learningSetup(t)
	u := srv.URL + "/api/learning"
	var s engine.Settings
	if code := call(t, "PUT", u+"/settings", `{"weeklyMinutes":240,"timezone":"Asia/Singapore","priorities":{"ai-security":2}}`, &s); code != 200 || s.WeeklyMinutes != 240 {
		t.Fatalf("settings: %d %+v", code, s)
	}
	call(t, "GET", u+"/settings", "", &s)
	if s.Timezone != "Asia/Singapore" || s.Priorities["ai-security"] != 2 {
		t.Errorf("settings not saved: %+v", s)
	}
	var e map[string]string
	for _, c := range []struct {
		method, path, body string
		code               int
	}{
		{"PUT", "/settings", `{"weeklyMinutes":5}`, 400},
		{"PUT", "/settings", `{"timezone":"Mars/Olympus"}`, 400},
		{"PUT", "/settings", `{"priorities":{"nope":1}}`, 400},
		{"PUT", "/settings", `{"bogus":1}`, 400},
		{"POST", "/done", `{"ref":"step:x/y/z"}`, 404},
		{"POST", "/done", `{"ref":"garbage"}`, 400},
		{"POST", "/undo", `{"ref":"item:5"}`, 404},
		{"GET", "/tracks/nope", "", 404},
		{"GET", "/tracks/ai-red-teaming/chapters/nope", "", 404},
		{"POST", "/tracks/ai-red-teaming/chapters/data-exfiltration/checkpoint", `{"met":[true]}`, 400},
		{"PUT", "/assessment", `{"answers":{"linux":"guru"}}`, 400},
		{"GET", "/history?limit=0", "", 400},
	} {
		if code := call(t, c.method, u+c.path, c.body, &e); code != c.code {
			t.Errorf("%s %s %s: %d, want %d (%v)", c.method, c.path, c.body, code, c.code, e)
		}
	}
	if code := call(t, "POST", u+"/tracks/ai-red-teaming/chapters/data-exfiltration/snooze", "", nil); code != 204 {
		t.Errorf("snooze: %d", code)
	}
}
