package delivery

import (
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// bindingFields looks up every input binding of the targets in the registry, strict. A binding
// whose field is unknown, outside the delivery scope or deprecated is a fault, so that a field
// lost after the save makes the Subscription invalid like a lost filter field.
func bindingFields(targets []TargetReference, declarations fields.Declarations) ([]fields.Field, error) {
	var found []fields.Field
	for _, target := range targets {
		for _, ref := range target.InputBindings {
			field, err := declarations.Field(ref)
			if err != nil {
				return nil, &fault{reason: ReasonFieldUnavailable, err: err}
			}
			found = append(found, field)
		}
	}
	return found, nil
}

// checkBindings validates the input bindings of one target at save: each names an input the
// descriptor declares and a Unit attribute field of the same kind. Reading the field's group is
// checked with the groups of the whole selection.
func checkBindings(bindings map[string]processing.FieldRef, descriptor TargetDescriptor,
	declarations fields.Declarations) error {
	if len(bindings) > MaxTargetInputs {
		return authz.ErrInvalidInput
	}
	for name, ref := range bindings {
		input, declared := descriptor.input(name)
		if !declared || !isUnitAttribute(ref) {
			return authz.ErrInvalidInput
		}
		field, err := declarations.Field(ref)
		if err != nil || field.Type.Kind != input.Kind {
			return authz.ErrInvalidInput
		}
	}
	return nil
}

// boundValue returns the value a binding reads from the Delivery's Units: the effective value
// of the field. Units without a value or with an explicit null do not count. conflict is true
// when Units hold different values.
func boundValue(units []readmodel.UnitRecord, ref processing.FieldRef) (value *processing.Value, conflict bool) {
	path := ref.Path
	for _, unit := range units {
		for _, attribute := range unit.Attributes {
			if attribute.SchemaKey != path[1] || attribute.Field != path[2] {
				continue
			}
			found, ok := descend(attribute.Value, path[3:])
			if !ok || found.Kind == processing.NullValue {
				continue
			}
			if value != nil && !sameValue(*value, found) {
				return nil, true
			}
			value = &found
		}
	}
	return value, false
}

// applyBindings overrides the constants of inputs with the bound Unit attribute values, in
// descriptor order. It returns CodeParameterConflict when the Units disagree on a bound value.
func applyBindings(inputs map[string]processing.Value, descriptor TargetDescriptor, target TargetReference,
	units []readmodel.UnitRecord) string {
	for _, field := range descriptor.Inputs {
		ref, bound := target.InputBindings[field.Name]
		if !bound || !isUnitAttribute(ref) {
			continue
		}
		value, conflict := boundValue(units, ref)
		if conflict {
			return CodeParameterConflict
		}
		if value != nil {
			inputs[field.Name] = *value
		}
	}
	return ""
}

// bindingGroups adds the field groups the bindings need to the set.
func bindingGroups(found []fields.Field, groups map[string]bool) {
	for _, field := range found {
		groups[field.Group] = true
	}
}
