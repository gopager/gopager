package processing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

// layoutFixture is an in-memory Declarations and FieldReader over a map keyed by FieldRef.
type layoutFixture struct {
	schemas map[string]FieldSchema
	values  map[string]FieldValue
}

func (f *layoutFixture) FieldSchema(ref FieldRef) (FieldSchema, error) {
	schema, ok := f.schemas[ref.key()]
	if !ok {
		return FieldSchema{}, errors.New("undeclared layout fixture field")
	}
	return schema, nil
}

func (f *layoutFixture) LookupField(_ context.Context, ref FieldRef) (FieldValue, error) {
	value, ok := f.values[ref.key()]
	if !ok {
		return FieldValue{State: FieldAbsent}, nil
	}
	return value, nil
}

func lref(name string) FieldRef { return FieldRef{Namespace: "source", Path: []string{name}} }

func lfixture(t *testing.T, fields map[string]any) *layoutFixture {
	t.Helper()
	fixture := &layoutFixture{schemas: map[string]FieldSchema{}, values: map[string]FieldValue{}}
	for name, raw := range fields {
		ref := lref(name)
		switch typed := raw.(type) {
		case string:
			value, _ := Literal(StringValue, typed)
			fixture.schemas[ref.key()] = FieldSchema{Kind: StringValue, Optional: true, Nullable: true}
			fixture.values[ref.key()] = FieldValue{State: FieldPresent, Value: &value}
		case Value:
			schema := FieldSchema{Kind: typed.Kind, Optional: true, Nullable: true}
			if typed.Kind == ListValue {
				schema.Element = &FieldSchema{Kind: StringValue}
			}
			if typed.Kind == ObjectValue {
				schema.Properties = map[string]FieldSchema{"a": {Kind: StringValue}}
			}
			if typed.Kind == NullValue {
				schema.Kind = StringValue
			}
			fixture.schemas[ref.key()] = schema
			fixture.values[ref.key()] = FieldValue{State: FieldPresent, Value: &typed}
		case nil:
			fixture.schemas[ref.key()] = FieldSchema{Kind: StringValue, Optional: true}
			fixture.values[ref.key()] = FieldValue{State: FieldAbsent}
		}
	}
	return fixture
}

func mustLayout(t *testing.T, document string) Layout {
	t.Helper()
	layout, err := DecodeLayout([]byte(document))
	if err != nil {
		t.Fatalf("DecodeLayout(%s): %v", document, err)
	}
	return layout
}

