package attributes

import (
	"slices"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// attributeNamespace is the namespace of Message attribute field references.
const attributeNamespace = "attributes"

// NewProvider returns the field provider of a schema catalog snapshot. Every field of a
// `message` schema, and every object property below it at any depth, becomes the field
// `attributes.<key>.<field>[.<property>…]`; list elements are not addressable. Every field of a
// `unit` schema and its object properties likewise become `unit.attributes.<key>.<field>[…]`
// (Message and Delivery filters) and `target.unit.attributes.<key>.<field>[…]` (Workflows). The
// provider reports the given revision and looks fields up in constant time, because a catalog is
// built once per registry revision.
func NewProvider(schemas []Schema, revision int64) fields.Provider {
	var result []fields.Field
	for _, schema := range schemas {
		names := make([]string, 0, len(schema.Fields))
		for name := range schema.Fields {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			switch schema.Subject {
			case SubjectMessage:
				result = appendFields(result, schema, []string{name}, messageShape)
			case SubjectUnit:
				result = appendFields(result, schema, []string{name}, unitShape...)
			}
		}
	}
	return fields.NewStaticProvider(revision, result)
}

// appendFields adds the field at path, once for each shape, and, for objects, its properties.
func appendFields(result []fields.Field, schema Schema, path []string, shapes ...shape) []fields.Field {
	field, fieldType, deprecated, ok := schema.Resolve(path)
	if !ok {
		return result
	}
	label := field.Label
	if label == "" {
		label = path[len(path)-1]
	}
	for _, shape := range shapes {
		result = append(result, fields.Field{
			Ref:         shape.ref(schema.Key, path),
			Type:        fieldType,
			Group:       shape.group,
			Filterable:  fields.FilterableType(fieldType),
			Label:       label,
			Description: field.Description,
			Icon:        field.Icon,
			Deprecated:  deprecated,
			Scopes:      shape.scopes,
		})
	}
	if field.Kind != processing.ObjectValue {
		return result
	}
	properties := make([]string, 0, len(field.Properties))
	for name := range field.Properties {
		properties = append(properties, name)
	}
	slices.Sort(properties)
	for _, name := range properties {
		result = appendFields(result, schema, append(slices.Clone(path), name), shapes...)
	}
	return result
}
