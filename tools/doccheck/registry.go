package main

import (
	"fmt"
	"regexp"
	"strconv"
)

// Fixed locations of the identifier sources, relative to the repository root.
const (
	decisionRegisterPath = "development/authoritative-plan.md"
	requirementCatalogue = "development/requirements-catalog.md"
	workPackageDirPrefix = "development/work-packages/"
	contractDirPrefix    = "development/contracts/"
	executionDirPrefix   = "development/execution/"
)

var (
	decisionRowRe    = regexp.MustCompile(`^\| D-(\d+) \|`)
	requirementRowRe = regexp.MustCompile(`^\| REQ-(\d+) \|`)
	workPackageFile  = regexp.MustCompile(`^` + workPackageDirPrefix + `ap-(\d+)-[^/]+\.md$`)
	contractFile     = regexp.MustCompile(`^` + contractDirPrefix + `c-(\d+)-[^/]+\.md$`)
	executionFile    = regexp.MustCompile(`^` + executionDirPrefix + `ap-(\d+)\.md$`)
	sourceFile       = regexp.MustCompile(`^` + executionDirPrefix + `source-ap-(\d+)\.md$`)
	evidenceHeading  = regexp.MustCompile(`^E-(\d+)\b`)
)

// registry holds every identifier that exists in the repository.
type registry struct {
	decisions    map[int]bool
	requirements map[int]bool
	workPackages map[int]bool
	contracts    map[int]bool
	// evidence maps an AP number to the E-numbers of its execution record; an AP
	// without an execution record has no entry.
	evidence map[int]map[int]bool
	// sourceEvidence maps an AP number of the old numbering to the E-numbers of its archived
	// source record.
	sourceEvidence map[int]map[int]bool
}

// buildRegistry derives the identifier sets from the parsed documents and
// returns the duplicate decision rows it met on the way.
func buildRegistry(docs []*document) (*registry, []finding) {
	r := &registry{
		decisions: map[int]bool{}, requirements: map[int]bool{},
		workPackages: map[int]bool{}, contracts: map[int]bool{},
		evidence: map[int]map[int]bool{}, sourceEvidence: map[int]map[int]bool{},
	}
	var dups []finding
	for _, d := range docs {
		if m := workPackageFile.FindStringSubmatch(d.path); m != nil {
			r.workPackages[atoi(m[1])] = true
		}
		if m := contractFile.FindStringSubmatch(d.path); m != nil {
			r.contracts[atoi(m[1])] = true
		}
		if m := executionFile.FindStringSubmatch(d.path); m != nil {
			r.evidence[atoi(m[1])] = evidenceNumbers(d)
		}
		if m := sourceFile.FindStringSubmatch(d.path); m != nil {
			r.sourceEvidence[atoi(m[1])] = evidenceNumbers(d)
		}
		switch d.path {
		case decisionRegisterPath:
			dups = append(dups, collectRows(d, decisionRowRe, "D", r.decisions)...)
		case requirementCatalogue:
			dups = append(dups, collectRows(d, requirementRowRe, "REQ", r.requirements)...)
		}
	}
	return r, dups
}

// collectRows records the IDs of the table rows in d matching re into set and
// returns a finding for every ID that appears in more than one row.
func collectRows(d *document, re *regexp.Regexp, prefix string, set map[int]bool) []finding {
	var dups []finding
	for i, line := range d.prose {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n := atoi(m[1])
		if set[n] {
			dups = append(dups, finding{d.path, i + 1, "duplicate-" + kindName(prefix), fmt.Sprintf("%s-%s listed again", prefix, m[1])})
		}
		set[n] = true
	}
	return dups
}

// kindName maps a register prefix to the noun used in finding kinds.
func kindName(prefix string) string {
	if prefix == "D" {
		return "decision"
	}
	return "requirement"
}

// evidenceNumbers returns the E-numbers of the headings of an execution record.
func evidenceNumbers(d *document) map[int]bool {
	set := map[int]bool{}
	for _, h := range d.headings {
		if m := evidenceHeading.FindStringSubmatch(h.text); m != nil {
			set[atoi(m[1])] = true
		}
	}
	return set
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
