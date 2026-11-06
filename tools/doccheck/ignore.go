package main

import (
	"fmt"
	"strings"
)

// applyIgnore drops the findings matched by an entry and reports every entry that matched
// nothing, so the list shrinks when the history it excuses is fixed or removed.
func applyIgnore(findings []finding, entries []ignoreEntry) []finding {
	used := make([]bool, len(entries))
	var out []finding
	for _, f := range findings {
		suppressed := false
		for i, e := range entries {
			if f.Path == e.path && f.Kind == e.kind && strings.HasPrefix(f.Detail, e.prefix) {
				used[i], suppressed = true, true
			}
		}
		if !suppressed {
			out = append(out, f)
		}
	}
	for i, e := range entries {
		if !used[i] {
			out = append(out, finding{e.config, 1, "stale-ignore",
				fmt.Sprintf("ignore entry %d (%s %s %s) matches no finding", e.index, e.path, e.kind, e.prefix)})
		}
	}
	return out
}
