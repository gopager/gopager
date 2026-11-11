package processing_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// TestBuiltinCatalogDeclaresOneGroupPerField keeps save-time classification total: every
// catalog field a definition can reference declares exactly one field group, and the
// compiled program reports each referenced field with it.
func TestBuiltinCatalogDeclaresOneGroupPerField(t *testing.T) {
	t.Parallel()
	registry := fields.New(fields.Builtin())
	declarations := registry.Declarations(fields.ScopeWorkflow, false)
	want := map[string]string{"protocol": "payload", "form": "payload", "subtype": "payload",
		"observedAt": "gateway", "network.id": "gateway", "network.name": "gateway", "payload.text.value": "payload"}
	for _, field := range registry.List(fields.ScopeWorkflow) {
		groups, err := declarations.FieldGroups(field.Ref)
		if err != nil || len(groups) != 1 {
			t.Fatalf("%s groups = %v, %v", field.Ref, groups, err)
		}
		if field.Ref.Namespace != "source" {
			continue
		}
		if expected, ok := want[strings.Join(field.Ref.Path, ".")]; ok && groups[0] != expected {
			t.Fatalf("%s group = %s, want %s", field.Ref, groups[0], expected)
		}
	}
	if _, err := declarations.FieldGroups(processing.FieldRef{Namespace: "source", Path: []string{"undeclared"}}); err == nil {
		t.Fatal("undeclared field reported a group")
	}
	protocol := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	name := processing.FieldRef{Namespace: "source", Path: []string{"network", "name"}}
	value := processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"pocsag"`)}
	schemas := []attributes.Schema{{Key: "meta", Subject: attributes.SubjectMessage,
		Fields: map[string]attributes.Field{"network": {Kind: processing.StringValue}}}}
	outputs := fields.New(fields.Builtin(), attributes.NewProvider(schemas, 1)).Declarations(fields.ScopeMessage, false)
	program, err := processing.CompileWorkflow(t.Context(), processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocol, Value: &value},
		Outputs:  map[string]map[string]processing.Assignment{"meta": {"network": {Field: &name}}}},
		declarations, outputs, nil)
	if err != nil {
		t.Fatal(err)
	}
	references := program.SourceReferences()
	if len(references) != 2 || references[0].Field.String() != "source.network.name" ||
		!slices.Equal(references[0].Groups, []string{"gateway"}) || references[1].Field.String() != "source.protocol" ||
		!slices.Equal(references[1].Groups, []string{"payload"}) {
		t.Fatalf("selector and output references were not classified: %+v", references)
	}
}
