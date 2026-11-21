// Package content holds Radar's curated, version-controlled data: news
// sources, the skill map, mentors and tracks. The database only stores state.
package content

import (
	_ "embed"
	"fmt"
	"net/url"
	"regexp"

	"go.yaml.in/yaml/v4"
)

//go:embed sources.yaml
var sourcesYAML []byte

type Category struct {
	ID   string `yaml:"id" json:"id"`
	Name string `yaml:"name" json:"name"`
}

type Source struct {
	ID       string `yaml:"id" json:"id"`
	Name     string `yaml:"name" json:"name"`
	Category string `yaml:"category" json:"category"`
	URL      string `yaml:"url" json:"url"`
	// Topics are skill-map topics the tagger falls back on when an item
	// matches none (checked by Feeds).
	Topics []string `yaml:"topics" json:"topics,omitempty"`
	// Mentor and Kind are set for mentor feeds (see Feeds), never in YAML.
	Mentor string `yaml:"-" json:"mentor,omitempty"`
	Kind   string `yaml:"-" json:"kind,omitempty"` // post | video
}

type Sources struct {
	Categories []Category `yaml:"categories"`
	Sources    []Source   `yaml:"sources"`
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// LoadSources parses and checks sources.yaml.
func LoadSources() (Sources, error) { return ParseSources(sourcesYAML) }

func ParseSources(data []byte) (Sources, error) {
	var s Sources
	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("sources.yaml: %w", err)
	}
	cats := map[string]bool{}
	for _, c := range s.Categories {
		cats[c.ID] = true
	}
	seen := map[string]bool{}
	for i, src := range s.Sources {
		u, err := url.Parse(src.URL)
		switch {
		case !idRe.MatchString(src.ID):
			return s, fmt.Errorf("sources.yaml: source %d: bad id %q", i, src.ID)
		case seen[src.ID]:
			return s, fmt.Errorf("sources.yaml: duplicate source id %q", src.ID)
		case !cats[src.Category]:
			return s, fmt.Errorf("sources.yaml: %s: unknown category %q", src.ID, src.Category)
		case src.Name == "":
			return s, fmt.Errorf("sources.yaml: %s: name is required", src.ID)
		case err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "":
			return s, fmt.Errorf("sources.yaml: %s: bad url %q", src.ID, src.URL)
		}
		seen[src.ID] = true
	}
	return s, nil
}
