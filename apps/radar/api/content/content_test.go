package content

import (
	"strings"
	"testing"
)

func TestShippedSourcesAreValid(t *testing.T) {
	s, err := LoadSources()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sources) < 10 || len(s.Categories) != 3 {
		t.Errorf("got %d sources, %d categories", len(s.Sources), len(s.Categories))
	}
}

func TestParseSourcesRejects(t *testing.T) {
	base := "categories: [{id: ai, name: AI}]\nsources:\n"
	for _, c := range []struct{ yaml, want string }{
		{"  - {id: A, name: x, category: ai, url: 'https://x.y'}", "bad id"},
		{"  - {id: a, name: x, category: ai, url: 'https://x.y'}\n  - {id: a, name: y, category: ai, url: 'https://x.y'}", "duplicate"},
		{"  - {id: a, name: x, category: swe, url: 'https://x.y'}", "unknown category"},
		{"  - {id: a, name: x, category: ai, url: 'ftp://x.y'}", "bad url"},
		{"  - {id: a, category: ai, url: 'https://x.y'}", "name is required"},
	} {
		if _, err := ParseSources([]byte(base + c.yaml)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.yaml, err, c.want)
		}
	}
}
