package main

import (
	"reflect"
	"strings"
	"testing"
)

// findingStrings runs the whole check on the tree at root and returns the rendered findings.
func findingStrings(t *testing.T, root string) []string {
	t.Helper()
	findings, err := analyze(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range findings {
		got = append(got, f.String())
	}
	return got
}

func expect(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings:\n%q\nwant:\n%q", got, want)
	}
}

const noteRef = `X-(?P<num>\\d+)(?:\\.\\.(?:X-)?(?P<end>\\d+))?`

func TestNoConfigChecksLinksAndAnchorsOnly(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"a.md": "# A\nX-01 X-02 [bad](gone.md)\n",
	})
	expect(t, findingStrings(t, root), []string{"a.md:2: broken-link: gone.md"})
}

func TestRegisterRowsAndRanges(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"notes/.doccheck.json": `{"kinds":[{"name":"note","reference":"` + noteRef + `","missing":"is not in the register",
			"rows":{"file":"register.md","pattern":"^\\| (?P<id>X-(?P<num>\\d+)) \\|"}}]}`,
		"notes/register.md": "| X-01 | one |\n| X-02 | two |\n| X-02 | again |\n",
		"other.md": "X-01 X-03 `X-77` UX-5 ID-6\n" +
			"```\nX-88\n```\n" +
			"X-01..X-03 X-1..2 X-9..X-3 X-1..X-99999\n",
	})
	expect(t, findingStrings(t, root), []string{
		"notes/register.md:3: duplicate-note: X-02 listed again",
		"other.md:1: unknown-note: X-03 is not in the register",
		"other.md:1: unknown-note: X-77 is not in the register",
		"other.md:5: unknown-note: X-03 is not in the register",
		"other.md:5: unknown-note: X-3 is not in the register",
		"other.md:5: unknown-note: X-9 is not in the register",
		"other.md:5: unknown-note: X-99999 is not in the register",
	})
}

func TestFileNameSet(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"notes/.doccheck.json": `{"kinds":[{"name":"item","reference":"` + noteRef + `","missing":"has no file",
			"files":"^items/x-(?P<num>\\d+)-[^/]+\\.md$"}]}`,
		"notes/items/x-01-first.md": "# First\n",
		"other/items/x-02-wrong.md": "# Outside the configuration directory\n",
		"use.md":                    "X-01 X-02\n",
	})
	expect(t, findingStrings(t, root), []string{"use.md:1: unknown-item: X-02 has no file"})
}

func TestHeadingSet(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		".doccheck.json": `{"kinds":[{"name":"topic","reference":"` + noteRef + `","missing":"has no heading",
			"headings":{"files":"^topics/.*\\.md$","pattern":"^X-(?P<num>\\d+)\\b"}}]}`,
		"topics/a.md": "# Topics\n\n## X-01 First\n\n### X-02 Second\n\n```\n# X-03 not a heading\n```\n",
		"use.md":      "X-01 X-02 X-03\n",
	})
	expect(t, findingStrings(t, root), []string{"use.md:1: unknown-topic: X-03 has no heading"})
}

const entryConfig = `{"headingKinds":[{"name":"entry",
	"reference":"(?:(?P<variant>old) )?Y-(?P<doc>\\d+) N-(?P<num>\\d+)(?:\\.\\.(?:N-)?(?P<end>\\d+))?",
	"sources":[%s]}]}`

const currentSource = `{"variant":"","files":"^records/y-(?P<doc>\\d+)\\.md$","heading":"^N-(?P<num>\\d+)\\b",
	"missingDocument":"has no record","missingHeading":"has no such heading"}`

const oldSource = `{"variant":"old","files":"^records/old-y-(?P<doc>\\d+)\\.md$","heading":"^N-(?P<num>\\d+)\\b",
	"missingDocument":"has no old record","missingHeading":"has no such old heading"}`

func TestCombinedHeadingFormWithAndWithoutVariant(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"notes/.doccheck.json":      strings.Replace(entryConfig, "%s", currentSource+","+oldSource, 1),
		"notes/records/y-01.md":     "# Record\n\n### N-01 First\n\n### N-02 Second\n",
		"notes/records/old-y-07.md": "# Old\n\n### N-38 Old entry\n",
		"use.md": "Y-01 N-01, Y-01 N-03, Y-02 N-01\n" +
			"old Y-07 N-38 and old Y-07 N-39 and old Y-01 N-01\n" +
			"Y-01 N-01..N-03\n",
	})
	expect(t, findingStrings(t, root), []string{
		"use.md:1: unknown-entry: Y-01 N-03: has no such heading",
		"use.md:1: unknown-entry: Y-02 N-01: has no record",
		"use.md:2: unknown-entry: old Y-01 N-01: has no old record",
		"use.md:2: unknown-entry: old Y-07 N-39: has no such old heading",
		"use.md:3: unknown-entry: Y-01 N-03: has no such heading",
	})
}

