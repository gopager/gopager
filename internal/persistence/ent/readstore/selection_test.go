package readstore_test

import (
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// testSchemas declares the Message attributes that the predicate tests address.
func testSchemas() []attributes.Schema {
	return []attributes.Schema{{Key: "incident", Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{
		"level":   {Kind: processing.IntegerValue},
		"name":    {Kind: processing.StringValue},
		"city":    {Kind: processing.StringValue},
		"ratio":   {Kind: processing.NumberValue},
		"urgent":  {Kind: processing.BooleanValue},
		"at":      {Kind: processing.TimestampValue},
		"precise": {Kind: processing.TimestampValue},
		"note":    {Kind: processing.StringValue, Nullable: true},
		"tags":    {Kind: processing.ListValue, Element: &attributes.Field{Kind: processing.StringValue}},
		"levels":  {Kind: processing.ListValue, Element: &attributes.Field{Kind: processing.IntegerValue}},
		"owner": {Kind: processing.ObjectValue, Properties: map[string]attributes.Field{
			"name": {Kind: processing.StringValue},
			"age":  {Kind: processing.IntegerValue, Optional: true},
		}},
	}}}
}

// testRegistry holds the built-in fields and the test attributes.
func testRegistry() *fields.Registry {
	return fields.New(fields.Builtin(), attributes.NewProvider(testSchemas(), 1))
}

// compile checks the predicate JSON against the test registry.
func compile(t testing.TB, predicate string) *fields.Filter {
	t.Helper()
	var decoded processing.Predicate
	if err := processing.Decode([]byte(predicate), &decoded); err != nil {
		t.Fatalf("predicate %s: %v", predicate, err)
	}
	filter, err := fields.CompileFilter(decoded, testRegistry().Declarations(fields.ScopeMessage, true))
	if err != nil {
		t.Fatalf("predicate %s: %v", predicate, err)
	}
	return filter
}

// ref renders a dotted field reference such as attributes.incident.level as JSON.
func ref(path string) string {
	segments := strings.Split(path, ".")
	return `{"namespace":"` + segments[0] + `","path":["` + strings.Join(segments[1:], `","`) + `"]}`
}

// value renders a typed value; raw is the JSON of its value.
func value(kind, raw string) string { return `{"kind":"` + kind + `","value":` + raw + `}` }

// compare renders a comparison leaf.
func compare(op, path, kind, raw string) string {
	return `{"op":"` + op + `","field":` + ref(path) + `,"value":` + value(kind, raw) + `}`
}

// member renders an `in` leaf.
func member(path, kind string, raws ...string) string {
	values := make([]string, len(raws))
	for i, raw := range raws {
		values[i] = value(kind, raw)
	}
	return `{"op":"in","field":` + ref(path) + `,"values":[` + strings.Join(values, ",") + `]}`
}

// present renders a presence leaf.
func present(path string, want bool) string {
	flag := "false"
	if want {
		flag = "true"
	}
	return `{"op":"present","field":` + ref(path) + `,"value":` + value("boolean", flag) + `}`
}

func logical(op string, args ...string) string {
	return `{"op":"` + op + `","args":[` + strings.Join(args, ",") + `]}`
}
