package units

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Unit-attribute sources of an EffectiveValue.
const (
	// SourceUnit marks a value set on the Unit (or template) itself.
	SourceUnit = "unit"
	// SourceOrganization marks a value inherited from the Organization's defaults.
	SourceOrganization = "organization"
)

// AttributeSettings are the Unit-attribute entries of a Unit or template: per `unit` Attribute
// schema key and top-level field, a FieldSetting of typed values. A missing field inherits the
// Organization default; Canonical drops explicit "inherit" entries, so stored settings hold only
// "value" and "unset" entries. A nil map has no entries and encodes as an empty object.
type AttributeSettings map[string]map[string]FieldSetting[processing.Value]

// AttributeDefaults are an Organization's Unit-attribute defaults: per schema key and top-level
// field a DefaultSetting. A missing field is unset; Canonical drops explicit "unset" entries. A
// nil map has no entries and encodes as an empty object.
type AttributeDefaults map[string]map[string]DefaultSetting[processing.Value]

// EffectiveAttributes are resolved Unit-attribute values per schema key and field, with Source
// SourceUnit or SourceOrganization. Fields without an effective value are absent. A nil map
// encodes as an empty object.
type EffectiveAttributes map[string]map[string]EffectiveValue[processing.Value]

// AttributeSchema is the part of a `unit` Attribute schema that Unit-attribute values are
// checked against: the Agencies it is assigned to and the value type of each top-level field.
type AttributeSchema struct {
	Agencies []uuid.UUID
	Fields   map[string]processing.FieldSchema
}

// AssignedTo reports whether the schema is assigned to the Agency.
func (s AttributeSchema) AssignedTo(agency uuid.UUID) bool {
	return slices.Contains(s.Agencies, agency)
}

// AttributeSchemas are the `unit` Attribute schemas by key.
type AttributeSchemas map[string]AttributeSchema

// AssignedTo returns the assignment test of the Agency for ResolveSettings and
// MaterializeSettings: whether a schema key names a schema assigned to the Agency.
func (s AttributeSchemas) AssignedTo(agency uuid.UUID) func(schemaKey string) bool {
	return func(schemaKey string) bool {
		schema, ok := s[schemaKey]
		return ok && schema.AssignedTo(agency)
	}
}

// AttributeCatalog supplies the `unit` Attribute schemas that Unit-attribute values are checked
// against. Both methods run in the caller's transaction.
type AttributeCatalog interface {
	// UnitSchemas returns every `unit` Attribute schema by key.
	UnitSchemas(ctx context.Context, tx platform.TransactionContext) (AttributeSchemas, error)
	// LockCatalog holds the schema catalog against changes for the rest of the write transaction,
	// so that a schema losing an Agency or being deleted waits for a write that validated values
	// against it, and then sees them.
	LockCatalog(ctx context.Context, tx platform.TransactionContext) error
}

// MarshalJSON encodes a nil map as an empty object.
func (a AttributeSettings) MarshalJSON() ([]byte, error) {
	return marshalObject(map[string]map[string]FieldSetting[processing.Value](a))
}

// UnmarshalJSON decodes the entries and puts every value in canonical form, so that stored and
// requested settings compare equal when they hold the same values.
func (a *AttributeSettings) UnmarshalJSON(data []byte) error {
	var decoded map[string]map[string]FieldSetting[processing.Value]
	if err := decodeStrict(data, &decoded); err != nil {
		return err
	}
	for _, fields := range decoded {
		for name, setting := range fields {
			if setting.Value != nil {
				value, err := canonicalValue(*setting.Value)
				if err != nil {
					return err
				}
				setting.Value = &value
				fields[name] = setting
			}
		}
	}
	*a = emptyToNil(decoded)
	return nil
}

// MarshalJSON encodes a nil map as an empty object.
func (a AttributeDefaults) MarshalJSON() ([]byte, error) {
	return marshalObject(map[string]map[string]DefaultSetting[processing.Value](a))
}

// UnmarshalJSON decodes the entries and puts every value in canonical form.
func (a *AttributeDefaults) UnmarshalJSON(data []byte) error {
	var decoded map[string]map[string]DefaultSetting[processing.Value]
	if err := decodeStrict(data, &decoded); err != nil {
		return err
	}
	for _, fields := range decoded {
		for name, setting := range fields {
			if setting.Value != nil {
				value, err := canonicalValue(*setting.Value)
				if err != nil {
					return err
				}
				setting.Value = &value
				fields[name] = setting
			}
		}
	}
	*a = emptyToNil(decoded)
	return nil
}

