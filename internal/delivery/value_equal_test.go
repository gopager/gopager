package delivery

import (
	"encoding/json"
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

// TestSameValueComparesMeaningNotEncoding expects equivalent encodings of one value to agree, so
// that Units with the same input value never cause a parameter conflict.
func TestSameValueComparesMeaningNotEncoding(t *testing.T) {
	t.Parallel()
	value := func(kind processing.ValueKind, raw string) processing.Value {
		return processing.Value{Kind: kind, Value: json.RawMessage(raw)}
	}
	for name, test := range map[string]struct {
		left, right processing.Value
		same        bool
	}{
		"integer leading zero": {value(processing.IntegerValue, `"01"`), value(processing.IntegerValue, `"1"`), true},
		"integer differs":      {value(processing.IntegerValue, `"2"`), value(processing.IntegerValue, `"1"`), false},
		"number exponent":      {value(processing.NumberValue, `1e2`), value(processing.NumberValue, `100`), true},
		"same instant":         {value(processing.TimestampValue, `"2026-11-12T10:00:00+01:00"`), value(processing.TimestampValue, `"2026-11-12T09:00:00Z"`), true},
		"other instant":        {value(processing.TimestampValue, `"2026-11-12T10:00:00Z"`), value(processing.TimestampValue, `"2026-11-12T09:00:00Z"`), false},
		"string spacing":       {value(processing.StringValue, ` "a" `), value(processing.StringValue, `"a"`), true},
		"kinds differ":         {value(processing.StringValue, `"1"`), value(processing.IntegerValue, `"1"`), false},
		"list of integers": {value(processing.ListValue, `[{"kind":"integer","value":"07"}]`),
			value(processing.ListValue, `[{"value":"7","kind":"integer"}]`), true},
		"object key order": {value(processing.ObjectValue, `{"a":{"kind":"boolean","value":true},"b":{"kind":"string","value":"x"}}`),
			value(processing.ObjectValue, `{"b":{"kind":"string","value":"x"},"a":{"kind":"boolean","value":true}}`), true},
		"object missing property": {value(processing.ObjectValue, `{"a":{"kind":"boolean","value":true}}`),
			value(processing.ObjectValue, `{}`), false},
		"invalid integer": {value(processing.IntegerValue, `"x"`), value(processing.IntegerValue, `"x"`), false},
	} {
		if got := sameValue(test.left, test.right); got != test.same {
			t.Errorf("%s: sameValue = %v, want %v", name, got, test.same)
		}
	}
}
