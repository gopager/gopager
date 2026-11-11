package attributes

import "github.com/gopager/gopager/internal/processing"

// CheckChange reports whether next may replace current as a new revision of the same schema.
// Allowed are: adding top-level fields, adding optional object properties at any level,
// setting or clearing deprecation, editing labels, descriptions and icons, and changing the
// Agencies. The key, the subject and the type of every existing field and property stay
// unchanged, and nothing is removed; any other change needs a new schema. Both schemas must
// be valid on their own.
func CheckChange(current, next Schema) error {
	if current.Key != next.Key {
		return invalid("the key of an Attribute schema cannot change")
	}
	if current.Subject != next.Subject {
		return invalid("the subject of an Attribute schema cannot change")
	}
	for name, field := range current.Fields {
		replacement, ok := next.Fields[name]
		if !ok {
			return invalid("an Attribute-schema field cannot be removed; deprecate it instead")
		}
		if !compatibleType(field, replacement) {
			return invalid("the type of an Attribute-schema field cannot change")
		}
	}
	return nil
}

// compatibleType reports whether next keeps the type of current and only adds optional
// properties to objects, at any depth.
func compatibleType(current, next Field) bool {
	if current.Kind != next.Kind || current.Nullable != next.Nullable || current.Optional != next.Optional {
		return false
	}
	switch current.Kind {
	case processing.ObjectValue:
		for name, property := range current.Properties {
			replacement, ok := next.Properties[name]
			if !ok || !compatibleType(property, replacement) {
				return false
			}
		}
		for name, property := range next.Properties {
			if _, existed := current.Properties[name]; !existed && !property.Optional {
				return false
			}
		}
	case processing.ListValue:
		if current.Element == nil || next.Element == nil {
			return current.Element == next.Element
		}
		return compatibleType(*current.Element, *next.Element)
	}
	return true
}
