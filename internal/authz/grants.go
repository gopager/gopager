// Package authz owns store-neutral structured grant semantics.
package authz

import (
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// Sentinel authorization outcomes, compared with errors.Is. ErrInvalidInput
// marks malformed requests or persisted grants, ErrForbidden a denied
// decision, ErrNotFound a target that does not exist or must not be revealed,
// and ErrRevisionConflict a stale expected revision or otherwise conflicting
// current state. They are *platform.Failure values carrying the public code
// (INVALID_INPUT, FORBIDDEN, NOT_FOUND, REVISION_CONFLICT), so platform.FailureCode
// stays correct when one crosses a transaction boundary; errors.Is still matches
// them by identity. Their messages are the safe English fallbacks.
var (
	ErrInvalidInput     error = platform.NewFailure(platform.CodeInvalidInput, nil)
	ErrForbidden        error = platform.NewFailure(platform.CodeForbidden, nil)
	ErrNotFound         error = platform.NewFailure(platform.CodeNotFound, nil)
	ErrRevisionConflict error = platform.NewFailure(platform.CodeRevisionConflict, nil)
)

// Scope restricts a grant or key to a data area. Kind is one of "all",
// "network", "unit", "unitCollection" or "account"; "all" must carry no
// References, every other kind needs 1..1024 non-nil UUID references (see
// ValidScope). A reference matches a Target by UUID value.
type Scope struct {
	Kind       string   `json:"kind"`
	References []string `json:"references,omitempty"`
}

// Grant gives a subject (a "principal" or a "userGroup", identified by a
// non-nil UUID SubjectID) the listed catalog Actions within Scope. A non-nil
// MaxAgeSeconds limits it to targets whose age anchor lies within that many
// seconds of the decision time. Delegable is carried for grant management;
// Authorize does not evaluate it. Grants are validated by ValidateGrant on
// every decision, so one malformed grant fails the whole decision.
type Grant struct {
	ID            string   `json:"id"`
	SubjectID     string   `json:"subjectId"`
	SubjectKind   string   `json:"subjectKind"`
	Actions       []string `json:"actions"`
	Scope         Scope    `json:"scope"`
	MaxAgeSeconds *int64   `json:"maxAgeSeconds,omitempty"`
	Delegable     bool     `json:"delegable"`
}

// Restrictions narrow what an API key may do on top of its owner's grants:
// only the listed Actions, within at least one of Scopes, only the listed
// Fields, and optionally only within MaxAgeSeconds. They never add rights the
// owner lacks. ValidateRestrictions rejects a nil or empty restriction.
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
	for _, action := range []string{"read", "create", "update", "delete"} {
		add("unit-template."+action, "all")
	}
	add("assignment-correction.read assignment-correction.retry", "all", "network", "unit", "unitCollection")
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
