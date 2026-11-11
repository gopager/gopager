package authz

import (
	"slices"

	"github.com/google/uuid"
)

// AgencyFilter selects Agencies: all of them, or the union of the listed Agencies, the Agencies
// of the listed Units and the Agencies of the Units in the listed Unit collections.
type AgencyFilter struct {
	All         bool
	Agencies    []uuid.UUID
	Units       []uuid.UUID
	Collections []uuid.UUID
}

// Empty reports whether the filter selects no Agency.
func (f AgencyFilter) Empty() bool {
	return !f.All && len(f.Agencies) == 0 && len(f.Units) == 0 && len(f.Collections) == 0
}

// AgencyTerm is the visibility one action contributes: an Agency is selected when the grants
// of the action select it and, for a restricted API key, the key's scopes select it too.
type AgencyTerm struct {
	Grant AgencyFilter
	// Key is nil for an authority without key restrictions.
	Key *AgencyFilter
}

// AgencyVisibility says which Agencies a caller may see a resource of. An Agency is visible
// when any term selects it; no term means nothing is visible. Stores translate it into their
// own query language, so this type stays free of persistence concerns.
type AgencyVisibility struct{ Terms []AgencyTerm }

// Unrestricted reports whether every Agency is visible, which lets a store skip the filter.
func (v AgencyVisibility) Unrestricted() bool {
	return slices.ContainsFunc(v.Terms, func(t AgencyTerm) bool {
		return t.Grant.All && (t.Key == nil || t.Key.All)
	})
}

// VisibleAgencies derives from the authority's grants (and key restrictions) the Agencies it
// may see resources of, for any of the actions. When account is true, a grant or key scope of
// kind "account" selects every Agency, for resources that are not Agency-bound in their
// actions; otherwise such scopes select nothing. Age limits do not apply to configuration
// metadata and are ignored. It returns ErrForbidden when no action yields any scope and
// ErrInvalidInput for an unknown action or malformed grant.
func VisibleAgencies(authority Authority, actions []string, account bool) (AgencyVisibility, error) {
	principal, err := uuid.Parse(authority.PrincipalID)
	if !authority.Active || err != nil || principal == uuid.Nil || authority.Revision <= 0 {
		return AgencyVisibility{}, ErrForbidden
	}
	var visibility AgencyVisibility
	for _, action := range actions {
		if !ValidAction(action) {
			return AgencyVisibility{}, ErrInvalidInput
		}
		var grant AgencyFilter
		for _, g := range authority.Grants {
			if err := ValidateGrant(g); err != nil {
				return AgencyVisibility{}, err
			}
			if slices.Contains(g.Actions, action) {
				if err := grant.add(g.Scope, account); err != nil {
					return AgencyVisibility{}, err
				}
			}
		}
		if grant.Empty() {
			continue
		}
		term := AgencyTerm{Grant: grant}
		if key := authority.Key; key != nil {
			if err := ValidateRestrictions(key); err != nil {
				return AgencyVisibility{}, err
			}
			if !slices.Contains(key.Actions, action) {
				continue
			}
			var restricted AgencyFilter
			for _, scope := range key.Scopes {
				if ActionAllowsScope(action, scope.Kind) {
					if err := restricted.add(scope, account); err != nil {
						return AgencyVisibility{}, err
					}
				}
			}
			if restricted.Empty() {
				continue
			}
			term.Key = &restricted
		}
		visibility.Terms = append(visibility.Terms, term)
	}
	if len(visibility.Terms) == 0 {
		return AgencyVisibility{}, ErrForbidden
	}
	return visibility, nil
}

// add merges one scope into the filter.
func (f *AgencyFilter) add(scope Scope, account bool) error {
	switch scope.Kind {
	case "all":
		f.All = true
		return nil
	case "account":
		f.All = f.All || account
		return nil
	case "agency", "unit", "unitCollection":
	default:
		return nil
	}
	for _, reference := range scope.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return ErrInvalidInput
		}
		switch scope.Kind {
		case "agency":
			f.Agencies = append(f.Agencies, id)
		case "unit":
			f.Units = append(f.Units, id)
		default:
			f.Collections = append(f.Collections, id)
		}
	}
	return nil
}
