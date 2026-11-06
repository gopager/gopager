package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// idKind describes one family of identifiers and the finding kind reported when
// a referenced member does not exist.
type idKind struct {
	prefix  string
	finding string
	re      *regexp.Regexp
}

// idRef is one referenced identifier number with the label used in findings.
type idRef struct {
	n     int
	label string
}

// refStart is the context a reference must start in: not glued to a longer word.
const refStart = `(?:^|[^A-Za-z0-9_-])`

func newIDKind(prefix, finding string) idKind {
	// Group 1: first number; group 2: end of a range such as D-61..D-95 or REQ-031..040.
	re := regexp.MustCompile(refStart + prefix + `-(\d+)(?:\.\.(?:` + prefix + `-)?(\d+))?`)
	return idKind{prefix: prefix, finding: finding, re: re}
}

var (
	decisionIDs    = newIDKind("D", "unknown-decision")
	requirementIDs = newIDKind("REQ", "unknown-requirement")
	workPackageIDs = newIDKind("AP", "unknown-work-package")
	contractIDs    = newIDKind("C", "unknown-contract")

	// evidenceRe matches "AP-03 E-05" and "AP-06 E-06..E-14".
	evidenceRe = regexp.MustCompile(refStart + `AP-(\d+) E-(\d+)(?:\.\.(?:E-)?(\d+))?`)
)

// maxRange bounds range expansion so a typo such as D-1..D-99999 cannot flood output.
const maxRange = 500

// references extracts every member referenced on line. A range expands to each
// member between its endpoints, so a gap inside a range is reported too; a
// reversed or oversized range falls back to its two endpoints.
func (k idKind) references(line string) []idRef {
	var out []idRef
	for _, m := range k.re.FindAllStringSubmatch(line, -1) {
		out = append(out, expand(k.prefix, m[1], m[2])...)
	}
	return out
}

// expand turns the number strings of a (possible) range into references.
func expand(prefix, first, last string) []idRef {
	start, _ := strconv.Atoi(first)
	ref := func(n int, text string) idRef { return idRef{n, prefix + "-" + text} }
	if last == "" {
		return []idRef{ref(start, first)}
	}
	end, _ := strconv.Atoi(last)
	if end < start || end-start > maxRange {
		return []idRef{ref(start, first), ref(end, last)}
	}
	width := len(first)
	var out []idRef
	for n := start; n <= end; n++ {
		out = append(out, ref(n, fmt.Sprintf("%0*d", width, n)))
	}
	return out
}

// evidenceRef is one "AP-nn E-m" reference.
type evidenceRef struct {
	ap, e int
	label string
}

// evidenceReferences extracts "AP-nn E-m" references, expanding E ranges.
func evidenceReferences(line string) []evidenceRef {
	var out []evidenceRef
	for _, m := range evidenceRe.FindAllStringSubmatch(line, -1) {
		ap, _ := strconv.Atoi(m[1])
		for _, e := range expand("E", m[2], m[3]) {
			out = append(out, evidenceRef{ap: ap, e: e.n, label: "AP-" + m[1] + " " + e.label})
		}
	}
	return out
}
