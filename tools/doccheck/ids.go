package main

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// refStart is the context a reference must start in: not glued to a longer word.
const refStart = `(?:^|[^A-Za-z0-9_-])`

// maxRange bounds range expansion so a typo such as X-1..X-99999 cannot flood output.
const maxRange = 500

// referencePattern finds references with the group indexes of a configured expression.
type referencePattern struct {
	re *regexp.Regexp
	// Group indexes; -1 when the expression has no such group.
	ref, num, end, doc, variant int
}

// reference is one referenced number with the label used in findings.
type reference struct {
	n, doc  int
	variant string
	label   string
}

// compileReference wraps expr in the boundary context. withDoc demands a "doc" group and
// forbids it otherwise.
func compileReference(expr string, withDoc bool) (*referencePattern, error) {
	if expr == "" {
		return nil, errors.New("reference is required")
	}
	re, err := regexp.Compile(refStart + `(?P<ref>` + expr + `)`)
	if err != nil {
		return nil, fmt.Errorf("reference: %w", err)
	}
	p := &referencePattern{re: re, ref: re.SubexpIndex("ref"), num: re.SubexpIndex("num"), end: re.SubexpIndex("end"),
		doc: re.SubexpIndex("doc"), variant: re.SubexpIndex("variant")}
	switch {
	case p.num < 0:
		return nil, errors.New(`reference: missing named group "num"`)
	case withDoc && p.doc < 0:
		return nil, errors.New(`reference: missing named group "doc"`)
	case !withDoc && p.doc >= 0:
		return nil, errors.New(`reference: group "doc" is only allowed in headingKinds`)
	}
	return p, nil
}

// references extracts every member referenced on line. A range expands to each member between
// its endpoints, so a gap inside a range is reported too; a reversed or oversized range falls
// back to its two endpoints.
func (p *referencePattern) references(line string) []reference {
	group := func(m []int, i int) string {
		if i < 0 || m[2*i] < 0 {
			return ""
		}
		return line[m[2*i]:m[2*i+1]]
	}
	var out []reference
	for _, m := range p.re.FindAllStringSubmatchIndex(line, -1) {
		prefix := line[m[2*p.ref]:m[2*p.num]]
		doc, _ := strconv.Atoi(group(m, p.doc))
		for _, r := range expand(prefix, group(m, p.num), group(m, p.end)) {
			r.doc, r.variant = doc, group(m, p.variant)
			out = append(out, r)
		}
	}
	return out
}

// expand turns the number strings of a (possible) range into references labelled prefix+number.
func expand(prefix, first, last string) []reference {
	start, _ := strconv.Atoi(first)
	ref := func(n int, text string) reference { return reference{n: n, label: prefix + text} }
	if last == "" {
		return []reference{ref(start, first)}
	}
	end, _ := strconv.Atoi(last)
	if end < start || end-start > maxRange {
		return []reference{ref(start, first), ref(end, last)}
	}
	width := len(first)
	var out []reference
	for n := start; n <= end; n++ {
		out = append(out, ref(n, fmt.Sprintf("%0*d", width, n)))
	}
	return out
}
