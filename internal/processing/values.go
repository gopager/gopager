// Package processing owns bounded, source-classified Workflow evaluation and configuration.
package processing

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gopager/gopager/internal/platform"
)

// Processing size and shape limits. Exceeding one is rejected as invalid input or a
// resource-limit failure, never truncated.
const (
	// MaxInputBytes bounds an encoded input document, value or predicate (1 MiB).
	MaxInputBytes = 1 << 20
	// MaxOutputBytes bounds an encoded evaluation result or plugin response (256 KiB).
	MaxOutputBytes = 256 << 10
	// MaxDepth bounds nesting of values, schemas, field paths, predicates and steps.
	MaxDepth = 32
	// MaxCollection bounds schema properties, output fields and list page sizes.
	MaxCollection = 1000
	// MaxArguments bounds predicate arguments/values, dependency groups and the targets of
	// one Transmission.
	MaxArguments = 1024
	// MaxSteps bounds the total number of steps in a Workflow definition, nested branch
	// steps included.
	MaxSteps = 128
)

// ValueKind is the closed set of typed value kinds used in field schemas and values.
type ValueKind string

// The value kinds. ObjectValue and ListValue are composite; NullValue is the kind of an
// explicit null, which a schema admits only when Nullable is set.
const (
	StringValue    ValueKind = "string"
	BooleanValue   ValueKind = "boolean"
	IntegerValue   ValueKind = "integer"
	NumberValue    ValueKind = "number"
	TimestampValue ValueKind = "timestamp"
	BytesValue     ValueKind = "bytes"
	ObjectValue    ValueKind = "object"
	ListValue      ValueKind = "list"
	NullValue      ValueKind = "null"
)

// FieldSchema declares a closed field type, with absence independent from explicit null.
type FieldSchema struct {
	Kind       ValueKind              `json:"kind"`
	Optional   bool                   `json:"optional,omitempty"`
	Nullable   bool                   `json:"nullable,omitempty"`
	Properties map[string]FieldSchema `json:"properties,omitempty"`
	Element    *FieldSchema           `json:"element,omitempty"`
}

// Value preserves exact typed wire data; Native validates it against an owning declaration.
type Value struct {
	Kind  ValueKind       `json:"kind"`
	Value json.RawMessage `json:"value"`
}

func invalid(cause error) error {
	return platform.NewFailure(platform.CodeInvalidInput, cause)
}

func resource(cause error) error {
	return platform.NewFailure(platform.CodeResourceLimit, cause)
}

// Decode accepts one bounded UTF-8 JSON document with no undeclared struct attributes.
func Decode(data []byte, target any) error {
	if len(data) > MaxInputBytes {
		return resource(errors.New("processing input exceeds encoded boundary"))
	}
	if !utf8.Valid(data) {
		return invalid(errors.New("processing JSON is not UTF-8"))
	}
	if err := validateJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return invalid(err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return invalid(errors.New("processing JSON contains trailing data"))
	}
	return nil
}

func validateJSON(data []byte) error {
	type frame struct {
		kind json.Delim
		keys map[string]bool
		key  bool
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var stack []frame
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return invalid(err)
		}
		if delimiter, ok := token.(json.Delim); ok {
			if delimiter == '{' || delimiter == '[' {
				if len(stack) >= MaxDepth {
					return resource(errors.New("JSON nesting boundary"))
				}
				stack = append(stack, frame{kind: delimiter, key: delimiter == '{', keys: make(map[string]bool)})
				continue
			}
			stack = stack[:len(stack)-1]
			if len(stack) > 0 && stack[len(stack)-1].kind == '{' {
				stack[len(stack)-1].key = true
			}
			continue
		}
		if len(stack) == 0 || stack[len(stack)-1].kind != '{' {
			continue
		}
		current := &stack[len(stack)-1]
		if current.key {
			name, ok := token.(string)
			if !ok || current.keys[name] {
				return invalid(errors.New("duplicate or invalid JSON property"))
			}
			current.keys[name], current.key = true, false
		} else {
			current.key = true
		}
	}
}

