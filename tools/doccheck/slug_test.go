package main

import "testing"

func TestSlugify(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in, want string }{
		{"plain", "Decision Register", "decision-register"},
		{"punctuation dropped", "Why? Because: (reasons), 100%!", "why-because-reasons-100"},
		{"hyphen kept", "Fail-closed rule", "fail-closed-rule"},
		{"underscore kept", "snake_case name", "snake_case-name"},
		{"code span", "The `go run` tool", "the-go-run-tool"},
		{"link text", "See [the plan](plan.md#x) now", "see-the-plan-now"},
		{"emphasis", "**Bold** and *italic*", "bold-and-italic"},
		{"underscore emphasis", "Some _emphasised_ word", "some-emphasised-word"},
		{"html anchor stripped", `<a id="d-01"></a>D-01 Title`, "d-01-title"},
		{"double space not collapsed", "A  B", "a--b"},
		{"slash", "Lite/Full profile", "litefull-profile"},
		{"unicode letters", "Übersicht Größe", "übersicht-größe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := slugify(tt.in); got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestAnchors(t *testing.T) {
	t.Parallel()
	src := "# Title\n\n## Same\n\n## Same\n\n## Same\n\n<a id=\"Custom\"></a>text\n\n## Heading {#kram}\n\n```\n## Fenced\n<a id=\"ghost\"></a>\n```\n"
	got := parseDocument("x.md", src).anchors()
	for _, want := range []string{"title", "same", "same-1", "same-2", "custom", "kram"} {
		if !got[want] {
			t.Errorf("missing anchor %q in %v", want, got)
		}
	}
	for _, not := range []string{"fenced", "ghost", "same-3"} {
		if got[not] {
			t.Errorf("unexpected anchor %q", not)
		}
	}
}

func TestNormalizeFragment(t *testing.T) {
	t.Parallel()
	if got := normalizeFragment("Foo%20Bar"); got != "foo bar" {
		t.Errorf("got %q", got)
	}
}
