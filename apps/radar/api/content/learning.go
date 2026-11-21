package content

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

//go:embed skills.yaml mentors.yaml tracks/*.yaml
var learningFS embed.FS

type Area struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

// Topic is a node of the skill map.
type Topic struct {
	ID       string   `yaml:"id" json:"id"`
	Name     string   `yaml:"name" json:"name"`
	Area     string   `yaml:"area" json:"area"`
	Requires []string `yaml:"requires" json:"requires"`
	Keywords []string `yaml:"keywords" json:"-"`
}

type Link struct {
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url" json:"url"`
}

// MentorFeed brings a mentor's new work in.
type MentorFeed struct {
	URL    string   `yaml:"url" json:"url"`
	Type   string   `yaml:"type" json:"type"` // post | video
	Topics []string `yaml:"topics" json:"topics"`
}

type Mentor struct {
	ID     string       `yaml:"id" json:"id"`
	Name   string       `yaml:"name" json:"name"`
	Handle string       `yaml:"handle" json:"handle,omitempty"`
	Role   string       `yaml:"role" json:"role"`
	Focus  string       `yaml:"focus" json:"focus"`
	Area   string       `yaml:"area" json:"area"`
	Links  []Link       `yaml:"links" json:"links"`
	Feeds  []MentorFeed `yaml:"feeds" json:"-"`
}

// Origin is a non-mentor source a step can credit.
type Origin struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
}

// Step is one ordered lesson item: a resource plus what to focus on.
type Step struct {
	ID      string `yaml:"id" json:"id"`
	Title   string `yaml:"title" json:"title"`
	Type    string `yaml:"type" json:"type"` // read | watch | lab | tool
	By      string `yaml:"by" json:"by"`
	URL     string `yaml:"url" json:"url"`
	Minutes int    `yaml:"minutes" json:"minutes"`
	Focus   string `yaml:"focus" json:"focus"`
}

type Practice struct {
	Title   string   `yaml:"title" json:"title"`
	Type    string   `yaml:"type" json:"type"` // lab | walkthrough
	URL     string   `yaml:"url" json:"url,omitempty"`
	Minutes int      `yaml:"minutes" json:"minutes"`
	Steps   []string `yaml:"steps" json:"steps"`
}

type Chapter struct {
	ID         string    `yaml:"id" json:"id"`
	Title      string    `yaml:"title" json:"title"`
	Summary    string    `yaml:"summary" json:"summary,omitempty"`
	Difficulty int       `yaml:"difficulty" json:"difficulty"`
	Topics     []string  `yaml:"topics" json:"topics"`
	Outcomes   []string  `yaml:"outcomes" json:"outcomes"`
	Steps      []Step    `yaml:"steps" json:"steps"`
	Practice   *Practice `yaml:"practice" json:"practice,omitempty"`
}

// Minutes is the chapter's estimated time: steps plus practice.
func (c Chapter) Minutes() int {
	n := 0
	for _, s := range c.Steps {
		n += s.Minutes
	}
	if c.Practice != nil {
		n += c.Practice.Minutes
	}
	return n
}

type Capstone struct {
	Title    string   `yaml:"title" json:"title"`
	Summary  string   `yaml:"summary" json:"summary"`
	Outcomes []string `yaml:"outcomes" json:"outcomes"`
	Steps    []string `yaml:"steps" json:"steps"`
}

// Track is a course: one skill, ordered chapters, a capstone.
type Track struct {
	ID       string    `yaml:"id" json:"id"`
	Title    string    `yaml:"title" json:"title"`
	Tagline  string    `yaml:"tagline" json:"tagline"`
	Area     string    `yaml:"area" json:"area"`
	Level    string    `yaml:"level" json:"level"` // intro | intermediate | advanced
	Mentors  []string  `yaml:"mentors" json:"mentors"`
	Status   string    `yaml:"status" json:"status,omitempty"` // "" (ready) | outline
	Chapters []Chapter `yaml:"chapters" json:"chapters"`
	Capstone *Capstone `yaml:"capstone" json:"capstone,omitempty"`
}

func (t Track) Outline() bool { return t.Status == "outline" }

// Curriculum is everything Radar teaches, loaded from content/.
type Curriculum struct {
	Areas      []Area   `json:"areas"`
	Topics     []Topic  `json:"topics"`
	Mentors    []Mentor `json:"mentors"`
	GapFillers []Origin `json:"gapFillers"`
	Tracks     []Track  `json:"tracks"`

	topics  map[string]*Topic
	origins map[string]string // id → display name (mentors + gap fillers)
}

