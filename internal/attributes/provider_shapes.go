package attributes

import (
	"slices"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// shape describes how the fields of a schema subject are exposed: the field reference namespace
// and the path segments before the schema key, the field group and the scopes.
type shape struct {
	namespace string
	prefix    []string
	group     string
	scopes    []fields.Scope
}

// messageShape exposes Message attributes as `attributes.<key>.<field>…`.
var messageShape = shape{namespace: attributeNamespace, group: "derived",
	scopes: []fields.Scope{fields.ScopeMessage, fields.ScopeDelivery}}

// unitShape exposes Unit attributes to Message and Delivery filters as `unit.attributes.<key>…`
// and to Workflows, for the Unit a target is assigned to, as `target.unit.attributes.<key>…`.
var unitShape = []shape{
	{namespace: "unit", prefix: []string{attributeNamespace}, group: "unit",
		scopes: []fields.Scope{fields.ScopeMessage, fields.ScopeDelivery}},
	{namespace: "target", prefix: []string{"unit", attributeNamespace}, group: "unit",
		scopes: []fields.Scope{fields.ScopeWorkflow}},
}

// ref returns the reference of the schema field at path.
func (s shape) ref(schemaKey string, path []string) processing.FieldRef {
	segments := append(slices.Clone(s.prefix), schemaKey)
	return processing.FieldRef{Namespace: s.namespace, Path: append(segments, path...)}
}
