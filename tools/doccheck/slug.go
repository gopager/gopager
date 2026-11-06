package main

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	mdLinkRe     = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	underEmphRe  = regexp.MustCompile(`(^|[\s(])_+([^_]+?)_+($|[\s).,;:])`)
	explicitIDRe = regexp.MustCompile(`<a\s+[^>]*?\b(?:id|name)\s*=\s*"([^"]+)"|\{#([A-Za-z0-9_.:-]+)\}`)
)

// slugify converts heading text to its GitHub anchor: inline Markdown is reduced
// to its text, the result is lower-cased, characters other than letters, digits,
// underscores, hyphens and spaces are removed, and each space becomes a hyphen.
func slugify(text string) string {
	text = htmlTagRe.ReplaceAllString(text, "")
	text = mdLinkRe.ReplaceAllString(text, "$1")
	text = underEmphRe.ReplaceAllString(text, "$1$2$3")
	text = strings.ReplaceAll(text, "`", "")
	text = strings.ReplaceAll(text, "*", "")
	var sb strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		switch {
		case r == ' ':
			sb.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// anchors returns every fragment a link into d can use: the GitHub slug of each
// heading (duplicates receive -1, -2, ...), explicit <a id>/<a name> anchors
// anywhere outside fenced code and kramdown {#id} attributes. Keys are lower-case.
func (d *document) anchors() map[string]bool {
	set := map[string]bool{}
	seen := map[string]int{}
	for _, h := range d.headings {
		s := slugify(h.text)
		if n := seen[s]; n > 0 {
			s += "-" + strconv.Itoa(n)
		}
		seen[slugify(h.text)]++
		set[s] = true
	}
	for _, line := range d.prose {
		for _, m := range explicitIDRe.FindAllStringSubmatch(line, -1) {
			set[strings.ToLower(m[1]+m[2])] = true
		}
	}
	return set
}

// normalizeFragment percent-decodes and lower-cases a link fragment.
func normalizeFragment(f string) string {
	if dec, err := url.PathUnescape(f); err == nil {
		f = dec
	}
	return strings.ToLower(f)
}
