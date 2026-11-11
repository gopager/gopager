package processing

import (
	"encoding/json"
	"testing"
)

func object(properties map[string]FieldSchema) FieldSchema {
	return FieldSchema{Kind: ObjectValue, Properties: properties}
}

// TestSchemaCompatible expects an assignment to be accepted exactly when every value of the
// source schema is a valid value of the target schema.
func TestSchemaCompatible(t *testing.T) {
	t.Parallel()
	text := FieldSchema{Kind: StringValue}
	optionalText := FieldSchema{Kind: StringValue, Optional: true}
	cases := map[string]struct {
		from, to FieldSchema
		want     bool
	}{
		"equal scalars":                    {text, text, true},
		"top-level optionality is ignored": {optionalText, text, true},
		"different kinds":                  {text, FieldSchema{Kind: IntegerValue}, false},
		"nullable into required":           {FieldSchema{Kind: StringValue, Nullable: true}, text, false},
		"required into nullable":           {text, FieldSchema{Kind: StringValue, Nullable: true}, true},
		"equal objects":                    {object(map[string]FieldSchema{"code": text}), object(map[string]FieldSchema{"code": text}), true},
		"target gained an optional property": {
			object(map[string]FieldSchema{"code": text}),
			object(map[string]FieldSchema{"code": text, "note": optionalText}), true},
		"target gained a required property": {
			object(map[string]FieldSchema{"code": text}),
			object(map[string]FieldSchema{"code": text, "note": text}), false},
		"source property unknown to the target": {
			object(map[string]FieldSchema{"code": text, "extra": text}),
			object(map[string]FieldSchema{"code": text}), false},
		"optional source property into required": {
			object(map[string]FieldSchema{"code": optionalText}),
			object(map[string]FieldSchema{"code": text}), false},
		"required source property into optional": {
			object(map[string]FieldSchema{"code": text}),
			object(map[string]FieldSchema{"code": optionalText}), true},
		"nested widening": {
			object(map[string]FieldSchema{"inner": object(map[string]FieldSchema{"a": text})}),
			object(map[string]FieldSchema{"inner": object(map[string]FieldSchema{"a": text, "b": optionalText})}), true},
		"list of widened objects": {
			FieldSchema{Kind: ListValue, Element: &FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{"a": text}}},
			FieldSchema{Kind: ListValue, Element: &FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{"a": text, "b": optionalText}}}, true},
		"list of different elements": {
			FieldSchema{Kind: ListValue, Element: &text},
			FieldSchema{Kind: ListValue, Element: &FieldSchema{Kind: IntegerValue}}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := schemaCompatible(c.from, c.to); got != c.want {
				t.Fatalf("schemaCompatible = %v, want %v", got, c.want)
			}
		})
	}
}

// TestWorkflowStillCompilesAfterTheOutputSchemaWidens expects a definition that assigns a step
// object to an Attribute field to compile and evaluate again after the schema gained an
// optional object property, which is a permitted schema change.
func TestWorkflowStillCompilesAfterTheOutputSchemaWidens(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t)
	code := FieldSchema{Kind: StringValue}
	owner := Value{Kind: ObjectValue, Value: json.RawMessage(`{"code":{"kind":"string","value":"A1"}}`)}
	definition := selectedDefinition()
	definition.Steps = []Step{{ID: "pick", Kind: "map", OutputSchema: map[string]FieldSchema{"owner": object(map[string]FieldSchema{"code": code})},
		Map: map[string]Assignment{"owner": {Literal: &owner}}}}
	ref := FieldRef{"step", []string{"pick", "owner"}}
	setOutput(&definition, "owner", Assignment{Field: &ref})

	for name, declared := range map[string]FieldSchema{
		"original": object(map[string]FieldSchema{"code": code}),
		"widened":  object(map[string]FieldSchema{"code": code, "note": {Kind: StringValue, Optional: true}}),
	} {
		program, err := CompileWorkflow(t.Context(), definition, source, outputFixture{"owner": declared}, nil)
		if err != nil {
			t.Fatalf("%s schema: compile: %v", name, err)
		}
		if _, err := program.Evaluate(t.Context(), source); err != nil {
			t.Fatalf("%s schema: evaluate: %v", name, err)
		}
	}
}
