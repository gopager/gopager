package processing

import (
	"context"
	"errors"
)

// attributesNamespace is the field namespace of Message attributes, which Workflow outputs write.
const attributesNamespace = "attributes"

// OutputRef returns the field reference of one Workflow output: the field of an Attribute schema.
func OutputRef(schemaKey, field string) FieldRef {
	return FieldRef{Namespace: attributesNamespace, Path: []string{schemaKey, field}}
}

// OutputFieldSchema resolves the type of one Workflow output through the output declarations. A
// top-level attribute field is always optional for the producer, because a Workflow writes a
// value only when it has one; nested property requirements are those of the schema.
func OutputFieldSchema(outputs Declarations, schemaKey, field string) (FieldSchema, error) {
	if outputs == nil {
		return FieldSchema{}, capabilityUnavailable(errors.New("output declarations unavailable"))
	}
	if !validLocalName(schemaKey) || !validLocalName(field) {
		return FieldSchema{}, invalid(errors.New("invalid output name"))
	}
	schema, err := outputs.FieldSchema(OutputRef(schemaKey, field))
	if err != nil {
		return FieldSchema{}, err
	}
	schema.Optional = true
	return schema, nil
}

// compileOutputs resolves the type of every declared output and checks that its assignment
// produces that type. It returns the schemas by schema key and field.
func compileOutputs(mappings map[string]map[string]Assignment, outputs Declarations,
	inputs Declarations) (map[string]map[string]FieldSchema, error) {
	result := make(map[string]map[string]FieldSchema, len(mappings))
	total := 0
	for _, key := range sortedNames(mappings) {
		result[key] = make(map[string]FieldSchema, len(mappings[key]))
		for _, field := range sortedNames(mappings[key]) {
			total++
			if total > MaxCollection {
				return nil, resource(errors.New("final output boundary"))
			}
			schema, err := OutputFieldSchema(outputs, key, field)
			if err != nil {
				return nil, err
			}
			if err := validateAssignment(mappings[key][field], schema, inputs); err != nil {
				return nil, err
			}
			result[key][field] = schema
		}
	}
	return result, nil
}

// evaluateOutputs produces derived values: whatever the source of an assignment, the result
// carries no per-value classification, because the definition is classified as a whole. Only
// present values are kept, so a Workflow never overwrites an attribute it has no value for.
func evaluateOutputs(ctx context.Context, schemas map[string]map[string]FieldSchema,
	mappings map[string]map[string]Assignment, frame *executionFrame) (map[string]map[string]FieldValue, error) {
	result := make(map[string]map[string]FieldValue, len(schemas))
	for _, key := range sortedNames(schemas) {
		for _, name := range sortedNames(schemas[key]) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			assignment := mappings[key][name]
			field := FieldValue{State: FieldAbsent}
			if assignment.Field != nil {
				var err error
				field, err = frame.LookupField(ctx, *assignment.Field)
				if err != nil {
					return nil, err
				}
			} else {
				field.State, field.Value = FieldPresent, assignment.Literal
			}
			if field.State != FieldPresent {
				continue
			}
			value, err := convertValue(*field.Value, schemas[key][name], assignment.Conversion)
			if err != nil {
				return nil, err
			}
			field.Value, field.Dependencies = &value, nil
			if err := validateFieldValue(schemas[key][name], field, false); err != nil {
				return nil, err
			}
			if result[key] == nil {
				result[key] = make(map[string]FieldValue)
			}
			result[key][name] = field
		}
	}
	return result, nil
}
