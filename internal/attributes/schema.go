// Package attributes owns Attribute schemas: administrator-defined, typed and nested field
// declarations with immutable keys and editable display metadata. A `message` schema declares
// the Message attributes that Workflows write; a `unit` schema declares Unit attributes that
// managers maintain. The package validates schemas and their changes; persistence and HTTP
// adapters live elsewhere.
package attributes

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/icons"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Limits of an Attribute schema. Field types additionally obey the limits of
// processing.FieldSchema (depth, properties per object).
const (
	// MaxKeyLength is the maximum length of a schema key in bytes.
	MaxKeyLength = 64
	// MaxNameLength is the maximum length of a field or property name in bytes.
	MaxNameLength = 64
	// MaxLabelLength is the maximum length of a label in characters.
	MaxLabelLength = 256
	// MaxDescriptionLength is the maximum length of a description in characters.
	MaxDescriptionLength = 2048
	// MaxAgencies is the maximum number of Agencies one schema is assigned to.
	MaxAgencies = 256
)

// Subject names what a schema's values describe.
type Subject string

// The schema subjects.
const (
	// SubjectMessage declares Message attributes, written only by Workflows.
	SubjectMessage Subject = "message"
	// SubjectUnit declares Unit attributes, maintained by managers.
	SubjectUnit Subject = "unit"
)

