package processing

import (
	"context"
	"slices"
	"strings"
)

// FieldGroupDeclarations is optionally implemented by Declarations whose owner also declares
// the field groups of each source and target field. Save-time validation and the
// per-definition classification are computed from these declared groups, never from values.
type FieldGroupDeclarations interface {
	// FieldGroups returns the owner-declared field groups of a source or target field.
	FieldGroups(FieldRef) ([]string, error)
}

// SourceReference is one source or target field a definition reads, together with the field
// groups its owner declares for it. A definition version's outputs are classified once as
// `derived` plus the union of the groups of all its SourceReferences.
type SourceReference struct {
	Field  FieldRef
	Groups []string
}

// String renders the reference as `namespace.path.segments` for error messages.
func (f FieldRef) String() string {
	return f.Namespace + "." + strings.Join(f.Path, ".")
}

// referenceSet collects the distinct source and target fields one part of a definition reads.
// Step fields are not collected: they are derived from source fields that are collected where
// they are first referenced.
type referenceSet struct {
	groups     FieldGroupDeclarations
	references []SourceReference
	seen       map[string]bool
}

func newReferenceSet(declarations Declarations) *referenceSet {
	groups, _ := declarations.(FieldGroupDeclarations)
	return &referenceSet{groups: groups, seen: make(map[string]bool)}
}

// record remembers ref once. A Declarations owner without group declarations yields a
// reference with no groups, which save-time validation then rejects as unclassifiable.
func (s *referenceSet) record(ref FieldRef) error {
	if s == nil || (ref.Namespace != "source" && ref.Namespace != "target") || s.seen[ref.key()] {
		return nil
	}
	reference := SourceReference{Field: FieldRef{Namespace: ref.Namespace, Path: slices.Clone(ref.Path)}}
	if s.groups != nil {
		groups, err := s.groups.FieldGroups(ref)
		if err != nil {
			return err
		}
		reference.Groups = slices.Clone(groups)
	}
	s.seen[ref.key()] = true
	s.references = append(s.references, reference)
	return nil
}

func (s *referenceSet) fields() []FieldRef {
	result := make([]FieldRef, len(s.references))
	for i, reference := range s.references {
		result[i] = reference.Field
	}
	return result
}

// SourceReferences returns, detached and in a stable order, every source and target field the
// definition reads in its selector, steps (installed-step inputs and both branch arms
// included) and outputs, with the groups declared by the source owner. It is computed at
// compile time from the definition and the declarations, so it does not depend on any source
// value, on which branch runs, or on a reader's rights.
func (p *WorkflowProgram) SourceReferences() []SourceReference {
	if p == nil {
		return nil
	}
	var result []SourceReference
	seen := make(map[string]bool)
	for _, set := range []*referenceSet{p.selectorReferences, p.bodyReferences} {
		for _, reference := range set.references {
			if seen[reference.Field.key()] {
				continue
			}
			seen[reference.Field.key()] = true
			reference.Field.Path = slices.Clone(reference.Field.Path)
			reference.Groups = slices.Clone(reference.Groups)
			result = append(result, reference)
		}
	}
	slices.SortFunc(result, func(a, b SourceReference) int { return strings.Compare(a.Field.key(), b.Field.key()) })
	return result
}

// referenceDependencies returns the union of the owner classifications of the referenced
// fields in one evaluation context (target is nil for the source-only context).
func (s *CapturedSource) referenceDependencies(ctx context.Context, fields []FieldRef, target *uint32) ([]Dependency, error) {
	var sets [][]Dependency
	view := capturedView{source: s, target: target}
	for _, ref := range fields {
		field, err := view.LookupField(ctx, ref)
		if err != nil {
			return nil, err
		}
		sets = append(sets, field.Dependencies)
	}
	return UnionDependencies(sets...)
}
