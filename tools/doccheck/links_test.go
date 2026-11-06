package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLinkTargets(t *testing.T) {
	tests := []struct {
		name, in string
		want     []string
	}{
		{"inline", "see [a](b.md#c) and [d](<e f.md>)", []string{"b.md#c", "e f.md"}},
		{"title", `[a](b.md "Title")`, []string{"b.md"}},
		{"reference definition", "[ref]: ../x.md#y", []string{"../x.md#y"}},
		{"image", "![alt](img.png)", []string{"img.png"}},
		{"none", "no links here", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := linkTargets(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("linkTargets(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsLocal(t *testing.T) {
	tests := map[string]bool{
		"a.md": true, "#frag": true, "../a.md#b": true, "/abs.md": true,
		"": false, "https://x.test/a": false, "mailto:a@b.c": false, "//cdn.test/x": false,
	}
	for in, want := range tests {
		if got := isLocal(in); got != want {
			t.Errorf("isLocal(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct{ src, link, want string }{
		{"development/a.md", "b.md", "development/b.md"},
		{"development/x/a.md", "../b.md", "development/b.md"},
		{"README.md", "docs/a.md", "docs/a.md"},
		{"docs/a.md", "/api/v1.yaml", "api/v1.yaml"},
		{"README.md", "../outside.md", ""},
	}
	for _, tt := range tests {
		if got := resolve(tt.src, tt.link); got != tt.want {
			t.Errorf("resolve(%q, %q) = %q, want %q", tt.src, tt.link, got, tt.want)
		}
	}
}

// writeTree creates the given files below a temporary directory and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestCheckLinksEndToEnd(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a.md": "# A\n[ok](b.md#section) [file](b.md) [self](#a) [dir](sub/)\n" +
			"[missing](nope.md) [badfrag](b.md#nothing) [selfbad](#zzz)\n" +
			"[ext](https://example.test/x.md) `[code](nope.md)`\n" +
			"```\n[fenced](nope.md)\n```\n",
		"b.md":      "# B\n\n## Section\n",
		"sub/c.txt": "x",
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
		"a.md:3: broken-link: nope.md",
		"a.md:3: broken-anchor: b.md#nothing",
		"a.md:3: broken-anchor: #zzz",
	}
	// sorted by kind, then detail, within a line
	want = []string{want[2], want[1], want[0]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("findings = %q\nwant       %q", got, want)
	}
}
