package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseDocumentBlanksFencedCode(t *testing.T) {
	t.Parallel()
	src := "intro\n```go\n[x](gone.md)\n```\n~~~~\n```\n~~~~\n[y](kept.md)\n"
	d := parseDocument("a.md", src)
	want := []string{"intro", "", "", "", "", "", "", "[y](kept.md)", ""}
	if !reflect.DeepEqual(d.prose, want) {
		t.Errorf("prose = %q, want %q", d.prose, want)
	}
}

func TestBlankInlineCode(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, in, want string }{
		{"single", "a `[x](y.md)` b", "a" + strings.Repeat(" ", 13) + "b"},
		{"double backticks", "a ``x ` y`` b", "a" + strings.Repeat(" ", 11) + "b"},
		{"unclosed kept", "a `b", "a `b"},
		{"none", "plain", "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := blankInlineCode(tt.in); got != tt.want {
				t.Errorf("blankInlineCode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseDocumentHeadings(t *testing.T) {
	t.Parallel()
	src := "# One\n#NotHeading\n## Two ##\n    # indented code\n```\n# fenced\n```\n### Three\n"
	d := parseDocument("a.md", src)
	var got []string
	for _, h := range d.headings {
		got = append(got, h.text)
	}
	if want := []string{"One", "Two", "Three"}; !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %q, want %q", got, want)
	}
}
