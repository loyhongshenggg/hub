package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"hub.localhost/apps/radar/api/internal/engine"
)

func TestLearningRoundTrip(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	empty, err := s.LoadLearning(ctx)
	if err != nil || len(empty.Progress) != 0 || empty.Assessed || empty.Settings.WeeklyMinutes != 300 {
		t.Fatalf("empty state: %+v %v", empty, err)
	}

	yes := true
	at := time.Unix(1_800_000_000, 0).UTC()
	st := engine.NewState()
	st.Progress["step:a/b/c"] = engine.Progress{Ref: "step:a/b/c", Status: engine.Done, At: at, Minutes: 20, Rating: engine.Hard, Worth: &yes}
	st.Progress["item:7"] = engine.Progress{Ref: "item:7", Status: engine.Dismissed, At: at}
	st.Chapters["a/b"] = engine.ChapterState{Started: at, Outcomes: []bool{true, false}, Rating: engine.Right, SnoozedAt: -1}
	st.Chapters["a/c"] = engine.ChapterState{Started: at, Completed: at, SnoozedAt: 3}
	st.Topics["prompt-injection"] = engine.TopicState{Theta: 2.4, N: 3, Assessed: true, Alpha: 2, Beta: 1, Obs: []float64{0, 0.6, 1}}
	st.TrackInterest["a"] = engine.Beta{Alpha: 3, Beta: 1}
	st.Extras = []engine.Extra{{Track: "a", Chapter: "b", Ref: "item:9", After: "step:a/b/c", Reason: "Background", At: at}}
	st.LastTrack, st.Assessed = "a", true
	st.Settings = engine.Settings{WeeklyMinutes: 240, Priorities: map[string]float64{"ai-security": 2}, Timezone: "Asia/Singapore"}

	if err := s.SaveLearning(ctx, st); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadLearning(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, st) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, st)
	}

	// Saving again replaces (undo removes rows).
	delete(st.Progress, "item:7")
	s.SaveLearning(ctx, st)
	if got, _ := s.LoadLearning(ctx); len(got.Progress) != 1 {
		t.Errorf("deleted progress came back: %+v", got.Progress)
	}
}

func TestMentorPool(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	mentorSeed(t, s)
	pool, err := s.MentorPool(ctx)
	if err != nil || len(pool) != 2 || pool[0].Mentor != "rehberger" || pool[0].Type != "read" {
		t.Fatalf("pool: %+v %v", pool, err)
	}
}
