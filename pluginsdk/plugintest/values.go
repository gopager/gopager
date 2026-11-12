package plugintest

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gopager/gopager/internal/processing"
)

// typedValue converts a plain Go value to the Core's typed wire value, choosing the kind from the
// Go type: strings, booleans, integers, floats, time.Time, map[string]any, []any and []string, or
// an already typed processing.Value. Whether the kind matches the manifest's declaration is left
// to the Core, as in production.
func typedValue(value any) (processing.Value, error) {
	switch v := value.(type) {
	case processing.Value:
		return v, nil
	case string:
		return processing.Literal(processing.StringValue, v)
	case bool:
		return processing.Literal(processing.BooleanValue, v)
	case int:
		return processing.Literal(processing.IntegerValue, strconv.FormatInt(int64(v), 10))
	case int32:
		return processing.Literal(processing.IntegerValue, strconv.FormatInt(int64(v), 10))
	case int64:
		return processing.Literal(processing.IntegerValue, strconv.FormatInt(v, 10))
	case float64:
		return processing.Literal(processing.NumberValue, v)
	case time.Time:
		return processing.Literal(processing.TimestampValue, v.UTC().Format(time.RFC3339Nano))
	case map[string]any:
		return typedObject(v)
	case []string:
		items := make([]any, len(v))
		for i, item := range v {
			items[i] = item
		}
		return typedList(items)
	case []any:
		return typedList(v)
	}
	return processing.Value{}, fmt.Errorf("unsupported Go type %T", value)
}

// typedObject converts the members of an object.
func typedObject(members map[string]any) (processing.Value, error) {
	converted := make(map[string]processing.Value, len(members))
	for name, member := range members {
		value, err := typedValue(member)
		if err != nil {
			return processing.Value{}, fmt.Errorf("%s: %w", name, err)
		}
		converted[name] = value
	}
	return processing.Literal(processing.ObjectValue, converted)
}

// typedList converts the elements of a list.
func typedList(items []any) (processing.Value, error) {
	converted := make([]processing.Value, len(items))
	for i, item := range items {
		value, err := typedValue(item)
		if err != nil {
			return processing.Value{}, fmt.Errorf("[%d]: %w", i, err)
		}
		converted[i] = value
	}
	return processing.Literal(processing.ListValue, converted)
}

// withSecretReferences returns a copy of values in which each dotted Secret path holds its
// reference string.
func withSecretReferences(values map[string]any, references map[string]string) map[string]any {
	merged := make(map[string]any, len(values)+len(references))
	for name, value := range values {
		merged[name] = value
	}
	for path, reference := range references {
		setPath(merged, strings.Split(path, "."), reference)
	}
	return merged
}

// setPath stores value at the nested path, copying every map on the way.
func setPath(target map[string]any, path []string, value any) {
	if len(path) == 1 {
		target[path[0]] = value
		return
	}
	child := map[string]any{}
	if existing, ok := target[path[0]].(map[string]any); ok {
		for name, member := range existing {
			child[name] = member
		}
	}
	setPath(child, path[1:], value)
	target[path[0]] = child
}
