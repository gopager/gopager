package authz

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// AgencyReach is the set of Agencies within which an action is permitted:
// every Agency when All is true, otherwise exactly Agencies.
type AgencyReach struct {
	All      bool
	Agencies []uuid.UUID
}

// Contains reports whether the reach covers the Agency.
func (r AgencyReach) Contains(agency uuid.UUID) bool {
	return r.All || slices.Contains(r.Agencies, agency)
}

// ReachableAgencies returns the Agencies in which action is permitted by the
// current owner grants intersected with a restricting key, for native list
// filtering of rows that carry no age anchor (Networks, Gateways). Only "all"
// and "agency" scopes contribute; other scope kinds never widen a list. Like
// Authorize, it fails with ErrForbidden for an inactive authority or when no
// grant (or the key) names the action, and with ErrInvalidInput for a malformed
// grant or for a matching grant or key with a maximum age, which needs an age
// anchor these rows do not have.
func ReachableAgencies(authority Authority, action string, now time.Time) (AgencyReach, error) {
	principal, err := uuid.Parse(authority.PrincipalID)
	if !authority.Active || err != nil || principal == uuid.Nil || authority.Revision <= 0 {
		return AgencyReach{}, ErrForbidden
	}
	if !ValidAction(action) || now.IsZero() {
		return AgencyReach{}, ErrInvalidInput
	}
	var owner AgencyReach
	named := false
	for _, grant := range authority.Grants {
		if err := ValidateGrant(grant); err != nil {
			return AgencyReach{}, err
		}
		if !slices.Contains(grant.Actions, action) {
			continue
		}
		if grant.MaxAgeSeconds != nil {
			return AgencyReach{}, ErrInvalidInput
		}
		named = true
		owner = owner.add(grant.Scope)
	}
	if !named {
		return AgencyReach{}, ErrForbidden
	}
	if authority.Key == nil {
		return owner, nil
	}
	key := authority.Key
	if err := ValidateRestrictions(key); err != nil {
		return AgencyReach{}, err
	}
	if !slices.Contains(key.Actions, action) {
		return AgencyReach{}, ErrForbidden
	}
	if key.MaxAgeSeconds != nil {
		return AgencyReach{}, ErrInvalidInput
	}
	var restricted AgencyReach
	for _, scope := range key.Scopes {
		if ActionAllowsScope(action, scope.Kind) {
			restricted = restricted.add(scope)
		}
	}
	return owner.intersect(restricted), nil
}

// add widens the reach by an "all" or "agency" scope and ignores other kinds.
func (r AgencyReach) add(scope Scope) AgencyReach {
	switch scope.Kind {
	case "all":
		return AgencyReach{All: true}
	case "agency":
		if r.All {
			return r
		}
		for _, reference := range scope.References {
			if id, err := uuid.Parse(reference); err == nil && !slices.Contains(r.Agencies, id) {
				r.Agencies = append(r.Agencies, id)
			}
		}
	}
	return r
}

// intersect returns the Agencies covered by both reaches.
func (r AgencyReach) intersect(other AgencyReach) AgencyReach {
	switch {
	case r.All:
		return other
	case other.All:
		return r
	}
	var result AgencyReach
	for _, agency := range r.Agencies {
		if slices.Contains(other.Agencies, agency) {
			result.Agencies = append(result.Agencies, agency)
		}
	}
	return result
}
