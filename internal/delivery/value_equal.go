package delivery

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/gopager/gopager/internal/processing"
)

// sameValue reports whether two typed values are equal by meaning, not by encoding: integers
// `"01"` and `"1"`, numbers `1e2` and `100`, and one instant written with different UTC offsets
// are the same value. Lists compare element by element, objects property by property. Values
// that do not decode as their kind are equal to nothing.
func sameValue(left, right processing.Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case processing.IntegerValue:
		a, okA := decodeInteger(left.Value)
		b, okB := decodeInteger(right.Value)
		return okA && okB && a == b
	case processing.NumberValue:
		var a, b float64
		return json.Unmarshal(left.Value, &a) == nil && json.Unmarshal(right.Value, &b) == nil && a == b
	case processing.TimestampValue:
		a, okA := decodeInstant(left.Value)
		b, okB := decodeInstant(right.Value)
		return okA && okB && a.Equal(b)
	case processing.StringValue, processing.BytesValue:
		var a, b string
		return json.Unmarshal(left.Value, &a) == nil && json.Unmarshal(right.Value, &b) == nil && a == b
	case processing.BooleanValue:
		var a, b bool
		return json.Unmarshal(left.Value, &a) == nil && json.Unmarshal(right.Value, &b) == nil && a == b
	case processing.NullValue:
		return true
	case processing.ListValue:
		var a, b []processing.Value
		if json.Unmarshal(left.Value, &a) != nil || json.Unmarshal(right.Value, &b) != nil || len(a) != len(b) {
			return false
		}
		for i := range a {
			if !sameValue(a[i], b[i]) {
				return false
			}
		}
		return true
	case processing.ObjectValue:
		var a, b map[string]processing.Value
		if json.Unmarshal(left.Value, &a) != nil || json.Unmarshal(right.Value, &b) != nil || len(a) != len(b) {
			return false
		}
		for name, value := range a {
			other, ok := b[name]
			if !ok || !sameValue(value, other) {
				return false
			}
		}
		return true
	}
	return false
}

// decodeInteger reads a typed integer, which travels as a decimal string.
func decodeInteger(raw json.RawMessage) (int64, bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.ParseInt(text, 10, 64)
	return number, err == nil
}

// decodeInstant reads a typed timestamp, an RFC 3339 string.
func decodeInstant(raw json.RawMessage) (time.Time, bool) {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return time.Time{}, false
	}
	instant, err := time.Parse(time.RFC3339Nano, text)
	return instant, err == nil
}
