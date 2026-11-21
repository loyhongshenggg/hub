package reader

import (
	"bytes"
	"mime"
	"net/url"
	"regexp"
	"strings"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var policy = func() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("data-youtube").Matching(ytID).OnElements("figure")
	p.AllowAttrs("loading").Matching(regexp.MustCompile(`^(lazy|eager)$`)).OnElements("img")
	p.AllowURLSchemes("https", "http", "mailto")
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)
	return p
}()

// extract pulls the article out of a page. It returns nil and a reason when
// the page has no readable article.
func extract(p *page) (*Article, string) {
	mt, _, _ := mime.ParseMediaType(p.contentType)
	if mt != "text/html" && mt != "application/xhtml+xml" {
		return nil, "Not an article (" + mt + ")"
	}
	doc, err := html.Parse(bytes.NewReader(p.body))
	if err != nil {
		return nil, "The page couldn't be parsed"
	}
	art, err := readability.FromDocument(doc, p.url)
	if err != nil || art.Node == nil {
		return nil, "No article found on the page"
	}
	videos(art.Node)
	var buf, text bytes.Buffer
	if err := art.RenderHTML(&buf); err != nil {
		return nil, "No article found on the page"
	}
	_ = art.RenderText(&text)
	words := len(strings.Fields(text.String()))
	hasVideo := bytes.Contains(buf.Bytes(), []byte("data-youtube="))
	if words < minWords && !(hasVideo && words >= minVideo) {
		return nil, "The page has too little text to read here"
	}
	a := &Article{
		Title:   strings.TrimSpace(art.Title()),
		Byline:  strings.TrimSpace(art.Byline()),
		Site:    strings.TrimSpace(art.SiteName()),
		HTML:    policy.Sanitize(buf.String()),
		Words:   words,
		Minutes: max(1, (words+229)/230),
	}
	if img := art.ImageURL(); strings.HasPrefix(img, "https://") {
		a.Image = img
	}
	if t, err := art.PublishedTime(); err == nil && !t.IsZero() {
		t = t.UTC()
		a.Published = &t
	}
	return a, ""
}

// videos replaces embedded players with something safe to keep: YouTube
// becomes <figure data-youtube="id"> (the UI renders a nocookie player),
// other iframes become a link.
func videos(n *html.Node) {
	var iframes []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Iframe {
			iframes = append(iframes, n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	for _, f := range iframes {
		src := attr(f, "src")
		u, err := url.Parse(src)
		var repl *html.Node
		if err == nil {
			if embed, ok := YouTubeEmbed(u); ok {
				id := embed[strings.LastIndex(embed, "/")+1:]
				id, _, _ = strings.Cut(id, "?")
				repl = &html.Node{Type: html.ElementNode, Data: "figure", DataAtom: atom.Figure,
					Attr: []html.Attribute{{Key: "data-youtube", Val: id}}}
			} else if u.Scheme == "https" {
				repl = &html.Node{Type: html.ElementNode, Data: "p", DataAtom: atom.P}
				a := &html.Node{Type: html.ElementNode, Data: "a", DataAtom: atom.A, Attr: []html.Attribute{{Key: "href", Val: u.String()}}}
				a.AppendChild(&html.Node{Type: html.TextNode, Data: "Embedded content: " + u.Hostname()})
				repl.AppendChild(a)
			}
		}
		if f.Parent != nil {
			if repl != nil {
				f.Parent.InsertBefore(repl, f)
			}
			f.Parent.RemoveChild(f)
		}
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
