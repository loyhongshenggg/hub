package merchant

import (
	_ "embed"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v4"

	"hub.localhost/apps/finance/api/internal/catalog"
)

//go:embed brands.yaml
var brandsYAML []byte

// Brand is a known merchant and how its descriptors look.
type Brand struct {
	Name     string   `yaml:"name"`
	Category string   `yaml:"category"`
	Check    bool     `yaml:"check"`
	Prefix   []string `yaml:"prefix"`
	Starts   []string `yaml:"starts"`
	Contains []string `yaml:"contains"`
	Exact    []string `yaml:"exact"`
}

// Keyword is a generic word that implies a category.
type Keyword struct {
	Category string   `yaml:"category"`
	Words    []string `yaml:"words"`
}

// Brands is the curated list (brands.yaml).
type Brands struct {
	Brands   []Brand   `yaml:"brands"`
	Keywords []Keyword `yaml:"keywords"`
}

// LoadBrands parses the embedded list and checks every category exists.
func LoadBrands() (*Brands, error) {
	var b Brands
	if err := yaml.Unmarshal(brandsYAML, &b); err != nil {
		return nil, err
	}
	for _, br := range b.Brands {
		if !catalog.Valid(br.Category) {
			return nil, fmt.Errorf("brand %s: unknown category %q", br.Name, br.Category)
		}
	}
	for _, k := range b.Keywords {
		if !catalog.Valid(k.Category) {
			return nil, fmt.Errorf("keyword: unknown category %q", k.Category)
		}
	}
	return &b, nil
}

// Match finds the brand for a descriptor.
func (b *Brands) Match(d Descriptor) (Brand, bool) {
	name := Squash(d.Name)
	raw := Squash(d.Raw)
	for _, br := range b.Brands {
		for _, p := range br.Prefix {
			if d.Prefix == p {
				return br, true
			}
		}
		for _, e := range br.Exact {
			if name == e {
				return br, true
			}
		}
		for _, s := range br.Starts {
			if strings.HasPrefix(name, s) || d.Prefix == "" && strings.HasPrefix(raw, s) {
				return br, true
			}
		}
		for _, c := range br.Contains {
			if strings.Contains(raw, c) {
				return br, true
			}
		}
	}
	return Brand{}, false
}

// Keyword finds a category from generic words in the name ("DENTAL").
func (b *Brands) Keyword(d Descriptor) (string, bool) {
	padded := " " + strings.Join(strings.Fields(strings.NewReplacer("*", " ", "-", " ", "@", " ", ".", " ").Replace(d.Name+" "+d.Branch)), " ") + " "
	squashed := Squash(d.Name)
	for _, k := range b.Keywords {
		for _, w := range k.Words {
			// Long words also match inside OCR-merged names ("THEDENTALSTUDIO").
			if strings.Contains(padded, " "+w+" ") || len(w) >= 6 && strings.Contains(squashed, Squash(w)) {
				return k.Category, true
			}
		}
	}
	return "", false
}
