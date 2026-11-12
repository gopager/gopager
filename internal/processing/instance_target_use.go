package processing

import (
	"errors"

	"github.com/google/uuid"
)

// MaxTargetUsePrincipals bounds the Principals a target use may list.
const MaxTargetUsePrincipals = 1000

// TargetUse states who may use a Delivery instance as a Subscription target: everyone, or the
// listed Principals. Groups are unsupported because the resolved authority carries none.
type TargetUse struct {
	Everyone     bool        `json:"everyone"`
	PrincipalIDs []uuid.UUID `json:"principalIds"`
}

// Validate bounds the list and rejects nil and duplicate Principal IDs.
func (u TargetUse) Validate() error {
	if len(u.PrincipalIDs) > MaxTargetUsePrincipals {
		return invalid(errors.New("target use principal list boundary"))
	}
	seen := make(map[uuid.UUID]struct{}, len(u.PrincipalIDs))
	for _, id := range u.PrincipalIDs {
		if _, duplicate := seen[id]; duplicate || id == uuid.Nil {
			return invalid(errors.New("invalid target use principal"))
		}
		seen[id] = struct{}{}
	}
	return nil
}

// AllowsTargetUse reports whether principal may use the instance as a Delivery target. An instance
// without a target use admits nobody.
func (i InstanceConfiguration) AllowsTargetUse(principal uuid.UUID) bool {
	if i.TargetUse == nil {
		return false
	}
	if i.TargetUse.Everyone {
		return true
	}
	for _, id := range i.TargetUse.PrincipalIDs {
		if id == principal {
			return true
		}
	}
	return false
}
