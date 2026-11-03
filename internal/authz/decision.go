package authz

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// Target is one server-derived authorization context, never a union of unrelated rows.
// WithoutUnit is true only for proven unassigned or targetless evidence.
type Target struct {
	NetworkID       uuid.UUID
	UnitID          uuid.UUID
	UnitCollections []uuid.UUID
	AccountID       uuid.UUID
	WithoutUnit     bool
	AgeAnchor       time.Time
}

// Authorize evaluates current owner grants and restricting key rights for one target.
// Callers must resolve authority and target state in the same protected transaction.
func Authorize(authority Authority, action string, fields []string, target Target, now time.Time) error {
	principal, err := uuid.Parse(authority.PrincipalID)
	if !authority.Active || err != nil || principal == uuid.Nil || authority.Revision <= 0 {
		return ErrForbidden
	}
	if !ValidAction(action) || len(fields) > 1024 || len(target.UnitCollections) > 1024 ||
		(target.WithoutUnit && target.UnitID != uuid.Nil) {
		return ErrInvalidInput
	}
	for _, field := range fields {
		if !ValidField(field) {
			return ErrInvalidInput
		}
	}
	if action == "without-unit.read" && !target.WithoutUnit {
		return ErrForbidden
	}
	if err := AuthorizeFields(authority, fields); err != nil {
		return err
	}
	if action == "message.detail" {
		if err := authorizeGrants(authority.Grants, "message.read", target, now); err != nil {
			return err
		}
		if err := authorizeGrants(authority.Grants, action, target, now); err != nil {
			return err
		}
	} else if err := authorizeGrants(authority.Grants, action, target, now); err != nil {
		return err
	}
	if authority.Key == nil {
		return nil
	}
	key := authority.Key
	if err := ValidateRestrictions(key); err != nil {
		return err
	}
	if !slices.Contains(key.Actions, action) ||
		(action == "message.detail" && !slices.Contains(key.Actions, "message.read")) {
		return ErrForbidden
	}
	for _, field := range fields {
		if !slices.Contains(key.Fields, field) {
			return ErrForbidden
		}
	}
	allowed := false
	for _, scope := range key.Scopes {
		if ActionAllowsScope(action, scope.Kind) && matchesTarget(scope, target) {
			allowed = true
			break
		}
	}
	if !allowed {
		return ErrForbidden
	}
	return authorizeAge(key.MaxAgeSeconds, target.AgeAnchor, now)
}

// authorizeGrants evaluates only scoped action and age eligibility.
func authorizeGrants(grants []Grant, action string, target Target, now time.Time) error {
	eligible := false
	for _, grant := range grants {
		if err := ValidateGrant(grant); err != nil {
			return err
		}
		if !slices.Contains(grant.Actions, action) || !matchesTarget(grant.Scope, target) {
			continue
		}
		if err := authorizeAge(grant.MaxAgeSeconds, target.AgeAnchor, now); err == nil {
			eligible = true
		} else if err != ErrForbidden {
			return err
		}
	}
	if !eligible {
		return ErrForbidden
	}
	return nil
}

// matchesTarget compares UUID values rather than caller spelling of references.
func matchesTarget(scope Scope, target Target) bool {
	if scope.Kind == "all" {
		return true
	}
	for _, reference := range scope.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return false
		}
		switch scope.Kind {
		case "network":
			if target.NetworkID == id {
				return true
			}
		case "unit":
			if target.UnitID == id {
				return true
			}
		case "unitCollection":
			if slices.Contains(target.UnitCollections, id) {
				return true
			}
		case "account":
			if target.AccountID == id {
				return true
			}
		}
	}
	return false
}

// AgeCutoff returns the inclusive earliest permitted timestamp without duration overflow.
// The floor covers the complete RFC 3339 timestamp range used by the contracts.
func AgeCutoff(seconds int64, now time.Time) (time.Time, error) {
	if seconds < 0 || now.IsZero() || now.Year() < 0 || now.Year() > 9999 {
		return time.Time{}, ErrInvalidInput
	}
	floor := int64(-62167219200)
	if seconds > now.Unix()-floor {
		return time.Unix(floor, 0).UTC(), nil
	}
	return time.Unix(now.Unix()-seconds, int64(now.Nanosecond())).UTC(), nil
}

// authorizeAge evaluates the same inclusive cutoff used by native query predicates.
func authorizeAge(seconds *int64, anchor, now time.Time) error {
	if seconds == nil {
		return nil
	}
	if anchor.IsZero() {
		return ErrInvalidInput
	}
	cutoff, err := AgeCutoff(*seconds, now)
	if err != nil {
		return err
	}
	if anchor.Before(cutoff) {
		return ErrForbidden
	}
	return nil
}