func TestIgnoreEntriesAndStaleIgnores(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"notes/.doccheck.json": `{"kinds":[{"name":"note","reference":"` + noteRef + `","missing":"is not in the register",
			"rows":{"file":"register.md","pattern":"^\\| X-(?P<num>\\d+) \\|"}}],
			"ignore":[
				{"path":"history.md","kind":"unknown-note","prefix":"X-18","reason":"old numbering"},
				{"path":"gone.md","kind":"broken-link","prefix":"nope.md","reason":"never matches"}]}`,
		"notes/register.md": "| X-01 | one |\n",
		"history.md":        "X-18 X-19\n",
	})
	expect(t, findingStrings(t, root), []string{
		"history.md:1: unknown-note: X-19 is not in the register",
		"notes/.doccheck.json:1: stale-ignore: ignore entry 2 (gone.md broken-link nope.md) matches no finding",
	})
}

func TestSkippedDirectoriesAreNotScanned(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{
		"node_modules/pkg/.doccheck.json": `{not json`,
		"node_modules/pkg/a.md":           "[bad](gone.md)\n",
	})
	expect(t, findingStrings(t, root), nil)
}

func TestInvalidConfigurationIsAnError(t *testing.T) {
	t.Parallel()
	rows := `"rows":{"file":"r.md","pattern":"^\\| X-(?P<num>\\d+)"}`
	tests := []struct{ name, config, want string }{
		{"not JSON", `{`, "unexpected EOF"},
		{"trailing data", `{} {}`, "unexpected data"},
		{"unknown field", `{"kind":[]}`, `unknown field "kind"`},
		{"unknown nested field", `{"kinds":[{"nam":"a"}]}`, `unknown field "nam"`},
		{"bad name", `{"kinds":[{"name":"Bad Name"}]}`, "must match"},
		{"duplicate name", `{"kinds":[` + kindJSON("a", rows) + `,` + kindJSON("a", rows) + `]}`, "used twice"},
		{"bad regex", `{"kinds":[{"name":"a","reference":"X-(","missing":"m"}]}`, "reference"},
		{"no num group", `{"kinds":[{"name":"a","reference":"X-\\d+","missing":"m"}]}`, `missing named group "num"`},
		{"no source", `{"kinds":[{"name":"a","reference":"X-(?P<num>\\d+)","missing":"m"}]}`, "exactly one of"},
		{"two sources", `{"kinds":[{"name":"a","reference":"X-(?P<num>\\d+)","missing":"m",` + rows + `,"files":"^(?P<num>\\d+)$"}]}`, "exactly one of"},
		{"row pattern without num", `{"kinds":[{"name":"a","reference":"X-(?P<num>\\d+)","missing":"m","rows":{"file":"r.md","pattern":"^X"}}]}`, `rows.pattern: missing named group "num"`},
		{"missing text", `{"kinds":[{"name":"a","reference":"X-(?P<num>\\d+)",` + rows + `}]}`, "missing is required"},
		{"doc group in plain kind", `{"kinds":[{"name":"a","reference":"X-(?P<doc>\\d+)(?P<num>\\d+)","missing":"m",` + rows + `}]}`, `"doc" is only allowed`},
		{"heading kind without sources", `{"headingKinds":[{"name":"a","reference":"(?P<doc>\\d+)-(?P<num>\\d+)","sources":[]}]}`, "at least one source"},
		{"variant without group", `{"headingKinds":[{"name":"a","reference":"(?P<doc>\\d+)-(?P<num>\\d+)","sources":[` +
			`{"variant":"old","files":"(?P<doc>\\d+)","heading":"(?P<num>\\d+)","missingDocument":"a","missingHeading":"b"}]}]}`, `needs a "variant" group`},
		{"ignore without reason", `{"ignore":[{"path":"a.md","kind":"k","prefix":"p"}]}`, "all required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := writeTree(t, map[string]string{".doccheck.json": tt.config})
			_, err := analyze(root)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), ".doccheck.json") {
				t.Errorf("error = %v, want it to name the file and contain %q", err, tt.want)
			}
		})
	}
}

func TestRunReportsConfigurationErrorsWithStatusTwo(t *testing.T) {
	t.Parallel()
	root := writeTree(t, map[string]string{".doccheck.json": `{"bogus":1}`})
	if got := run(root, false); got != 2 {
		t.Errorf("run = %d, want 2", got)
	}
}

func TestRunStatusForFindings(t *testing.T) {
	t.Parallel()
	if got := run(writeTree(t, map[string]string{"a.md": "# A\n"}), false); got != 0 {
		t.Errorf("clean tree: run = %d, want 0", got)
	}
	if got := run(writeTree(t, map[string]string{"a.md": "[x](gone.md)\n"}), true); got != 1 {
		t.Errorf("broken link: run = %d, want 1", got)
	}
}

// kindJSON returns a complete kind object named name with the given source field.
func kindJSON(name, source string) string {
	return `{"name":"` + name + `","reference":"X-(?P<num>\\d+)","missing":"m",` + source + `}`
}
