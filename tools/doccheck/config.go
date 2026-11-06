package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// configFileName is the name of the configuration files discovered anywhere in the scanned tree.
const configFileName = ".doccheck.json"

// config is the content of one .doccheck.json file. Without any such file only relative links
// and heading anchors are checked. All paths and regular expressions in a file are relative to
// the directory holding it, except ignore paths, which are relative to the repository root.
// Unknown fields are rejected. The kinds apply to every Markdown file below the repository root.
//
// Regular expressions use the Go (RE2) syntax. A reference expression must contain a named
// group "num" (the number) and may contain a group "end" (the last number of a range, written
// like "X-1..X-5" or "X-1..5"); the checker adds the word-boundary context and expands ranges
// so that every member is looked up. Reported labels are the matched text up to the number
// followed by the number with the width of its first spelling.
//
//	{
//	  "kinds": [
//	    {
//	      "name": "note",                       // finding kinds: "unknown-note", "duplicate-note"
//	      "reference": "X-(?P<num>\\d+)(?:\\.\\.(?:X-)?(?P<end>\\d+))?",
//	      "missing": "is not in the register",  // detail appended to an unknown reference
//	      // exactly one of the following three ways to build the set of known numbers:
//	      "rows": {"file": "notes/register.md", "pattern": "^\\| (?P<id>X-(?P<num>\\d+)) \\|"},
//	      "files": "^items/x-(?P<num>\\d+)-[^/]+\\.md$",
//	      "headings": {"files": "^notes/.*\\.md$", "pattern": "^(?P<id>X-(?P<num>\\d+))\\b"}
//	    }
//	  ],
//	  "headingKinds": [
//	    {
//	      "name": "entry",                      // a number inside a numbered document
//	      "reference": "(?:(?P<variant>old) )?Y-(?P<doc>\\d+) E-(?P<num>\\d+)(?:\\.\\.(?:E-)?(?P<end>\\d+))?",
//	      "sources": [
//	        {
//	          "variant": "",                      // text of group "variant"; "" if it did not match
//	          "files": "^records/y-(?P<doc>\\d+)\\.md$",
//	          "heading": "^E-(?P<num>\\d+)\\b",
//	          "missingDocument": "has no record file",
//	          "missingHeading": "has no such heading"
//	        }
//	      ]
//	    }
//	  ],
//	  "ignore": [
//	    {"path": "notes/old.md", "kind": "unknown-note", "prefix": "X-9", "reason": "kept as written"}
//	  ]
//	}
//
// "rows" collects the numbers of the table-row lines of one register file (outside fenced code)
// matching "pattern"; a number in two rows is reported as "duplicate-<name>", naming group "id"
// if present. "files" matches Markdown file paths relative to the configuration directory and
// takes the number from the path. "headings" collects the numbers from the heading texts of
// the Markdown files matching "files". A "headingKinds" reference names a document (group
// "doc", taken from the path by the source's "files" expression) and a heading number (group
// "num") inside it; the optional group "variant" selects the source. Missing references are
// reported as "unknown-<name>".
//
// An ignore entry suppresses findings of the given repository-relative path and finding kind
// whose detail starts with prefix; the reason is mandatory, and an entry that matches nothing
// is reported as "stale-ignore" at the configuration file.
type config struct {
	Kinds        []kindConfig        `json:"kinds"`
	HeadingKinds []headingKindConfig `json:"headingKinds"`
	Ignore       []ignoreConfig      `json:"ignore"`
}

// kindConfig declares one family of numbered references; see config.
type kindConfig struct {
	Name      string          `json:"name"`
	Reference string          `json:"reference"`
	Missing   string          `json:"missing"`
	Rows      *rowsConfig     `json:"rows"`
	Files     string          `json:"files"`
	Headings  *headingsConfig `json:"headings"`
}

// rowsConfig names a register file and the pattern of its table rows.
type rowsConfig struct {
	File    string `json:"file"`
	Pattern string `json:"pattern"`
}

// headingsConfig selects Markdown files and the heading pattern that yields numbers.
type headingsConfig struct {
	Files   string `json:"files"`
	Pattern string `json:"pattern"`
}

// headingKindConfig declares references that combine a document number and a heading number.
type headingKindConfig struct {
	Name      string                `json:"name"`
	Reference string                `json:"reference"`
	Sources   []headingSourceConfig `json:"sources"`
}

// headingSourceConfig says where the documents of one variant live and how their headings are numbered.
type headingSourceConfig struct {
	Variant         string `json:"variant"`
	Files           string `json:"files"`
	Heading         string `json:"heading"`
	MissingDocument string `json:"missingDocument"`
	MissingHeading  string `json:"missingHeading"`
}

// ignoreConfig is one accepted finding; see config.
type ignoreConfig struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Prefix string `json:"prefix"`
	Reason string `json:"reason"`
}

// ruleSet is a validated configuration with compiled expressions, bound to its directory.
type ruleSet struct {
	// path is the repository-relative path of the configuration file and dir its directory.
	path, dir    string
	kinds        []*kindRule
	headingKinds []*headingKindRule
	ignore       []ignoreEntry
}

// kindRule is a compiled kindConfig together with its set of known numbers.
type kindRule struct {
	cfg      kindConfig
	ref      *referencePattern
	rows     *regexp.Regexp
	files    *regexp.Regexp
	headFile *regexp.Regexp
	headPat  *regexp.Regexp
	known    map[int]bool
}

// headingKindRule is a compiled headingKindConfig.
type headingKindRule struct {
	name    string
	ref     *referencePattern
	sources []*headingSource
}

