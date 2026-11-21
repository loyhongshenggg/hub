package engine

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"hub.localhost/apps/radar/api/content"
)

// ChapterSummary is a chapter's row on a track page.
type ChapterSummary struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Difficulty int      `json:"difficulty"`
	Topics     []string `json:"topics"`
	Minutes    int      `json:"minutes"`
	Status     string   `json:"status"` // new | active | done
	StepsDone  int      `json:"stepsDone"`
	StepsTotal int      `json:"stepsTotal"`
}

type TrackView struct {
	TrackSummary
	Chapters []ChapterSummary  `json:"chapterList"`
	Capstone *content.Capstone `json:"capstone,omitempty"`
	// CapstoneDone is set once the capstone is marked done.
	CapstoneDone bool             `json:"capstoneDone"`
	MentorInfo   []content.Mentor `json:"mentorInfo"`
}

// Track renders a track page.
func (e *Engine) Track(s *State, pool []Item, id string, now time.Time) (TrackView, error) {
	tr := e.tracks[id]
	if tr == nil {
		return TrackView{}, ErrNotFound
	}
	d := e.Dashboard(s, pool, now)
	var v TrackView
	for _, ts := range d.Tracks {
		if ts.ID == id {
			v.TrackSummary = ts
		}
	}
	for _, ch := range tr.Chapters {
		cv, _ := e.Chapter(s, pool, id, ch.ID, now)
		st := "new"
		switch {
		case cv.Completed:
			st = "done"
		case cv.Started:
			st = "active"
		}
		v.Chapters = append(v.Chapters, ChapterSummary{ID: ch.ID, Title: ch.Title, Summary: ch.Summary,
			Difficulty: ch.Difficulty, Topics: ch.Topics, Minutes: ch.Minutes(), Status: st,
			StepsDone: cv.StepsDone, StepsTotal: cv.StepsTotal})
	}
	v.Capstone = tr.Capstone
	if p, ok := s.Progress[CapstoneRef(id)]; ok && p.Status == Done {
		v.CapstoneDone = true
	}
	v.MentorInfo = []content.Mentor{}
	for _, m := range e.cur.Mentors {
		if slices.Contains(tr.Mentors, m.ID) {
			v.MentorInfo = append(v.MentorInfo, m)
		}
	}
	return v, nil
}

// Context is the chapter a ref was (or would be) done in: the chapter it
// was inserted into, else its own. Empty for items and capstones.
func (e *Engine) Context(s *State, ref string) (track, chapter string) {
	key := s.extraOf(ref)
	if key == "" {
		if r, err := ParseRef(ref); err == nil && (r.Kind == "step" || r.Kind == "practice") {
			key = chapterKey(r.Track, r.Chapter)
		}
	}
	if cr, ok := e.chapters[key]; ok {
		return cr.track.ID, cr.chapter.ID
	}
	return "", ""
}

// HistoryEntry is one finished piece of content.
type HistoryEntry struct {
	Progress
	Kind         string `json:"kind"` // step | practice | capstone | item
	Title        string `json:"title"`
	URL          string `json:"url,omitempty"`
	By           string `json:"by,omitempty"`
	Track        string `json:"track,omitempty"`
	TrackTitle   string `json:"trackTitle,omitempty"`
	Chapter      string `json:"chapter,omitempty"`
	ChapterTitle string `json:"chapterTitle,omitempty"`
}

// History lists finished content newest first, before a time (zero: now).
func (e *Engine) History(s *State, pool []Item, before time.Time, limit int) []HistoryEntry {
	var ps []Progress
	for _, p := range s.Progress {
		if p.Status == Done && (before.IsZero() || p.At.Before(before)) {
			ps = append(ps, p)
		}
	}
	slices.SortFunc(ps, func(a, b Progress) int {
		if c := b.At.Compare(a.At); c != 0 {
			return c
		}
		return cmp.Compare(a.Ref, b.Ref)
	})
	out := []HistoryEntry{}
	for _, p := range ps[:min(limit, len(ps))] {
		h := HistoryEntry{Progress: p, Title: p.Ref}
		r, _ := ParseRef(p.Ref)
		h.Kind = r.Kind
		if tid, cid := e.Context(s, p.Ref); tid != "" {
			cr := e.chapters[chapterKey(tid, cid)]
			h.Track, h.TrackTitle, h.Chapter, h.ChapterTitle = tid, cr.track.Title, cid, cr.chapter.Title
		}
		switch r.Kind {
		case "step":
			if st := e.steps[p.Ref]; st != nil {
				h.Title, h.URL, h.By = st.Title, st.URL, e.cur.OriginName(st.By)
			}
		case "practice":
			if cr, ok := e.chapters[chapterKey(r.Track, r.Chapter)]; ok && cr.chapter.Practice != nil {
				h.Title, h.URL = cr.chapter.Practice.Title, cr.chapter.Practice.URL
			}
		case "capstone":
			if tr := e.tracks[r.Track]; tr != nil && tr.Capstone != nil {
				h.Title, h.Track, h.TrackTitle = tr.Capstone.Title, tr.ID, tr.Title
			}
		case "item":
			if it, ok := findItem(pool, r.Item); ok {
				h.Title, h.URL, h.By = it.Title, it.URL, e.cur.OriginName(it.Mentor)
			} else {
				h.Title = "Removed item"
			}
		}
		out = append(out, h)
	}
	return out
}

// Assessment lists topics for the first-run self-check, grouped by area
// order, with the current answer if any.
type AssessmentTopic struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Area  string `json:"area"`
	Level string `json:"level,omitempty"` // new | some | solid
}

func (e *Engine) Assessment(s *State) []AssessmentTopic {
	var out []AssessmentTopic
	for _, a := range e.cur.Areas {
		for _, t := range e.cur.Topics {
			if t.Area != a.ID {
				continue
			}
			at := AssessmentTopic{ID: t.ID, Name: t.Name, Area: a.ID}
			if ts, ok := s.Topics[t.ID]; ok && ts.Assessed {
				for lvl, th := range levels {
					if ts.Theta >= th-0.75 && ts.Theta < th+0.75 {
						at.Level = lvl
					}
				}
				if at.Level == "" && ts.Theta >= 4 {
					at.Level = "solid"
				}
			}
			out = append(out, at)
		}
	}
	return out
}

// ValidSettings checks settings against the curriculum.
func (e *Engine) ValidSettings(st Settings) error {
	if st.WeeklyMinutes < 30 || st.WeeklyMinutes > 3000 {
		return errInvalidf("weeklyMinutes must be 30–3000")
	}
	for a, w := range st.Priorities {
		if _, ok := e.areas[a]; !ok {
			return errInvalidf("unknown area %q", a)
		}
		if w < 0 || w > 3 {
			return errInvalidf("priority for %s must be 0–3", a)
		}
	}
	if st.Timezone != "" {
		if _, err := time.LoadLocation(st.Timezone); err != nil || strings.Contains(st.Timezone, "..") {
			return errInvalidf("unknown timezone %q", st.Timezone)
		}
	}
	return nil
}
