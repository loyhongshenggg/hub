// Package tagger is Radar's rule-based tagger (the "Claude off" mode): it
// maps an item's title and summary onto skill-map topics with keyword
// matching, and estimates difficulty, type and minutes from simple cues.
//
// Generic keywords ("dns", "claude") must not outrank specific ones
// ("dns exfil", "claude code"), so:
//   - the longest keyword match wins a span; shorter keywords inside it
//     don't count;
//   - topics deeper in the prerequisite graph weigh more;
//   - topics with many synonyms ("cve", "flaw", "exploit") don't win by
//     volume: further title keywords add less and less, and the summary
//     (long, repetitive) counts once per topic;
//   - a foundations topic is kept only when it is the best match;
//   - a source's own topics (a mentor's focus) get a small prior.
package tagger

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"sort"
	"strings"

	"hub.localhost/apps/radar/api/content"
	"hub.localhost/apps/radar/api/internal/store"
)

// Tags is what the tagger infers about an item.
type Tags = store.Tags

// algorithm is bumped when Tag's rules change, so stored tags get redone.
const algorithm = "rules-2"

const (
	titleWeight   = 3.0
	summaryWeight = 1.0
	depthBonus    = 0.5 // per prerequisite level
	minScore      = 1.5 // one title hit, or one summary hit on a non-root topic
	extraHit      = 0.5 // each further title keyword, up to maxTitleHits
	maxTitleHits  = 3
	sourcePrior   = minScore
	maxTopics     = 3
	foundations   = "foundations"
)

type keyword struct {
	topic string
	re    *regexp.Regexp
	n     int // length, for longest-match
}

type Tagger struct {
	// Version identifies the rules and the skill map they came from; items
	// tagged at another version are retagged.
	Version  string
	keywords []keyword
	weight   map[string]float64 // topic → specificity multiplier
	area     map[string]string
}

// New compiles one word-boundary regexp per keyword of every topic.
func New(c *content.Curriculum) *Tagger {
	t := &Tagger{weight: map[string]float64{}, area: map[string]string{}}
	byID := map[string]content.Topic{}
	for _, tp := range c.Topics {
		byID[tp.ID] = tp
	}
	var depth func(id string) int
	depth = func(id string) int { // the curriculum loader rejects cycles
		d := 0
		for _, r := range byID[id].Requires {
			d = max(d, depth(r)+1)
		}
		return d
	}
	h := sha256.New()
	for _, tp := range c.Topics {
		h.Write([]byte(tp.ID + "|" + tp.Area + "|" + strings.Join(tp.Requires, ",") + "|" + strings.Join(tp.Keywords, ",") + "\n"))
		t.weight[tp.ID] = 1 + depthBonus*float64(depth(tp.ID))
		t.area[tp.ID] = tp.Area
		for _, kw := range tp.Keywords {
			t.keywords = append(t.keywords, keyword{
				topic: tp.ID,
				re:    regexp.MustCompile(`(?i)(?:^|[^\pL\pN])(` + regexp.QuoteMeta(kw) + `)(?:$|[^\pL\pN])`),
				n:     len(kw),
			})
		}
	}
	t.Version = algorithm + "-" + hex.EncodeToString(h.Sum(nil))[:12]
	// Longest keywords claim their spans first.
	sort.SliceStable(t.keywords, func(i, j int) bool { return t.keywords[i].n > t.keywords[j].n })
	return t
}

// hits returns, per topic, how many distinct keywords matched text, where a
// match inside a span already claimed by a longer keyword doesn't count.
func (t *Tagger) hits(text string) map[string]int {
	var taken [][2]int
	overlaps := func(a, b int) bool {
		for _, s := range taken {
			if a < s[1] && s[0] < b {
				return true
			}
		}
		return false
	}
	out := map[string]int{}
	for _, kw := range t.keywords {
		matched := false
		for _, m := range kw.re.FindAllStringSubmatchIndex(text, -1) {
			if overlaps(m[2], m[3]) {
				continue
			}
			taken = append(taken, [2]int{m[2], m[3]})
			matched = true
		}
		if matched {
			out[kw.topic]++
		}
	}
	return out
}

var (
	easyRe = regexp.MustCompile(`(?i)\b(intro|introduction|beginner|basics|101|what is|getting started|explained|primer|overview)\b`)
	hardRe = regexp.MustCompile(`(?i)\b(deep dive|advanced|internals|exploit chain|cve-\d|rce|remote code execution|zero-day|0-day|kernel|bypass|heap)\b`)
)

// Item is the input to Tag.
type Item struct {
	Title   string
	Summary string
	URL     string
	// Kind is the source's declared type (post | video), if known.
	Kind string
	// Defaults are topics to fall back on when nothing matches.
	Defaults []string
}

// Tag infers topics (best first, at most 3), difficulty (1–5), type and minutes.
func (t *Tagger) Tag(it Item) Tags {
	score := map[string]float64{}
	field := func(text string, w float64, maxHits int) {
		for topic, n := range t.hits(text) {
			score[topic] += w * (1 + extraHit*float64(min(n, maxHits)-1)) * t.weight[topic]
		}
	}
	field(it.Title, titleWeight, maxTitleHits)
	field(it.Summary, summaryWeight, 1)
	if len(score) == 0 {
		return t.finish(it, slices.Clone(it.Defaults))
	}
	for _, d := range it.Defaults {
		if _, ok := t.weight[d]; ok {
			score[d] += sourcePrior
		}
	}
	var ranked []string
	for topic, s := range score {
		if s >= minScore {
			ranked = append(ranked, topic)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if score[ranked[i]] != score[ranked[j]] {
			return score[ranked[i]] > score[ranked[j]]
		}
		return ranked[i] < ranked[j]
	})
	var topics []string
	for i, topic := range ranked {
		if len(topics) == maxTopics {
			break
		}
		if i > 0 && t.area[topic] == foundations {
			continue // "dns" in an exfil post isn't a networking lesson
		}
		topics = append(topics, topic)
	}
	if len(topics) == 0 {
		topics = slices.Clone(it.Defaults)
	}
	return t.finish(it, topics)
}

// finish adds difficulty, type and minutes.
func (t *Tagger) finish(it Item, topics []string) Tags {
	if topics == nil {
		topics = []string{}
	}
	text := it.Title + " " + it.Summary
	difficulty := 3
	switch {
	case hardRe.MatchString(text):
		difficulty = 4
	case easyRe.MatchString(text):
		difficulty = 2
	}

	typ, minutes := "read", 10
	switch {
	case it.Kind == "video" || strings.Contains(it.URL, "youtube.com/watch") || strings.Contains(it.URL, "youtu.be/"):
		typ, minutes = "watch", 20
	case strings.Contains(it.URL, "arxiv.org/"):
		minutes = 40
	case strings.Contains(it.URL, "pwn.college/"):
		typ, minutes = "lab", 60
	}
	return Tags{Topics: topics, Difficulty: difficulty, Type: typ, Minutes: minutes}
}
