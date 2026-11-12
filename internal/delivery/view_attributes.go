package delivery

import (
	"encoding/json"

	"github.com/gopager/gopager/internal/processing"
)

// attributesNamespace is the namespace of Message attribute fields.
const attributesNamespace = "attributes"

// attributeCandidates resolves attributes.<schemaKey>.<field>[.<property>…] over the effective
// values the subscriber may read: the untargeted value first, then the values of targets
// assigned to a relevant Unit by ascending position. A target value whose Unit the subscriber
// cannot see has no Unit and is never relevant.
func (v *view) attributeCandidates(path []string) []processing.Value {
	if v.message == nil || v.message.Item.Attributes == nil || len(path) < 2 {
		return nil
	}
	var values []processing.Value
	for _, attribute := range *v.message.Item.Attributes {
		if attribute.SchemaKey != path[0] || attribute.Field != path[1] {
			continue
		}
		if attribute.TargetPosition != nil && (attribute.UnitID == nil || !v.relevantUnit(*attribute.UnitID)) {
			continue
		}
		if value, ok := descend(attribute.Value, path[2:]); ok {
			values = append(values, value)
		}
	}
	return values
}

// descend follows object properties from a value; ok is false when one is missing or the value
// on the way is no object.
func descend(value processing.Value, properties []string) (processing.Value, bool) {
	for _, property := range properties {
		var object map[string]processing.Value
		if value.Kind != processing.ObjectValue || json.Unmarshal(value.Value, &object) != nil {
			return processing.Value{}, false
		}
		next, found := object[property]
		if !found {
			return processing.Value{}, false
		}
		value = next
	}
	return value, true
}
