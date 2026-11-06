package management

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

type groupedDeclarations struct{ preparationDeclarations }

// FieldGroups declares "flag" as payload data and "secret" as gateway data.
func (groupedDeclarations) FieldGroups(ref processing.FieldRef) ([]string, error) {
	if ref.Path[0] == "secret" {
		return []string{"gateway"}, nil
	}
	return []string{"payload"}, nil
}

// cause returns the private cause text of a failure: the platform error model keeps it out of
// Error(), so the offending reference is asserted on the wrapped error.
func cause(err error) string {
	if wrapped := errors.Unwrap(err); wrapped != nil {
		return wrapped.Error()
	}
	return err.Error()
}

func authorWithFields(fields ...string) authz.Authority {
	principal := uuid.NewString()
	authority := authz.Authority{PrincipalID: principal, Revision: 1, Active: true}
	if len(fields) != 0 {
		authority.FieldPermissions = []authz.FieldPermission{{ID: uuid.NewString(), Revision: 1,
			SubjectID: principal, SubjectKind: "principal", Fields: fields}}
	}
	return authority
}

func referencingDefinition(t *testing.T, ref processing.FieldRef) *processing.WorkflowProgram {
	t.Helper()
	value, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &processing.FieldRef{Namespace: "source", Path: []string{"flag"}}, Value: &value},
		OutputSchema: map[string]processing.FieldSchema{"out": {Kind: processing.BooleanValue}},
		Outputs:      map[string]processing.Assignment{"out": {Field: &ref}}}
	program, err := processing.CompileWorkflow(context.Background(), definition, groupedDeclarations{preparationDeclarations{
		"source\x00flag":   {Kind: processing.BooleanValue},
		"source\x00secret": {Kind: processing.BooleanValue},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

// TestAuthorizeReferencesRejectsUnreadableFieldGroupNamingTheReference pins save-time
// validation: the definition is judged by the author's current field permissions and the
// declared groups of every referenced source field, wherever the reference occurs.
func TestAuthorizeReferencesRejectsUnreadableFieldGroupNamingTheReference(t *testing.T) {
	program := referencingDefinition(t, processing.FieldRef{Namespace: "source", Path: []string{"secret"}})
	references := program.SourceReferences()

	err := authorizeReferences(authorWithFields("payload"), references)
	if platform.FailureCode(err) != platform.CodeInvalidInput ||
		!strings.Contains(cause(err), "source.secret") || !strings.Contains(cause(err), "gateway") {
		t.Fatalf("unreadable output reference = %v, want INVALID_INPUT naming source.secret and its group", err)
	}
	if err := authorizeReferences(authorWithFields("payload", "gateway"), references); err != nil {
		t.Fatalf("readable references rejected: %v", err)
	}
	selectorOnly := referencingDefinition(t, processing.FieldRef{Namespace: "source", Path: []string{"flag"}}).SourceReferences()
	if err := authorizeReferences(authorWithFields("payload"), selectorOnly); err != nil {
		t.Fatalf("payload-only definition rejected for a payload reader: %v", err)
	}
	if err := authorizeReferences(authorWithFields(), selectorOnly); platform.FailureCode(err) != platform.CodeInvalidInput ||
		!strings.Contains(cause(err), "source.flag") {
		t.Fatalf("selector reference without any field permission = %v", err)
	}
}

// TestAuthorizeReferencesFailsClosedWithoutDeclaredGroups rejects a reference whose owner
// declared no field group, because its classification cannot be proven readable.
func TestAuthorizeReferencesFailsClosedWithoutDeclaredGroups(t *testing.T) {
	err := authorizeReferences(authorWithFields("payload"), []processing.SourceReference{{
		Field: processing.FieldRef{Namespace: "source", Path: []string{"flag"}}}})
	if platform.FailureCode(err) != platform.CodeInvalidInput || !strings.Contains(cause(err), "source.flag") {
		t.Fatalf("ungrouped reference accepted: %v", err)
	}
}
