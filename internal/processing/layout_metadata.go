package processing

import (
	"cmp"
	"slices"
)

// blockShape is the single description of a block kind: the properties it allows and the ones it
// requires. Validation and the editor metadata both read it, so they cannot drift apart.
type blockShape struct {
	kind     string
	allowed  property
	required property
}

var blockShapes = []blockShape{
	{blockText, propText, propText},
	{blockField, propField | propLabel | propIcon, propField},
	{blockChip, propField | propLabel | propIcon, propField},
	{blockLink, propLabel | propURL, propLabel | propURL},
	{blockSection, propTitle | propBlocks, propBlocks},
	{blockConditional, propWhen | propThen | propElse, propWhen | propThen},
	{blockFallback, propBlocks, propBlocks},
}

// propertyNames maps each property bit to its JSON name.
var propertyNames = []struct {
	bit  property
	name string
}{
	{propText, "text"}, {propField, "field"}, {propLabel, "label"}, {propTitle, "title"},
	{propURL, "url"}, {propWhen, "when"}, {propBlocks, "blocks"}, {propThen, "then"},
	{propElse, "else"}, {propIcon, "icon"},
}

// allowedProperties returns the properties a block kind may carry, and false for an unknown kind.
func allowedProperties(kind string) (property, bool) {
	for _, shape := range blockShapes {
		if shape.kind == kind {
			return shape.allowed, true
		}
	}
	return 0, false
}

// LayoutBlockSpec describes one block kind for editors.
type LayoutBlockSpec struct {
	// Kind is the block kind.
	Kind string `json:"kind"`
	// Properties are the JSON names of the properties the kind allows, sorted.
	Properties []string `json:"properties"`
	// Required are the properties that must be present, a subset of Properties, sorted.
	Required []string `json:"required"`
}

// LayoutBlockSpecs returns every block kind with its allowed and required properties, sorted by
// kind, so that editors offer exactly what validation accepts.
func LayoutBlockSpecs() []LayoutBlockSpec {
	specs := make([]LayoutBlockSpec, 0, len(blockShapes))
	for _, shape := range blockShapes {
		specs = append(specs, LayoutBlockSpec{Kind: shape.kind,
			Properties: propertyList(shape.allowed), Required: propertyList(shape.required)})
	}
	slices.SortFunc(specs, func(a, b LayoutBlockSpec) int { return cmp.Compare(a.Kind, b.Kind) })
	return specs
}

// propertyList returns the sorted JSON names of the properties in set; never nil.
func propertyList(set property) []string {
	names := []string{}
	for _, item := range propertyNames {
		if set&item.bit != 0 {
			names = append(names, item.name)
		}
	}
	slices.Sort(names)
	return names
}

// LayoutBounds are the structural limits a layout definition and its rendering obey.
type LayoutBounds struct {
	// Blocks bounds the blocks of one layout, nested ones included.
	Blocks int `json:"blocks"`
	// Depth bounds block nesting.
	Depth int `json:"depth"`
	// LiteralBytes bounds a text literal.
	LiteralBytes int `json:"literalBytes"`
	// LabelBytes bounds a label or title.
	LabelBytes int `json:"labelBytes"`
	// TemplateBytes bounds a link URL template.
	TemplateBytes int `json:"templateBytes"`
	// URLFields bounds the fields of one link URL.
	URLFields int `json:"urlFields"`
	// FallbackMin and FallbackMax bound the alternatives of a fallback block.
	FallbackMin int `json:"fallbackMin"`
	FallbackMax int `json:"fallbackMax"`
	// OutputBytes bounds one rendered body.
	OutputBytes int `json:"outputBytes"`
}

// LayoutLimits returns the limits that definition validation and rendering enforce.
func LayoutLimits() LayoutBounds {
	return LayoutBounds{Blocks: maxLayoutBlocks, Depth: maxLayoutDepth, LiteralBytes: maxLayoutLiteralBytes,
		LabelBytes: maxLayoutLabelBytes, TemplateBytes: maxLayoutTemplate, URLFields: maxLayoutURLFields,
		FallbackMin: 2, FallbackMax: maxFallbackAlts, OutputBytes: MaxLayoutOutputBytes}
}

// LayoutOutputKinds returns the output kinds a layout can render to.
func LayoutOutputKinds() []string {
	return []string{LayoutText, LayoutMarkdown, LayoutJSON}
}
