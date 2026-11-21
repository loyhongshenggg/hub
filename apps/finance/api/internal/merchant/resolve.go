package merchant

import (
	"sync/atomic"

	"hub.localhost/apps/finance/api/internal/catalog"
)

// Match is the resolved merchant for a descriptor.
type Match struct {
	Name       string  `json:"name"`
	Category   string  `json:"category"`
	Confidence float64 `json:"confidence"` // 0..1
	// Source: rule (yours) | special | brand | keyword | osm | acra | claude | none
	Source string `json:"source"`
	Ref    string `json:"ref,omitempty"`
	Detail string `json:"detail,omitempty"`
	Check  bool   `json:"check,omitempty"` // ask the user to confirm
	// Kind overrides the transaction kind (a rule can say "transfer").
	Kind string `json:"-"`
}

// Confidence thresholds (docs/finance/design.md §3).
const (
	AutoConfidence  = 0.8  // used without a flag
	CheckConfidence = 0.55 // used, flagged to check
)

// Resolver runs the cascade.
type Resolver struct {
	Brands *Brands
	// Rule looks up the user's own correction for a descriptor key.
	Rule func(key string) (Match, bool)
	dir  atomic.Pointer[Directory]
}

// SetDirectory swaps in a new directory index.
func (r *Resolver) SetDirectory(d *Directory) { r.dir.Store(d) }

// Directory returns the current index (may be nil).
func (r *Resolver) Directory() *Directory { return r.dir.Load() }

// Resolve cleans a descriptor and returns its merchant and transaction kind.
func (r *Resolver) Resolve(raw string, credit bool) (Descriptor, Match, string) {
	d := Clean(raw)
	kind := catalog.Purchase
	if credit {
		kind = catalog.Refund
	}
	switch d.Special {
	case "payment":
		return d, Match{Name: "Card payment", Confidence: 1, Source: "special"}, catalog.Payment
	case "fee":
		return d, Match{Name: Title(d.Name), Category: "fees", Confidence: 1, Source: "special"}, catalog.Fee
	}
	if r.Rule != nil {
		if m, ok := r.Rule(d.Key); ok {
			m.Source, m.Confidence, m.Check = "rule", 1, false
			if m.Kind != "" {
				kind = m.Kind
			}
			return d, m, kind
		}
	}
	if d.Special == "transit" {
		return d, Match{Name: "SimplyGo (bus/MRT)", Category: "transport", Confidence: 1, Source: "special"}, kind
	}
	return d, r.match(d), kind
}

func (r *Resolver) match(d Descriptor) Match {
	if r.Brands != nil {
		if b, ok := r.Brands.Match(d); ok {
			return Match{Name: b.Name, Category: b.Category, Confidence: 0.95, Source: "brand", Check: b.Check}
		}
	}
	fallback := Match{Name: Title(d.Name), Category: "other", Source: "none", Check: true}
	if r.Brands != nil {
		if cat, ok := r.Brands.Keyword(d); ok {
			fallback = Match{Name: Title(d.Name), Category: cat, Confidence: 0.65, Source: "keyword", Check: true}
		}
	}
	if dir := r.dir.Load(); dir != nil {
		cs := dir.Lookup(d, 3)
		if len(cs) > 0 && cs[0].Score >= CheckConfidence {
			c := cs[0]
			conf := c.Score
			// Two equally good entries in different categories: less sure.
			if len(cs) > 1 && cs[0].Score-cs[1].Score < 0.02 && cs[1].Category != c.Category {
				conf -= 0.1
			}
			if conf >= CheckConfidence && conf >= fallback.Confidence {
				name, detail := c.Name, joinNonEmpty(c.Branch, c.Detail)
				if c.Source == "acra" {
					name = Title(coreName(c.Name))
				}
				if !c.Strong {
					// Only similar ("SMILE DENTAL STUDIO" ~ "Bright Dental Studio"):
					// keep the statement's name, borrow the category.
					name, detail = Title(d.Name), "similar to "+name
					conf = min(conf, AutoConfidence-0.01)
				}
				return Match{Name: name, Category: c.Category, Confidence: conf, Source: c.Source,
					Ref: c.Ref, Detail: detail, Check: conf < AutoConfidence}
			}
		}
	}
	return fallback
}
