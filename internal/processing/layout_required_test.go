package processing

import (
	"encoding/json"
	"testing"
)

// TestRequiredBlockPropertiesMatchValidation keeps the required properties the editor metadata
// advertises in agreement with validation in both directions: a block with exactly those
// properties is valid, and dropping any single one of them is rejected.
func TestRequiredBlockPropertiesMatchValidation(t *testing.T) {
	t.Parallel()
	text := map[string]any{"kind": "text", "text": "x"}
	complete := map[string]map[string]any{
		"text":    {"text": "x"},
		"field":   {"field": map[string]any{"namespace": "message", "path": []string{"text"}}},
		"chip":    {"field": map[string]any{"namespace": "message", "path": []string{"text"}}},
		"link":    {"label": "l", "url": map[string]any{"template": "https://x", "fields": []any{}}},
		"section": {"blocks": []any{text}},
		"conditional": {"when": map[string]any{"op": "eq", "field": map[string]any{"namespace": "message", "path": []string{"text"}},
			"value": map[string]any{"kind": "string", "value": "x"}}, "then": []any{text}},
		"fallback": {"blocks": []any{text, text}},
	}
	decode := func(block map[string]any) error {
		raw, err := json.Marshal(map[string]any{"schemaVersion": 1, "outputKind": "text", "blocks": []any{block}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = DecodeLayout(raw)
		return err
	}
	for _, spec := range LayoutBlockSpecs() {
		properties, ok := complete[spec.Kind]
		if !ok {
			t.Fatalf("no complete example for block kind %q", spec.Kind)
		}
		// Only the advertised required properties: validation must accept exactly these.
		block := map[string]any{"kind": spec.Kind}
		for _, name := range spec.Required {
			value, ok := properties[name]
			if !ok {
				t.Fatalf("no example value for %s.%s", spec.Kind, name)
			}
			block[name] = value
		}
		if err := decode(block); err != nil {
			t.Fatalf("%s with only its advertised required properties: %v", spec.Kind, err)
		}
		for _, required := range spec.Required {
			missing := map[string]any{}
			for name, value := range block {
				if name != required {
					missing[name] = value
				}
			}
			if err := decode(missing); err == nil {
				t.Fatalf("%s without %q was accepted although the metadata requires it", spec.Kind, required)
			}
		}
	}
}
