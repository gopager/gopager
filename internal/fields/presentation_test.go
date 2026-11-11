package fields

import (
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

func TestDeclarationsExposeLabelAndIconOfDeclaredFields(t *testing.T) {
	t.Parallel()
	level := processing.FieldRef{Namespace: "attributes", Path: []string{"alerts", "level"}}
	old := processing.FieldRef{Namespace: "attributes", Path: []string{"alerts", "old"}}
	registry := New(NewStaticProvider(1, []Field{
		{Ref: level, Type: processing.FieldSchema{Kind: processing.StringValue}, Label: "Level", Icon: "bell", Scopes: []Scope{ScopeDelivery}},
		{Ref: old, Type: processing.FieldSchema{Kind: processing.StringValue}, Label: "Old", Deprecated: true, Scopes: []Scope{ScopeDelivery}},
	}))
	lenient, strict := registry.Declarations(ScopeDelivery, false), registry.Declarations(ScopeDelivery, true)
	if label, icon, ok := lenient.FieldPresentation(level); !ok || label != "Level" || icon != "bell" {
		t.Errorf("presentation = %q, %q, %v", label, icon, ok)
	}
	if _, _, ok := lenient.FieldPresentation(processing.FieldRef{Namespace: "attributes", Path: []string{"alerts", "none"}}); ok {
		t.Error("undeclared field has a presentation")
	}
	if _, _, ok := lenient.FieldPresentation(old); !ok {
		t.Error("deprecated field has no presentation outside strict mode")
	}
	if _, _, ok := strict.FieldPresentation(old); ok {
		t.Error("deprecated field has a presentation in strict mode")
	}
}
