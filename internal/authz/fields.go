package authz

import (
	"slices"

	"github.com/google/uuid"
)

// FieldPermission enables global field groups for a Principal or flat User group.
// It contains no data scope, action or age condition.
type FieldPermission struct {
	ID          string   `json:"id"`
	Revision    int64    `json:"revision,string"`
	SubjectID   string   `json:"subjectId"`
	SubjectKind string   `json:"subjectKind"`
	Fields      []string `json:"fields"`
	Delegable   bool     `json:"delegable"`
}

// ValidateFieldPermission checks persisted identity, revision and explicit field groups.
func ValidateFieldPermission(permission FieldPermission) error {
	id, err := uuid.Parse(permission.ID)
	if err != nil || id == uuid.Nil || permission.Revision <= 0 ||
		(permission.SubjectKind != "principal" && permission.SubjectKind != "userGroup") ||
		len(permission.Fields) == 0 || len(permission.Fields) > 1024 {
		return ErrInvalidInput
	}
	subject, err := uuid.Parse(permission.SubjectID)
	if err != nil || subject == uuid.Nil {
		return ErrInvalidInput
	}
	for _, field := range permission.Fields {
		if !ValidField(field) {
			return ErrInvalidInput
		}
	}
	return nil
}

// AuthorizeFields checks global current field permissions and the restricting key.
// Success grants no data rows or operation rights.
func AuthorizeFields(authority Authority, fields []string) error {
	principal, err := uuid.Parse(authority.PrincipalID)
	if err != nil || principal == uuid.Nil || !authority.Active || authority.Revision <= 0 {
		return ErrForbidden
	}
	if len(fields) > 1024 {
		return ErrInvalidInput
	}
	for _, field := range fields {
		if !ValidField(field) {
			return ErrInvalidInput
		}
	}
	allowed := make(map[string]bool)
	for _, permission := range authority.FieldPermissions {
		if err := ValidateFieldPermission(permission); err != nil {
			return err
		}
		subject, _ := uuid.Parse(permission.SubjectID)
		if permission.SubjectKind == "principal" && subject != principal {
			return ErrInvalidInput
		}
		for _, field := range permission.Fields {
			allowed[field] = true
		}
	}
	if authority.Key != nil {
		if err := ValidateRestrictions(authority.Key); err != nil {
			return err
		}
	}
	for _, field := range fields {
		if !allowed[field] || (authority.Key != nil && !slices.Contains(authority.Key.Fields, field)) {
			return ErrForbidden
		}
	}
	return nil
}

// AuthorizeFieldDelegation requires installation-wide permission management and
// globally delegable fields, without deriving them from a scoped action grant.
func AuthorizeFieldDelegation(authority Authority, fields []string) error {
	if err := AuthorizeFields(authority, fields); err != nil {
		return err
	}
	manage := false
	for _, grant := range authority.Grants {
		if err := ValidateGrant(grant); err != nil {
			return err
		}
		if grant.Scope.Kind == "all" && slices.Contains(grant.Actions, "grant.manage") {
			manage = true
		}
	}
	if !manage {
		return ErrForbidden
	}
	if authority.Key != nil {
		if !slices.Contains(authority.Key.Actions, "grant.manage") ||
			!slices.ContainsFunc(authority.Key.Scopes, func(scope Scope) bool { return scope.Kind == "all" }) {
			return ErrForbidden
		}
	}
	for _, field := range fields {
		if !slices.ContainsFunc(authority.FieldPermissions, func(permission FieldPermission) bool {
			return permission.Delegable && slices.Contains(permission.Fields, field)
		}) {
			return ErrForbidden
		}
	}
	return nil
}
