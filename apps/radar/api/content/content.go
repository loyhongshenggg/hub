// Package content holds Radar's curated, version-controlled data: the news
// sources (and, later, mentors). The database only stores state.
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
