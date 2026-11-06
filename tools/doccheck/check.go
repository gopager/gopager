package main

import (
	"fmt"
	"sort"
)

// finding is one problem at a source line.
type finding struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func (f finding) String() string {
	return fmt.Sprintf("%s:%d: %s: %s", f.Path, f.Line, f.Kind, f.Detail)
}

// checkAll runs every rule over the documents and returns the sorted, de-duplicated findings.
// root is the directory holding the repository; rules are the loaded configurations, if any.
func checkAll(root string, docs []*document, rules []*ruleSet) []finding {
	var out []finding
	for _, rs := range rules {
		out = append(out, rs.build(docs)...)
	}
	anchorSets := map[string]map[string]bool{}
	for _, d := range docs {
		anchorSets[d.path] = d.anchors()
	}
	out = append(out, checkLinks(root, docs, anchorSets)...)
	for _, d := range docs {
		for _, rs := range rules {
			out = append(out, rs.check(d)...)
		}
	}
	return sortUnique(out)
}

// check verifies every reference of rs outside fenced code in d. Inline code is included on
// purpose: identifiers are routinely written in backticks.
func (rs *ruleSet) check(d *document) []finding {
	var out []finding
	for i, line := range d.prose {
		for _, k := range rs.kinds {
			for _, ref := range k.ref.references(line) {
				if !k.known[ref.n] {
					out = append(out, finding{d.path, i + 1, "unknown-" + k.cfg.Name, ref.label + " " + k.cfg.Missing})
				}
			}
		}
		for _, k := range rs.headingKinds {
			for _, ref := range k.ref.references(line) {
				if msg := k.missing(ref); msg != "" {
					out = append(out, finding{d.path, i + 1, "unknown-" + k.name, msg})
				}
			}
		}
	}
	return out
}

// missing returns the problem with a reference, or "".
func (k *headingKindRule) missing(ref reference) string {
	for _, s := range k.sources {
		if s.cfg.Variant != ref.variant {
			continue
		}
		set, ok := s.known[ref.doc]
		switch {
		case !ok:
			return ref.label + ": " + s.cfg.MissingDocument
		case !set[ref.n]:
			return ref.label + ": " + s.cfg.MissingHeading
		}
		return ""
	}
	return fmt.Sprintf("%s: no source for variant %q", ref.label, ref.variant)
}

// sortUnique orders findings by path, line, kind, detail and drops exact duplicates.
func sortUnique(in []finding) []finding {
	sort.Slice(in, func(i, j int) bool {
		a, b := in[i], in[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Detail < b.Detail
	})
	out := in[:0]
	for i, f := range in {
		if i == 0 || f != in[i-1] {
			out = append(out, f)
		}
	}
	return out
}