// UnmarshalJSON retains null presence and rejects fields outside the typed-value envelope.
func (v *Value) UnmarshalJSON(data []byte) error {
	type wireValue Value
	var decoded wireValue
	if err := Decode(data, &decoded); err != nil {
		return err
	}
	if decoded.Kind == "" || len(decoded.Value) == 0 {
		return invalid(errors.New("incomplete typed value"))
	}
	*v = Value(decoded)
	return nil
}

// Validate rejects malformed or excessive declarations before any native adaptation.
func (s FieldSchema) Validate() error { return s.validate(1) }

func (s FieldSchema) validate(depth int) error {
	if depth > MaxDepth || len(s.Properties) > MaxCollection {
		return resource(errors.New("processing schema boundary"))
	}
	switch s.Kind {
	case ObjectValue:
		if s.Element != nil {
			return invalid(errors.New("object schema has a list element"))
		}
		for name, property := range s.Properties {
			if name == "" || !utf8.ValidString(name) {
				return invalid(errors.New("invalid declared property name"))
			}
			if err := property.validate(depth + 1); err != nil {
				return err
			}
		}
	case ListValue:
		if s.Element == nil || len(s.Properties) != 0 {
			return invalid(errors.New("invalid list declaration"))
		}
		return s.Element.validate(depth + 1)
	case StringValue, BooleanValue, IntegerValue, NumberValue, TimestampValue, BytesValue:
		if s.Element != nil || len(s.Properties) != 0 {
			return invalid(errors.New("scalar schema contains child declarations"))
		}
	default:
		return invalid(errors.New("unknown declared value kind"))
	}
	return nil
}

// Native validates the exact wire kind and recursively returns CEL-compatible native values.
func (v Value) Native(schema FieldSchema) (any, error) {
	if err := schema.Validate(); err != nil {
		return nil, err
	}
	decodedBytes := 0
	return v.native(schema, 1, &decodedBytes)
}