// MarshalJSON encodes a nil map as an empty object.
func (a EffectiveAttributes) MarshalJSON() ([]byte, error) {
	return marshalObject(map[string]map[string]EffectiveValue[processing.Value](a))
}

// decodeStrict decodes data and rejects undeclared properties, as the strict request decoding of
// the management routes does for the other setting members; a custom unmarshaler would otherwise
// hide them from that decoder.
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func marshalObject[T any](value map[string]map[string]T) ([]byte, error) {
	if value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(value)
}

// emptyToNil returns nil for a map without field entries, so that an absent and an empty
// attributes member compare equal.
func emptyToNil[T any](value map[string]map[string]T) map[string]map[string]T {
	for key, fields := range value {
		if len(fields) == 0 {
			delete(value, key)
		}
	}
	if len(value) == 0 {
		return nil
	}
	return value
}

// canonicalValue re-encodes the JSON of a typed value with sorted object keys and without
// insignificant white space. PostgreSQL reorders and re-spaces jsonb, so only the canonical form
// makes a stored value equal to the same requested one.
func canonicalValue(value processing.Value) (processing.Value, error) {
	if value.Kind == "" || len(value.Value) == 0 {
		return processing.Value{}, authz.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(value.Value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return processing.Value{}, authz.ErrInvalidInput
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return processing.Value{}, authz.ErrInvalidInput
	}
	return processing.Value{Kind: value.Kind, Value: encoded}, nil
}

// Canonical checks the modes, drops "inherit" entries and puts values in canonical form. It
// does not check the values against schemas (see Check). It returns authz.ErrInvalidInput for an
// empty key or field name, an unknown mode, a value in "inherit" or "unset" mode or a missing
// value in "value" mode.
func (a AttributeSettings) Canonical() (AttributeSettings, error) {
	result := AttributeSettings{}
	for key, fields := range a {
		if key == "" {
			return nil, authz.ErrInvalidInput
		}
		for name, setting := range fields {
			if name == "" {
				return nil, authz.ErrInvalidInput
			}
			canonical, err := canonicalFieldSetting(setting, func(processing.Value) error { return nil })
			if err != nil {
				return nil, err
			}
			if canonical.Mode == "inherit" {
				continue
			}
			if canonical.Value != nil {
				value, err := canonicalValue(*canonical.Value)
				if err != nil {
					return nil, err
				}
				canonical.Value = &value
			}
			if result[key] == nil {
				result[key] = map[string]FieldSetting[processing.Value]{}
			}
			result[key][name] = canonical
		}
	}
	return emptyToNil(result), nil
}

// Canonical checks the modes, drops "unset" entries and puts values in canonical form. It does
// not check the values against schemas (see Check). It returns authz.ErrInvalidInput for an empty
// key or field name, an unknown mode, a value in "unset" mode or a missing value in "value" mode.
func (a AttributeDefaults) Canonical() (AttributeDefaults, error) {
	result := AttributeDefaults{}
	for key, fields := range a {
		if key == "" {
			return nil, authz.ErrInvalidInput
		}
		for name, setting := range fields {
			if name == "" {
				return nil, authz.ErrInvalidInput
			}
			canonical, err := canonicalDefaultSetting(setting, func(processing.Value) error { return nil })
			if err != nil {
				return nil, err
			}
			if canonical.Mode == "unset" {
				continue
			}
			value, err := canonicalValue(*canonical.Value)
			if err != nil {
				return nil, err
			}
			canonical.Value = &value
			if result[key] == nil {
				result[key] = map[string]DefaultSetting[processing.Value]{}
			}
			result[key][name] = canonical
		}
	}
	return emptyToNil(result), nil
}

// Check validates canonical entries against the `unit` schemas: every entry names an existing
// schema and a declared top-level field, and every value decodes against the field's type. With
// a non-nil agency (a Unit) the schema must also be assigned to it; templates pass nil. It
// returns authz.ErrInvalidInput on the first violation.
func (a AttributeSettings) Check(schemas AttributeSchemas, agency *uuid.UUID) error {
	for key, fields := range a {
		schema, ok := schemas[key]
		if !ok || (agency != nil && !schema.AssignedTo(*agency)) {
			return authz.ErrInvalidInput
		}
		for name, setting := range fields {
			if err := checkValue(schema, name, setting.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// Check validates canonical defaults against the `unit` schemas: every entry names an existing
// schema and a declared top-level field, and every value decodes against the field's type.
// Organizations have no Agency, so assignments are not checked. It returns
// authz.ErrInvalidInput on the first violation.
func (a AttributeDefaults) Check(schemas AttributeSchemas) error {
	for key, fields := range a {
		schema, ok := schemas[key]
		if !ok {
			return authz.ErrInvalidInput
		}
		for name, setting := range fields {
			if err := checkValue(schema, name, setting.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkValue checks that the field is declared and that a present value decodes against it.
func checkValue(schema AttributeSchema, field string, value *processing.Value) error {
	fieldType, ok := schema.Fields[field]
	if !ok {
		return authz.ErrInvalidInput
	}
	if value == nil {
		return nil
	}
	fieldType.Optional = true
	if _, err := value.Native(fieldType); err != nil {
		return authz.ErrInvalidInput
	}
	return nil
}

// ResolveAttributes computes the effective Unit-attribute values: a "value" entry with source
// SourceUnit, no value for "unset", and for a missing field the Organization's default with
// source SourceOrganization. defaults is nil without an Organization. assigned, when not nil,
// restricts inherited defaults to schemas it reports as assigned to the Unit's Agency; templates
// pass nil because they have no Agency.
func ResolveAttributes(settings AttributeSettings, defaults AttributeDefaults, assigned func(schemaKey string) bool) EffectiveAttributes {
	result := EffectiveAttributes{}
	set := func(key, field string, value processing.Value, source string) {
		if result[key] == nil {
			result[key] = map[string]EffectiveValue[processing.Value]{}
		}
		copied := value
		copied.Value = slices.Clone(value.Value)
		result[key][field] = EffectiveValue[processing.Value]{Value: &copied, Source: source}
	}
	for key, fields := range settings {
		for field, setting := range fields {
			if setting.Mode == "value" && setting.Value != nil {
				set(key, field, *setting.Value, SourceUnit)
			}
		}
	}
	for key, fields := range defaults {
		if assigned != nil && !assigned(key) {
			continue
		}
		for field, setting := range fields {
			if _, own := settings[key][field]; own || setting.Mode != "value" || setting.Value == nil {
				continue
			}
			set(key, field, *setting.Value, SourceOrganization)
		}
	}
	return emptyToNil(result)
}

// MaterializeAttributes turns every still-inherited field with an effective value into a
// "value" entry, as an Organization deletion does before it detaches the Unit or template.
// assigned has the meaning of ResolveAttributes. Explicit entries are kept.
func MaterializeAttributes(settings AttributeSettings, defaults AttributeDefaults, assigned func(schemaKey string) bool) AttributeSettings {
	result := settings.clone()
	for key, fields := range ResolveAttributes(settings, defaults, assigned) {
		for field, effective := range fields {
			if effective.Source != SourceOrganization {
				continue
			}
			if result[key] == nil {
				result[key] = map[string]FieldSetting[processing.Value]{}
			}
			result[key][field] = FieldSetting[processing.Value]{Mode: "value", Value: effective.Value}
		}
	}
	return emptyToNil(result)
}

// CopyAttributes returns the entries of a Unit created from a template: for every field of every
// schema assigned to the Unit's Agency, the template's effective value as "value", else "unset",
// so that later template or Organization edits do not change the Unit. Values of schemas not
// assigned to the Agency are not copied.
func CopyAttributes(effective EffectiveAttributes, schemas AttributeSchemas, agency uuid.UUID) AttributeSettings {
	result := AttributeSettings{}
	for key, schema := range schemas {
		if !schema.AssignedTo(agency) {
			continue
		}
		fields := make(map[string]FieldSetting[processing.Value], len(schema.Fields))
		for field := range schema.Fields {
			if value, ok := effective[key][field]; ok && value.Value != nil {
				copied := *value.Value
				copied.Value = slices.Clone(copied.Value)
				fields[field] = FieldSetting[processing.Value]{Mode: "value", Value: &copied}
				continue
			}
			fields[field] = FieldSetting[processing.Value]{Mode: "unset"}
		}
		result[key] = fields
	}
	return emptyToNil(result)
}

// clone copies the maps; values are immutable and shared.
func (a AttributeSettings) clone() AttributeSettings {
	if a == nil {
		return AttributeSettings{}
	}
	result := make(AttributeSettings, len(a))
	for key, fields := range a {
		copied := make(map[string]FieldSetting[processing.Value], len(fields))
		for field, setting := range fields {
			copied[field] = setting
		}
		result[key] = copied
	}
	return result
}

// Empty reports whether there are no entries.
func (a AttributeSettings) Empty() bool { return len(a) == 0 }

// Empty reports whether there are no entries.
func (a AttributeDefaults) Empty() bool { return len(a) == 0 }
