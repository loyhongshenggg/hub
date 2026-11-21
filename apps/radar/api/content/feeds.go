package content

import (
	"fmt"
	"strings"
)

// Feeds is everything the fetcher follows: the news sources plus one source
// per mentor feed (id "<mentor>-posts" / "<mentor>-videos"). Mentor items
// become learning content; recent ones also show in News.
func Feeds(s Sources, c *Curriculum) ([]Source, error) {
	topics := map[string]bool{}
	for _, t := range c.Topics {
		topics[t.ID] = true
	}
	out := make([]Source, 0, len(s.Sources)+len(c.Mentors))
	seen := map[string]bool{}
	for _, src := range s.Sources {
		for _, t := range src.Topics {
			if !topics[t] {
				return nil, fmt.Errorf("sources.yaml: %s: unknown topic %q", src.ID, t)
			}
		}
		src.Kind = "post"
		seen[src.ID] = true
		out = append(out, src)
	}
	for _, m := range c.Mentors {
		for i, f := range m.Feeds {
			id := m.ID + "-" + f.Type + "s"
			if n := countType(m.Feeds[:i], f.Type); n > 0 {
				id = fmt.Sprintf("%s-%d", id, n+1)
			}
			if seen[id] {
				return nil, fmt.Errorf("mentor feed id %q clashes with a news source", id)
			}
			seen[id] = true
			out = append(out, Source{
				ID:       id,
				Name:     m.Name + " · " + map[string]string{"post": "posts", "video": "videos"}[f.Type],
				Category: newsCategory(m.Area),
				URL:      f.URL,
				Topics:   f.Topics,
				Mentor:   m.ID,
				Kind:     f.Type,
			})
		}
	}
	return out, nil
}

func countType(feeds []MentorFeed, typ string) int {
	n := 0
	for _, f := range feeds {
		if f.Type == typ {
			n++
		}
	}
	return n
}

// newsCategory files a mentor's feed under a News category.
func newsCategory(area string) string {
	if strings.HasSuffix(area, "-swe") {
		return "swe"
	}
	return "security"
}
