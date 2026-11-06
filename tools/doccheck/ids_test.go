package main

import (
	"reflect"
	"testing"
)

func labels(refs []idRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.label)
	}
	return out
}

func TestReferences(t *testing.T) {
	tests := []struct {
		name string
		kind idKind
		in   string
		want []string
	}{
		{"single", decisionIDs, "see D-07 and D-100", []string{"D-07", "D-100"}},
		{"slash list", decisionIDs, "D-01/D-02", []string{"D-01", "D-02"}},
		{"range both prefixed", decisionIDs, "D-61..D-64", []string{"D-61", "D-62", "D-63", "D-64"}},
		{"range short end", requirementIDs, "REQ-031..033", []string{"REQ-031", "REQ-032", "REQ-033"}},
		{"reversed range falls back to endpoints", decisionIDs, "D-9..D-3", []string{"D-9", "D-3"}},
		{"oversized range falls back to endpoints", decisionIDs, "D-1..D-99999", []string{"D-1", "D-99999"}},
		{"not glued to words", decisionIDs, "UD-5 and ID-6 and x-D-7", nil},
		{"lower case anchors ignored", decisionIDs, "(plan.md#d-07)", nil},
		{"work package", workPackageIDs, "AP-06's scope and AP-01..AP-03", []string{"AP-06", "AP-01", "AP-02", "AP-03"}},
		{"contract not TECH", contractIDs, "TECH-01 and C-12.", []string{"C-12"}},
		{"contract range", contractIDs, "C-01..C-03", []string{"C-01", "C-02", "C-03"}},
		{"requirement not in word", requirementIDs, "XREQ-001 REQ-002", []string{"REQ-002"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labels(tt.kind.references(tt.in)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("references(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEvidenceReferences(t *testing.T) {
	var got []string
	for _, r := range evidenceReferences("AP-03 E-05, [AP-06 E-06..E-08](x) and AP-4 E-1") {
		got = append(got, r.label)
	}
	want := []string{"AP-03 E-05", "AP-06 E-06", "AP-06 E-07", "AP-06 E-08", "AP-4 E-1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCheckIDsEndToEnd(t *testing.T) {
	root := writeTree(t, map[string]string{
		"development/authoritative-plan.md":    "| D-01 | one |\n| D-02 | two |\n| D-02 | again |\n",
		"development/requirements-catalog.md":  "| REQ-001 | a |\n| REQ-002 | b |\n",
		"development/work-packages/ap-01-x.md": "# AP-01\n",
		"development/contracts/c-01-x.md":      "# C-01\n",
		"development/execution/ap-01.md":       "# Record\n\n### E-01 First\n",
		"notes.md": "D-01 D-03 REQ-002 REQ-009 AP-01 AP-02 C-01 C-02 `D-77`\n" +
			"AP-01 E-01, AP-01 E-02, AP-02 E-01\n" +
			"```\nD-88 REQ-888\n```\n" +
			"D-01..D-03\n",
	})
	docs, err := loadDocuments(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range checkAll(root, docs) {
		got = append(got, f.String())
	}
	want := []string{
		"development/authoritative-plan.md:3: duplicate-decision: D-02 listed again",
		"notes.md:1: unknown-contract: C-02 no file in development/contracts/",
		"notes.md:1: unknown-decision: D-03 not in the decision register",
		"notes.md:1: unknown-decision: D-77 not in the decision register",
		"notes.md:1: unknown-requirement: REQ-009 not in the requirements catalog",
		"notes.md:1: unknown-work-package: AP-02 no file in development/work-packages/",
		"notes.md:2: unknown-evidence: AP-01 E-02: no E-02 heading in development/execution/ap-01.md",
		"notes.md:2: unknown-evidence: AP-02 E-01: no development/execution/ap-02.md",
		"notes.md:2: unknown-work-package: AP-02 no file in development/work-packages/",
		"notes.md:6: unknown-decision: D-03 not in the decision register",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings:\n%q\nwant:\n%q", got, want)
	}
}
