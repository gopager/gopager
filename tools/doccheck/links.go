package main

import (
	"os"
	"path"
	"regexp"
	"strings"
)

var (
	inlineLinkRe = regexp.MustCompile(`\]\(\s*(<[^>]*>|[^)\s]*)`)
	refDefRe     = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:\s*(<[^>]*>|\S+)`)
	schemeRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// linkTargets returns the raw link destinations on one blanked-code line:
// inline links and reference definitions.
func linkTargets(plain string) []string {
	var out []string
	for _, m := range inlineLinkRe.FindAllStringSubmatch(plain, -1) {
		out = append(out, strings.Trim(m[1], "<>"))
	}
	if m := refDefRe.FindStringSubmatch(plain); m != nil {
		out = append(out, strings.Trim(m[1], "<>"))
	}
	return out
}

// isLocal reports whether a destination points into the repository: not empty,
// not an absolute URL (http:, mailto:, ...) and not protocol-relative.
func isLocal(target string) bool {
	return target != "" && !schemeRe.MatchString(target) && !strings.HasPrefix(target, "//")
}

// splitTarget separates a destination into its path and fragment.
func splitTarget(target string) (p, frag string) {
	p, frag, _ = strings.Cut(target, "#")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p, frag
}

// resolve joins a link path to the directory of the source file; a leading slash
// is relative to the repository root. The result is slash-separated and relative
// to the root, or "" when it escapes the root.
func resolve(srcPath, linkPath string) string {
	var joined string
	if rest, ok := strings.CutPrefix(linkPath, "/"); ok {
		joined = path.Clean(rest)
	} else {
		joined = path.Join(path.Dir(srcPath), linkPath)
	}
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return ""
	}
	return joined
}

// checkLinks reports broken file targets and fragments for every document.
// anchorSets maps repository-relative Markdown paths to their anchors.
func checkLinks(root string, docs []*document, anchorSets map[string]map[string]bool) []finding {
	var out []finding
	for _, d := range docs {
		for i, line := range d.plain {
			for _, t := range linkTargets(line) {
				if isLocal(t) {
					out = append(out, checkLink(root, d, i+1, t, anchorSets)...)
				}
			}
		}
	}
	return out
}

// checkLink verifies one destination found on line n of d.
func checkLink(root string, d *document, n int, target string, anchorSets map[string]map[string]bool) []finding {
	p, frag := splitTarget(target)
	dest := d.path
	if p != "" {
		dest = resolve(d.path, p)
		if dest == "" {
			return []finding{{d.path, n, "broken-link", target + " (outside repository)"}}
		}
		if _, err := os.Stat(path.Join(root, dest)); err != nil {
			return []finding{{d.path, n, "broken-link", target}}
		}
	}
	if frag == "" {
		return nil
	}
	set, ok := anchorSets[dest]
	if !ok || set[normalizeFragment(frag)] {
		return nil
	}
	return []finding{{d.path, n, "broken-anchor", target}}
}
