package processing_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestClosedTypedValuesPreserveWireSemantics(t *testing.T) {
	for _, item := range []struct {
		raw    string
		schema processing.FieldSchema
		valid  bool
	}{
		{`{"kind":"integer","value":"9223372036854775807"}`, processing.FieldSchema{Kind: processing.IntegerValue}, true},
		{`{"kind":"integer","value":9223372036854775807}`, processing.FieldSchema{Kind: processing.IntegerValue}, false},
		{`{"kind":"integer","value":"9223372036854775808"}`, processing.FieldSchema{Kind: processing.IntegerValue}, false},
		{`{"kind":"number","value":1.25}`, processing.FieldSchema{Kind: processing.NumberValue}, true},
		{`{"kind":"number","value":"1.25"}`, processing.FieldSchema{Kind: processing.NumberValue}, false},
		{`{"kind":"number","value":1e1000}`, processing.FieldSchema{Kind: processing.NumberValue}, false},
		{`{"kind":"timestamp","value":"2026-11-01T12:00:00+01:00"}`, processing.FieldSchema{Kind: processing.TimestampValue}, true},
		{`{"kind":"timestamp","value":"2026-11-01T12:00:00"}`, processing.FieldSchema{Kind: processing.TimestampValue}, false},
		{`{"kind":"bytes","value":"AAEC/w=="}`, processing.FieldSchema{Kind: processing.BytesValue}, true},
		{`{"kind":"bytes","value":""}`, processing.FieldSchema{Kind: processing.BytesValue}, true},
		{`{"kind":"bytes","value":"AAEC/w"}`, processing.FieldSchema{Kind: processing.BytesValue}, false},
		{`{"kind":"bytes","value":"AAEC/w== "}`, processing.FieldSchema{Kind: processing.BytesValue}, false},
		{`{"kind":"bytes","value":"__8="}`, processing.FieldSchema{Kind: processing.BytesValue}, false},
		{`{"kind":"null","value":null}`, processing.FieldSchema{Kind: processing.StringValue, Nullable: true}, true},
		{`{"kind":"null","value":null}`, processing.FieldSchema{Kind: processing.StringValue}, false},
		{`{"kind":"string","value":null}`, processing.FieldSchema{Kind: processing.StringValue, Nullable: true}, false},
		{`{"kind":"string","value":"a\u0000b"}`, processing.FieldSchema{Kind: processing.StringValue}, true},
		{`{"kind":"string","value":"a","other":true}`, processing.FieldSchema{Kind: processing.StringValue}, false},
		{`{"kind":"string","kind":"integer","value":"1"}`, processing.FieldSchema{Kind: processing.IntegerValue}, false},
		{`{"kind":"string"}`, processing.FieldSchema{Kind: processing.StringValue}, false},
		{`{"kind":"string","value":"a"} {}`, processing.FieldSchema{Kind: processing.StringValue}, false},
	} {
		t.Run(item.raw, func(t *testing.T) {
			var value processing.Value
			err := processing.Decode([]byte(item.raw), &value)
			if err == nil {
				_, err = value.Native(item.schema)
			}
			if (err == nil) != item.valid {
				t.Fatalf("typed validity differs: %v", err)
			}
		})
	}
	var binary processing.Value
	if err := processing.Decode([]byte(`{"kind":"bytes","value":"AAEC/w=="}`), &binary); err != nil {
		t.Fatal(err)
	}
	native, err := binary.Native(processing.FieldSchema{Kind: processing.BytesValue})
	if err != nil || !bytes.Equal(native.([]byte), []byte{0, 1, 2, 255}) {
		t.Fatalf("bytes value changed: %v, %v", native, err)
	}
	if _, err := (processing.Value{Kind: processing.NullValue, Value: json.RawMessage("null")}).
		Native(processing.FieldSchema{Kind: processing.BytesValue}); err == nil {
		t.Fatal("nullable bytes accepted null without declaration")
	}
	if _, err := (processing.Value{Kind: processing.NullValue, Value: json.RawMessage("null")}).
		Native(processing.FieldSchema{Kind: processing.BytesValue, Nullable: true}); err != nil {
		t.Fatalf("declared nullable bytes rejected null: %v", err)
	}
}

func TestValueCollectionsAndJSONBounds(t *testing.T) {
	integer := processing.FieldSchema{Kind: processing.IntegerValue}
	schema := processing.FieldSchema{Kind: processing.ObjectValue, Properties: map[string]processing.FieldSchema{
		"ids": {Kind: processing.ListValue, Element: &integer},
	}}
	for _, item := range []struct {
		raw   string
		valid bool
	}{
		{`{"kind":"object","value":{"ids":{"kind":"list","value":[{"kind":"integer","value":"7"}]}}}`, true},
		{`{"kind":"object","value":{"ids":{"kind":"list","value":[{"kind":"string","value":"7"}]}}}`, false},
		{`{"kind":"object","value":{}}`, false},
		{`{"kind":"object","value":{"foreign":{"kind":"integer","value":"7"}}}`, false},
	} {
		var value processing.Value
		err := processing.Decode([]byte(item.raw), &value)
		if err == nil {
			_, err = value.Native(schema)
		}
		if (err == nil) != item.valid {
			t.Fatalf("closed collection validity differs: %v", err)
		}
	}
	var raw json.RawMessage
	exact := []byte(`"` + strings.Repeat("a", processing.MaxInputBytes-2) + `"`)
	if err := processing.Decode(exact, &raw); err != nil {
		t.Fatal(err)
	}
	if err := processing.Decode(append(exact, ' '), &raw); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("one byte above boundary accepted: %v", err)
	}
	for depth := 32; depth <= 33; depth++ {
		data := []byte(strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth))
		err := processing.Decode(data, &raw)
		if (err == nil) != (depth == 32) {
			t.Fatalf("wrong nesting boundary at %d: %v", depth, err)
		}
	}
	if err := processing.Decode([]byte{'"', 0xff, '"'}, &raw); err == nil {
		t.Fatal("invalid UTF-8 was silently replaced")
	}
}
