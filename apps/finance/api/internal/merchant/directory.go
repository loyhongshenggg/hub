package merchant

import (
	"regexp"
	"sort"
	"strings"
)

// Entry is a place or registered business from an open dataset.
type Entry struct {
	Source   string `json:"source"` // osm | acra
	Ref      string `json:"ref"`    // "node/123" or a UEN
	Name     string `json:"name"`   // display name
	Branch   string `json:"branch,omitempty"`
	Category string `json:"category"`
	Address  string `json:"address,omitempty"`
	Detail   string `json:"detail,omitempty"` // OSM type or SSIC description
}

type indexed struct {
	Entry
	squash string   // core name, letters+digits
	tokens []string // core name words (no stopwords)
	where  string   // upper-case branch + address
}

// Directory is an in-memory index for fuzzy merchant lookups.
type Directory struct {
	items   []indexed
	byToken map[string][]int32
	byPfx   map[string][]int32
}

var (
	reLegal  = regexp.MustCompile(`\b(PTE\.?\s*LTD\.?|PTE\.?|PRIVATE LIMITED|LIMITED|LTD\.?|LLP|L\.L\.P\.?|INC\.?|CORPORATION|CORP\.?|CO\.)\s*$`)
	reMarker = regexp.MustCompile(`\((S|SG|SINGAPORE|PTE)\)|\bS'PORE\b`)
	reWordCh = regexp.MustCompile(`[^A-Z0-9& ]+`)
)

var stopwords = set("THE AND & OF SG SINGAPORE PTE LTD CO S")

// coreName strips legal suffixes and markers: "GAO JI FOOD (S) PTE. LTD." →
// "GAO JI FOOD".
func coreName(name string) string {
	s := strings.ToUpper(name)
	s = reMarker.ReplaceAllString(s, " ")
	for i := 0; i < 3; i++ {
		t := strings.TrimSpace(reLegal.ReplaceAllString(strings.TrimSpace(s), ""))
		if t == s || t == "" {
			break
		}
		s = t
	}
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "SINGAPORE"))
	return squashSpaces(s)
}

func tokens(s string) []string {
	var out []string
	for _, f := range strings.Fields(reWordCh.ReplaceAllString(strings.ToUpper(s), " ")) {
		if !stopwords[f] {
			out = append(out, f)
		}
	}
	return out
}

// NewDirectory indexes entries.
func NewDirectory(entries []Entry) *Directory {
	d := &Directory{byToken: map[string][]int32{}, byPfx: map[string][]int32{}}
	for _, e := range entries {
		core := coreName(e.Name)
		it := indexed{Entry: e, squash: Squash(core), tokens: tokens(core), where: strings.ToUpper(e.Branch + " " + e.Address + " " + e.Name)}
		if len(it.squash) < 2 {
			continue
		}
		i := int32(len(d.items))
		d.items = append(d.items, it)
		seen := map[string]bool{}
		for _, t := range it.tokens {
			if len(t) >= 2 && !seen[t] {
				seen[t] = true
				d.byToken[t] = append(d.byToken[t], i)
			}
		}
		if len(it.squash) >= 4 {
			p := it.squash[:4]
			d.byPfx[p] = append(d.byPfx[p], i)
		}
	}
	return d
}

// Len is the number of indexed entries.
func (d *Directory) Len() int {
	if d == nil {
		return 0
	}
	return len(d.items)
}

// Candidate is a scored directory entry.
type Candidate struct {
	Entry
	Score float64 `json:"score"`
	// Strong: the descriptor is this entry's name (possibly truncated), not
	// just a similar one, so the entry's name can replace it.
	Strong bool `json:"strong"`
}

// Lookup returns the best matches for a descriptor (highest first).
func (d *Directory) Lookup(desc Descriptor, limit int) []Candidate {
	if d.Len() == 0 {
		return nil
	}
	dq := Squash(desc.Name)
	dt := tokens(desc.Name)
	if len(dq) < 3 || len(dt) == 0 {
		return nil // nothing but stopwords ("SINGAPORE")
	}
	cand := map[int32]bool{}
	add := func(ids []int32) {
		for _, i := range ids {
			if len(cand) >= 4000 {
				return
			}
			cand[i] = true
		}
	}
	for _, t := range dt {
		if ids := d.byToken[t]; len(t) >= 3 && (len(ids) <= 3000 || len(dt) == 1) {
			add(ids)
		}
	}
	if len(dq) >= 4 {
		add(d.byPfx[dq[:4]])
	}
	terms := placeTerms(desc.Branch)
	var out []Candidate
	for i := range cand {
		it := &d.items[i]
		if s, strong := score(dq, dt, terms, it); s > 0.3 {
			out = append(out, Candidate{Entry: it.Entry, Score: s, Strong: strong})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		if out[a].Source != out[b].Source {
			return out[a].Source == "osm" // consumer-facing names
		}
		return len(out[a].Name) < len(out[b].Name)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// score rates how well a descriptor matches an entry (0..1).
//
// Descriptors are often cut at ~22 characters, so a descriptor that is a
// prefix of the entry's name scores high ("DAILY FUSION CUISI" →
// "DAILY FUSION CUISINE"). Otherwise word overlap (the last descriptor word
// may be truncated) and character-trigram similarity. A branch hint that
// matches the entry's address adds a little.
func score(dq string, dt, terms []string, it *indexed) (float64, bool) {
	cq := it.squash
	var s float64
	prefix := false
	switch {
	case len(dq) >= 5 && strings.HasPrefix(cq, dq):
		prefix = true
		s = 0.78 + 0.22*float64(len(dq))/float64(len(cq))
		if len(dq) < 8 {
			s -= 0.12
		}
	case len(cq) >= 5 && strings.HasPrefix(dq, cq):
		prefix = true
		s = 0.68 + 0.27*float64(len(cq))/float64(len(dq))
	}
	matched := 0
	for i, t := range dt {
		for _, c := range it.tokens {
			if c == t || i == len(dt)-1 && len(t) >= 3 && strings.HasPrefix(c, t) {
				matched++
				break
			}
		}
	}
	if matched == 0 && !prefix {
		return 0, false
	}
	tj := float64(matched) / float64(max(len(dt), len(it.tokens), 1))
	s = max(s, 0.5*tj+0.5*trigramDice(dq, cq))
	for _, t := range terms {
		if strings.Contains(it.where, t) {
			s += 0.08
			break
		}
	}
	strong := prefix || matched == len(dt) && len(dt) == len(it.tokens)
	return min(s, 1), strong
}

func trigrams(s string) map[string]bool {
	m := map[string]bool{}
	p := "  " + s + " "
	for i := 0; i+3 <= len(p); i++ {
		m[p[i:i+3]] = true
	}
	return m
}

func trigramDice(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	ta, tb := trigrams(a), trigrams(b)
	inter := 0
	for g := range ta {
		if tb[g] {
			inter++
		}
	}
	return 2 * float64(inter) / float64(len(ta)+len(tb))
}
