package engine

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/content"
)

var t0 = time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC) // a Monday

func newEngine(t *testing.T) *Engine {
	t.Helper()
	c, err := content.LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	return New(c)
}

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

func TestRefs(t *testing.T) {
	for _, s := range []string{"step:a/b/c", "practice:a/b", "capstone:a", "item:42"} {
		if _, err := ParseRef(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"", "step:a/b", "item:x", "item:-1", "nope:a", "practice:a"} {
		if _, err := ParseRef(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestFreshStartDashboard(t *testing.T) {
	e, s := newEngine(t), NewState()
	d := e.Dashboard(s, nil, t0)
	if !d.NeedsAssessment || d.Continue == nil || d.Continue.Resume || len(d.UpNext) != 2 {
		t.Fatalf("dashboard: %+v", d)
	}
	if d.Continue.Index != 0 || d.Continue.Next == nil || d.Continue.Next.Kind != "step" {
		t.Errorf("should start a track at chapter 1, step 1: %+v", d.Continue)
	}
	if len(d.Tracks) != len(e.cur.Tracks) || d.Week.Goal != 300 {
		t.Errorf("tracks/week: %d %+v", len(d.Tracks), d.Week)
	}
	// Stable: same state, same answer.
	if !reflect.DeepEqual(e.Rank(s), e.Rank(s)) {
		t.Error("Rank is not stable for the same state")
	}
}

func TestDoneStartsChapterAndContinues(t *testing.T) {
	e, s := newEngine(t), NewState()
	ref := StepRef("ai-red-teaming", "prompt-injection-basics", e.cur.Track("ai-red-teaming").Chapters[0].Steps[0].ID)
	if err := e.Done(s, nil, ref, Right, yes(), t0); err != nil {
		t.Fatal(err)
	}
	d := e.Dashboard(s, nil, t0)
	if d.Continue == nil || !d.Continue.Resume || d.Continue.Chapter != "prompt-injection-basics" || d.Continue.StepsDone != 1 {
		t.Fatalf("continue: %+v", d.Continue)
	}
	if d.Week.Minutes == 0 || d.Week.Days[0] != d.Week.Minutes {
		t.Errorf("week: %+v", d.Week)
	}
	if s.LastTrack != "ai-red-teaming" || s.track("ai-red-teaming").Alpha != 2 {
		t.Errorf("state: last=%s track=%+v", s.LastTrack, s.track("ai-red-teaming"))
	}
	for _, up := range d.UpNext {
		if up.Track == "ai-red-teaming" {
			t.Errorf("up next repeats the current track's chapter: %+v", up)
		}
	}
	if err := e.Done(s, nil, "step:nope/x/y", Right, nil, t0); err != ErrNotFound {
		t.Errorf("unknown step: %v", err)
	}
	if err := e.Done(s, nil, ref, "meh", nil, t0); err == nil {
		t.Error("bad rating accepted")
	}
}

func TestTooHardInsertsCorrective(t *testing.T) {
	e, s := newEngine(t), NewState()
	ch := e.cur.Track("ai-red-teaming").Chapters[1] // data-exfiltration (requires prompt-injection)
	ref := StepRef("ai-red-teaming", ch.ID, ch.Steps[0].ID)
	if err := e.Done(s, nil, ref, Hard, nil, t0); err != nil {
		t.Fatal(err)
	}
	if len(s.Extras) != 1 {
		t.Fatalf("extras: %+v", s.Extras)
	}
	v, _ := e.Chapter(s, nil, "ai-red-teaming", ch.ID, t0)
	if v.Steps[1].Kind != "corrective" || v.Steps[1].Reason == "" {
		t.Fatalf("corrective not right after the hard step: %+v", v.Steps[:2])
	}
	if v.Next.Ref != v.Steps[1].Ref {
		t.Errorf("next should be the corrective, got %s", v.Next.Ref)
	}
	// It must be easier, on a prerequisite or the same topic.
	x := s.Extras[0]
	if k := e.stepCh[x.Ref]; k != "" && e.chapters[k].chapter.Difficulty >= ch.Difficulty {
		t.Errorf("corrective not easier: %s", x.Ref)
	}
	if !strings.HasPrefix(x.Reason, "Background on prompt injection") {
		t.Errorf("reason: %q", x.Reason)
	}
	// Doing it doesn't start the chapter it came from.
	e.Done(s, nil, x.Ref, Right, nil, t0.Add(time.Hour))
	if own := e.stepCh[x.Ref]; own != "" && !s.chapter(own).Started.IsZero() {
		t.Errorf("corrective started its own chapter %s", own)
	}
	// One corrective per step.
	e.Done(s, nil, ref, Hard, nil, t0.Add(2*time.Hour))
	if len(s.Extras) != 1 {
		t.Errorf("second corrective for the same step: %d", len(s.Extras))
	}
}

func TestTooEasyOffersSkip(t *testing.T) {
	e, s := newEngine(t), NewState()
	ch := e.cur.Track("agentic-engineering").Chapters[0]
	e.Done(s, nil, StepRef("agentic-engineering", ch.ID, ch.Steps[0].ID), Easy, nil, t0)
	v, _ := e.Chapter(s, nil, "agentic-engineering", ch.ID, t0)
	if !v.OfferSkip {
		t.Fatal("no skip offered after a too-easy step")
	}
	if err := e.SkipToPractice(s, "agentic-engineering", ch.ID, t0); err != nil {
		t.Fatal(err)
	}
	v, _ = e.Chapter(s, nil, "agentic-engineering", ch.ID, t0)
	if v.Next == nil || v.Next.Kind != "practice" || v.OfferSkip || v.StepsDone != v.StepsTotal {
		t.Errorf("after skip: next=%+v offer=%v %d/%d", v.Next, v.OfferSkip, v.StepsDone, v.StepsTotal)
	}
	if w := e.Week(s, t0); w.Minutes != ch.Steps[0].Minutes {
		t.Errorf("skipped steps counted as minutes: %d", w.Minutes)
	}
}

func TestCheckpoint(t *testing.T) {
	e, s := newEngine(t), NewState()
	tr := "ai-red-teaming"
	ch := e.cur.Track(tr).Chapters[0]
	met := make([]bool, len(ch.Outcomes))
	if _, err := e.Checkpoint(s, nil, tr, ch.ID, met[:1], Right, nil, false, t0); err == nil {
		t.Error("wrong number of outcomes accepted")
	}
	res, err := e.Checkpoint(s, nil, tr, ch.ID, met, Hard, no(), false, t0)
	if err != nil || res.Completed || res.Corrective == "" {
		t.Fatalf("unmet: %+v %v", res, err)
	}
	v, _ := e.Chapter(s, nil, tr, ch.ID, t0)
	if v.Completed || !slices.ContainsFunc(v.Steps, func(sv StepView) bool { return sv.Kind == "corrective" }) {
		t.Errorf("chapter after unmet checkpoint: completed=%v", v.Completed)
	}
	for i := range met {
		met[i] = true
	}
	before := s.Topic("prompt-injection").Theta
	res, _ = e.Checkpoint(s, nil, tr, ch.ID, met, Right, yes(), false, t0.Add(time.Hour))
	if !res.Completed {
		t.Fatal("all met but not completed")
	}
	if th := s.Topic("prompt-injection").Theta; th < float64(ch.Difficulty) || th < before {
		t.Errorf("mastery didn't raise skill: %.2f → %.2f", before, th)
	}
	if d := e.Dashboard(s, nil, t0); d.Continue.Chapter == ch.ID {
		t.Error("completed chapter recommended again")
	}

	// Force: continue anyway.
	ch2 := e.cur.Track(tr).Chapters[1]
	res, _ = e.Checkpoint(s, nil, tr, ch2.ID, make([]bool, len(ch2.Outcomes)), Right, nil, true, t0)
	if !res.Completed {
		t.Error("force didn't complete")
	}
}

func TestPrerequisitesAndSnooze(t *testing.T) {
	e, s := newEngine(t), NewState()
	asm := e.chapters["exploitation-fundamentals/assembly-and-memory"].chapter
	if u := e.unmet(s, asm); !slices.Equal(u, []string{"linux"}) {
		t.Errorf("unmet: %v", u)
	}
	e.Assess(s, map[string]string{"linux": "some"})
	if u := e.unmet(s, asm); len(u) != 0 {
		t.Errorf("assessed prerequisite still unmet: %v", u)
	}
	if err := e.Assess(s, map[string]string{"nope": "some"}); err == nil {
		t.Error("unknown topic accepted")
	}
	if err := e.Assess(s, map[string]string{"linux": "guru"}); err == nil {
		t.Error("unknown level accepted")
	}

	first := e.Rank(s)[0]
	e.Snooze(s, first.Track, first.Chapter)
	for _, c := range e.Rank(s) {
		if c.Track == first.Track {
			t.Errorf("snoozed track still on the frontier: %+v", c)
		}
	}
	// It comes back after snoozeFor completions elsewhere.
	other := e.Rank(s)
	for i := range snoozeFor {
		c := other[i%len(other)]
		ch := e.chapters[chapterKey(c.Track, c.Chapter)].chapter
		s.Chapters[chapterKey(c.Track, c.Chapter)] = ChapterState{Completed: t0, SnoozedAt: -1}
		_ = ch
		other = e.Rank(s)
	}
	if !slices.ContainsFunc(e.Rank(s), func(c Candidate) bool { return c.Track == first.Track }) {
		t.Error("snoozed chapter never came back")
	}
}

func TestFreshAndDismiss(t *testing.T) {
	e, s := newEngine(t), NewState()
	pool := []Item{
		{ID: 1, Title: "New exfil trick", Type: "read", Minutes: 10, Difficulty: 3, Topics: []string{"data-exfiltration"}, Mentor: "rehberger", Published: t0.AddDate(0, 0, -2)},
		{ID: 2, Title: "Old exfil trick", Type: "read", Minutes: 10, Difficulty: 3, Topics: []string{"data-exfiltration"}, Mentor: "rehberger", Published: t0.AddDate(-1, 0, 0)},
		{ID: 3, Title: "News", Type: "read", Minutes: 10, Difficulty: 3, Topics: []string{"data-exfiltration"}, Published: t0},
	}
	v, _ := e.Chapter(s, pool, "ai-red-teaming", "data-exfiltration", t0)
	var fresh []StepView
	for _, sv := range v.Steps {
		if sv.Kind == "fresh" {
			fresh = append(fresh, sv)
		}
	}
	if len(fresh) != 1 || fresh[0].Ref != "item:1" || !fresh[0].Optional || fresh[0].Reason != "Fresh from Johann Rehberger" {
		t.Fatalf("fresh: %+v", fresh)
	}
	if d := e.Dashboard(s, pool, t0); d.Fresh != 1 {
		t.Errorf("fresh count: %d", d.Fresh)
	}
	if err := e.Dismiss(s, pool, "item:1", t0); err != nil {
		t.Fatal(err)
	}
	v, _ = e.Chapter(s, pool, "ai-red-teaming", "data-exfiltration", t0)
	if slices.ContainsFunc(v.Steps, func(sv StepView) bool { return sv.Kind == "fresh" }) {
		t.Error("dismissed fresh step still shown")
	}
	// Optional steps don't block the chapter.
	if v.StepsTotal != len(e.chapters["ai-red-teaming/data-exfiltration"].chapter.Steps) {
		t.Errorf("optional counted: %d", v.StepsTotal)
	}
}

func TestWeekTimezone(t *testing.T) {
	e, s := newEngine(t), NewState()
	s.Settings.Timezone = "Asia/Singapore" // UTC+8
	monday := time.Date(2026, 11, 23, 0, 30, 0, 0, time.FixedZone("SGT", 8*3600))
	s.Progress["a"] = Progress{Ref: "a", Status: Done, At: monday, Minutes: 20}
	s.Progress["b"] = Progress{Ref: "b", Status: Done, At: monday.Add(-time.Hour), Minutes: 30} // Sunday, last week
	s.Progress["c"] = Progress{Ref: "c", Status: Skipped, At: monday, Minutes: 99}
	w := e.Week(s, monday.Add(50*time.Hour))
	if w.Minutes != 20 || w.Days[0] != 20 {
		t.Errorf("week: %+v", w)
	}
}

func TestEloAndProgressModel(t *testing.T) {
	ts := TopicState{Theta: 2, Alpha: 1, Beta: 1}
	if e := expected(2, 2.5); e < 0.59 || e > 0.61 {
		t.Errorf("content zpdStretch above skill should expect 'just right': %.3f", e)
	}
	if up := observe(ts, 3, 1); up.Theta <= 2 {
		t.Error("too easy should raise skill")
	}
	if down := observe(ts, 3, 0); down.Theta >= 2 {
		t.Error("too hard should lower skill")
	}
	if learningProgress(TopicState{Obs: []float64{0, 0, 0, 1, 1, 1}}) < 0.9 {
		t.Error("improving topic should show progress")
	}
	if learningProgress(TopicState{Obs: []float64{1, 1, 1, 1, 1, 1}}) != 0 {
		t.Error("flat topic should show none")
	}
	if fit(2, 2.5) < fit(2, 4) || fit(2, 2.5) < fit(2, 1) {
		t.Error("fit should peak just above skill")
	}
}
