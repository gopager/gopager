package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ignoreListPath is the repository-relative file listing findings that are accepted on
// purpose, typically references in archived history that must keep their original wording.
const ignoreListPath = "tools/doccheck/ignore.txt"

// ignoreEntry suppresses the findings with this path and kind whose detail starts with prefix.
type ignoreEntry struct {
	line               int
	path, kind, prefix string
}

// parseIgnore reads entries of the form "path kind detail-prefix  # reason"; blank lines and
// lines starting with # are skipped.
func parseIgnore(text string) ([]ignoreEntry, error) {
	var entries []ignoreEntry
	scanner := bufio.NewScanner(strings.NewReader(text))
	for n := 1; scanner.Scan(); n++ {
		line := scanner.Text()
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 3 {
			return nil, fmt.Errorf("%s:%d: want \"path kind detail-prefix\"", ignoreListPath, n)
		}
		entries = append(entries, ignoreEntry{line: n, path: fields[0], kind: fields[1], prefix: strings.Join(fields[2:], " ")})
	}
	return entries, scanner.Err()
}

// loadIgnore reads the ignore list below root; a missing file means no entries.
func loadIgnore(root string) ([]ignoreEntry, error) {
	data, err := os.ReadFile(filepath.Join(root, ignoreListPath))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseIgnore(string(data))
}

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
			out = append(out, finding{ignoreListPath, e.line, "stale-ignore",
				fmt.Sprintf("%s %s %s matches no finding", e.path, e.kind, e.prefix)})
		}
	}
	return out
}
