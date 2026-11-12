package declarations

import (
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/units"
)

// unitGroup is the field group of everything a target reads from its Unit.
const unitGroup = "unit"

// isUnitAttributeRef reports whether a target reference names a Unit attribute value,
// target.unit.attributes.<schemaKey>.<field>[.<property>…].
func isUnitAttributeRef(ref processing.FieldRef) bool {
	return ref.Namespace == "target" && len(ref.Path) >= 4 &&
		ref.Path[0] == "unit" && ref.Path[1] == "attributes"
}

// UnitAttributeDeclarations returns the target declarations of every Unit attribute the registry
// offers to Workflows, so that a source projection carries (possibly absent) every field a
// Workflow may read. The result is empty when no `unit` Attribute schema declares a field.
func UnitAttributeDeclarations(registry *fields.Registry) []processing.FieldDeclaration {
	var result []processing.FieldDeclaration
	for _, field := range registry.ListAll(fields.ScopeWorkflow) {
		if !isUnitAttributeRef(field.Ref) {
			continue
		}
		schema := field.Type
		schema.Optional = true
		result = append(result, processing.FieldDeclaration{Reference: field.Ref, Schema: schema,
			Groups: []string{unitGroup}})
	}
	return result
}

// UnitAttributeFields returns the captured fields of one target for the given declarations. An
// attribute is present when the target's Unit has an effective value, descended into the object
// properties of the path, and absent otherwise, which includes a target without a Unit
// (effective nil). Lists are not descended into; they are captured as a whole value. dependencies
// are attached to every field.
func UnitAttributeFields(declared []processing.FieldDeclaration, effective units.EffectiveAttributes,
	dependencies []processing.Dependency,
) ([]processing.CapturedField, error) {
	result := make([]processing.CapturedField, 0, len(declared))
	for _, declaration := range declared {
		value, err := unitAttributeValue(effective, declaration.Reference.Path[2:])
		if err != nil {
			return nil, err
		}
		state := processing.FieldAbsent
		if value != nil {
			state = processing.FieldPresent
		}
		result = append(result, processing.CapturedField{Reference: declaration.Reference,
			Schema: declaration.Schema, Field: processing.FieldValue{State: state, Value: value,
				Dependencies: slices.Clone(dependencies)}})
	}
	return result, nil
}

// unitAttributeValue returns the effective value at <schemaKey>.<field>[.<property>…], or nil.
func unitAttributeValue(effective units.EffectiveAttributes, path []string) (*processing.Value, error) {
	own, ok := effective[path[0]][path[1]]
	if !ok || own.Value == nil {
		return nil, nil
	}
	value := own.Value
	for _, property := range path[2:] {
		if value.Kind != processing.ObjectValue {
			return nil, nil
		}
		var members map[string]processing.Value
		if err := json.Unmarshal(value.Value, &members); err != nil {
			return nil, err
		}
		member, ok := members[property]
		if !ok {
			return nil, nil
		}
		value = &member
	}
	copied := *value
	copied.Value = slices.Clone(value.Value)
	return &copied, nil
}

// UnitAttributes are the Unit attributes a source projection captures: the declared target
// fields and the effective attributes of the Units the targets resolve to. The zero value
// declares nothing.
type UnitAttributes struct {
	// Declared are the target declarations, see UnitAttributeDeclarations.
	Declared []processing.FieldDeclaration
	// Effective are the effective attributes by Unit.
	Effective map[uuid.UUID]units.EffectiveAttributes
}

// TargetFields returns the captured Unit-attribute fields of one target; a target without a Unit
// (nil) carries them absent. Attributes are captured with the source, outside the context
// revision, so a later change to a Unit's attributes never re-admits a processed Transmission.
func (a UnitAttributes) TargetFields(unit *uuid.UUID, dependencies []processing.Dependency,
) ([]processing.CapturedField, error) {
	if len(a.Declared) == 0 {
		return nil, nil
	}
	var effective units.EffectiveAttributes
	if unit != nil {
		effective = a.Effective[*unit]
	}
	return UnitAttributeFields(a.Declared, effective, dependencies)
}