func TestLayoutDecodeStrict(t *testing.T) {
	t.Parallel()
	valid := `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"text","text":"hi"}]}`
	if _, err := DecodeLayout([]byte(valid)); err != nil {
		t.Fatalf("valid layout rejected: %v", err)
	}
	for _, item := range []struct{ name, document string }{
		{"unknown property", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"text","text":"x","html":"y"}]}`},
		{"unknown top property", `{"schemaVersion":1,"outputKind":"text","extra":1,"blocks":[{"kind":"text","text":"x"}]}`},
		{"unknown kind", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"html","text":"x"}]}`},
		{"wrong version", `{"schemaVersion":2,"outputKind":"text","blocks":[{"kind":"text","text":"x"}]}`},
		{"bad output kind", `{"schemaVersion":1,"outputKind":"html","blocks":[{"kind":"text","text":"x"}]}`},
		{"no blocks", `{"schemaVersion":1,"outputKind":"text","blocks":[]}`},
		{"foreign property for kind", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"text","text":"x","label":"l"}]}`},
		{"empty text", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"text","text":""}]}`},
		{"link without label", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"link","url":{"template":"https://x","fields":[]}}]}`},
		{"fallback with one", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"fallback","blocks":[{"kind":"text","text":"x"}]}]}`},
		{"conditional without then", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"conditional","when":{"op":"and","args":[]}}]}`},
		{"field without field", `{"schemaVersion":1,"outputKind":"text","blocks":[{"kind":"field"}]}`},
		{"not json", `{`},
	} {
		_, err := DecodeLayout([]byte(item.document))
		if platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: code = %q (%v), want INVALID_INPUT", item.name, platform.FailureCode(err), err)
		}
	}
}

func layoutOfBlocks(blocks []LayoutBlock) Layout {
	return Layout{SchemaVersion: LayoutSchemaVersion, OutputKind: LayoutText, Blocks: blocks}
}

func nestedSections(depth int) []LayoutBlock {
	block := LayoutBlock{Kind: "text", Text: "leaf"}
	for i := 1; i < depth; i++ {
		block = LayoutBlock{Kind: "section", Blocks: []LayoutBlock{block}}
	}
	return []LayoutBlock{block}
}

func TestLayoutLimits(t *testing.T) {
	t.Parallel()
	many := make([]LayoutBlock, 257)
	for i := range many {
		many[i] = LayoutBlock{Kind: "text", Text: "x"}
	}
	fixture := lfixture(t, map[string]any{"a": "v", "b": "w"})
	field := lref("a")
	other := lref("b")
	url := func(template string, fields ...FieldRef) []LayoutBlock {
		return []LayoutBlock{{Kind: "link", Label: "l", URL: &LayoutURL{Template: template, Fields: fields}}}
	}
	for _, item := range []struct {
		name   string
		blocks []LayoutBlock
		code   platform.ErrorCode
	}{
		{"257 blocks", many, platform.CodeResourceLimit},
		{"256 blocks ok", many[:256], ""},
		{"depth 8 ok", nestedSections(8), ""},
		{"depth 9", nestedSections(9), platform.CodeResourceLimit},
		{"literal too long", []LayoutBlock{{Kind: "text", Text: strings.Repeat("a", 4097)}}, platform.CodeResourceLimit},
		{"literal max", []LayoutBlock{{Kind: "text", Text: strings.Repeat("a", 4096)}}, ""},
		{"label too long", []LayoutBlock{{Kind: "field", Field: &field, Label: strings.Repeat("a", 257)}}, platform.CodeResourceLimit},
		{"template too long", url("https://x/" + strings.Repeat("a", 2048)), platform.CodeResourceLimit},
		{"placeholder without field", url("https://x/{0}"), platform.CodeInvalidInput},
		{"placeholder out of range", url("https://x/{1}", field), platform.CodeInvalidInput},
		{"unused field", url("https://x/{0}", field, other), platform.CodeInvalidInput},
		{"stray brace", url("https://x/{a}"), platform.CodeInvalidInput},
		{"unknown field", []LayoutBlock{{Kind: "field", Field: &FieldRef{Namespace: "source", Path: []string{"missing"}}}}, platform.CodeInvalidInput},
		{"bad namespace", []LayoutBlock{{Kind: "field", Field: &FieldRef{Namespace: "x", Path: []string{"a"}}}}, platform.CodeInvalidInput},
		{"ok link", url("https://x/{0}/{1}", field, other), ""},
	} {
		_, err := CompileLayout(context.Background(), layoutOfBlocks(item.blocks), fixture)
		if got := platform.FailureCode(err); got != item.code {
			t.Errorf("%s: code = %q (%v), want %q", item.name, got, err, item.code)
		}
	}
}

func TestLayoutFieldsDeduplicatedAndComplete(t *testing.T) {
	t.Parallel()
	fixture := lfixture(t, map[string]any{"a": "v", "b": "w", "c": "x"})
	layout := mustLayout(t, `{"schemaVersion":1,"outputKind":"text","blocks":[
		{"kind":"field","field":{"namespace":"source","path":["a"]}},
		{"kind":"link","label":"l","url":{"template":"https://x/{0}","fields":[{"namespace":"source","path":["b"]}]}},
		{"kind":"conditional","when":{"op":"eq","field":{"namespace":"source","path":["c"]},"value":{"kind":"string","value":"x"}},
			"then":[{"kind":"chip","field":{"namespace":"source","path":["a"]}}]}]}`)
	program, err := CompileLayout(context.Background(), layout, fixture)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, ref := range program.Fields() {
		names = append(names, ref.Path[0])
	}
	if got := strings.Join(names, ","); got != "a,b,c" {
		t.Fatalf("Fields() = %s, want a,b,c", got)
	}
	if program.OutputKind() != LayoutText {
		t.Fatalf("OutputKind() = %q", program.OutputKind())
	}
}

func TestLayoutRoundTripJSON(t *testing.T) {
	t.Parallel()
	layout := mustLayout(t, `{"schemaVersion":1,"outputKind":"json","blocks":[{"kind":"text","text":"x"}]}`)
	encoded, err := json.Marshal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeLayout(encoded); err != nil {
		t.Fatalf("round trip: %v", err)
	}
}
