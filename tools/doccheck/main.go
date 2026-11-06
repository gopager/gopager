// Command doccheck verifies cross-references in the repository's Markdown files: relative
// links and heading fragments always, and numbered identifiers (decisions, work packages,
// evidence entries and the like) wherever a .doccheck.json file below the root declares them.
// The schema is documented on the config type.
//
// Usage: go run ./tools/doccheck [-root dir] [-json]
//
// Findings print as "path:line: kind: detail", sorted. Findings accepted on purpose are listed
// with a reason in a .doccheck.json file; an entry that matches nothing is itself reported.
// The exit status is 1 when there are findings, 2 on an operational or configuration error and
// 0 otherwise.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// skippedDirs are directory names never scanned: tooling state, VCS data and
// third-party trees.
var skippedDirs = map[string]bool{
	".git": true, ".claude": true, ".superpowers": true, "node_modules": true, "vendor": true,
}

func main() {
	root := flag.String("root", ".", "repository root")
	asJSON := flag.Bool("json", false, "print findings as a JSON array")
	flag.Parse()
	os.Exit(run(*root, *asJSON))
}

// run executes the check and returns the process exit status.
func run(root string, asJSON bool) int {
	findings, err := analyze(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doccheck:", err)
		return 2
	}
	if asJSON {
		if findings == nil {
			findings = []finding{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(findings)
	} else {
		for _, f := range findings {
			fmt.Println(f)
		}
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

// analyze scans the tree below root, loads every configuration and returns the findings that
// are not ignored. A configuration error is returned as an error.
func analyze(root string) ([]finding, error) {
	docs, configs, err := scan(root)
	if err != nil {
		return nil, err
	}
	var rules []*ruleSet
	var ignore []ignoreEntry
	for _, c := range configs {
		rs, err := loadRules(root, c)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rs)
		ignore = append(ignore, rs.ignore...)
	}
	return sortUnique(applyIgnore(checkAll(root, docs, rules), ignore)), nil
}

// scan parses every Markdown file below root, skipping skippedDirs, and returns the
// repository-relative paths of the configuration files it met.
func scan(root string) (docs []*document, configs []string, err error) {
	err = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if p != root && skippedDirs[e.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if e.Name() == configFileName {
			configs = append(configs, rel)
			return nil
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			return nil
		}
		text, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		docs = append(docs, parseDocument(rel, string(text)))
		return nil
	})
	return docs, configs, err
}