// headingSource is a compiled headingSourceConfig; known maps a document number to the
// heading numbers it contains.
type headingSource struct {
	cfg     headingSourceConfig
	files   *regexp.Regexp
	heading *regexp.Regexp
	known   map[int]map[int]bool
}

// ignoreEntry suppresses the findings with this path and kind whose detail starts with prefix.
// index is the 1-based position in the configuration file's ignore list.
type ignoreEntry struct {
	config, path, kind, prefix string
	index                      int
}

var kindNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// loadRules reads, validates and compiles the configuration file at the repository-relative
// slash path rel below root.
func loadRules(root, rel string) (*ruleSet, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: unexpected data after the top-level object", rel)
	}
	rs, err := compileConfig(c, rel)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", rel, err)
	}
	return rs, nil
}

// compileConfig validates c and compiles its expressions.
func compileConfig(c config, rel string) (*ruleSet, error) {
	rs := &ruleSet{path: rel, dir: path.Dir(rel)}
	names := map[string]bool{}
	claim := func(label, name string) error {
		if !kindNameRe.MatchString(name) {
			return fmt.Errorf("%s: name %q must match %s", label, name, kindNameRe)
		}
		if names[name] {
			return fmt.Errorf("%s: name %q is used twice", label, name)
		}
		names[name] = true
		return nil
	}
	for i, k := range c.Kinds {
		label := fmt.Sprintf("kinds[%d]", i)
		if err := claim(label, k.Name); err != nil {
			return nil, err
		}
		r, err := compileKind(k)
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", label, k.Name, err)
		}
		rs.kinds = append(rs.kinds, r)
	}
	for i, k := range c.HeadingKinds {
		label := fmt.Sprintf("headingKinds[%d]", i)
		if err := claim(label, k.Name); err != nil {
			return nil, err
		}
		r, err := compileHeadingKind(k)
		if err != nil {
			return nil, fmt.Errorf("%s (%s): %w", label, k.Name, err)
		}
		rs.headingKinds = append(rs.headingKinds, r)
	}
	for i, e := range c.Ignore {
		if e.Path == "" || e.Kind == "" || e.Prefix == "" || strings.TrimSpace(e.Reason) == "" {
			return nil, fmt.Errorf("ignore[%d]: path, kind, prefix and reason are all required", i)
		}
		rs.ignore = append(rs.ignore, ignoreEntry{config: rel, path: e.Path, kind: e.Kind, prefix: e.Prefix, index: i + 1})
	}
	return rs, nil
}

// compileKind validates one kind: exactly one source of known numbers, all groups present.
func compileKind(k kindConfig) (*kindRule, error) {
	if k.Missing == "" {
		return nil, errors.New("missing is required")
	}
	ref, err := compileReference(k.Reference, false)
	if err != nil {
		return nil, err
	}
	r := &kindRule{cfg: k, ref: ref, known: map[int]bool{}}
	sources := 0
	if k.Rows != nil {
		sources++
		if k.Rows.File == "" {
			return nil, errors.New("rows.file is required")
		}
		if r.rows, err = compileWithGroups("rows.pattern", k.Rows.Pattern, "num"); err != nil {
			return nil, err
		}
	}
	if k.Files != "" {
		sources++
		if r.files, err = compileWithGroups("files", k.Files, "num"); err != nil {
			return nil, err
		}
	}
	if k.Headings != nil {
		sources++
		if r.headFile, err = compileWithGroups("headings.files", k.Headings.Files); err != nil {
			return nil, err
		}
		if r.headPat, err = compileWithGroups("headings.pattern", k.Headings.Pattern, "num"); err != nil {
			return nil, err
		}
	}
	if sources != 1 {
		return nil, errors.New("exactly one of rows, files and headings is required")
	}
	return r, nil
}

// compileHeadingKind validates one heading kind and its sources.
func compileHeadingKind(k headingKindConfig) (*headingKindRule, error) {
	ref, err := compileReference(k.Reference, true)
	if err != nil {
		return nil, err
	}
	if len(k.Sources) == 0 {
		return nil, errors.New("at least one source is required")
	}
	r := &headingKindRule{name: k.Name, ref: ref}
	seen := map[string]bool{}
	for i, s := range k.Sources {
		label := fmt.Sprintf("sources[%d]", i)
		if seen[s.Variant] {
			return nil, fmt.Errorf("%s: variant %q is used twice", label, s.Variant)
		}
		seen[s.Variant] = true
		if s.Variant != "" && ref.variant < 0 {
			return nil, fmt.Errorf("%s: variant %q needs a \"variant\" group in reference", label, s.Variant)
		}
		if s.MissingDocument == "" || s.MissingHeading == "" {
			return nil, fmt.Errorf("%s: missingDocument and missingHeading are required", label)
		}
		src := &headingSource{cfg: s, known: map[int]map[int]bool{}}
		if src.files, err = compileWithGroups(label+".files", s.Files, "doc"); err != nil {
			return nil, err
		}
		if src.heading, err = compileWithGroups(label+".heading", s.Heading, "num"); err != nil {
			return nil, err
		}
		r.sources = append(r.sources, src)
	}
	return r, nil
}

// compileWithGroups compiles expr and checks that it has the named groups.
func compileWithGroups(field, expr string, groups ...string) (*regexp.Regexp, error) {
	if expr == "" {
		return nil, fmt.Errorf("%s is required", field)
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	for _, g := range groups {
		if re.SubexpIndex(g) < 0 {
			return nil, fmt.Errorf("%s: missing named group %q", field, g)
		}
	}
	return re, nil
}

// relTo returns p relative to dir, or false when p is not below dir. dir "." is the root.
func relTo(dir, p string) (string, bool) {
	if dir == "." {
		return p, true
	}
	return strings.CutPrefix(p, dir+"/")
}
