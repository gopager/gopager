package declarations

import (
	"encoding/json"
	"slices"
	"strings"

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

// DefinitionUnitAttributeRefs returns the distinct Unit attributes a definition reads
// (`target.unit.attributes.<key>…`), in order of appearance. Every field reference of a
// definition is an explicit `{namespace, path}` object, so walking its JSON finds them without
// compiling the definition or loading the plugins of its installed steps.
func DefinitionUnitAttributeRefs(definition processing.Definition) ([]processing.FieldRef, error) {
	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, err
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return nil, err
	}
	var result []processing.FieldRef
	seen := map[string]bool{}
	var walk func(value any)
	walk = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, element := range typed {
				walk(element)
			}
		case map[string]any:
			if ref, ok := fieldRef(typed); ok && isUnitAttributeRef(ref) {
				key := strings.Join(ref.Path, "\x00")
				if !seen[key] {
					seen[key] = true
					result = append(result, ref)
				}
				return
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(document)
	return result, nil
}

// fieldRef decodes a `{namespace, path}` object.
func fieldRef(object map[string]any) (processing.FieldRef, bool) {
	namespace, ok := object["namespace"].(string)
	elements, isList := object["path"].([]any)
	if !ok || !isList {
		return processing.FieldRef{}, false
	}
	path := make([]string, 0, len(elements))
	for _, element := range elements {
		segment, ok := element.(string)
		if !ok {
			return processing.FieldRef{}, false
		}
		path = append(path, segment)
	}
	return processing.FieldRef{Namespace: namespace, Path: path}, true
}

// UnitAttributeDeclarations returns the target declarations of the given Unit attribute
// references, typed by the registry, so that a source projection carries (possibly absent) exactly
// the fields a Workflow reads and nothing else. Deprecated fields are declared too, because an
// active version may still read them. A reference the registry does not offer is an error.
func UnitAttributeDeclarations(registry *fields.Registry, refs []processing.FieldRef,
) ([]processing.FieldDeclaration, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	source := registry.Declarations(fields.ScopeWorkflow, false)
	result := make([]processing.FieldDeclaration, 0, len(refs))
	for _, ref := range refs {
		schema, err := source.FieldSchema(ref)
		if err != nil {
			return nil, err
		}
		schema.Optional = true
		result = append(result, processing.FieldDeclaration{Reference: ref, Schema: schema,
			Groups: []string{unitGroup}})
	}
	return result, nil
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
	// Declared are the target declarations of the read attributes, see UnitAttributeDeclarations.
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
