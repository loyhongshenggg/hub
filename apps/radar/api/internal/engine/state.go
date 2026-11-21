// Package engine is Radar's adaptive learning engine. It is pure: it reads
// and updates an in-memory State (persisted by the store) against the
// curated curriculum, so it can be tested with simulated learners.
//
// Within a chapter, steps keep their order; "too easy" offers a skip to
// practice and "too hard" inserts a corrective step. When no chapter is in
// progress, the next one is picked from each track's frontier by Thompson-
// sampled interest, Elo-style fit, learning progress (ZPDES), momentum and
// area priority, with prerequisites as a soft gate. See docs/radar/design.md.
package engine

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Rating is how a finished step or chapter felt.
type Rating string

const (
	Unrated Rating = ""
	Easy    Rating = "easy"
	Right   Rating = "right"
	Hard    Rating = "hard"
)

func (r Rating) Valid() bool { return r == Unrated || r == Easy || r == Right || r == Hard }

// Progress statuses.
const (
	Done      = "done"
	Skipped   = "skipped"   // passed over (e.g. skip to practice); no minutes
	Dismissed = "dismissed" // an optional fresh or corrective step waved off
)

// Progress records what happened to one piece of content.
type Progress struct {
	Ref     string    `json:"ref"`
	Status  string    `json:"status"`
	At      time.Time `json:"at"`
	Minutes int       `json:"minutes"`
	Rating  Rating    `json:"rating,omitempty"`
	Worth   *bool     `json:"worth,omitempty"`
}

// ChapterState is a chapter's lifecycle.
type ChapterState struct {
	Started   time.Time `json:"started"`
	Completed time.Time `json:"completed"`
	// Outcomes is the last checkpoint answer, one per chapter outcome.
	Outcomes []bool `json:"outcomes"`
	Rating   Rating `json:"rating,omitempty"`
	Worth    *bool  `json:"worth,omitempty"`
	// SnoozedAt is the completed-chapter count when it was snoozed (-1:
	// never). It stays out of recommendations for snoozeFor completions.
	SnoozedAt int `json:"snoozedAt"`
}

// TopicState is what Radar believes about one skill-map topic.
type TopicState struct {
	Theta    float64   `json:"theta"` // skill on the 1–5 difficulty scale
	N        int       `json:"n"`     // observations so far
	Assessed bool      `json:"assessed"`
	Alpha    float64   `json:"alpha"` // interest: Beta(α, β)
	Beta     float64   `json:"beta"`
	Obs      []float64 `json:"obs"` // recent outcomes in [0,1], oldest first
}

// Beta is a Beta(α, β) belief about how worthwhile something is.
type Beta struct {
	Alpha float64 `json:"alpha"`
	Beta  float64 `json:"beta"`
}

func (b Beta) Mean() float64 { return b.Alpha / (b.Alpha + b.Beta) }

// Extra is a step the engine inserted into a chapter.
type Extra struct {
	Track   string    `json:"track"`
	Chapter string    `json:"chapter"`
	Ref     string    `json:"ref"`
	After   string    `json:"after"` // shown after this ref ("" = before practice)
	Reason  string    `json:"reason"`
	At      time.Time `json:"at"`
}

// Settings are the learner's preferences.
type Settings struct {
	WeeklyMinutes int                `json:"weeklyMinutes"`
	Priorities    map[string]float64 `json:"priorities"` // area → weight (default 1)
	Timezone      string             `json:"timezone"`   // IANA name; "" = server local
}

// State is everything the engine knows about the learner.
type State struct {
	Progress map[string]Progress
	Chapters map[string]ChapterState // key: "<track>/<chapter>"
	Topics   map[string]TopicState
	// TrackInterest is interest per track (topic interest is in Topics).
	TrackInterest map[string]Beta
	Extras        []Extra
	LastTrack     string
	Assessed      bool
	Settings      Settings
}

func NewState() *State {
	return &State{
		Progress:      map[string]Progress{},
		Chapters:      map[string]ChapterState{},
		Topics:        map[string]TopicState{},
		TrackInterest: map[string]Beta{},
		Settings:      DefaultSettings(),
	}
}

func DefaultSettings() Settings {
	return Settings{WeeklyMinutes: 300, Priorities: map[string]float64{}}
}

// Topic returns a topic's state, with priors for one never seen.
func (s *State) Topic(id string) TopicState {
	if t, ok := s.Topics[id]; ok {
		return t
	}
	return TopicState{Theta: defaultTheta, Alpha: 1, Beta: 1}
}

func (s *State) track(id string) Beta {
	if b, ok := s.TrackInterest[id]; ok {
		return b
	}
	return Beta{1, 1}
}

func (s *State) chapter(key string) ChapterState {
	if c, ok := s.Chapters[key]; ok {
		return c
	}
	return ChapterState{SnoozedAt: -1}
}

func (s *State) completedCount() int {
	n := 0
	for _, c := range s.Chapters {
		if !c.Completed.IsZero() {
			n++
		}
	}
	return n
}

// Item is a fetched item (a mentor post or video) the engine can offer as a
// fresh or corrective step.
type Item struct {
	ID         int64     `json:"id"`
	Title      string    `json:"title"`
	URL        string    `json:"url"`
	Type       string    `json:"type"`
	Minutes    int       `json:"minutes"`
	Difficulty int       `json:"difficulty"`
	Topics     []string  `json:"topics"`
	Mentor     string    `json:"mentor"`
	Published  time.Time `json:"published"`
}

// Refs name content: "step:<track>/<chapter>/<step>", "practice:<track>/<chapter>",
// "capstone:<track>", "item:<id>".
type Ref struct {
	Kind    string // step | practice | capstone | item
	Track   string
	Chapter string
	Step    string
	Item    int64
}

func StepRef(track, chapter, step string) string { return "step:" + track + "/" + chapter + "/" + step }
func PracticeRef(track, chapter string) string   { return "practice:" + track + "/" + chapter }
func CapstoneRef(track string) string            { return "capstone:" + track }
func ItemRef(id int64) string                    { return "item:" + strconv.FormatInt(id, 10) }
func chapterKey(track, chapter string) string    { return track + "/" + chapter }

func ParseRef(s string) (Ref, error) {
	kind, rest, ok := strings.Cut(s, ":")
	if !ok || rest == "" {
		return Ref{}, fmt.Errorf("bad ref %q", s)
	}
	parts := strings.Split(rest, "/")
	r := Ref{Kind: kind}
	switch {
	case kind == "step" && len(parts) == 3:
		r.Track, r.Chapter, r.Step = parts[0], parts[1], parts[2]
	case kind == "practice" && len(parts) == 2:
		r.Track, r.Chapter = parts[0], parts[1]
	case kind == "capstone" && len(parts) == 1:
		r.Track = parts[0]
	case kind == "item" && len(parts) == 1:
		id, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || id <= 0 {
			return Ref{}, fmt.Errorf("bad ref %q", s)
		}
		r.Item = id
	default:
		return Ref{}, fmt.Errorf("bad ref %q", s)
	}
	return r, nil
}

func errInvalidf(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)
}
