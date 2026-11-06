package main

import (
	"path"
	"regexp"
	"strconv"
)

// build derives the sets of known numbers of rs from the parsed documents and returns the
// duplicate register rows it met on the way.
func (rs *ruleSet) build(docs []*document) []finding {
	var dups []finding
	for _, k := range rs.kinds {
		for _, d := range docs {
			dups = append(dups, k.collect(rs.dir, d)...)
		}
	}
	for _, k := range rs.headingKinds {
		for _, s := range k.sources {
			for _, d := range docs {
				s.collect(rs.dir, d)
			}
		}
	}
	return dups
}

// collect adds the numbers document d contributes to the known set of k.
func (k *kindRule) collect(dir string, d *document) []finding {
	if k.rows != nil {
		if d.path != path.Join(dir, k.cfg.Rows.File) {
			return nil
		}
		return k.collectRows(d)
	}
	rel, below := relTo(dir, d.path)
	if !below {
		return nil
	}
	switch {
	case k.files != nil:
		if m := k.files.FindStringSubmatch(rel); m != nil {
			k.add(k.files, m)
		}
	case k.headFile.MatchString(rel):
		for _, h := range d.headings {
			if m := k.headPat.FindStringSubmatch(h.text); m != nil {
				k.add(k.headPat, m)
			}
		}
	}
	return nil
}

// collectRows records the numbers of the matching table rows of the register d and returns a
// finding for every number that appears in more than one row.
func (k *kindRule) collectRows(d *document) []finding {
	var dups []finding
	for i, line := range d.prose {
		m := k.rows.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, ok := groupInt(k.rows, m, "num")
		if !ok {
			continue
		}
		if k.known[n] {
			id := strconv.Itoa(n)
			if g := k.rows.SubexpIndex("id"); g >= 0 && m[g] != "" {
				id = m[g]
			}
			dups = append(dups, finding{d.path, i + 1, "duplicate-" + k.cfg.Name, id + " listed again"})
		}
		k.known[n] = true
	}
	return dups
}

// add records the "num" group of match m as known.
func (k *kindRule) add(re *regexp.Regexp, m []string) {
	if n, ok := groupInt(re, m, "num"); ok {
		k.known[n] = true
	}
}

// collect records the heading numbers of d if its path names a document of this source.
func (s *headingSource) collect(dir string, d *document) {
	rel, below := relTo(dir, d.path)
	if !below {
		return
	}
	m := s.files.FindStringSubmatch(rel)
	if m == nil {
		return
	}
	doc, ok := groupInt(s.files, m, "doc")
	if !ok {
		return
	}
	set := map[int]bool{}
	for _, h := range d.headings {
		if hm := s.heading.FindStringSubmatch(h.text); hm != nil {
			if n, ok := groupInt(s.heading, hm, "num"); ok {
				set[n] = true
			}
		}
	}
	s.known[doc] = set
}

// groupInt returns the integer in the named group of match m.
func groupInt(re *regexp.Regexp, m []string, name string) (int, bool) {
	n, err := strconv.Atoi(m[re.SubexpIndex(name)])
	return n, err == nil
}