var (
	keyPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// Field declares one attribute field: a typed value (the kinds of processing.FieldSchema)
// with display metadata kept beside the type. Top-level fields are always optional; Optional
// applies to object properties only.
type Field struct {
	Kind        processing.ValueKind `json:"kind"`
	Optional    bool                 `json:"optional,omitempty"`
	Nullable    bool                 `json:"nullable,omitempty"`
	Properties  map[string]Field     `json:"properties,omitempty"`
	Element     *Field               `json:"element,omitempty"`
	Label       string               `json:"label,omitempty"`
	Description string               `json:"description,omitempty"`
	Icon        string               `json:"icon,omitempty"`
	// Deprecated hides the field from new references; stored values stay readable.
	Deprecated bool `json:"deprecated,omitempty"`
}

// Schema is one Attribute schema.
type Schema struct {
	Key         string           `json:"key"`
	Label       string           `json:"label"`
	Description string           `json:"description,omitempty"`
	Subject     Subject          `json:"subject"`
	Fields      map[string]Field `json:"fields"`
	Agencies    []uuid.UUID      `json:"agencies"`
	// Deprecated hides the whole schema from new references; stored values stay readable.
	Deprecated bool      `json:"deprecated"`
	Revision   int64     `json:"revision,string"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func invalid(message string) error {
	return platform.NewFailure(platform.CodeInvalidInput, errors.New(message))
}

// ValidKey reports whether key is a schema key: 1 to MaxKeyLength lowercase ASCII letters,
// digits and '-', starting with a letter.
func ValidKey(key string) bool {
	return len(key) <= MaxKeyLength && keyPattern.MatchString(key)
}

// ValidName reports whether name is a field or property name: 1 to MaxNameLength ASCII
// letters, digits and '_', starting with a letter. The restriction keeps dotted field paths
// unambiguous.
func ValidName(name string) bool {
	return len(name) <= MaxNameLength && namePattern.MatchString(name)
}

// Type returns the value type of a field without display metadata. Top-level fields are
// returned as optional by FieldSchema of Schema; nested properties keep their Optional flag.
func (f Field) Type() processing.FieldSchema {
	schema := processing.FieldSchema{Kind: f.Kind, Optional: f.Optional, Nullable: f.Nullable}
	if f.Properties != nil {
		schema.Properties = make(map[string]processing.FieldSchema, len(f.Properties))
		for name, property := range f.Properties {
			schema.Properties[name] = property.Type()
		}
	}
	if f.Element != nil {
		element := f.Element.Type()
		schema.Element = &element
	}
	return schema
}

// Validate checks the schema's own content: key, subject, display metadata, field names and
// types, and Agencies. It does not check whether the Agencies exist.
func (s Schema) Validate() error {
	if !ValidKey(s.Key) {
		return invalid("invalid Attribute-schema key")
	}
	if s.Subject != SubjectMessage && s.Subject != SubjectUnit {
		return invalid("invalid Attribute-schema subject")
	}
	if err := validateText(s.Label, true, MaxLabelLength); err != nil {
		return err
	}
	if err := validateText(s.Description, false, MaxDescriptionLength); err != nil {
		return err
	}
	if len(s.Fields) == 0 {
		return invalid("Attribute schema declares no field")
	}
	if len(s.Fields) > processing.MaxCollection {
		return platform.NewFailure(platform.CodeResourceLimit, errors.New("attribute-schema field boundary"))
	}
	for name, field := range s.Fields {
		if err := validateField(name, field, true); err != nil {
			return err
		}
		if err := field.Type().Validate(); err != nil {
			return err
		}
	}
	if len(s.Agencies) == 0 || len(s.Agencies) > MaxAgencies {
		return invalid("an Attribute schema needs 1 to 256 Agencies")
	}
	seen := make(map[uuid.UUID]bool, len(s.Agencies))
	for _, agency := range s.Agencies {
		if agency == uuid.Nil || seen[agency] {
			return invalid("invalid or duplicate Agency")
		}
		seen[agency] = true
	}
	return nil
}

func validateField(name string, field Field, topLevel bool) error {
	if !ValidName(name) {
		return invalid("invalid attribute field name")
	}
	if topLevel && field.Optional {
		return invalid("top-level attribute fields are always optional; omit optional")
	}
	return validateMetadata(field, true)
}

// validateMetadata checks the display metadata of a field and its children. allowDeprecated is
// false for list elements, which are not addressable on their own.
func validateMetadata(field Field, allowDeprecated bool) error {
	if err := validateText(field.Label, false, MaxLabelLength); err != nil {
		return err
	}
	if err := validateText(field.Description, false, MaxDescriptionLength); err != nil {
		return err
	}
	if field.Icon != "" && !icons.Valid(field.Icon) {
		return invalid("invalid icon identifier")
	}
	if field.Deprecated && !allowDeprecated {
		return invalid("list elements cannot be deprecated")
	}
	for name, property := range field.Properties {
		if err := validateField(name, property, false); err != nil {
			return err
		}
	}
	if field.Element != nil {
		return validateMetadata(*field.Element, false)
	}
	return nil
}

func validateText(text string, required bool, maxLength int) error {
	if !utf8.ValidString(text) || strings.ContainsRune(text, '\x00') || utf8.RuneCountInString(text) > maxLength {
		return invalid("invalid label or description")
	}
	if required && strings.TrimSpace(text) == "" {
		return invalid("label is required")
	}
	return nil
}

// FieldSchema returns the value type of a top-level field, always optional, and whether it
// exists.
func (s Schema) FieldSchema(name string) (processing.FieldSchema, bool) {
	field, ok := s.Fields[name]
	if !ok {
		return processing.FieldSchema{}, false
	}
	schema := field.Type()
	schema.Optional = true
	return schema, true
}

// Resolve follows a path of a top-level field name and object property names. It returns the
// addressed field, its value type (optional when any step on the path is optional, which every
// top-level field is) and whether any field on the path is deprecated. Paths do not descend
// into list elements.
func (s Schema) Resolve(path []string) (Field, processing.FieldSchema, bool, bool) {
	if len(path) == 0 {
		return Field{}, processing.FieldSchema{}, false, false
	}
	field, ok := s.Fields[path[0]]
	if !ok {
		return Field{}, processing.FieldSchema{}, false, false
	}
	deprecated := s.Deprecated || field.Deprecated
	for _, name := range path[1:] {
		if field.Kind != processing.ObjectValue {
			return Field{}, processing.FieldSchema{}, false, false
		}
		field, ok = field.Properties[name]
		if !ok {
			return Field{}, processing.FieldSchema{}, false, false
		}
		deprecated = deprecated || field.Deprecated
	}
	schema := field.Type()
	schema.Optional = true
	return field, schema, deprecated, true
}
