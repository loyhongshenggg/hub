package reader

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var ytID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// YouTubeEmbed turns a YouTube link into a privacy-friendly embed URL,
// keeping a start time (t=90, t=1m30s, start=90).
func YouTubeEmbed(u *url.URL) (string, bool) {
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	var id string
	switch host {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "youtube-nocookie.com":
		switch p := strings.Split(strings.Trim(u.Path, "/"), "/"); {
		case len(p) == 1 && p[0] == "watch":
			id = u.Query().Get("v")
		case len(p) == 2 && (p[0] == "embed" || p[0] == "live" || p[0] == "shorts" || p[0] == "v"):
			id = p[1]
		}
	}
	if !ytID.MatchString(id) {
		return "", false
	}
	embed := "https://www.youtube-nocookie.com/embed/" + id
	if s := startSeconds(u.Query()); s > 0 {
		embed += "?start=" + strconv.Itoa(s)
	}
	return embed, true
}

var hms = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s?)?$`)

func startSeconds(q url.Values) int {
	v := q.Get("t")
	if v == "" {
		v = q.Get("start")
	}
	m := hms.FindStringSubmatch(v)
	if m == nil {
		return 0
	}
	n := func(s string) int { i, _ := strconv.Atoi(s); return i }
	return n(m[1])*3600 + n(m[2])*60 + n(m[3])
}

var arxivRe = regexp.MustCompile(`^/(?:abs|pdf)/(\d{4}\.\d{4,5}(?:v\d+)?)(?:\.pdf)?$`)

// readableURL maps a link to the version Radar reads best: arXiv abstract
// and PDF pages become their HTML full text.
func readableURL(u *url.URL) string {
	if strings.EqualFold(u.Hostname(), "arxiv.org") {
		if m := arxivRe.FindStringSubmatch(u.Path); m != nil {
			return "https://arxiv.org/html/" + m[1]
		}
	}
	return u.String()
}
