// Package authz owns store-neutral structured grant semantics.
package authz

import (
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidInput     = errors.New("invalid authorization input")
	ErrForbidden        = errors.New("this action is not permitted")
	ErrNotFound         = errors.New("the requested item is not available")
	ErrRevisionConflict = errors.New("this item changed; reload before saving")
)

type Scope struct {
	Kind       string   `json:"kind"`
	References []string `json:"references,omitempty"`
}

type Grant struct {
	ID            string   `json:"id"`
	SubjectID     string   `json:"subjectId"`
	SubjectKind   string   `json:"subjectKind"`
	Actions       []string `json:"actions"`
	Scope         Scope    `json:"scope"`
	MaxAgeSeconds *int64   `json:"maxAgeSeconds,omitempty"`
	Delegable     bool     `json:"delegable"`
}

type Restrictions struct {
	Actions       []string `json:"actions"`
	Scopes        []Scope  `json:"scopes"`
	Fields        []string `json:"fields,omitempty"`
	MaxAgeSeconds *int64   `json:"maxAgeSeconds,omitempty"`
}

// Authority is loaded by the server for one primary-store authorization decision.
type Authority struct {
	PrincipalID      string
	Revision         int64
	Active           bool
	Grants           []Grant
	FieldPermissions []FieldPermission
	Key              *Restrictions
}

var actionScopes = buildActionScopes()

func buildActionScopes() map[string][]string {
	out := make(map[string][]string)
	add := func(names string, scopes ...string) {
		for _, name := range strings.Fields(names) {
			out[name] = scopes
		}
	}
	add("message.read message.detail", "all", "network", "unit", "unitCollection")
	add("without-unit.read", "all", "network")
	for _, resource := range []string{"unit", "organization", "unit-collection", "address-rule", "suppression"} {
		for _, action := range []string{"read", "create", "update", "delete"} {
			add(resource+"."+action, "all", "network", "unit", "unitCollection")
		}
	}
	for _, resource := range []string{"gateway", "network"} {
		for _, action := range []string{"read", "create", "update", "delete"} {
			add(resource+"."+action, "all", "network")
		}
	}
	add("grouping.configure", "all")
	add("account.read account.create account.update account.delete account.delete-own grant.manage "+
		"password.local recovery.request recovery.issue recovery.consume session.manage-own "+
		"key.create-own key.manage-own key.manage-technical subscription.manage-own subscription.manage-global",
		"all", "account")
	add("delivery.read delivery.retry delivery.redeliver", "all", "network", "unit", "unitCollection", "account")
	add("rule.read rule.create rule.update rule.delete rule.preview rule.activate rule.reprocess presentation.manage",
		"all", "network", "unit")
	add("notice.manage plugin.configure operations.manage", "all")
	add("audit.read", "all", "network", "account")
	add("plugin.internal-read", "all", "network", "unit", "unitCollection")
	return out
}

// Actions returns an independent sorted copy of the explicit grantable catalog.
func Actions() []string {
	out := make([]string, 0, len(actionScopes))
	for action := range actionScopes {
		out = append(out, action)
	}
	slices.Sort(out)
	return out
}

// ValidAction recognizes only explicit catalog entries, never action wildcards.
func ValidAction(action string) bool {
	_, ok := actionScopes[action]
	return ok
}

// ActionAllowsScope checks the explicit operation/scope compatibility.
func ActionAllowsScope(action, kind string) bool {
	return slices.Contains(actionScopes[action], kind)
}

// ValidField recognizes standard fields or syntactically namespaced extension fields.
// The owning field registry must additionally validate any extension declaration.
func ValidField(field string) bool {
	if slices.Contains([]string{"payload", "address", "unit", "gateway", "quality", "derived"}, field) {
		return true
	}
	parts := strings.Split(field, ".")
	return len(parts) == 3 && parts[0] == "extension" && parts[1] != "" && parts[2] != ""
}

// ValidateGrant rejects unknown actions, malformed references and implicit wildcards.
func ValidateGrant(grant Grant) error {
	if len(grant.Actions) == 0 || len(grant.Actions) > 1024 ||
		(grant.SubjectKind != "principal" && grant.SubjectKind != "userGroup") ||
		!ValidScope(grant.Scope) || (grant.MaxAgeSeconds != nil && *grant.MaxAgeSeconds < 0) {
		return ErrInvalidInput
	}
	if id, err := uuid.Parse(grant.SubjectID); err != nil || id == uuid.Nil {
		return ErrInvalidInput
	}
	for _, action := range grant.Actions {
		if !ActionAllowsScope(action, grant.Scope.Kind) {
			return ErrInvalidInput
		}
	}
	return nil
}

// ValidScope rejects empty restrictions and invalid server references.
func ValidScope(scope Scope) bool {
	if !slices.Contains([]string{"all", "network", "unit", "unitCollection", "account"}, scope.Kind) {
		return false
	}
	if scope.Kind == "all" {
		return len(scope.References) == 0
	}
	if len(scope.References) == 0 || len(scope.References) > 1024 {
		return false
	}
	for _, ref := range scope.References {
		if id, err := uuid.Parse(ref); err != nil || id == uuid.Nil {
			return false
		}
	}
	return true
}

// ValidateRestrictions rejects an absent key restriction instead of granting owner access.
func ValidateRestrictions(restrictions *Restrictions) error {
	if restrictions == nil || len(restrictions.Actions) == 0 || len(restrictions.Actions) > 1024 ||
		len(restrictions.Scopes) == 0 || len(restrictions.Scopes) > 1024 ||
		len(restrictions.Fields) > 1024 ||
		(restrictions.MaxAgeSeconds != nil && *restrictions.MaxAgeSeconds < 0) {
		return ErrInvalidInput
	}
	for _, action := range restrictions.Actions {
		if !ValidAction(action) {
			return ErrInvalidInput
		}
	}
	for _, scope := range restrictions.Scopes {
		if !ValidScope(scope) {
			return ErrInvalidInput
		}
	}
	for _, field := range restrictions.Fields {
		if !ValidField(field) {
			return ErrInvalidInput
		}
	}
	return nil
}
