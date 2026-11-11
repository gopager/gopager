package fields

import "github.com/gopager/gopager/internal/processing"

var (
	workflowScope = []Scope{ScopeWorkflow}
	messageScope  = []Scope{ScopeMessage, ScopeDelivery}
	deliveryScope = []Scope{ScopeDelivery}
)

// builtinSpec is one row of the built-in catalog.
type builtinSpec struct {
	namespace string
	path      []string
	schema    processing.FieldSchema
	group     string
	label     string
	scopes    []Scope
}

func scalar(kind processing.ValueKind, optional bool) processing.FieldSchema {
	return processing.FieldSchema{Kind: kind, Optional: optional}
}

func listOf(kind processing.ValueKind) processing.FieldSchema {
	return processing.FieldSchema{Kind: processing.ListValue, Optional: true,
		Element: &processing.FieldSchema{Kind: kind}}
}

var (
	reqString, optString = scalar(processing.StringValue, false), scalar(processing.StringValue, true)
	optInt               = scalar(processing.IntegerValue, true)
	optTime              = scalar(processing.TimestampValue, true)
	optBool              = scalar(processing.BooleanValue, true)
)

// builtinSpecs is the closed catalog of server-owned fields. Message and Delivery fields are
// all optional because not every event or Message supplies them.
var builtinSpecs = []builtinSpec{
	{"source", []string{"protocol"}, reqString, "payload", "Protocol", workflowScope},
	{"source", []string{"form"}, reqString, "payload", "Form", workflowScope},
	{"source", []string{"subtype"}, optString, "payload", "Subtype", workflowScope},
	{"source", []string{"observedAt"}, scalar(processing.TimestampValue, false), "gateway", "Observed at", workflowScope},
	{"source", []string{"network", "id"}, reqString, "gateway", "Network ID", workflowScope},
	{"source", []string{"network", "name"}, reqString, "gateway", "Network name", workflowScope},
	{"source", []string{"payload", "text", "value"}, optString, "payload", "Text payload", workflowScope},
	{"source", []string{"payload", "numeric", "value"}, optString, "payload", "Numeric payload", workflowScope},
	{"source", []string{"payload", "binary"}, scalar(processing.BytesValue, true), "payload", "Binary payload", workflowScope},
	{"source", []string{"payload", "flex", "shortSignal", "sourceCode"}, optInt, "payload", "FLEX short-signal source code", workflowScope},
	{"source", []string{"payload", "zvei", "double_tone"},
		processing.FieldSchema{Kind: processing.StringValue, Optional: true, Nullable: true}, "payload", "ZVEI double tone", workflowScope},
	{"source", []string{"payload", "eas", "originator"}, optString, "payload", "EAS originator", workflowScope},
	{"source", []string{"payload", "eas", "eventCode"}, optString, "payload", "EAS event code", workflowScope},
	{"source", []string{"payload", "eas", "purgeTime"}, optString, "payload", "EAS purge time", workflowScope},
	{"source", []string{"payload", "eas", "issueTime"}, optString, "payload", "EAS issue time", workflowScope},
	{"target", []string{"address", "number"}, optInt, "address", "Address number", workflowScope},
	{"target", []string{"address", "text"}, optString, "address", "Address text", workflowScope},
	{"target", []string{"function"}, optInt, "address", "Function", workflowScope},
	{"target", []string{"unit", "id"}, optString, "unit", "Unit ID", workflowScope},
	{"target", []string{"unit", "collections"}, processing.FieldSchema{Kind: processing.ListValue, Optional: true,
		Element: &processing.FieldSchema{Kind: processing.StringValue}}, "unit", "Unit collections", workflowScope},

	{"message", []string{"id"}, optString, "", "Message ID", messageScope},
	{"message", []string{"time"}, optTime, "", "Message time", messageScope},
	{"message", []string{"processingStatus"}, optString, "", "Processing status", messageScope},
	{"message", []string{"correctionStatus"}, optString, "", "Correction status", messageScope},
	{"message", []string{"protocol"}, optString, "payload", "Protocol", messageScope},
	{"message", []string{"form"}, optString, "payload", "Form", messageScope},
	{"message", []string{"text"}, optString, "payload", "Text", messageScope},
	{"message", []string{"numeric"}, optString, "payload", "Numeric", messageScope},
	{"message", []string{"unitIds"}, listOf(processing.StringValue), "unit", "Unit IDs", messageScope},
	{"message", []string{"unitNames"}, listOf(processing.StringValue), "unit", "Unit names", messageScope},

	{"event", []string{"type"}, optString, "", "Event type", deliveryScope},
	{"event", []string{"occurredAt"}, optTime, "", "Occurred at", deliveryScope},
	{"event", []string{"lateJoin"}, optBool, "", "Late join", deliveryScope},
	{"event", []string{"reason"}, optString, "", "Reason", deliveryScope},
	{"unit", []string{"id"}, optString, "unit", "Unit ID", deliveryScope},
	{"unit", []string{"name"}, optString, "unit", "Unit name", deliveryScope},
	{"gateway", []string{"id"}, optString, "gateway", "Gateway ID", deliveryScope},
	{"gateway", []string{"name"}, optString, "gateway", "Gateway name", deliveryScope},
	{"gateway", []string{"oldState"}, optString, "gateway", "Old state", deliveryScope},
	{"gateway", []string{"newState"}, optString, "gateway", "New state", deliveryScope},
	{"gateway", []string{"contactTime"}, optTime, "gateway", "Contact time", deliveryScope},
}

// Builtin returns the provider of the server-owned fields: Workflow source and target fields,
// Message fields and Delivery event fields. Its revision is 0.
func Builtin() Provider {
	fields := make([]Field, len(builtinSpecs))
	for i, spec := range builtinSpecs {
		fields[i] = Field{
			Ref:        processing.FieldRef{Namespace: spec.namespace, Path: spec.path},
			Type:       spec.schema,
			Group:      spec.group,
			Filterable: FilterableType(spec.schema),
			Label:      spec.label,
			Scopes:     spec.scopes,
		}
	}
	return NewStaticProvider(0, fields)
}
