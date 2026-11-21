package tagger

import (
	"slices"
	"testing"

	"hub.localhost/apps/radar/api/content"
)

func newTagger(t *testing.T) *Tagger {
	c, err := content.LoadCurriculum()
	if err != nil {
		t.Fatal(err)
	}
	return New(c)
}

func TestTagTopics(t *testing.T) {
	tg := newTagger(t)
	cases := []struct {
		item Item
		want []string // must contain, in this order of preference for the first
	}{
		{Item{Title: "Claude Code: Data Exfiltration with DNS (CVE-2025-55284)", Summary: "prompt injection leaks secrets via dns exfil"}, []string{"data-exfiltration", "agentic-engineering"}},
		{Item{Title: "Hackers exploit critical vCenter zero-day", Summary: "remote code execution"}, []string{"vulnerabilities"}},
		{Item{Title: "Mutex poisoning is not for everyone"}, nil},
		{Item{Title: "Return Oriented Programming", Summary: "build rop chains to bypass NX"}, []string{"binary-exploitation"}},
		{Item{Title: "Writing custom ESLint rules for agents", Summary: "a lint rule as a guardrail"}, []string{"verification"}},
		{Item{Title: "MCP: Untrusted Servers and Confused Clients", Summary: "model context protocol tool poisoning"}, []string{"agent-security"}},
	}
	for _, c := range cases {
		got := tg.Tag(c.item).Topics
		if slices.Contains(got, "networking") {
			t.Errorf("%q: generic foundations topic in %v", c.item.Title, got)
		}
		if c.want == nil {
			if len(got) > 0 {
				t.Errorf("%q: topics %v, want none", c.item.Title, got)
			}
			continue
		}
		if len(got) == 0 || got[0] != c.want[0] {
			t.Errorf("%q: topics %v, want first %q", c.item.Title, got, c.want[0])
		}
		for _, w := range c.want {
			if !slices.Contains(got, w) {
				t.Errorf("%q: topics %v missing %q", c.item.Title, got, w)
			}
		}
	}
}

func TestTagWordBoundariesAndDefaults(t *testing.T) {
	tg := newTagger(t)
	// "trope" must not match "rop"; "malloc" alone shouldn't drag in unrelated topics.
	got := tg.Tag(Item{Title: "A trope about tropes", Defaults: []string{"malware-analysis"}})
	if !slices.Equal(got.Topics, []string{"malware-analysis"}) {
		t.Errorf("defaults not used: %v", got.Topics)
	}
}

func TestTagSynonymsAndSourcePrior(t *testing.T) {
	tg := newTagger(t)
	// Many vulnerability synonyms in the summary must not beat the subject.
	got := tg.Tag(Item{
		Title:   "Remote Code Execution in Copilot via Indirect Prompt Injection",
		Summary: "The vulnerability (CVE-2026-1) is a critical flaw; the exploit needs no interaction.",
	}).Topics
	if len(got) == 0 || got[0] != "prompt-injection" {
		t.Errorf("synonyms won: %v", got)
	}
	// A mentor's focus topic rides along when something else matched.
	got = tg.Tag(Item{Title: "From SELECT to SYSADMIN (CVE-2026-65669)", Defaults: []string{"prompt-injection"}}).Topics
	if !slices.Contains(got, "prompt-injection") || !slices.Contains(got, "vulnerabilities") {
		t.Errorf("source prior missing: %v", got)
	}
}

func TestTagDifficultyTypeMinutes(t *testing.T) {
	tg := newTagger(t)
	if g := tg.Tag(Item{Title: "Prompt injection explained", URL: "https://x.dev/a"}); g.Difficulty != 2 || g.Type != "read" || g.Minutes != 10 {
		t.Errorf("easy post: %+v", g)
	}
	if g := tg.Tag(Item{Title: "Heap exploitation deep dive", Kind: "video", URL: "https://www.youtube.com/watch?v=abc"}); g.Difficulty != 4 || g.Type != "watch" || g.Minutes != 20 {
		t.Errorf("hard video: %+v", g)
	}
	if g := tg.Tag(Item{Title: "Some paper", URL: "https://arxiv.org/abs/2302.10149"}); g.Minutes != 40 {
		t.Errorf("paper: %+v", g)
	}
}
