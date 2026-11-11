package processing

import (
	"context"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

// presentedFixture adds registry-style display metadata to the layout fixture.
type presentedFixture struct {
	*layoutFixture
	presentation map[string][2]string
}

func (f presentedFixture) FieldPresentation(ref FieldRef) (string, string, bool) {
	value, ok := f.presentation[ref.Path[0]]
	return value[0], value[1], ok
}

func presentedLayout(t *testing.T, kind, blocks string) string {
	t.Helper()
	fixture := presentedFixture{layoutFixture: lfixture(t, map[string]any{"a": "alpha", "b": "beta"}),
		presentation: map[string][2]string{"a": {"Registry A", "registry-a"}, "b": {"Registry B", ""}}}
	layout := mustLayout(t, `{"schemaVersion":1,"outputKind":"`+kind+`","blocks":`+blocks+`}`)
	program, err := CompileLayout(context.Background(), layout, fixture)
	if err != nil {
		t.Fatalf("CompileLayout: %v", err)
	}
	rendered, err := program.Render(context.Background(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	return string(rendered.Body)
}

func TestLayoutBlocksTakeLabelAndIconFromTheDeclarationsUnlessOverridden(t *testing.T) {
	t.Parallel()
	blocks := `[{"kind":"field","field":` + fa + `},
		{"kind":"chip","field":` + fb + `},
		{"kind":"field","field":` + fa + `,"label":"Own","icon":"own.icon"},
		{"kind":"chip","field":` + fb + `,"icon":"chip-icon"}]`
	got := presentedLayout(t, "json", blocks)
	want := `[{"kind":"field","label":"Registry A","icon":"registry-a","value":{"kind":"string","value":"alpha"}},` +
		`{"kind":"chip","label":"Registry B","value":{"kind":"string","value":"beta"}},` +
		`{"kind":"field","label":"Own","icon":"own.icon","value":{"kind":"string","value":"alpha"}},` +
		`{"kind":"chip","label":"Registry B","icon":"chip-icon","value":{"kind":"string","value":"beta"}}]`
	if got != want {
		t.Errorf("json:\n got %s\nwant %s", got, want)
	}
}

func TestLayoutTextAndMarkdownIgnoreIcons(t *testing.T) {
	t.Parallel()
	blocks := `[{"kind":"field","field":` + fa + `},{"kind":"chip","field":` + fb + `,"icon":"chip-icon"}]`
	for kind, want := range map[string]string{
		"text":     "Registry A: alpha\n[Registry B: beta]",
		"markdown": "**Registry A:** alpha\n\\[Registry B: beta\\]",
	} {
		if got := presentedLayout(t, kind, blocks); got != want {
			t.Errorf("%s:\n got %q\nwant %q", kind, got, want)
		}
	}
}

func TestLayoutWithoutPresentationDeclarationsKeepsBlocksAsWritten(t *testing.T) {
	t.Parallel()
	got, err := renderLayout(t, "json", `[{"kind":"field","field":`+fa+`},{"kind":"field","field":`+fb+`,"icon":"x"}]`,
		lfixture(t, map[string]any{"a": "alpha", "b": "beta"}))
	want := `[{"kind":"field","value":{"kind":"string","value":"alpha"}},{"kind":"field","icon":"x","value":{"kind":"string","value":"beta"}}]`
	if err != nil || got != want {
		t.Errorf("got %s, %v\nwant %s", got, err, want)
	}
}

func TestLayoutIconIsValidatedAndAllowedOnFieldAndChipOnly(t *testing.T) {
	t.Parallel()
	for name, document := range map[string]string{
		"invalid icon":      `{"kind":"field","field":` + fa + `,"icon":"not valid!"}`,
		"icon on text":      `{"kind":"text","text":"x","icon":"x"}`,
		"icon on link":      `{"kind":"link","label":"L","icon":"x","url":{"template":"https://example.com/{0}","fields":[` + fa + `]}}`,
		"icon on a section": `{"kind":"section","icon":"x","blocks":[{"kind":"text","text":"x"}]}`,
	} {
		_, err := DecodeLayout([]byte(`{"schemaVersion":1,"outputKind":"json","blocks":[` + document + `]}`))
		if platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s: %v, want INVALID_INPUT", name, err)
		}
	}
	if _, err := DecodeLayout([]byte(`{"schemaVersion":1,"outputKind":"json","blocks":[{"kind":"chip","field":` + fa + `,"icon":"a.b_c-1"}]}`)); err != nil {
		t.Errorf("valid icon rejected: %v", err)
	}
}
