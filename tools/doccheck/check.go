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

// checkAll runs every rule over the documents and returns the sorted,
// de-duplicated findings. root is the directory holding the repository.
func checkAll(root string, docs []*document) []finding {
	reg, out := buildRegistry(docs)
	anchorSets := map[string]map[string]bool{}
	for _, d := range docs {
		anchorSets[d.path] = d.anchors()
	}
	out = append(out, checkLinks(root, docs, anchorSets)...)
	for _, d := range docs {
		out = append(out, checkIDs(d, reg)...)
	}
	return sortUnique(out)
}

// checkIDs verifies every identifier referenced outside fenced code in d. Inline
// code is included on purpose: IDs are routinely written in backticks.
func checkIDs(d *document, reg *registry) []finding {
	var out []finding
	families := []struct {
		kind idKind
		set  map[int]bool
		what string
	}{
		{decisionIDs, reg.decisions, "not in the decision register"},
		{requirementIDs, reg.requirements, "not in the requirements catalog"},
		{workPackageIDs, reg.workPackages, "no file in development/work-packages/"},
		{contractIDs, reg.contracts, "no file in development/contracts/"},
	}
	for i, line := range d.prose {
		for _, f := range families {
			for _, ref := range f.kind.references(line) {
				if !f.set[ref.n] {
					out = append(out, finding{d.path, i + 1, f.kind.finding, ref.label + " " + f.what})
				}
			}
		}
		for _, ref := range evidenceReferences(line) {
			if msg := missingEvidence(reg, ref); msg != "" {
				out = append(out, finding{d.path, i + 1, "unknown-evidence", msg})
			}
		}
	}
	return out
}

// missingEvidence returns the problem with an "AP-nn E-m" reference, or "".
func missingEvidence(reg *registry, ref evidenceRef) string {
	set, ok := reg.evidence[ref.ap]
	if !ok {
		return fmt.Sprintf("%s: no development/execution/ap-%02d.md", ref.label, ref.ap)
	}
	if !set[ref.e] {
		return fmt.Sprintf("%s: no E-%02d heading in development/execution/ap-%02d.md", ref.label, ref.e, ref.ap)
	}
	return ""
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
