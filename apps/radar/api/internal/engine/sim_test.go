package engine

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"
)

// learner is a simulated user with a true skill and interest per area.
type learner struct {
	skill    map[string]float64 // area → true skill (1–5)
	interest map[string]float64 // area → P(worth it)
	r        *rand.Rand
}

func (l *learner) area(e *Engine, topics []string) string { return e.topics[topics[0]].Area }

func (l *learner) rating(e *Engine, topics []string, d float64) Rating {
	gap := d - l.skill[l.area(e, topics)]
	switch {
	case gap <= -1:
		return Easy
	case gap >= 1.5:
		return Hard
	}
	return Right
}

func (l *learner) worth(e *Engine, topics []string) *bool {
	b := l.r.Float64() < l.interest[l.area(e, topics)]
	return &b
}

// learn: working through content at difficulty d nudges true skill up.
func (l *learner) learn(e *Engine, topics []string, d float64) {
	a := l.area(e, topics)
	if l.skill[a] < d+0.5 {
		l.skill[a] = min(5, l.skill[a]+0.1)
	}
}

type simResult struct {
	completed []string // chapter keys in completion order
	capstones []string
	actions   int
	extras    int
}

// simulate runs a learner until nothing is left (or the budget runs out).
func simulate(t *testing.T, e *Engine, s *State, l *learner, budget int) simResult {
	t.Helper()
	now := t0
	var res simResult
	tries := map[string]int{}
	for res.actions = 0; res.actions < budget; res.actions++ {
		d := e.Dashboard(s, nil, now)
		c := d.Continue
		if c == nil {
			break
		}
		n := c.Next
		if n == nil {
			t.Fatalf("recommendation without a next step: %+v", c)
		}
		now = now.Add(45 * time.Minute)
		key := chapterKey(c.Track, c.Chapter)
		var topics []string
		var diff float64
		if c.Kind == "chapter" {
			ch := e.chapters[key].chapter
			topics, diff = ch.Topics, float64(ch.Difficulty)
		}
		switch n.Kind {
		case "step", "corrective", "practice":
			if c.Kind != "chapter" {
				t.Fatalf("step outside a chapter: %+v", c)
			}
			if err := e.Done(s, nil, n.Ref, l.rating(e, topics, diff), l.worth(e, topics), now); err != nil {
				t.Fatalf("done %s: %v", n.Ref, err)
			}
			l.learn(e, topics, diff)
		case "checkpoint":
			met := make([]bool, len(e.chapters[key].chapter.Outcomes))
			ok := l.skill[l.area(e, topics)] >= diff-0.5
			for i := range met {
				met[i] = ok || i == 0
			}
			tries[key]++
			r, err := e.Checkpoint(s, nil, c.Track, c.Chapter, met, l.rating(e, topics, diff), l.worth(e, topics), tries[key] > 1, now)
			if err != nil {
				t.Fatalf("checkpoint %s: %v", key, err)
			}
			if r.Completed {
				if slices.Contains(res.completed, key) {
					t.Fatalf("chapter %s completed twice", key)
				}
				res.completed = append(res.completed, key)
			}
		case "capstone":
			if err := e.Done(s, nil, n.Ref, Right, yes(), now); err != nil {
				t.Fatal(err)
			}
			res.capstones = append(res.capstones, c.Track)
		default:
			t.Fatalf("unexpected next kind %q", n.Kind)
		}
	}
	res.extras = len(s.Extras)
	return res
}

func readyChapters(e *Engine) (n int, tracks int) {
	for _, tr := range e.cur.Tracks {
		if !tr.Outline() {
			n += len(tr.Chapters)
			tracks++
		}
	}
	return n, tracks
}

