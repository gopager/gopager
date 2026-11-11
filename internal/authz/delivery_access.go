package authz

import (
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DeliveryTarget is the recorded context a Delivery is authorized against: its subscriber and
// the Agencies, Units and Unit collections of the data it was materialized from.
type DeliveryTarget struct {
	SubscriberID    uuid.UUID
	Agencies        []uuid.UUID
	Units           []uuid.UUID
	UnitCollections []uuid.UUID
	AgeAnchor       time.Time
}

// NewDeliveryAccess compiles the grants and the restricting key of one delivery.* action into
// clauses. A store evaluates them before it filters or pages Deliveries; AllowsDelivery evaluates
// them for one Delivery. Clause kinds are "all", "account", "agency", "unit" and "unitCollection".
func NewDeliveryAccess(authority Authority, action string, now time.Time) (ActionAccess, error) {
	if !strings.HasPrefix(action, "delivery.") || !ValidAction(action) {
		return ActionAccess{}, ErrInvalidInput
	}
	return actionAccess(authority, action, now)
}

// AllowsDelivery reports whether the access permits the Delivery: some owner clause matches the
// subscriber's account, one of the Agencies, Units or Unit collections, or is "all", without the
// age anchor preceding the clause's cutoff; a restricting key must match the same way.
func (a ActionAccess) AllowsDelivery(target DeliveryTarget) bool {
	match := func(c AccessClause) bool { return c.matchesDelivery(target) }
	if !slices.ContainsFunc(a.Owner, match) {
		return false
	}
	if !a.Restricted {
		return true
	}
	if a.KeyCutoff != nil && target.AgeAnchor.Before(*a.KeyCutoff) {
		return false
	}
	return slices.ContainsFunc(a.Key, match)
}

// matchesDelivery evaluates one clause against a Delivery context.
func (c AccessClause) matchesDelivery(target DeliveryTarget) bool {
	if c.Cutoff != nil && target.AgeAnchor.Before(*c.Cutoff) {
		return false
	}
	any := func(ids []uuid.UUID) bool {
		return slices.ContainsFunc(ids, func(id uuid.UUID) bool { return slices.Contains(c.References, id) })
	}
	switch c.Kind {
	case "all":
		return true
	case "account":
		return target.SubscriberID != uuid.Nil && slices.Contains(c.References, target.SubscriberID)
	case "agency":
		return any(target.Agencies)
	case "unit":
		return any(target.Units)
	case "unitCollection":
		return any(target.UnitCollections)
	}
	return false
}
