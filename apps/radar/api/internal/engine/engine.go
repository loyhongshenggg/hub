package engine

import (
	"cmp"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"hub.localhost/apps/radar/api/content"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

type chapterRef struct {
	track   *content.Track
	chapter *content.Chapter
	index   int
}

// Engine answers "what next?" for a State against a curriculum.
type Engine struct {
	cur      *content.Curriculum
	chapters map[string]chapterRef     // "<track>/<chapter>"
	steps    map[string]*content.Step  // step ref → step
	stepCh   map[string]string         // step ref → chapter key
	requires map[string][]string       // topic → prerequisite topics
	topics   map[string]*content.Topic // id → topic
	areas    map[string]string         // area id → name
	tracks   map[string]*content.Track // id → track
}

func New(cur *content.Curriculum) *Engine {
	e := &Engine{
		cur:      cur,
		chapters: map[string]chapterRef{},
		steps:    map[string]*content.Step{},
		stepCh:   map[string]string{},
		requires: map[string][]string{},
		topics:   map[string]*content.Topic{},
		areas:    map[string]string{},
		tracks:   map[string]*content.Track{},
	}
	for i := range cur.Topics {
		t := &cur.Topics[i]
		e.topics[t.ID], e.requires[t.ID] = t, t.Requires
	}
	for _, a := range cur.Areas {
		e.areas[a.ID] = a.Name
	}
	for ti := range cur.Tracks {
		tr := &cur.Tracks[ti]
		e.tracks[tr.ID] = tr
		for ci := range tr.Chapters {
			ch := &tr.Chapters[ci]
			key := chapterKey(tr.ID, ch.ID)
			e.chapters[key] = chapterRef{tr, ch, ci}
			for si := range ch.Steps {
				ref := StepRef(tr.ID, ch.ID, ch.Steps[si].ID)
				e.steps[ref], e.stepCh[ref] = &ch.Steps[si], key
			}
		}
	}
	return e
}

func (e *Engine) topicName(id string) string {
	if t := e.topics[id]; t != nil {
		return t.Name
	}
	return id
}

// --- choosing the next chapter ---------------------------------------------

// Candidate is a chapter (or capstone) Radar could recommend next.
type Candidate struct {
	Kind    string   `json:"kind"` // chapter | capstone
	Track   string   `json:"track"`
	Chapter string   `json:"chapter,omitempty"`
	Score   float64  `json:"score"`
	Why     []string `json:"why"`
	Unmet   []string `json:"unmet,omitempty"` // prerequisite topics not met
}

// frontier is each ready track's first unfinished chapter (or its capstone
// once all chapters are done), skipping snoozed chapters.
func (e *Engine) frontier(s *State) []Candidate {
	done := s.completedCount()
	var out []Candidate
	for _, tr := range e.cur.Tracks {
		if tr.Outline() {
			continue
		}
		found := false
		for _, ch := range tr.Chapters {
			cs := s.chapter(chapterKey(tr.ID, ch.ID))
			if !cs.Completed.IsZero() {
				continue
			}
			found = true
			if cs.SnoozedAt < 0 || done >= cs.SnoozedAt+snoozeFor {
				out = append(out, Candidate{Kind: "chapter", Track: tr.ID, Chapter: ch.ID})
			} // else snoozed: the whole track waits
			break
		}
		if !found && tr.Capstone != nil {
			if p, ok := s.Progress[CapstoneRef(tr.ID)]; !ok || p.Status != Done {
				out = append(out, Candidate{Kind: "capstone", Track: tr.ID})
			}
		}
	}
	return out
}

// unmet lists prerequisite topics of a chapter that are not met: skill below
// metTheta, and not taught by the chapter itself.
func (e *Engine) unmet(s *State, ch *content.Chapter) []string {
	var out []string
	for _, t := range ch.Topics {
		for _, r := range e.requires[t] {
			if slices.Contains(ch.Topics, r) || slices.Contains(out, r) {
				continue
			}
			if s.Topic(r).Theta < metTheta {
				out = append(out, r)
			}
		}
	}
	return out
}

// seed makes Thompson sampling stable between actions: the same state
// gives the same recommendation until something changes.
func seed(s *State) uint64 {
	h := uint64(len(s.Progress))*1_000_003 + uint64(s.completedCount())*7919
	for _, c := range s.Chapters {
		h += uint64(c.SnoozedAt+2) * 31
	}
	return h
}

// Rank scores the frontier, best first.
func (e *Engine) Rank(s *State) []Candidate {
	cands := e.frontier(s)
	r := rand.New(rand.NewPCG(seed(s), 0x5eed))
	// Topics some frontier chapter is missing: teaching them unlocks it.
	missing := map[string]bool{}
	for _, c := range cands {
		if c.Kind == "chapter" {
			for _, t := range e.unmet(s, e.chapters[chapterKey(c.Track, c.Chapter)].chapter) {
				missing[t] = true
			}
		}
	}
	for i := range cands {
		c := &cands[i]
		tr := e.tracks[c.Track]
		prio := 1.0
		if p, ok := s.Settings.Priorities[tr.Area]; ok {
			prio = p
		}
		// Momentum holds you in a track you like, and lets go of one you don't.
		stay := 0.0
		if s.LastTrack == tr.ID {
			stay = momentum * 2 * s.track(tr.ID).Mean()
		}
		if c.Kind == "capstone" {
			c.Score = prio*0.6 + stay
			c.Why = []string{"All chapters of " + tr.Title + " done — finish with the capstone"}
			continue
		}
		ch := e.chapters[chapterKey(c.Track, c.Chapter)].chapter
		d := float64(ch.Difficulty)
		var interest, theta, lp float64
		bestLP, bestLPTopic := 0.0, ""
		for _, t := range ch.Topics {
			ts := s.Topic(t)
			interest += sampleBeta(r, ts.Alpha, ts.Beta)
			theta += ts.Theta
			p := learningProgress(ts)
			lp += p
			if len(ts.Obs) >= 4 && p > bestLP {
				bestLP, bestLPTopic = p, t
			}
		}
		n := float64(len(ch.Topics))
		interest, theta, lp = interest/n, theta/n, lp/n
		ti := s.track(tr.ID)
		interest = (interest + sampleBeta(r, ti.Alpha, ti.Beta)) / 2
		f := fit(theta, d)
		c.Score = prio * (wInterest*interest + wFit*f + wProgress*lp)
		c.Unmet = e.unmet(s, ch)
		c.Score -= unmetCost * float64(len(c.Unmet))
		unlocks := false
		for _, t := range ch.Topics {
			unlocks = unlocks || missing[t]
		}
		if unlocks {
			c.Score += unlockGap
		}
		c.Score += stay
		c.Why = e.why(s, tr, ch, theta, f, bestLP, bestLPTopic, unlocks, prio, c.Unmet)
	}
	slices.SortStableFunc(cands, func(a, b Candidate) int { return cmp.Compare(b.Score, a.Score) })
	return cands
}

func (e *Engine) why(s *State, tr *content.Track, ch *content.Chapter, theta, f, lp float64, lpTopic string,
	unlocks bool, prio float64, unmet []string) []string {
	var out []string
	if s.LastTrack == tr.ID {
		out = append(out, "Continues "+tr.Title)
	}
	liked := false
	for _, t := range ch.Topics {
		ts := s.Topic(t)
		if ts.Alpha-1 >= 2 && ts.Alpha/(ts.Alpha+ts.Beta) > 0.6 {
			out = append(out, fmt.Sprintf("You found %s worth it", strings.ToLower(e.topicName(t))))
			liked = true
			break
		}
	}
	if ti := s.track(tr.ID); !liked && ti.Alpha-1 >= 2 && ti.Mean() > 0.6 {
		out = append(out, "You've found "+tr.Title+" worth it")
	}
	switch {
	case f > 0.75:
		out = append(out, "Right at your level")
	case float64(ch.Difficulty)-theta > 1.5:
		out = append(out, "A stretch for you")
	case float64(ch.Difficulty) < theta:
		out = append(out, "Should feel easy")
	}
	if lpTopic != "" && lp > 0.25 {
		out = append(out, fmt.Sprintf("You're improving fast in %s", strings.ToLower(e.topicName(lpTopic))))
	}
	if unlocks {
		out = append(out, "Unlocks chapters that build on it")
	}
	if prio > 1 {
		out = append(out, "Priority area: "+e.areas[tr.Area])
	}
	for _, t := range unmet {
		out = append(out, "Builds on "+strings.ToLower(e.topicName(t))+" — new to you")
	}
	return out
}

// --- chapter view -------------------------------------------------------

// StepView is one row of a chapter: a curated step, an inserted corrective
// or fresh step, the practice, or the checkpoint.
type StepView struct {
	Ref      string   `json:"ref"`
	Kind     string   `json:"kind"` // step | corrective | fresh | practice | checkpoint | capstone
	Title    string   `json:"title"`
	Type     string   `json:"type"` // read | watch | lab | tool | walkthrough
	URL      string   `json:"url,omitempty"`
	By       string   `json:"by,omitempty"`
	Minutes  int      `json:"minutes"`
	Focus    string   `json:"focus,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Steps    []string `json:"steps,omitempty"` // practice walkthrough
	Status   string   `json:"status,omitempty"`
	Rating   Rating   `json:"rating,omitempty"`
	Optional bool     `json:"optional,omitempty"`
}

type ChapterView struct {
	Track       string     `json:"track"`
	TrackTitle  string     `json:"trackTitle"`
	Chapter     string     `json:"chapter"`
	Index       int        `json:"index"` // 0-based position in the track
	Count       int        `json:"count"` // chapters in the track
	Title       string     `json:"title"`
	Summary     string     `json:"summary"`
	Difficulty  int        `json:"difficulty"`
	Topics      []string   `json:"topics"`
	Outcomes    []string   `json:"outcomes"`
	Steps       []StepView `json:"steps"`
	Practice    *StepView  `json:"practice,omitempty"`
	Checkpoint  StepView   `json:"checkpoint"`
	Met         []bool     `json:"met"` // last checkpoint answers
	Started     bool       `json:"started"`
	Completed   bool       `json:"completed"`
	OfferSkip   bool       `json:"offerSkip"` // last step felt too easy: offer to jump to practice
	Next        *StepView  `json:"next"`      // nil when completed
	Minutes     int        `json:"minutes"`
	MinutesDone int        `json:"minutesDone"`
	StepsDone   int        `json:"stepsDone"`
	StepsTotal  int        `json:"stepsTotal"`
}

func (e *Engine) stepView(ref string, st *content.Step, s *State) StepView {
	v := StepView{Ref: ref, Kind: "step", Title: st.Title, Type: st.Type, URL: st.URL, By: e.cur.OriginName(st.By),
		Minutes: st.Minutes, Focus: st.Focus}
	e.withProgress(&v, s)
	return v
}

func (e *Engine) itemView(it Item, kind string, s *State) StepView {
	v := StepView{Ref: ItemRef(it.ID), Kind: kind, Title: it.Title, Type: it.Type, URL: it.URL,
		By: e.cur.OriginName(it.Mentor), Minutes: it.Minutes}
	e.withProgress(&v, s)
	return v
}

func (e *Engine) withProgress(v *StepView, s *State) {
	if p, ok := s.Progress[v.Ref]; ok {
		v.Status, v.Rating = p.Status, p.Rating
	}
}

func findItem(pool []Item, id int64) (Item, bool) {
	for _, it := range pool {
		if it.ID == id {
			return it, true
		}
	}
	return Item{}, false
}

// extraView renders an inserted step (a curated step or an item).
func (e *Engine) extraView(x Extra, s *State, pool []Item) (StepView, bool) {
	r, err := ParseRef(x.Ref)
	if err != nil {
		return StepView{}, false
	}
	var v StepView
	switch r.Kind {
	case "step":
		st := e.steps[x.Ref]
		if st == nil {
			return StepView{}, false
		}
		v = e.stepView(x.Ref, st, s)
	case "item":
		it, ok := findItem(pool, r.Item)
		if !ok {
			return StepView{}, false
		}
		v = e.itemView(it, "corrective", s)
	default:
		return StepView{}, false
	}
	v.Kind, v.Reason = "corrective", x.Reason
	return v, true
}

// fresh lists recent mentor items on a chapter's topics, newest first.
func (e *Engine) fresh(s *State, pool []Item, ch *content.Chapter, now time.Time, used map[string]bool) []Item {
	var out []Item
	for _, it := range pool {
		if it.Mentor == "" || now.Sub(it.Published) > freshDays*24*time.Hour || used[ItemRef(it.ID)] {
			continue
		}
		if _, seen := s.Progress[ItemRef(it.ID)]; seen {
			continue
		}
		for _, t := range it.Topics {
			if slices.Contains(ch.Topics, t) {
				out = append(out, it)
				break
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Item) int { return b.Published.Compare(a.Published) })
	return out
}

// Chapter renders a chapter with progress, inserted steps and what's next.
func (e *Engine) Chapter(s *State, pool []Item, track, chapter string, now time.Time) (ChapterView, error) {
	key := chapterKey(track, chapter)
	cr, ok := e.chapters[key]
	if !ok {
		return ChapterView{}, ErrNotFound
	}
	tr, ch := cr.track, cr.chapter
	cs := s.chapter(key)
	v := ChapterView{
		Track: tr.ID, TrackTitle: tr.Title, Chapter: ch.ID, Index: cr.index, Count: len(tr.Chapters),
		Title: ch.Title, Summary: ch.Summary, Difficulty: ch.Difficulty, Topics: ch.Topics, Outcomes: ch.Outcomes,
		Met: cs.Outcomes, Started: !cs.Started.IsZero(), Completed: !cs.Completed.IsZero(), Steps: []StepView{},
	}
	if v.Met == nil {
		v.Met = make([]bool, len(ch.Outcomes))
	}
	// Extras for this chapter, by the ref they follow.
	after := map[string][]StepView{}
	used := map[string]bool{}
	for _, x := range s.Extras {
		if x.Track != track || x.Chapter != chapter {
			continue
		}
		used[x.Ref] = true
		if xv, ok := e.extraView(x, s, pool); ok && xv.Status != Dismissed {
			after[x.After] = append(after[x.After], xv)
		}
	}
	for _, st := range ch.Steps {
		ref := StepRef(track, chapter, st.ID)
		v.Steps = append(v.Steps, e.stepView(ref, &st, s))
		v.Steps = append(v.Steps, after[ref]...)
	}
	v.Steps = append(v.Steps, after[""]...)
	if !v.Completed {
		fresh := e.fresh(s, pool, ch, now, used)
		for _, it := range fresh[:min(maxFresh, len(fresh))] {
			fv := e.itemView(it, "fresh", s)
			fv.Optional = true
			fv.Reason = "Fresh from " + e.cur.OriginName(it.Mentor)
			v.Steps = append(v.Steps, fv)
		}
	}
	if p := ch.Practice; p != nil {
		pv := StepView{Ref: PracticeRef(track, chapter), Kind: "practice", Title: p.Title, Type: p.Type, URL: p.URL,
			Minutes: p.Minutes, Steps: p.Steps}
		e.withProgress(&pv, s)
		v.Practice = &pv
	}
	v.Checkpoint = StepView{Ref: "checkpoint:" + key, Kind: "checkpoint", Title: "Checkpoint", Type: "checkpoint"}
	if v.Completed {
		v.Checkpoint.Status = Done
	}

	// Totals and what's next: the first required row not done or skipped.
	var lastDone *StepView
	for i := range v.Steps {
		sv := &v.Steps[i]
		if sv.Optional {
			if sv.Status == Done {
				v.MinutesDone += sv.Minutes
			}
			continue
		}
		v.StepsTotal++
		v.Minutes += sv.Minutes
		switch sv.Status {
		case Done:
			v.StepsDone++
			v.MinutesDone += sv.Minutes
			lastDone = sv
		case Skipped:
			v.StepsDone++
		default:
			if v.Next == nil && !v.Completed {
				v.Next = sv
			}
		}
	}
	if v.Practice != nil {
		v.Minutes += v.Practice.Minutes
		if v.Practice.Status == Done {
			v.MinutesDone += v.Practice.Minutes
		}
		if v.Next == nil && !v.Completed && v.Practice.Status == "" {
			v.Next = v.Practice
		}
	}
	if v.Next == nil && !v.Completed {
		v.Next = &v.Checkpoint
	}
	// Too easy on the latest step, with reading still ahead: offer the skip.
	if v.Next != nil && v.Next.Kind != "practice" && v.Next.Kind != "checkpoint" && lastDone != nil {
		latest := Progress{}
		for _, sv := range v.Steps {
			if p, ok := s.Progress[sv.Ref]; ok && p.Status == Done && !p.At.Before(latest.At) {
				latest = p
			}
		}
		v.OfferSkip = latest.Rating == Easy
	}
	return v, nil
}

// --- dashboard ----------------------------------------------------------

// Recommendation is a chapter (or capstone) to work on, with its next step.
type Recommendation struct {
	Kind         string    `json:"kind"` // chapter | capstone
	Track        string    `json:"track"`
	TrackTitle   string    `json:"trackTitle"`
	Chapter      string    `json:"chapter,omitempty"`
	ChapterTitle string    `json:"chapterTitle,omitempty"`
	Index        int       `json:"index"`
	Count        int       `json:"count"`
	Difficulty   int       `json:"difficulty,omitempty"`
	Resume       bool      `json:"resume"` // already in progress
	Next         *StepView `json:"next"`
	StepsDone    int       `json:"stepsDone"`
	StepsTotal   int       `json:"stepsTotal"`
	MinutesLeft  int       `json:"minutesLeft"`
	Why          []string  `json:"why"`
}

type TrackSummary struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Tagline     string   `json:"tagline"`
	Area        string   `json:"area"`
	Level       string   `json:"level"`
	Mentors     []string `json:"mentors"`
	Outline     bool     `json:"outline"`
	Chapters    int      `json:"chapters"`
	Completed   int      `json:"completed"`
	Minutes     int      `json:"minutes"`
	MinutesDone int      `json:"minutesDone"`
	Status      string   `json:"status"` // new | active | done
}

type Dashboard struct {
	NeedsAssessment bool             `json:"needsAssessment"`
	Continue        *Recommendation  `json:"continue"`
	UpNext          []Recommendation `json:"upNext"`
	Week            Week             `json:"week"`
	Tracks          []TrackSummary   `json:"tracks"`
	Fresh           int              `json:"fresh"` // fresh mentor items not yet seen
}

// inProgress is the most recently active started, unfinished chapter.
func (e *Engine) inProgress(s *State) string {
	best, bestAt := "", time.Time{}
	for key, cs := range s.Chapters {
		if cs.Started.IsZero() || !cs.Completed.IsZero() {
			continue
		}
		if _, ok := e.chapters[key]; !ok {
			continue
		}
		at := cs.Started
		for ref, p := range s.Progress {
			if e.stepCh[ref] == key || strings.HasPrefix(ref, "practice:"+key) {
				if p.At.After(at) {
					at = p.At
				}
			}
		}
		if best == "" || at.After(bestAt) || (at.Equal(bestAt) && key < best) {
			best, bestAt = key, at
		}
	}
	return best
}

func (e *Engine) recommend(s *State, pool []Item, c Candidate, now time.Time) Recommendation {
	tr := e.tracks[c.Track]
	r := Recommendation{Kind: c.Kind, Track: tr.ID, TrackTitle: tr.Title, Why: c.Why, Count: len(tr.Chapters)}
	if c.Kind == "capstone" {
		r.Index = len(tr.Chapters)
		r.ChapterTitle = tr.Capstone.Title
		r.Next = &StepView{Ref: CapstoneRef(tr.ID), Kind: "capstone", Title: tr.Capstone.Title, Type: "walkthrough",
			Focus: tr.Capstone.Summary, Steps: tr.Capstone.Steps}
		return r
	}
	v, _ := e.Chapter(s, pool, c.Track, c.Chapter, now)
	r.Chapter, r.ChapterTitle, r.Index, r.Difficulty = v.Chapter, v.Title, v.Index, v.Difficulty
	r.Resume, r.Next, r.StepsDone, r.StepsTotal = v.Started, v.Next, v.StepsDone, v.StepsTotal
	r.MinutesLeft = max(0, v.Minutes-v.MinutesDone)
	return r
}

// Dashboard is the learning home: continue, up next, the week, the tracks.
func (e *Engine) Dashboard(s *State, pool []Item, now time.Time) Dashboard {
	d := Dashboard{NeedsAssessment: !s.Assessed, UpNext: []Recommendation{}, Week: e.Week(s, now)}
	ranked := e.Rank(s)
	cur := e.inProgress(s)
	if cur != "" {
		cr := e.chapters[cur]
		rec := e.recommend(s, pool, Candidate{Kind: "chapter", Track: cr.track.ID, Chapter: cr.chapter.ID}, now)
		rec.Why = []string{} // the card already says where you are
		d.Continue = &rec
	}
	for _, c := range ranked {
		if c.Kind == "chapter" && chapterKey(c.Track, c.Chapter) == cur {
			continue
		}
		rec := e.recommend(s, pool, c, now)
		if d.Continue == nil {
			d.Continue = &rec
			continue
		}
		if len(d.UpNext) < 2 {
			d.UpNext = append(d.UpNext, rec)
		}
	}
	for _, tr := range e.cur.Tracks {
		ts := TrackSummary{ID: tr.ID, Title: tr.Title, Tagline: tr.Tagline, Area: tr.Area, Level: tr.Level,
			Mentors: tr.Mentors, Outline: tr.Outline(), Chapters: len(tr.Chapters), Status: "new"}
		for _, ch := range tr.Chapters {
			ts.Minutes += ch.Minutes()
			cs := s.chapter(chapterKey(tr.ID, ch.ID))
			if !cs.Completed.IsZero() {
				ts.Completed++
			}
			if !cs.Started.IsZero() {
				ts.Status = "active"
			}
		}
		for ref, p := range s.Progress {
			if p.Status != Done {
				continue
			}
			if k := e.stepCh[ref]; strings.HasPrefix(k, tr.ID+"/") || strings.HasPrefix(ref, "practice:"+tr.ID+"/") {
				ts.MinutesDone += p.Minutes
			}
		}
		if p, ok := s.Progress[CapstoneRef(tr.ID)]; ts.Completed == ts.Chapters && ts.Chapters > 0 &&
			(tr.Capstone == nil || (ok && p.Status == Done)) {
			ts.Status = "done"
		}
		d.Tracks = append(d.Tracks, ts)
	}
	for _, it := range pool {
		if _, seen := s.Progress[ItemRef(it.ID)]; !seen && it.Mentor != "" && now.Sub(it.Published) <= freshDays*24*time.Hour {
			d.Fresh++
		}
	}
	return d
}

// --- the week -----------------------------------------------------------

type Week struct {
	Start   time.Time `json:"start"`
	Goal    int       `json:"goal"`    // minutes
	Minutes int       `json:"minutes"` // done this week
	Days    [7]int    `json:"days"`    // Monday first
}

// Week sums minutes done this week (Monday 00:00 in the learner's zone).
func (e *Engine) Week(s *State, now time.Time) Week {
	loc := time.Local
	if s.Settings.Timezone != "" {
		if l, err := time.LoadLocation(s.Settings.Timezone); err == nil {
			loc = l
		}
	}
	t := now.In(loc)
	day := (int(t.Weekday()) + 6) % 7 // Monday = 0
	start := time.Date(t.Year(), t.Month(), t.Day()-day, 0, 0, 0, 0, loc)
	w := Week{Start: start, Goal: s.Settings.WeeklyMinutes}
	for _, p := range s.Progress {
		if p.Status != Done || p.At.Before(start) {
			continue
		}
		at := p.At.In(loc)
		i := int(time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, loc).Sub(start).Hours()+12) / 24
		if i >= 0 && i < 7 {
			w.Days[i] += p.Minutes
			w.Minutes += p.Minutes
		}
	}
	return w
}
