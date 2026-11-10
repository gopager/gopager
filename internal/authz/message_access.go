package authz

import (
	"slices"
	"time"

	"github.com/google/uuid"
)

// MessageFieldGroups are the standard field groups that shape Message projections and filters,
// in a stable order.
var MessageFieldGroups = []string{"payload", "address", "unit", "gateway", "quality", "derived"}

// AccessClause is one alternative of a row-eligibility predicate: the scope Kind ("all",
// "agency", "unit" or "unitCollection") with its References, and an optional inclusive
// Cutoff that the row's age anchor must not precede.
type AccessClause struct {
	Kind       string
	References []uuid.UUID
	Cutoff     *time.Time
}

// matches evaluates the clause for one authorization target like Authorize does.
func (c AccessClause) matches(target Target) bool {
	if c.Cutoff != nil && target.AgeAnchor.Before(*c.Cutoff) {
		return false
	}
	switch c.Kind {
	case "all":
		return true
	case "agency":
		return slices.Contains(c.References, target.AgencyID)
	case "unit":
		return target.UnitID != uuid.Nil && slices.Contains(c.References, target.UnitID)
	case "unitCollection":
		return slices.ContainsFunc(target.UnitCollections, func(id uuid.UUID) bool { return slices.Contains(c.References, id) })
	}
	return false
}

// ActionAccess is the store-neutral form of one action's row eligibility: a row is eligible
// when any Owner clause matches it and, if a restricting key exists, any Key clause matches
// it and the row is not older than KeyCutoff. The clauses carry each grant's own age so that
// one grant never lends its scope to another grant's age limit.
type ActionAccess struct {
	Owner      []AccessClause
	Restricted bool
	Key        []AccessClause
	KeyCutoff  *time.Time
}

// Allows evaluates the access for one target. It matches Authorize for the same action
// when the fields argument of Authorize is empty.
func (a ActionAccess) Allows(target Target) bool {
	if !slices.ContainsFunc(a.Owner, func(c AccessClause) bool { return c.matches(target) }) {
		return false
	}
	if !a.Restricted {
		return true
	}
	if a.KeyCutoff != nil && target.AgeAnchor.Before(*a.KeyCutoff) {
		return false
	}
	return slices.ContainsFunc(a.Key, func(c AccessClause) bool { return c.matches(target) })
}

// Granted reports whether any row could be eligible at all: the owner holds the action in some
// grant and a restricting key, if any, allows it with at least one scope.
func (a ActionAccess) Granted() bool {
	return len(a.Owner) > 0 && (!a.Restricted || len(a.Key) > 0)
}

// MessageAccess is the row eligibility of the Message query: which appearances of a Message
// (a Unit or a Unit-less row) the caller may read and view in detail.
type MessageAccess struct {
	Read        ActionAccess
	Detail      ActionAccess
	WithoutUnit ActionAccess
}

// NewMessageAccess derives the eligibility from the caller's current authority at now. It
// fails like Authorize: ErrForbidden for an inactive authority and ErrInvalidInput for a
// malformed grant or key. Callers must resolve the authority in the transaction that reads
// the rows.
func NewMessageAccess(authority Authority, now time.Time) (MessageAccess, error) {
	principal, err := uuid.Parse(authority.PrincipalID)
	if !authority.Active || err != nil || principal == uuid.Nil || authority.Revision <= 0 {
		return MessageAccess{}, ErrForbidden
	}
	if now.IsZero() {
		return MessageAccess{}, ErrInvalidInput
	}
	for _, grant := range authority.Grants {
		if err := ValidateGrant(grant); err != nil {
			return MessageAccess{}, err
		}
	}
	if authority.Key != nil {
		if err := ValidateRestrictions(authority.Key); err != nil {
			return MessageAccess{}, err
		}
	}
	var access MessageAccess
	if access.Read, err = actionAccess(authority, "message.read", now); err != nil {
		return MessageAccess{}, err
	}
	if access.Detail, err = actionAccess(authority, "message.detail", now); err != nil {
		return MessageAccess{}, err
	}
	if access.WithoutUnit, err = actionAccess(authority, "without-unit.read", now); err != nil {
		return MessageAccess{}, err
	}
	return access, nil
}

// ReadEligible reports whether the target is a readable appearance: message.read on it and,
// for a Unit-less target, without-unit.read as well.
func (m MessageAccess) ReadEligible(target Target) bool {
	return m.Read.Allows(target) && (!target.WithoutUnit || m.WithoutUnit.Allows(target))
}

// DetailEligible reports whether the target is a readable appearance that also allows detail.
func (m MessageAccess) DetailEligible(target Target) bool {
	return m.ReadEligible(target) && m.Detail.Allows(target)
}

// actionAccess builds the clauses of one action from grants and the restricting key.
func actionAccess(authority Authority, action string, now time.Time) (ActionAccess, error) {
	var access ActionAccess
	for _, grant := range authority.Grants {
		if !slices.Contains(grant.Actions, action) {
			continue
		}
		clause, err := accessClause(grant.Scope, grant.MaxAgeSeconds, now)
		if err != nil {
			return ActionAccess{}, err
		}
		access.Owner = append(access.Owner, clause)
	}
	key := authority.Key
	if key == nil {
		return access, nil
	}
	access.Restricted = true
	if !slices.Contains(key.Actions, action) || (action == "message.detail" && !slices.Contains(key.Actions, "message.read")) {
		return access, nil
	}
	for _, scope := range key.Scopes {
		if !ActionAllowsScope(action, scope.Kind) {
			continue
		}
		clause, err := accessClause(scope, nil, now)
		if err != nil {
			return ActionAccess{}, err
		}
		access.Key = append(access.Key, clause)
	}
	if key.MaxAgeSeconds != nil {
		cutoff, err := AgeCutoff(*key.MaxAgeSeconds, now)
		if err != nil {
			return ActionAccess{}, err
		}
		access.KeyCutoff = &cutoff
	}
	return access, nil
}

// accessClause converts a validated scope and optional maximum age into a clause.
func accessClause(scope Scope, maxAge *int64, now time.Time) (AccessClause, error) {
	clause := AccessClause{Kind: scope.Kind}
	for _, reference := range scope.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return AccessClause{}, ErrInvalidInput
		}
		clause.References = append(clause.References, id)
	}
	if maxAge != nil {
		cutoff, err := AgeCutoff(*maxAge, now)
		if err != nil {
			return AccessClause{}, err
		}
		clause.Cutoff = &cutoff
	}
	return clause, nil
}

// ReadableFieldGroups returns the standard field groups the caller may read now, in the order
// of MessageFieldGroups. A group is readable when AuthorizeFields allows it, which already
// intersects a restricting key.
func ReadableFieldGroups(authority Authority) ([]string, error) {
	var groups []string
	for _, group := range MessageFieldGroups {
		err := AuthorizeFields(authority, []string{group})
		if err == nil {
			groups = append(groups, group)
			continue
		}
		if err != ErrForbidden {
			return nil, err
		}
	}
	return groups, nil
}
