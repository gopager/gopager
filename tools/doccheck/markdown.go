package main

import (
	"regexp"
	"strings"
)

// document is a parsed Markdown file. All line slices are index-aligned with the
// source lines (index 0 is line 1).
type document struct {
	// path is the repository-relative, slash-separated file path.
	path string
	// raw holds the unmodified source lines.
	raw []string
	// prose holds each line with fenced code block lines (including the fences) blanked.
	prose []string
	// plain is prose with inline code spans additionally blanked to spaces.
	plain []string
	// headings are the ATX headings outside fenced code blocks.
	headings []heading
}

// heading is an ATX heading and its source line (1-based).
type heading struct {
	line int
	text string
}

var (
	fenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})")
	headingRe = regexp.MustCompile(`^ {0,3}#{1,6}[ \t]+(.*?)(?:[ \t]+#+)?[ \t]*$`)
)

// parseDocument splits text into lines and classifies fenced code, inline code
// and headings so that later rules only see real prose.
func parseDocument(path, text string) *document {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	d := &document{path: path, raw: lines, prose: make([]string, len(lines)), plain: make([]string, len(lines))}
	var fence string
	for i, line := range lines {
		if fence != "" {
			if closesFence(line, fence) {
				fence = ""
			}
			continue
		}
		if m := fenceRe.FindStringSubmatch(line); m != nil {
			fence = m[1]
			continue
		}
		d.prose[i] = line
		d.plain[i] = blankInlineCode(line)
		if m := headingRe.FindStringSubmatch(line); m != nil {
			d.headings = append(d.headings, heading{line: i + 1, text: m[1]})
		}
	}
	return d
}

// closesFence reports whether line closes a fence opened with the given marker:
// the same character, at least as long, and nothing else on the line.
func closesFence(line, open string) bool {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return false
	}
	t = strings.TrimRight(t, " \t")
	return len(t) >= len(open) && strings.Trim(t, string(open[0])) == ""
}

// blankInlineCode replaces every inline code span (backticks included) by spaces,
// keeping the column positions of the rest of the line.
func blankInlineCode(line string) string {
	b := []byte(line)
	for i := 0; i < len(b); {
		if b[i] != '`' {
			i++
			continue
		}
		n := runLength(b, i)
		end := closingRun(b, i+n, n)
		if end < 0 {
			i += n
			continue
		}
		for j := i; j < end+n; j++ {
			b[j] = ' '
		}
		i = end + n
	}
	return string(b)
}

// runLength returns the number of consecutive backticks starting at i.
func runLength(b []byte, i int) int {
	n := 0
	for i+n < len(b) && b[i+n] == '`' {
		n++
	}
	return n
}

// closingRun returns the index of the next backtick run of exactly n backticks at
// or after from, or -1.
func closingRun(b []byte, from, n int) int {
	for j := from; j < len(b); {
		if b[j] != '`' {
			j++
			continue
		}
		m := runLength(b, j)
		if m == n {
			return j
		}
		j += m
	}
	return -1
}
