package engine

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
)

// content resolves a ref to what the engine needs for an update: minutes,
// difficulty, topics, and the chapter it belongs to ("" for items, capstones).
type resolved struct {
	minutes    int
	difficulty float64
	topics     []string
	chapter    string // chapter key
	track      string
}

func (e *Engine) resolve(ref string, pool []Item) (resolved, error) {
	r, err := ParseRef(ref)
	if err != nil {
		return resolved{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	switch r.Kind {
	case "step":
		st := e.steps[ref]
		if st == nil {
			return resolved{}, ErrNotFound
		}
		cr := e.chapters[e.stepCh[ref]]
		return resolved{st.Minutes, float64(cr.chapter.Difficulty), cr.chapter.Topics, e.stepCh[ref], cr.track.ID}, nil
	case "practice":
		cr, ok := e.chapters[chapterKey(r.Track, r.Chapter)]
		if !ok || cr.chapter.Practice == nil {
			return resolved{}, ErrNotFound
		}
		return resolved{cr.chapter.Practice.Minutes, float64(cr.chapter.Difficulty), cr.chapter.Topics,
			chapterKey(r.Track, r.Chapter), r.Track}, nil
	case "capstone":
		tr := e.tracks[r.Track]
		if tr == nil || tr.Capstone == nil {
			return resolved{}, ErrNotFound
		}
		var topics []string
		top := 0
		for _, ch := range tr.Chapters {
			top = max(top, ch.Difficulty)
			for _, t := range ch.Topics {
				if !slices.Contains(topics, t) {
					topics = append(topics, t)
				}
			}
		}
		return resolved{0, float64(top), topics, "", tr.ID}, nil
	case "item":
		it, ok := findItem(pool, r.Item)
		if !ok {
			return resolved{}, ErrNotFound
		}
		d := it.Difficulty
		if d == 0 {
			d = 3
		}
		return resolved{it.Minutes, float64(d), it.Topics, "", ""}, nil
	}
	return resolved{}, ErrInvalid
}

// extraOf returns the chapter key a ref was inserted into, if any.
func (s *State) extraOf(ref string) string {
	for _, x := range s.Extras {
		if x.Ref == ref {
			return chapterKey(x.Track, x.Chapter)
		}
	}
	return ""
}

// rate applies a rating and a worth-it answer to topics and, if given, a track.
func (s *State) rate(track string, topics []string, d float64, rating Rating, worth *bool) {
	if track != "" && worth != nil {
		b := s.track(track)
		if *worth {
			b.Alpha++
		} else {
			b.Beta++
		}
		s.TrackInterest[track] = b
	}
	sv, rated := outcome(rating)
	for _, t := range topics {
		ts := s.Topic(t)
		if rated {
			ts = observe(ts, d, sv)
		}
		if worth != nil {
			if *worth {
				ts.Alpha++
			} else {
				ts.Beta++
			}
		}
		s.Topics[t] = ts
	}
}

// Done marks content finished (it goes to history) and learns from the
// rating. A "too hard" rating on a chapter step inserts a corrective step.
func (e *Engine) Done(s *State, pool []Item, ref string, rating Rating, worth *bool, now time.Time) error {
	if !rating.Valid() {
		return fmt.Errorf("%w: rating %q", ErrInvalid, rating)
	}
	res, err := e.resolve(ref, pool)
	if err != nil {
		return err
	}
	s.Progress[ref] = Progress{Ref: ref, Status: Done, At: now, Minutes: res.minutes, Rating: rating, Worth: worth}
	s.rate(res.track, res.topics, res.difficulty, rating, worth)

	// Which chapter was this done in? An inserted step belongs to the
	// chapter it was inserted into; otherwise its own.
	ctx := s.extraOf(ref)
	if ctx == "" {
		ctx = res.chapter
	}
	if ctx != "" {
		cs := s.chapter(ctx)
		if cs.Started.IsZero() && cs.Completed.IsZero() {
			cs.Started = now
		}
		s.Chapters[ctx] = cs
		s.LastTrack = e.chapters[ctx].track.ID
	} else if res.track != "" {
		s.LastTrack = res.track
	}
	if rating == Hard && ctx != "" && !strings.HasPrefix(ref, "practice:") {
		e.insertCorrective(s, pool, ctx, ref, "prereq", now)
	}
	return nil
}

// Undo takes content back out of history. Skill and interest updates stay:
// they were evidence when given.
func (e *Engine) Undo(s *State, ref string) error {
	if _, ok := s.Progress[ref]; !ok {
		return ErrNotFound
	}
	delete(s.Progress, ref)
	return nil
}

// Dismiss waves off an optional step (fresh or corrective).
func (e *Engine) Dismiss(s *State, pool []Item, ref string, now time.Time) error {
	if _, err := e.resolve(ref, pool); err != nil {
		return err
	}
	if p, ok := s.Progress[ref]; ok && p.Status == Done {
		return fmt.Errorf("%w: already done", ErrInvalid)
	}
	s.Progress[ref] = Progress{Ref: ref, Status: Dismissed, At: now}
	return nil
}

// SkipToPractice marks a chapter's remaining reading as skipped.
func (e *Engine) SkipToPractice(s *State, track, chapter string, now time.Time) error {
	key := chapterKey(track, chapter)
	cr, ok := e.chapters[key]
	if !ok {
		return ErrNotFound
	}
	for _, st := range cr.chapter.Steps {
		ref := StepRef(track, chapter, st.ID)
		if _, seen := s.Progress[ref]; !seen {
			s.Progress[ref] = Progress{Ref: ref, Status: Skipped, At: now}
		}
	}
	for _, x := range s.Extras {
		if chapterKey(x.Track, x.Chapter) == key {
			if _, seen := s.Progress[x.Ref]; !seen {
				s.Progress[x.Ref] = Progress{Ref: x.Ref, Status: Skipped, At: now}
			}
		}
	}
	cs := s.chapter(key)
	if cs.Started.IsZero() {
		cs.Started = now
	}
	s.Chapters[key] = cs
	s.LastTrack = track
	return nil
}

// CheckpointResult says whether the chapter is complete and, if not, which
// corrective step was added.
type CheckpointResult struct {
	Completed  bool   `json:"completed"`
	Corrective string `json:"corrective,omitempty"` // ref
}

// Checkpoint records which outcomes the learner can now do, plus a rating.
// All met → the chapter completes and topic skill rises to its difficulty
// (mastery). Any unmet → a corrective step (another angle) is added and the
// chapter stays open, unless force ("continue anyway").
func (e *Engine) Checkpoint(s *State, pool []Item, track, chapter string, met []bool, rating Rating, worth *bool,
	force bool, now time.Time) (CheckpointResult, error) {
	key := chapterKey(track, chapter)
	cr, ok := e.chapters[key]
	if !ok {
		return CheckpointResult{}, ErrNotFound
	}
	if len(met) != len(cr.chapter.Outcomes) {
		return CheckpointResult{}, fmt.Errorf("%w: want %d outcome answers", ErrInvalid, len(cr.chapter.Outcomes))
	}
	if !rating.Valid() {
		return CheckpointResult{}, fmt.Errorf("%w: rating %q", ErrInvalid, rating)
	}
	cs := s.chapter(key)
	if !cs.Completed.IsZero() {
		return CheckpointResult{Completed: true}, nil
	}
	n := 0
	for _, m := range met {
		if m {
			n++
		}
	}
	frac := float64(n) / float64(max(1, len(met)))
	d := float64(cr.chapter.Difficulty)
	for _, t := range cr.chapter.Topics {
		s.Topics[t] = observe(s.Topic(t), d, frac)
	}
	s.rate(track, cr.chapter.Topics, d, rating, worth)
	if cs.Started.IsZero() {
		cs.Started = now
	}
	cs.Outcomes, cs.Rating, cs.Worth = slices.Clone(met), rating, worth
	s.LastTrack = track
	res := CheckpointResult{}
	if n == len(met) || force {
		cs.Completed = now
		res.Completed = true
		if n == len(met) {
			for _, t := range cr.chapter.Topics {
				ts := s.Topic(t)
				ts.Theta = max(ts.Theta, d)
				s.Topics[t] = ts
			}
		}
	} else {
		res.Corrective = e.insertCorrective(s, pool, key, "", "review", now)
	}
	s.Chapters[key] = cs
	return res, nil
}

// Snooze passes on a recommended chapter for now; it counts against the
// topics' interest a little and comes back after a couple of chapters.
func (e *Engine) Snooze(s *State, track, chapter string) error {
	key := chapterKey(track, chapter)
	cr, ok := e.chapters[key]
	if !ok {
		return ErrNotFound
	}
	cs := s.chapter(key)
	cs.SnoozedAt = s.completedCount()
	s.Chapters[key] = cs
	for _, t := range cr.chapter.Topics {
		ts := s.Topic(t)
		ts.Beta += 0.5
		s.Topics[t] = ts
	}
	return nil
}

// Assess seeds skill from the first-run self-assessment: topic → new | some | solid.
func (e *Engine) Assess(s *State, answers map[string]string) error {
	for t, lvl := range answers {
		if e.topics[t] == nil {
			return fmt.Errorf("%w: unknown topic %q", ErrInvalid, t)
		}
		if _, ok := levels[lvl]; !ok {
			return fmt.Errorf("%w: level %q (want new, some or solid)", ErrInvalid, lvl)
		}
	}
	for t, lvl := range answers {
		ts := s.Topic(t)
		ts.Theta, ts.Assessed = levels[lvl], true
		s.Topics[t] = ts
	}
	s.Assessed = true
	return nil
}

// insertCorrective adds one step to a chapter, after `after` ("" = before
// practice). Mode "prereq" (a step was too hard) looks for easier content
// on the chapter's prerequisite topics, then its own; "review" (outcomes
// not met) looks for another angle on the chapter's own topics, at most one
// level harder. Curated steps from other chapters and mentor items both
// qualify.
func (e *Engine) insertCorrective(s *State, pool []Item, key, after, mode string, now time.Time) string {
	cr := e.chapters[key]
	ch := cr.chapter
	if after != "" {
		for _, x := range s.Extras {
			if chapterKey(x.Track, x.Chapter) == key && x.After == after {
				return "" // one corrective per step
			}
		}
	}
	want := map[string]float64{} // topic → preference
	for _, t := range ch.Topics {
		want[t] = 1
	}
	if mode == "prereq" {
		for _, t := range ch.Topics {
			for _, r := range e.requires[t] {
				if !slices.Contains(ch.Topics, r) {
					w := 1.5
					if s.Topic(r).Theta < metTheta {
						w = 2
					}
					want[r] = max(want[r], w)
				}
			}
		}
	}
	taken := map[string]bool{}
	for _, x := range s.Extras {
		if chapterKey(x.Track, x.Chapter) == key {
			taken[x.Ref] = true
		}
	}
	type cand struct {
		ref, reason string
		score       float64
	}
	var cands []cand
	maxD := float64(ch.Difficulty) + 1 // review
	if mode == "prereq" {
		maxD = float64(ch.Difficulty) - 1 // strictly easier
	}
	consider := func(ref string, topics []string, d float64, typ, from string) {
		if taken[ref] || d > maxD {
			return
		}
		if _, seen := s.Progress[ref]; seen {
			return
		}
		best, bestT := 0.0, ""
		for _, t := range topics {
			if want[t] > best {
				best, bestT = want[t], t
			}
		}
		if best == 0 {
			return
		}
		score := best - 0.3*d
		if typ == "lab" || typ == "tool" {
			score -= 0.5
		}
		reason := "Another angle on " + strings.ToLower(e.topicName(bestT))
		if !slices.Contains(ch.Topics, bestT) {
			reason = "Background on " + strings.ToLower(e.topicName(bestT))
		}
		if from != "" {
			reason += " — from " + from
		}
		cands = append(cands, cand{ref, reason, score})
	}
	for _, tr := range e.cur.Tracks {
		for _, och := range tr.Chapters {
			if chapterKey(tr.ID, och.ID) == key {
				continue
			}
			for _, st := range och.Steps {
				consider(StepRef(tr.ID, och.ID, st.ID), och.Topics, float64(och.Difficulty), st.Type, `"`+och.Title+`"`)
			}
		}
	}
	for _, it := range pool {
		if it.Mentor == "" || it.Difficulty == 0 {
			continue
		}
		consider(ItemRef(it.ID), it.Topics, float64(it.Difficulty), it.Type, "")
	}
	if len(cands) == 0 {
		return ""
	}
	slices.SortStableFunc(cands, func(a, b cand) int { return cmp.Compare(b.score, a.score) })
	c := cands[0]
	s.Extras = append(s.Extras, Extra{Track: cr.track.ID, Chapter: ch.ID, Ref: c.ref, After: after, Reason: c.reason, At: now})
	return c.ref
}
