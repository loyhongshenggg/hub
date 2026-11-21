package content

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestShippedCurriculumIsValid(t *testing.T) {
	c, err := LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	ready := 0
	steps := 0
	for _, tr := range c.Tracks {
		if !tr.Outline() {
			ready++
		}
		for _, ch := range tr.Chapters {
			steps += len(ch.Steps)
		}
	}
	if ready != 3 || len(c.Tracks) != 7 {
		t.Errorf("tracks: %d ready of %d", ready, len(c.Tracks))
	}
	if steps < 80 {
		t.Errorf("only %d steps", steps)
	}
	if c.Topic("binary-exploitation") == nil || c.OriginName("rehberger") != "Johann Rehberger" {
		t.Error("lookups broken")
	}
	ae := c.Track("agentic-engineering")
	if ae == nil || ae.Chapters[0].Minutes() <= 0 {
		t.Error("agentic-engineering track or minutes missing")
	}
}

const skills = `areas: [{id: a, name: A}]
topics:
  - {id: x, name: X, area: a, keywords: [x]}
  - {id: y, name: Y, area: a, requires: [x]}
`
const mentors = `mentors:
  - {id: m, name: M, role: r, focus: f, area: a, links: [], feeds: []}
gapFillers:
  - {id: g, name: G, url: 'https://g.example/'}
`
const track = `id: t
title: T
tagline: tag
area: a
level: intro
mentors: [m]
chapters:
  - id: c1
    title: C1
    difficulty: 2
    topics: [y]
    outcomes: [do it]
    steps:
      - {id: s1, title: S1, type: read, by: g, url: 'https://g.example/1', minutes: 5, focus: look}
    practice: {title: P, type: walkthrough, minutes: 5, steps: [go]}
capstone: {title: C, summary: s, outcomes: [o], steps: [x]}
`

func parse(skillsY, mentorsY, trackY string) error {
	_, err := ParseCurriculum(fstest.MapFS{
		"skills.yaml":   {Data: []byte(skillsY)},
		"mentors.yaml":  {Data: []byte(mentorsY)},
		"tracks/t.yaml": {Data: []byte(trackY)},
	})
	return err
}

func TestParseCurriculumOK(t *testing.T) {
	if err := parse(skills, mentors, track); err != nil {
		t.Fatal(err)
	}
}

func TestParseCurriculumRejects(t *testing.T) {
	cases := []struct{ name, skills, track, want string }{
		{"cycle", strings.Replace(skills, "keywords: [x]}", "keywords: [x], requires: [y]}", 1), track, "cycle"},
		{"unknown requires", strings.Replace(skills, "requires: [x]", "requires: [z]", 1), track, `unknown topic "z"`},
		{"unknown field", skills, strings.Replace(track, "tagline: tag", "tagline: tag\ncolour: red", 1), "colour"},
		{"file name mismatch", skills, strings.Replace(track, "id: t\n", "id: other\n", 1), "must match the file name"},
		{"bad step type", skills, strings.Replace(track, "type: read", "type: podcast", 1), "want read, watch, lab or tool"},
		{"unknown by", skills, strings.Replace(track, "by: g", "by: nobody", 1), "not a mentor or gap filler"},
		{"bad url", skills, strings.Replace(track, "'https://g.example/1'", "'javascript:alert(1)'", 1), "bad url"},
		{"no steps when ready", skills, track[:strings.Index(track, "    steps:")] + "capstone: {title: C, summary: s, outcomes: [o], steps: [x]}\n", "no steps"},
		{"no capstone", skills, track[:strings.Index(track, "capstone:")], "capstone is required"},
		{"difficulty", skills, strings.Replace(track, "difficulty: 2", "difficulty: 9", 1), "difficulty 9"},
		{"lab without url", skills, strings.Replace(track, "type: walkthrough", "type: lab", 1), "a lab needs a url"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := parse(c.skills, mentors, c.track)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

func TestOutlineTracksMayOmitStepsAndCapstone(t *testing.T) {
	outline := track[:strings.Index(track, "    steps:")]
	outline = strings.Replace(outline, "mentors: [m]", "mentors: [m]\nstatus: outline", 1)
	if err := parse(skills, mentors, outline); err != nil {
		t.Fatal(err)
	}
}
