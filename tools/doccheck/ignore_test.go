package main

import (
	"reflect"
	"testing"
)

// TestIgnoreListSuppressesOnlyListedFindings pins that an ignore entry suppresses exactly the
// findings with its path, kind and detail prefix, that comments and blank lines are allowed,
// and that an entry matching nothing is reported, so the list cannot go stale silently.
func TestIgnoreListSuppressesOnlyListedFindings(t *testing.T) {
	t.Parallel()
	entries, err := parseIgnore("# history\n\ndevelopment/x.md unknown-work-package AP-18  # old sequence\nold.md broken-link gone.go # never matches\n")
	if err != nil {
		t.Fatal(err)
	}
	in := []finding{
		{"development/x.md", 3, "unknown-work-package", "AP-18 no file in development/work-packages/"},
		{"development/x.md", 4, "unknown-work-package", "AP-19 no file in development/work-packages/"},
		{"development/y.md", 1, "unknown-work-package", "AP-18 no file in development/work-packages/"},
	}
	got := applyIgnore(in, entries)
	want := []finding{
		{"development/x.md", 4, "unknown-work-package", "AP-19 no file in development/work-packages/"},
		{"development/y.md", 1, "unknown-work-package", "AP-18 no file in development/work-packages/"},
		{ignoreListPath, 4, "stale-ignore", "old.md broken-link gone.go matches no finding"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}
