package processing

import (
	"bytes"
	"errors"
	"slices"

	"github.com/google/uuid"
)

const (
	// MaxWorkflowAgencies bounds the Agencies one Workflow is assigned to.
	MaxWorkflowAgencies = 256
	// MaxWorkflowNarrowingReferences bounds the Networks or Units one narrowing refers to.
	MaxWorkflowNarrowingReferences = 1024
)

// WorkflowScope selects the Transmissions a Workflow may process: the Agencies it is assigned to
// and, optionally, a narrowing to some of their Radio networks or Units. A Workflow reaches no
// Agency implicitly, so a new Agency has no Workflow until one is assigned.
type WorkflowScope struct {
	Agencies  []uuid.UUID        `json:"agencies"`
	Narrowing *WorkflowNarrowing `json:"narrowing"`
}

// WorkflowNarrowing restricts a Workflow to the listed Radio networks ("network") or Units
// ("unit"); every reference belongs to one of the assigned Agencies.
type WorkflowNarrowing struct {
	Kind       string      `json:"kind"`
	References []uuid.UUID `json:"references"`
}

// Canonical returns a detached, sorted copy of the scope, or an invalid-input failure when a set
// is empty, oversized, contains the nil UUID or a duplicate, or the narrowing kind is unknown.
// Membership of references in the assigned Agencies needs master data and is checked by the
// management service.
func (s WorkflowScope) Canonical() (WorkflowScope, error) {
	agencies, err := canonicalIDs(s.Agencies, MaxWorkflowAgencies)
	if err != nil {
		return WorkflowScope{}, err
	}
	result := WorkflowScope{Agencies: agencies}
	if s.Narrowing == nil {
		return result, nil
	}
	if s.Narrowing.Kind != "network" && s.Narrowing.Kind != "unit" {
		return WorkflowScope{}, invalid(errors.New("unknown Workflow narrowing kind"))
	}
	references, err := canonicalIDs(s.Narrowing.References, MaxWorkflowNarrowingReferences)
	if err != nil {
		return WorkflowScope{}, err
	}
	result.Narrowing = &WorkflowNarrowing{Kind: s.Narrowing.Kind, References: references}
	return result, nil
}

// Validate checks the shape rules of Canonical without requiring the identifiers to be sorted.
func (s WorkflowScope) Validate() error {
	_, err := s.Canonical()
	return err
}

// canonicalIDs returns a sorted copy of 1..limit distinct non-nil identifiers.
func canonicalIDs(in []uuid.UUID, limit int) ([]uuid.UUID, error) {
	if len(in) == 0 || len(in) > limit {
		return nil, invalid(errors.New("workflow scope set size out of range"))
	}
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	for i, id := range out {
		if id == uuid.Nil || (i > 0 && out[i-1] == id) {
			return nil, invalid(errors.New("workflow scope holds a nil or duplicate identifier"))
		}
	}
	return out, nil
}