func (c *Curriculum) Topic(id string) *Topic { return c.topics[id] }

// OriginName is the display name of a step's "by" (mentor or gap filler).
func (c *Curriculum) OriginName(id string) string { return c.origins[id] }

func (c *Curriculum) Track(id string) *Track {
	for i := range c.Tracks {
		if c.Tracks[i].ID == id {
			return &c.Tracks[i]
		}
	}
	return nil
}

// LoadCurriculum parses and validates the embedded content.
func LoadCurriculum() (*Curriculum, error) { return ParseCurriculum(learningFS) }

func decodeStrict(r io.Reader, v any) error {
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	return dec.Decode(v)
}

func decodeFile(fsys fs.FS, name string, v any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return err
	}
	if err := decodeStrict(bytes.NewReader(data), v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// ParseCurriculum reads skills.yaml, mentors.yaml and tracks/*.yaml from fsys.
func ParseCurriculum(fsys fs.FS) (*Curriculum, error) {
	var skills struct {
		Areas  []Area  `yaml:"areas"`
		Topics []Topic `yaml:"topics"`
	}
	var people struct {
		Mentors    []Mentor `yaml:"mentors"`
		GapFillers []Origin `yaml:"gapFillers"`
	}
	if err := decodeFile(fsys, "skills.yaml", &skills); err != nil {
		return nil, err
	}
	if err := decodeFile(fsys, "mentors.yaml", &people); err != nil {
		return nil, err
	}
	c := &Curriculum{Areas: skills.Areas, Topics: skills.Topics, Mentors: people.Mentors, GapFillers: people.GapFillers}

	files, err := fs.Glob(fsys, "tracks/*.yaml")
	if err != nil {
		return nil, err
	}
	slices.Sort(files)
	for _, f := range files {
		var t Track
		if err := decodeFile(fsys, f, &t); err != nil {
			return nil, err
		}
		if want := strings.TrimSuffix(path.Base(f), ".yaml"); t.ID != want {
			return nil, fmt.Errorf("%s: id %q must match the file name %q", f, t.ID, want)
		}
		c.Tracks = append(c.Tracks, t)
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}

var (
	stepTypes     = []string{"read", "watch", "lab", "tool"}
	practiceTypes = []string{"lab", "walkthrough"}
	levels        = []string{"intro", "intermediate", "advanced"}
	feedTypes     = []string{"post", "video"}
)

func checkURL(where, raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("%s: bad url %q", where, raw)
	}
	return nil
}

func (c *Curriculum) validate() error {
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	areas := map[string]bool{}
	for _, a := range c.Areas {
		if !idRe.MatchString(a.ID) || areas[a.ID] {
			bad("area %q: bad or duplicate id", a.ID)
		}
		areas[a.ID] = true
	}
	c.topics = map[string]*Topic{}
	for i := range c.Topics {
		t := &c.Topics[i]
		if !idRe.MatchString(t.ID) || c.topics[t.ID] != nil {
			bad("topic %q: bad or duplicate id", t.ID)
		}
		if !areas[t.Area] {
			bad("topic %s: unknown area %q", t.ID, t.Area)
		}
		c.topics[t.ID] = t
	}
	for _, t := range c.Topics {
		for _, r := range t.Requires {
			if c.topics[r] == nil {
				bad("topic %s: requires unknown topic %q", t.ID, r)
			}
		}
	}
	if cyc := c.topicCycle(); cyc != "" {
		bad("skill map has a cycle: %s", cyc)
	}

	c.origins = map[string]string{}
	for _, m := range c.Mentors {
		if !idRe.MatchString(m.ID) || c.origins[m.ID] != "" {
			bad("mentor %q: bad or duplicate id", m.ID)
		}
		c.origins[m.ID] = m.Name
		if !areas[m.Area] {
			bad("mentor %s: unknown area %q", m.ID, m.Area)
		}
		for _, l := range m.Links {
			if err := checkURL("mentor "+m.ID, l.URL); err != nil {
				errs = append(errs, err)
			}
		}
		for _, f := range m.Feeds {
			if err := checkURL("mentor "+m.ID+" feed", f.URL); err != nil {
				errs = append(errs, err)
			}
			if !slices.Contains(feedTypes, f.Type) {
				bad("mentor %s: feed type %q (want post or video)", m.ID, f.Type)
			}
			for _, t := range f.Topics {
				if c.topics[t] == nil {
					bad("mentor %s: feed topic %q unknown", m.ID, t)
				}
			}
		}
	}
	for _, g := range c.GapFillers {
		if !idRe.MatchString(g.ID) || c.origins[g.ID] != "" {
			bad("gap filler %q: bad or duplicate id", g.ID)
		}
		c.origins[g.ID] = g.Name
		if err := checkURL("gap filler "+g.ID, g.URL); err != nil {
			errs = append(errs, err)
		}
	}

	tracks := map[string]bool{}
	for _, t := range c.Tracks {
		where := "track " + t.ID
		if !idRe.MatchString(t.ID) || tracks[t.ID] {
			bad("%s: bad or duplicate id", where)
		}
		tracks[t.ID] = true
		if t.Title == "" || t.Tagline == "" {
			bad("%s: title and tagline are required", where)
		}
		if !areas[t.Area] {
			bad("%s: unknown area %q", where, t.Area)
		}
		if !slices.Contains(levels, t.Level) {
			bad("%s: level %q (want intro, intermediate or advanced)", where, t.Level)
		}
		if t.Status != "" && t.Status != "outline" {
			bad("%s: status %q (want empty or outline)", where, t.Status)
		}
		for _, m := range t.Mentors {
			if c.origins[m] == "" {
				bad("%s: unknown mentor %q", where, m)
			}
		}
		if len(t.Chapters) == 0 {
			bad("%s: no chapters", where)
		}
		if !t.Outline() && t.Capstone == nil {
			bad("%s: a capstone is required (or mark the track status: outline)", where)
		}
		chapters, steps := map[string]bool{}, map[string]bool{}
		for _, ch := range t.Chapters {
			cw := where + " chapter " + ch.ID
			if !idRe.MatchString(ch.ID) || chapters[ch.ID] {
				bad("%s: bad or duplicate id", cw)
			}
			chapters[ch.ID] = true
			if ch.Title == "" || len(ch.Outcomes) == 0 {
				bad("%s: title and at least one outcome are required", cw)
			}
			if ch.Difficulty < 1 || ch.Difficulty > 5 {
				bad("%s: difficulty %d (want 1–5)", cw, ch.Difficulty)
			}
			if len(ch.Topics) == 0 {
				bad("%s: at least one topic is required", cw)
			}
			for _, tp := range ch.Topics {
				if c.topics[tp] == nil {
					bad("%s: unknown topic %q", cw, tp)
				}
			}
			if !t.Outline() && len(ch.Steps) == 0 {
				bad("%s: no steps (only outline tracks may omit them)", cw)
			}
			for _, s := range ch.Steps {
				sw := cw + " step " + s.ID
				if !idRe.MatchString(s.ID) || steps[s.ID] {
					bad("%s: bad or duplicate id (unique per track)", sw)
				}
				steps[s.ID] = true
				if s.Title == "" || s.Focus == "" {
					bad("%s: title and focus are required", sw)
				}
				if !slices.Contains(stepTypes, s.Type) {
					bad("%s: type %q (want read, watch, lab or tool)", sw, s.Type)
				}
				if c.origins[s.By] == "" {
					bad("%s: by %q is not a mentor or gap filler", sw, s.By)
				}
				if s.Minutes <= 0 {
					bad("%s: minutes must be positive", sw)
				}
				if err := checkURL(sw, s.URL); err != nil {
					errs = append(errs, err)
				}
			}
			if p := ch.Practice; p != nil {
				pw := cw + " practice"
				if p.Title == "" || len(p.Steps) == 0 || p.Minutes <= 0 {
					bad("%s: title, steps and minutes are required", pw)
				}
				if !slices.Contains(practiceTypes, p.Type) {
					bad("%s: type %q (want lab or walkthrough)", pw, p.Type)
				}
				if p.Type == "lab" && p.URL == "" {
					bad("%s: a lab needs a url", pw)
				}
				if p.URL != "" {
					if err := checkURL(pw, p.URL); err != nil {
						errs = append(errs, err)
					}
				}
			}
		}
	}
	return errors.Join(errs...)
}

// topicCycle returns a description of a requires-cycle, or "".
func (c *Curriculum) topicCycle() string {
	const (
		white = iota
		grey
		black
	)
	color := map[string]int{}
	var stack []string
	var found string
	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		if t := c.topics[id]; t != nil {
			for _, r := range t.Requires {
				switch color[r] {
				case grey:
					i := slices.Index(stack, r)
					found = strings.Join(append(slices.Clone(stack[i:]), r), " → ")
					return true
				case white:
					if visit(r) {
						return true
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	for _, t := range c.Topics {
		if color[t.ID] == white && visit(t.ID) {
			return found
		}
	}
	return ""
}
