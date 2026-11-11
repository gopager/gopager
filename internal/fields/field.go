// Package fields is the registry of addressable fields: the one place that declares which
// field references exist, their types, field groups and display metadata, and in which scope
// (Workflow definitions, Message queries, Delivery filters and layouts) they may be used.
// Providers contribute fields; the Registry combines them and checks predicates against them.
package fields

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Scope names a place where fields are referenced.
type Scope string

// The scopes.
const (
	// ScopeWorkflow covers the source and target fields of Workflow definitions.
	ScopeWorkflow Scope = "workflow"
	// ScopeMessage covers Message search, the live stream and saved views.
	ScopeMessage Scope = "message"
	// ScopeDelivery covers Subscription filters and Layouts.
	ScopeDelivery Scope = "delivery"
)

// Field declares one addressable field.
type Field struct {
	// Ref is the field reference.
	Ref processing.FieldRef
	// Type is the declared value type.
	Type processing.FieldSchema
	// Group is the field group a reader needs; empty when no field permission is needed.
	Group string
	// Filterable reports whether predicates may compare the field, see FilterableType.
	Filterable bool
	// Label, Description and Icon are display metadata.
	Label, Description, Icon string
	// Deprecated hides the field from listings and, in strict lookups, from new references.
	Deprecated bool
	// Scopes lists where the field may be referenced.
	Scopes []Scope
}

// In reports whether the field may be referenced in the scope.
func (f Field) In(scope Scope) bool { return slices.Contains(f.Scopes, scope) }

// Path returns the dotted reference: the namespace followed by the path segments.
func (f Field) Path() string { return refPath(f.Ref) }

func refPath(ref processing.FieldRef) string {
	return ref.Namespace + "." + strings.Join(ref.Path, ".")
}

// refKey is an unambiguous map key of a reference; unlike the dotted path it cannot collide
// for segments that contain dots.
func refKey(ref processing.FieldRef) string {
	return ref.Namespace + "\x00" + strings.Join(ref.Path, "\x00")
}

// FilterableType reports whether a type can be compared in predicates: a scalar other than
// bytes, or a list whose element is such a scalar.
func FilterableType(schema processing.FieldSchema) bool {
	if schema.Kind == processing.ListValue {
		return schema.Element != nil && isComparableScalar(schema.Element.Kind)
	}
	return isComparableScalar(schema.Kind)
}

func isComparableScalar(kind processing.ValueKind) bool {
	switch kind {
	case processing.StringValue, processing.BooleanValue, processing.IntegerValue,
		processing.NumberValue, processing.TimestampValue:
		return true
	}
	return false
}

// scalarType returns the type of the individual candidate values of a filterable field: the
// element type of a list, otherwise the type itself.
func scalarType(schema processing.FieldSchema) processing.FieldSchema {
	if schema.Kind == processing.ListValue && schema.Element != nil {
		return *schema.Element
	}
	return schema
}

// validRef checks the structure of a reference; owners decide what exists.
func validRef(ref processing.FieldRef) error {
	if ref.Namespace == "" || !validSegment(ref.Namespace) || len(ref.Path) == 0 || len(ref.Path) > processing.MaxDepth {
		return invalidInput("invalid field reference")
	}
	for _, segment := range ref.Path {
		if !validSegment(segment) {
			return invalidInput("invalid field path")
		}
	}
	return nil
}

func validSegment(segment string) bool {
	return segment != "" && utf8.ValidString(segment) && !strings.ContainsRune(segment, '\x00')
}

func invalidInput(message string) error {
	return platform.NewFailure(platform.CodeInvalidInput, errors.New(message))
}

func resourceLimit(message string) error {
	return platform.NewFailure(platform.CodeResourceLimit, errors.New(message))
}
