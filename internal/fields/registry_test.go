package fields

import (
	"testing"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestFilterableType(t *testing.T) {
	t.Parallel()
	element := func(kind processing.ValueKind) *processing.FieldSchema { return &processing.FieldSchema{Kind: kind} }
	tests := []struct {
		name   string
		schema processing.FieldSchema
		want   bool
	}{
		{"string", processing.FieldSchema{Kind: processing.StringValue}, true},
		{"boolean", processing.FieldSchema{Kind: processing.BooleanValue}, true},
		{"integer", processing.FieldSchema{Kind: processing.IntegerValue}, true},
		{"number", processing.FieldSchema{Kind: processing.NumberValue}, true},
		{"timestamp", processing.FieldSchema{Kind: processing.TimestampValue}, true},
		{"bytes", processing.FieldSchema{Kind: processing.BytesValue}, false},
		{"object", processing.FieldSchema{Kind: processing.ObjectValue}, false},
		{"list of string", processing.FieldSchema{Kind: processing.ListValue, Element: element(processing.StringValue)}, true},
		{"list of bytes", processing.FieldSchema{Kind: processing.ListValue, Element: element(processing.BytesValue)}, false},
		{"list of list", processing.FieldSchema{Kind: processing.ListValue, Element: &processing.FieldSchema{
			Kind: processing.ListValue, Element: element(processing.StringValue)}}, false},
		{"list without element", processing.FieldSchema{Kind: processing.ListValue}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := FilterableType(test.schema); got != test.want {
				t.Fatalf("FilterableType = %v, want %v", got, test.want)
			}
		})
	}
}

func TestRegistryLookup(t *testing.T) {
	t.Parallel()
	registry := New(testProvider(), Builtin())
	tests := []struct {
		name    string
		scope   Scope
		ref     processing.FieldRef
		strict  bool
		wantErr bool
	}{
		{"known", ScopeMessage, ref("t", "s"), true, false},
		{"builtin", ScopeMessage, ref("message", "id"), true, false},
		{"unknown", ScopeMessage, ref("t", "nope"), false, true},
		{"out of scope", ScopeMessage, ref("t", "other"), false, true},
		{"deprecated strict", ScopeMessage, ref("t", "old"), true, true},
		{"deprecated lenient", ScopeMessage, ref("t", "old"), false, false},
		{"empty namespace", ScopeMessage, processing.FieldRef{Path: []string{"s"}}, false, true},
		{"nul in path", ScopeMessage, ref("t", "a\x00b"), false, true},
		{"too deep", ScopeMessage, processing.FieldRef{Namespace: "t", Path: make([]string, processing.MaxDepth+1)}, false, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := registry.Lookup(test.scope, test.ref, test.strict)
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, test.wantErr)
			}
			if err != nil && platform.FailureCode(err) != platform.CodeInvalidInput {
				t.Fatalf("code = %v", platform.FailureCode(err))
			}
		})
	}
}

func TestRegistryFirstProviderWins(t *testing.T) {
	t.Parallel()
	mk := func(label string) Provider {
		return NewStaticProvider(0, []Field{{Ref: ref("x", "a"), Label: label, Scopes: []Scope{ScopeMessage}}})
	}
	registry := New(mk("first"), mk("second"))
	field, err := registry.Lookup(ScopeMessage, ref("x", "a"), false)
	if err != nil || field.Label != "first" {
		t.Fatalf("got %+v, %v", field, err)
	}
	if list := registry.List(ScopeMessage); len(list) != 1 || list[0].Label != "first" {
		t.Fatalf("List = %+v", list)
	}
}

func TestRegistryListAndRevision(t *testing.T) {
	t.Parallel()
	registry := New(testProvider(), Builtin())
	list := registry.List(ScopeMessage)
	for i, field := range list {
		if field.Deprecated || !field.In(ScopeMessage) {
			t.Fatalf("unexpected %s", field.Path())
		}
		if i > 0 && list[i-1].Path() >= field.Path() {
			t.Fatalf("not sorted at %s", field.Path())
		}
	}
	if registry.Revision() != "3.0" || New().Revision() != "" {
		t.Fatalf("revision = %q", registry.Revision())
	}
}

func TestDeclarations(t *testing.T) {
	t.Parallel()
	declarations := New(testProvider()).Declarations(ScopeMessage, true)
	schema, err := declarations.FieldSchema(ref("t", "i"))
	if err != nil || schema.Kind != processing.IntegerValue {
		t.Fatalf("schema = %+v, %v", schema, err)
	}
	groups, err := declarations.FieldGroups(ref("t", "ls"))
	if err != nil || len(groups) != 1 || groups[0] != "unit" {
		t.Fatalf("groups = %v, %v", groups, err)
	}
	groups, err = declarations.FieldGroups(ref("t", "n"))
	if err != nil || groups == nil || len(groups) != 0 {
		t.Fatalf("groups = %#v, %v", groups, err)
	}
	if _, err := declarations.FieldGroups(ref("t", "old")); err == nil {
		t.Fatal("strict declarations must reject a deprecated field")
	}
	field, err := declarations.Field(ref("t", "s"))
	if err != nil || field.Label != "s" {
		t.Fatalf("field = %+v, %v", field, err)
	}
}
