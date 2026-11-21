package statement

import (
	"regexp"
	"sort"
	"strings"

	"hub.localhost/apps/finance/api/internal/ocr"
)

// line is a row of words that share a baseline, sorted left to right.
type line struct {
	page   int
	y0, y1 int
	words  []ocr.Word
}

func (l line) text() string {
	parts := make([]string, len(l.words))
	for i, w := range l.words {
		parts[i] = w.Text
	}
	return strings.Join(parts, " ")
}

func (l line) x0() int {
	if len(l.words) == 0 {
		return 0
	}
	return l.words[0].X0
}

func (l line) height() int { return max(1, l.y1-l.y0) }

// groupLines clusters words into lines by vertical overlap.
func groupLines(page int, words []ocr.Word) []line {
	ws := make([]ocr.Word, 0, len(words))
	for _, w := range words {
		if strings.TrimSpace(w.Text) != "" {
			ws = append(ws, w)
		}
	}
	sort.Slice(ws, func(i, j int) bool { return center(ws[i]) < center(ws[j]) })
	// A word joins the current line when its vertical centre is within ~half
	// a word height of the line's mean centre. Comparing against the mean
	// (not the growing box) stops neighbouring rows from chaining together.
	var out []line
	var sumC, sumH, n float64
	for _, w := range ws {
		c, h := float64(center(w)), float64(max(1, w.Y1-w.Y0))
		if len(out) > 0 {
			l := &out[len(out)-1]
			mc, mh := sumC/n, sumH/n
			if c-mc <= 0.45*max(mh, h) {
				l.words = append(l.words, w)
				l.y0, l.y1 = min(l.y0, w.Y0), max(l.y1, w.Y1)
				sumC, sumH, n = sumC+c, sumH+h, n+1
				continue
			}
		}
		out = append(out, line{page: page, y0: w.Y0, y1: w.Y1, words: []ocr.Word{w}})
		sumC, sumH, n = c, h, 1
	}
	for i := range out {
		sort.Slice(out[i].words, func(a, b int) bool { return out[i].words[a].X0 < out[i].words[b].X0 })
	}
	return out
}

func center(w ocr.Word) int { return (w.Y0 + w.Y1) / 2 }

// column limits a line to words whose left edge is in [x0, x1).
func (l line) column(x0, x1 int) line {
	out := line{page: l.page, y0: l.y1, y1: l.y0}
	for _, w := range l.words {
		if w.X0 >= x0 && w.X0 < x1 {
			out.words = append(out.words, w)
			out.y0, out.y1 = min(out.y0, w.Y0), max(out.y1, w.Y1)
		}
	}
	if len(out.words) == 0 {
		out.y0, out.y1 = l.y0, l.y1
	}
	return out
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9&]+`)

// norm lowercases and drops punctuation, for label matching.
func norm(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

// box is a rectangle in page pixels.
type box struct{ page, x0, y0, x1, y1 int }

// findLabel returns the boxes of every place the label's words appear in a
// row (case- and punctuation-insensitive).
func findLabel(lines []line, label string) []box {
	want := strings.Fields(label)
	for i := range want {
		want[i] = norm(want[i])
	}
	var out []box
	for _, l := range lines {
	scan:
		for i := 0; i+len(want) <= len(l.words); i++ {
			for j, wt := range want {
				if norm(l.words[i+j].Text) != wt {
					continue scan
				}
			}
			first, last := l.words[i], l.words[i+len(want)-1]
			b := box{page: l.page, x0: first.X0, x1: last.X1, y0: first.Y0, y1: first.Y1}
			for _, w := range l.words[i : i+len(want)] {
				b.y0, b.y1 = min(b.y0, w.Y0), max(b.y1, w.Y1)
			}
			out = append(out, b)
		}
	}
	return out
}