func TestSimFinishesEverythingInOrder(t *testing.T) {
	e := newEngine(t)
	for seed := range uint64(5) {
		s := NewState()
		l := &learner{
			skill:    map[string]float64{"foundations": 2, "offense": 1.5, "ai-security": 2.5, "defense": 2, "ai-swe": 3},
			interest: map[string]float64{"foundations": 0.6, "offense": 0.6, "ai-security": 0.6, "defense": 0.6, "ai-swe": 0.6},
			r:        rand.New(rand.NewPCG(seed, 1)),
		}
		res := simulate(t, e, s, l, 3000)
		want, tracks := readyChapters(e)
		if len(res.completed) != want || len(res.capstones) != tracks {
			t.Fatalf("seed %d: completed %d/%d chapters, %d/%d capstones in %d actions",
				seed, len(res.completed), want, len(res.capstones), tracks, res.actions)
		}
		// Within a track, chapters complete in course order.
		last := map[string]int{}
		for _, key := range res.completed {
			cr := e.chapters[key]
			if prev, ok := last[cr.track.ID]; ok && cr.index != prev+1 {
				t.Errorf("seed %d: %s out of order", seed, key)
			}
			last[cr.track.ID] = cr.index
		}
		if d := e.Dashboard(s, nil, t0); d.Continue != nil {
			t.Errorf("seed %d: still recommends %+v after everything is done", seed, d.Continue)
		}
	}
}

// firstTracks counts which track the first n completed chapters came from.
func firstTracks(res simResult, n int) map[string]int {
	out := map[string]int{}
	for _, key := range res.completed[:min(n, len(res.completed))] {
		out[strings.Split(key, "/")[0]]++
	}
	return out
}

func TestSimInterestSteersTracks(t *testing.T) {
	e := newEngine(t)
	skill := map[string]float64{"foundations": 2, "offense": 2, "ai-security": 2.5, "defense": 2, "ai-swe": 2.5}
	cases := []struct {
		name  string
		likes string // area
		track string
	}{
		{"security fan", "ai-security", "ai-red-teaming"},
		{"swe fan", "ai-swe", "agentic-engineering"},
	}
	for _, c := range cases {
		wins := 0
		for seed := range uint64(6) {
			interest := map[string]float64{"foundations": 0.1, "offense": 0.1, "ai-security": 0.1, "defense": 0.1, "ai-swe": 0.1}
			interest[c.likes] = 0.95
			l := &learner{skill: map[string]float64{}, interest: interest, r: rand.New(rand.NewPCG(seed, 2))}
			for k, v := range skill {
				l.skill[k] = v
			}
			res := simulate(t, e, NewState(), l, 3000)
			if got := firstTracks(res, 8); got[c.track] >= 5 {
				wins++
			} else {
				t.Logf("%s seed %d: first 8 chapters %v", c.name, seed, got)
			}
		}
		if wins < 5 {
			t.Errorf("%s: favourite track led the first 8 chapters in only %d/6 runs", c.name, wins)
		}
	}
}

func TestSimSkillEstimatesFollowTruth(t *testing.T) {
	e := newEngine(t)
	run := func(swe float64) float64 {
		s := NewState()
		l := &learner{
			skill:    map[string]float64{"foundations": 2, "offense": 2, "ai-security": 2, "defense": 2, "ai-swe": swe},
			interest: map[string]float64{"foundations": 0.5, "offense": 0.5, "ai-security": 0.5, "defense": 0.5, "ai-swe": 0.5},
			r:        rand.New(rand.NewPCG(7, 3)),
		}
		simulate(t, e, s, l, 40)
		return s.Topic("agentic-engineering").Theta
	}
	strong, weak := run(5), run(1)
	if strong <= weak {
		t.Errorf("skill estimate: strong learner %.2f ≤ weak learner %.2f", strong, weak)
	}
}

func TestSimStrugglingLearnerGetsCorrectives(t *testing.T) {
	e := newEngine(t)
	s := NewState()
	s.Settings.Priorities = map[string]float64{"ai-security": 2} // steer into red teaming
	l := &learner{
		skill:    map[string]float64{"foundations": 1, "offense": 1, "ai-security": 1, "defense": 1, "ai-swe": 1},
		interest: map[string]float64{"foundations": 0.5, "offense": 0.5, "ai-security": 0.9, "defense": 0.5, "ai-swe": 0.5},
		r:        rand.New(rand.NewPCG(9, 4)),
	}
	res := simulate(t, e, s, l, 200)
	if res.extras == 0 {
		t.Fatal("a learner rating everything too hard got no corrective steps")
	}
	for _, x := range s.Extras {
		if _, err := e.resolve(x.Ref, nil); err != nil {
			t.Errorf("corrective %s doesn't resolve: %v", x.Ref, err)
		}
	}
	t.Logf("%d correctives over %d actions", res.extras, res.actions)
}