func (v Value) native(schema FieldSchema, depth int, decodedBytes *int) (any, error) {
	if depth > MaxDepth {
		return nil, resource(errors.New("typed value nesting boundary"))
	}
	if v.Kind == NullValue {
		if !schema.Nullable || !bytes.Equal(bytes.TrimSpace(v.Value), []byte("null")) {
			return nil, invalid(errors.New("undeclared nullable value"))
		}
		return nil, nil
	}
	if v.Kind != schema.Kind || bytes.Equal(bytes.TrimSpace(v.Value), []byte("null")) {
		return nil, invalid(errors.New("typed value does not match declaration"))
	}
	switch v.Kind {
	case StringValue, IntegerValue, TimestampValue:
		var text string
		if err := Decode(v.Value, &text); err != nil {
			return nil, err
		}
		if v.Kind == StringValue {
			return text, nil
		}
		if v.Kind == IntegerValue {
			number, err := strconv.ParseInt(text, 10, 64)
			if err != nil {
				return nil, invalid(err)
			}
			return number, nil
		}
		instant, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return nil, invalid(err)
		}
		// PostgreSQL has no year zero; such a value would fail every later cast of the stored
		// value, so it is rejected here, where all typed values are checked.
		if instant.Year() < 1 || instant.UTC().Year() < 1 {
			return nil, invalid(errors.New("timestamp before year 1"))
		}
		return instant, nil
	case BytesValue:
		var encoded string
		if err := Decode(v.Value, &encoded); err != nil {
			return nil, err
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || base64.StdEncoding.EncodeToString(decoded) != encoded {
			return nil, invalid(errors.New("bytes value is not canonical Base64"))
		}
		*decodedBytes += len(decoded)
		if *decodedBytes > MaxInputBytes {
			return nil, resource(errors.New("decoded processing bytes boundary"))
		}
		return decoded, nil
	case BooleanValue:
		var boolean bool
		if err := Decode(v.Value, &boolean); err != nil {
			return nil, err
		}
		return boolean, nil
	case NumberValue:
		var number json.Number
		if err := Decode(v.Value, &number); err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(v.Value)) == 0 || bytes.TrimSpace(v.Value)[0] == '"' {
			return nil, invalid(errors.New("numeric string is not a number"))
		}
		native, err := number.Float64()
		if err != nil || math.IsNaN(native) || math.IsInf(native, 0) {
			return nil, invalid(errors.New("nonfinite processing number"))
		}
		// A nonzero literal that rounds to zero underflows; PostgreSQL rejects it when the
		// stored text is cast, so it is rejected here, where all typed values are checked.
		if native == 0 && nonzeroMantissa(string(number)) {
			return nil, invalid(errors.New("processing number underflows"))
		}
		return native, nil
	case ObjectValue:
		var properties map[string]Value
		if err := Decode(v.Value, &properties); err != nil {
			return nil, err
		}
		if len(properties) > MaxCollection {
			return nil, resource(errors.New("object collection boundary"))
		}
		native := make(map[string]any, len(properties))
		for name, value := range properties {
			declaration, ok := schema.Properties[name]
			if !ok {
				return nil, invalid(errors.New("undeclared processing property"))
			}
			converted, err := value.native(declaration, depth+1, decodedBytes)
			if err != nil {
				return nil, err
			}
			native[name] = converted
		}
		for name, declaration := range schema.Properties {
			if _, present := properties[name]; !present && !declaration.Optional {
				return nil, invalid(errors.New("required processing property is absent"))
			}
		}
		return native, nil
	case ListValue:
		var values []Value
		if err := Decode(v.Value, &values); err != nil {
			return nil, err
		}
		if len(values) > MaxCollection {
			return nil, resource(errors.New("list collection boundary"))
		}
		native := make([]any, len(values))
		for i, value := range values {
			converted, err := value.native(*schema.Element, depth+1, decodedBytes)
			if err != nil {
				return nil, err
			}
			native[i] = converted
		}
		return native, nil
	}
	return nil, invalid(fmt.Errorf("unsupported value kind %q", v.Kind))
}

func decodedBytesInJSON(encoded []byte) (int, error) {
	var visit func(any) (int, error)
	visit = func(value any) (int, error) {
		switch current := value.(type) {
		case []any:
			total := 0
			for _, item := range current {
				size, err := visit(item)
				if err != nil {
					return 0, err
				}
				total += size
				if total > MaxInputBytes {
					return 0, resource(errors.New("decoded processing bytes boundary"))
				}
			}
			return total, nil
		case map[string]any:
			if current["kind"] == string(BytesValue) {
				encodedValue, ok := current["value"].(string)
				if !ok {
					return 0, invalid(errors.New("bytes value is not a Base64 string"))
				}
				decoded, err := base64.StdEncoding.DecodeString(encodedValue)
				if err != nil || base64.StdEncoding.EncodeToString(decoded) != encodedValue {
					return 0, invalid(errors.New("bytes value is not canonical Base64"))
				}
				return len(decoded), nil
			}
			total := 0
			for _, item := range current {
				size, err := visit(item)
				if err != nil {
					return 0, err
				}
				total += size
				if total > MaxInputBytes {
					return 0, resource(errors.New("decoded processing bytes boundary"))
				}
			}
			return total, nil
		default:
			return 0, nil
		}
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, invalid(err)
	}
	return visit(value)
}

// Literal creates an explicitly typed value, preserving integer wire precision.
func Literal(kind ValueKind, value any) (Value, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return Value{}, invalid(err)
	}
	if len(encoded) > MaxInputBytes {
		return Value{}, resource(errors.New("processing literal boundary"))
	}
	return Value{Kind: kind, Value: encoded}, nil
}

// nonzeroMantissa reports whether the significand of a JSON number literal has a nonzero digit.
func nonzeroMantissa(literal string) bool {
	if i := strings.IndexAny(literal, "eE"); i >= 0 {
		literal = literal[:i]
	}
	return strings.ContainsAny(literal, "123456789")
}
