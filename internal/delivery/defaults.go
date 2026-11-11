package delivery

import "github.com/gopager/gopager/internal/processing"

// field returns a reference to a static catalog field.
func field(namespace, name string) *processing.FieldRef {
	return &processing.FieldRef{Namespace: namespace, Path: []string{name}}
}

// defaultBlocks are the blocks of every built-in layout: the event, the Message text or another
// Payload form, its Units, the Gateway state for Gateway facts and the Message time. A field the
// subscriber may not read is absent and renders nothing.
func defaultBlocks() []processing.LayoutBlock {
	return []processing.LayoutBlock{
		{Kind: "field", Field: field("event", "type"), Label: "Event"},
		{Kind: "fallback", Blocks: []processing.LayoutBlock{
			{Kind: "field", Field: field("message", "text")},
			{Kind: "field", Field: field("message", "numeric")},
			{Kind: "field", Field: field("message", "form"), Label: "Form"},
		}},
		{Kind: "field", Field: field("message", "unitNames"), Label: "Units"},
		{Kind: "field", Field: field("gateway", "name"), Label: "Gateway"},
		{Kind: "field", Field: field("gateway", "newState"), Label: "State"},
		{Kind: "field", Field: field("message", "time"), Label: "Time"},
	}
}

// DefaultLayouts returns the built-in layout per output kind, used when a Subscription names no
// layout.
func DefaultLayouts() map[string]processing.Layout {
	layouts := map[string]processing.Layout{}
	for _, kind := range []string{processing.LayoutText, processing.LayoutMarkdown, processing.LayoutJSON} {
		layouts[kind] = processing.Layout{SchemaVersion: processing.LayoutSchemaVersion, OutputKind: kind, Blocks: defaultBlocks()}
	}
	return layouts
}
