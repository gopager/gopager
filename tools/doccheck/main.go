// Command doccheck verifies cross-references in the repository's Markdown files:
// relative links and heading fragments, decision (D-n), requirement (REQ-nnn),
// work-package (AP-nn), execution-evidence (AP-nn E-m) and contract (C-nn) IDs.
//
// Usage: go run ./tools/doccheck [-root dir] [-json]
//
// Findings print as "path:line: kind: detail", sorted. The exit status is 1 when
// there are findings, 2 on an operational error and 0 otherwise.
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
	docs, err := loadDocuments(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doccheck:", err)
		return 2
	}
	findings := checkAll(root, docs)
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

// loadDocuments parses every Markdown file below root, skipping skippedDirs.
func loadDocuments(root string) ([]*document, error) {
	var docs []*document
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if p != root && skippedDirs[e.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			return nil
		}
		text, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		docs = append(docs, parseDocument(filepath.ToSlash(rel), string(text)))
		return nil
	})
	return docs, err
}
