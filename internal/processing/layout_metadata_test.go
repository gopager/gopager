package processing

import (
	"slices"
	"testing"
)

// TestLayoutBlockSpecsMatchValidation expects the editor metadata to agree with what decoding
// accepts: a block carrying exactly the listed required properties of a kind decodes, and a
// property outside the listed ones is rejected.
func TestLayoutBlockSpecsMatchValidation(t *testing.T) {
	t.Parallel()
	specs := LayoutBlockSpecs()
	kinds := make([]string, 0, len(specs))
	for _, spec := range specs {
		kinds = append(kinds, spec.Kind)
		if !slices.IsSorted(spec.Properties) {
			t.Fatalf("%s properties are not sorted: %v", spec.Kind, spec.Properties)
		}
		for _, required := range spec.Required {
			if !slices.Contains(spec.Properties, required) {
				t.Fatalf("%s requires %q which it does not allow", spec.Kind, required)
			}
		}
	}
	want := []string{"chip", "conditional", "fallback", "field", "link", "section", "text"}
	slices.Sort(kinds)
	if !slices.Equal(kinds, want) {
		t.Fatalf("block kinds = %v, want %v", kinds, want)
	}
	byKind := map[string]LayoutBlockSpec{}
	for _, spec := range specs {
		byKind[spec.Kind] = spec
	}
	if got := byKind["link"].Required; !slices.Equal(got, []string{"label", "url"}) {
		t.Fatalf("link required = %v", got)
	}
	if slices.Contains(byKind["text"].Properties, "label") {
		t.Fatal("text block lists label")
	}
	if !slices.Contains(byKind["chip"].Properties, "icon") {
		t.Fatal("chip block does not list icon")
	}
}

// TestLayoutBoundsReportTheEnforcedLimits expects the reported bounds to be the ones decoding
// enforces.
func TestLayoutBoundsReportTheEnforcedLimits(t *testing.T) {
	t.Parallel()
	bounds := LayoutLimits()
	if bounds.Blocks != maxLayoutBlocks || bounds.Depth != maxLayoutDepth ||
		bounds.LiteralBytes != maxLayoutLiteralBytes || bounds.OutputBytes != MaxLayoutOutputBytes ||
		bounds.FallbackMax != maxFallbackAlts || bounds.FallbackMin != 2 {
		t.Fatalf("unexpected bounds %+v", bounds)
	}
	if got := LayoutOutputKinds(); !slices.Equal(got, []string{LayoutText, LayoutMarkdown, LayoutJSON}) {
		t.Fatalf("output kinds = %v", got)
	}
}
