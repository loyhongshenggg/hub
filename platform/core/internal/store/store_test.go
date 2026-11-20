package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hub.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}

	pos, yes := 2, true
	if err := s.SetPrefs(ctx, "notes", Prefs{Pinned: true, Position: &pos, AutoStart: &yes}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPrefs(ctx, "todo", Prefs{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UIState(ctx, "tabs"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.SetUIState(ctx, "tabs", []byte(`{"open":["notes"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUIState(ctx, "bad", []byte(`{`)); err == nil {
		t.Error("invalid JSON accepted")
	}
	s.Close()

	// Reopen: data persists, migrations don't rerun.
	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.Prefs(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if !p.Pinned || p.Position == nil || *p.Position != 2 || p.AutoStart == nil || !*p.AutoStart {
		t.Errorf("notes prefs = %+v", p)
	}
	if p, _ := s.Prefs(ctx, "todo"); p.Position != nil || p.AutoStart != nil || p.Pinned {
		t.Errorf("todo prefs = %+v", p)
	}
	if v, err := s.UIState(ctx, "tabs"); err != nil || string(v) != `{"open":["notes"]}` {
		t.Errorf("tabs = %s, %v", v, err)
	}
}
