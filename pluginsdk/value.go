package pluginsdk

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Typed value kinds used by Value.
const (
	// KindString is a JSON string.
	KindString = "string"
	// KindInteger is a decimal string in the int64 range.
	KindInteger = "integer"
	// KindBoolean is a JSON boolean.
	KindBoolean = "boolean"
	// KindNumber is a finite JSON number.
	KindNumber = "number"
	// KindObject is a map of property name to Value.
	KindObject = "object"
	// KindList is an array of Value.
	KindList = "list"
	// KindNull is the explicit null.
	KindNull = "null"
)

// Value is a typed value in the Core's envelope {"kind": K, "value": V}.
type Value struct {
	// Kind names the type of Value.
	Kind string `json:"kind"`
	// Value is the encoded value; its encoding depends on Kind.
	Value json.RawMessage `json:"value"`
}

// String returns the value of a string Value, or an error when the kind differs.
func (v Value) String() (string, error) {
	var out string
	return out, v.decode(KindString, &out)
}

// Boolean returns the value of a boolean Value, or an error when the kind differs.
func (v Value) Boolean() (bool, error) {
	var out bool
	return out, v.decode(KindBoolean, &out)
}

// Number returns the value of a number Value, or an error when the kind differs.
func (v Value) Number() (float64, error) {
	var out float64
	return out, v.decode(KindNumber, &out)
}

// Integer returns the value of an integer Value, whose encoding is a decimal string, or an
// error when the kind differs or the text is not an int64.
func (v Value) Integer() (int64, error) {
	var text string
	if err := v.decode(KindInteger, &text); err != nil {
		return 0, err
	}
	out, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("pluginsdk: invalid integer value: %w", err)
	}
	return out, nil
}

// decode checks the kind and unmarshals the value into target.
func (v Value) decode(kind string, target any) error {
	if v.Kind != kind {
		return fmt.Errorf("pluginsdk: value is of kind %q, not %q", v.Kind, kind)
	}
	if err := json.Unmarshal(v.Value, target); err != nil {
		return fmt.Errorf("pluginsdk: invalid %s value: %w", kind, err)
	}
	return nil
}

// Plain converts the typed value to the plain JSON it stands for: integers become numbers,
// objects and lists are converted recursively, other kinds keep their encoding. It returns
// an error for an unknown kind or a malformed value.
func (v Value) Plain() (json.RawMessage, error) {
	switch v.Kind {
	case KindInteger:
		number, err := v.Integer()
		if err != nil {
			return nil, err
		}
		return json.RawMessage(strconv.FormatInt(number, 10)), nil
	case KindObject:
		var members map[string]Value
		if err := json.Unmarshal(v.Value, &members); err != nil {
			return nil, fmt.Errorf("pluginsdk: invalid object value: %w", err)
		}
		return plainObject(members)
	case KindList:
		var elements []Value
		if err := json.Unmarshal(v.Value, &elements); err != nil {
			return nil, fmt.Errorf("pluginsdk: invalid list value: %w", err)
		}
		plains := make([]json.RawMessage, len(elements))
		for i, element := range elements {
			plain, err := element.Plain()
			if err != nil {
				return nil, err
			}
			plains[i] = plain
		}
		return json.Marshal(plains)
	case KindString, KindBoolean, KindNumber, KindNull, "timestamp", "bytes":
		return v.Value, nil
	default:
		return nil, fmt.Errorf("pluginsdk: unknown value kind %q", v.Kind)
	}
}

// plainObject converts typed members to one plain JSON object.
func plainObject(members map[string]Value) (json.RawMessage, error) {
	plains := make(map[string]json.RawMessage, len(members))
	for name, member := range members {
		plain, err := member.Plain()
		if err != nil {
			return nil, fmt.Errorf("property %q: %w", name, err)
		}
		plains[name] = plain
	}
	return json.Marshal(plains)
}
