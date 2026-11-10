package processing

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func renderLayout(t *testing.T, kind, blocks string, fixture *layoutFixture) (string, error) {
	t.Helper()
	layout := mustLayout(t, `{"schemaVersion":1,"outputKind":"`+kind+`","blocks":`+blocks+`}`)
	program, err := CompileLayout(context.Background(), layout, fixture)
	if err != nil {
		t.Fatalf("CompileLayout: %v", err)
	}
	rendered, err := program.Render(context.Background(), fixture)
	if err != nil {
		return "", err
	}
	if rendered.OutputKind != kind {
		t.Fatalf("OutputKind = %q, want %q", rendered.OutputKind, kind)
	}
	return string(rendered.Body), nil
}

const fa = `{"namespace":"source","path":["a"]}`
const fb = `{"namespace":"source","path":["b"]}`

func TestLayoutRenderKinds(t *testing.T) {
	t.Parallel()
	fixture := lfixture(t, map[string]any{"a": "alpha", "b": "beta", "gone": nil})
	gone := `{"namespace":"source","path":["gone"]}`
	eq := `{"op":"eq","field":` + fa + `,"value":{"kind":"string","value":"alpha"}}`
	ne := `{"op":"eq","field":` + fa + `,"value":{"kind":"string","value":"zzz"}}`
	blocks := `[{"kind":"text","text":"hello"},
		{"kind":"field","field":` + fa + `,"label":"A"},
		{"kind":"field","field":` + fb + `},
		{"kind":"chip","field":` + fa + `,"label":"A"},
		{"kind":"chip","field":` + fb + `},
		{"kind":"field","field":` + gone + `,"label":"G"},
		{"kind":"link","label":"L","url":{"template":"https://example.com/{0}","fields":[` + fa + `]}},
		{"kind":"section","title":"T","blocks":[{"kind":"text","text":"in"}]},
		{"kind":"conditional","when":` + eq + `,"then":[{"kind":"text","text":"yes"}],"else":[{"kind":"text","text":"no"}]},
		{"kind":"conditional","when":` + ne + `,"then":[{"kind":"text","text":"yes"}],"else":[{"kind":"text","text":"no"}]},
		{"kind":"fallback","blocks":[{"kind":"field","field":` + gone + `},{"kind":"text","text":"second"},{"kind":"text","text":"third"}]}]`
	for _, item := range []struct{ kind, want string }{
		{"text", "hello\nA: alpha\nbeta\n[A: alpha]\n[beta]\nL: https://example.com/alpha\nT\nin\nyes\nno\nsecond"},
		{"markdown", "hello\n**A:** alpha\nbeta\n\\[A: alpha\\]\n\\[beta\\]\n[L](https://example.com/alpha)\n**T**\nin\nyes\nno\nsecond"},
	} {
		got, err := renderLayout(t, item.kind, blocks, fixture)
		if err != nil {
			t.Fatalf("%s: %v", item.kind, err)
		}
		if got != item.want {
			t.Errorf("%s:\n got %q\nwant %q", item.kind, got, item.want)
		}
	}
	got, err := renderLayout(t, "json", blocks, fixture)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"kind":"text","text":"hello"},` +
		`{"kind":"field","label":"A","value":{"kind":"string","value":"alpha"}},` +
		`{"kind":"field","value":{"kind":"string","value":"beta"}},` +
		`{"kind":"chip","label":"A","value":{"kind":"string","value":"alpha"}},` +
		`{"kind":"chip","value":{"kind":"string","value":"beta"}},` +
		`{"kind":"link","label":"L","url":"https://example.com/alpha"},` +
		`{"kind":"section","title":"T","items":[{"kind":"text","text":"in"}]},` +
		`{"kind":"text","text":"yes"},{"kind":"text","text":"no"},{"kind":"text","text":"second"}]`
	if got != want {
		t.Errorf("json:\n got %s\nwant %s", got, want)
	}
	if !json.Valid([]byte(got)) {
		t.Error("json output invalid")
	}
}

func TestLayoutRenderEmpty(t *testing.T) {
	t.Parallel()
	fixture := lfixture(t, map[string]any{"gone": nil, "n": Value{Kind: NullValue, Value: json.RawMessage("null")}})
	blocks := `[{"kind":"field","field":{"namespace":"source","path":["gone"]}},{"kind":"chip","field":{"namespace":"source","path":["n"]}}]`
	for kind, want := range map[string]string{"text": "", "markdown": "", "json": "[]"} {
		got, err := renderLayout(t, kind, blocks, fixture)
		if err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", kind, got, err, want)
		}
	}
}

func TestLayoutValueFormatting(t *testing.T) {
	t.Parallel()
	list, _ := Literal(ListValue, []Value{{Kind: StringValue, Value: json.RawMessage(`"x"`)}, {Kind: StringValue, Value: json.RawMessage(`"y"`)}})
	object, _ := Literal(ObjectValue, map[string]Value{"a": {Kind: StringValue, Value: json.RawMessage(`"z"`)}})
	fixture := &layoutFixture{schemas: map[string]FieldSchema{}, values: map[string]FieldValue{}}
	add := func(name string, schema FieldSchema, value Value) {
		fixture.schemas[lref(name).key()] = schema
		fixture.values[lref(name).key()] = FieldValue{State: FieldPresent, Value: &value}
	}
	add("int", FieldSchema{Kind: IntegerValue}, Value{Kind: IntegerValue, Value: json.RawMessage(`"42"`)})
	add("num", FieldSchema{Kind: NumberValue}, Value{Kind: NumberValue, Value: json.RawMessage(`1.50`)})
	add("bool", FieldSchema{Kind: BooleanValue}, Value{Kind: BooleanValue, Value: json.RawMessage(`true`)})
	add("ts", FieldSchema{Kind: TimestampValue}, Value{Kind: TimestampValue, Value: json.RawMessage(`"2026-01-02T05:04:05+02:00"`)})
	add("bytes", FieldSchema{Kind: BytesValue}, Value{Kind: BytesValue, Value: json.RawMessage(`"aGk="`)})
	add("list", FieldSchema{Kind: ListValue, Element: &FieldSchema{Kind: StringValue}}, Value{Kind: ListValue, Value: list.Value})
	add("obj", FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{"a": {Kind: StringValue}}}, Value{Kind: ObjectValue, Value: object.Value})
	var blocks []string
	for _, name := range []string{"int", "num", "bool", "ts", "bytes", "list", "obj"} {
		blocks = append(blocks, `{"kind":"field","field":{"namespace":"source","path":["`+name+`"]}}`)
	}
	got, err := renderLayout(t, "text", "["+strings.Join(blocks, ",")+"]", fixture)
	if err != nil {
		t.Fatal(err)
	}
	want := "42\n1.50\ntrue\n2026-01-02T03:04:05Z\naGk=\nx, y\n" + `{"a":{"kind":"string","value":"z"}}`
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestLayoutMarkdownEscaping(t *testing.T) {
	t.Parallel()
	hostile := "*_[]()<>#<script>alert(1)</script> [x](javascript:alert(1))"
	fixture := lfixture(t, map[string]any{"a": hostile})
	got, err := renderLayout(t, "markdown", `[{"kind":"field","field":`+fa+`,"label":"*l*"},{"kind":"text","text":"a*b"}]`, fixture)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range got {
		if (r == '<' || r == '>') && (i == 0 || got[i-1] != '\\') {
			t.Errorf("unescaped angle bracket at %d in %q", i, got)
		}
	}
	want := `**\*l\*:** \*\_\[\]\(\)\<\>\#\<script\>alert\(1\)\</script\> \[x\]\(javascript:alert\(1\)\)` + "\na\\*b"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestLayoutLinks(t *testing.T) {
	t.Parallel()
	for _, item := range []struct {
		name, value, template, text, markdown string
	}{
		{"https kept", "ok", "https://e.com/{0}", "L: https://e.com/ok", "[L](https://e.com/ok)"},
		{"encoding", "a b/c", "https://e.com/{0}", "L: https://e.com/a%20b%2Fc", "[L](https://e.com/a%20b%2Fc)"},
		{"javascript omitted", "javascript:alert(1)", "{0}", "", ""},
		{"scheme ftp omitted", "x", "ftp://e.com/{0}", "", ""},
		{"mailto kept", "a@b.c", "mailto:{0}", "L: mailto:a@b.c", "[L](mailto:a@b.c)"},
		{"query delimiters", "alarm&recipient=other+x y#f", "https://e.com/s?q={0}",
			"L: https://e.com/s?q=alarm%26recipient%3Dother%2Bx%20y%23f", "[L](https://e.com/s?q=alarm%26recipient%3Dother%2Bx%20y%23f)"},
		{"parens encoded", "x", "https://e.com/(a)/{0}", "L: https://e.com/(a)/x", "[L](https://e.com/%28a%29/x)"},
		{"http without host omitted", "x", "http:///{0}", "", ""},
	} {
		fixture := lfixture(t, map[string]any{"a": item.value})
		block := `[{"kind":"link","label":"L","url":{"template":"` + item.template + `","fields":[` + fa + `]}}]`
		for kind, want := range map[string]string{"text": item.text, "markdown": item.markdown} {
			got, err := renderLayout(t, kind, block, fixture)
			if err != nil || got != want {
				t.Errorf("%s/%s: got %q, %v; want %q", item.name, kind, got, err, want)
			}
		}
	}
	fixture := lfixture(t, map[string]any{"gone": nil})
	got, err := renderLayout(t, "text", `[{"kind":"link","label":"L","url":{"template":"https://e.com/{0}","fields":[{"namespace":"source","path":["gone"]}]}}]`, fixture)
	if err != nil || got != "" {
		t.Errorf("absent link field: %q, %v", got, err)
	}
}

func TestLayoutControlCharacters(t *testing.T) {
	t.Parallel()
	fixture := lfixture(t, map[string]any{"a": "x\r\ny\tz\x00\x1b"})
	for _, kind := range []string{"text", "markdown"} {
		got, err := renderLayout(t, kind, `[{"kind":"field","field":`+fa+`},{"kind":"text","text":"l\tm\rn"}]`, fixture)
		if err != nil {
			t.Fatal(err)
		}
		if got != "x\ny z\nl mn" {
			t.Errorf("%s: got %q", kind, got)
		}
	}
}

func TestLayoutOutputBound(t *testing.T) {
	t.Parallel()
	fixture := lfixture(t, map[string]any{"a": strings.Repeat("a", 20<<10)})
	blocks := `[` + strings.Repeat(`{"kind":"field","field":`+fa+`},`, 3) + `{"kind":"field","field":` + fa + `}]`
	for _, kind := range []string{"text", "markdown", "json"} {
		_, err := renderLayout(t, kind, blocks, fixture)
		if platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Errorf("%s: code = %q (%v), want RESOURCE_LIMIT", kind, platform.FailureCode(err), err)
		}
	}
	got, err := renderLayout(t, "text", `[{"kind":"field","field":`+fa+`}]`, fixture)
	if err != nil || len(got) != 20<<10 {
		t.Errorf("single field: len %d, %v", len(got), err)
	}
}

func TestLayoutFallbackSkipsEmptySection(t *testing.T) {
	t.Parallel()
	fixture := lfixture(t, map[string]any{"gone": nil})
	gone := `{"namespace":"source","path":["gone"]}`
	got, err := renderLayout(t, "text", `[{"kind":"fallback","blocks":[
		{"kind":"section","title":"T","blocks":[{"kind":"field","field":`+gone+`}]},{"kind":"text","text":"alt"}]}]`, fixture)
	if err != nil || got != "alt" {
		t.Errorf("got %q, %v", got, err)
	}
}

// TestLayoutLargeFieldIsChargedOnceForItsOutputKind proves that a field is charged against the
// output bound with the one representation its output kind emits, so a 40 KiB value fits in
// every kind, while a value beyond the bound still fails.
func TestLayoutLargeFieldIsChargedOnceForItsOutputKind(t *testing.T) {
	t.Parallel()
	large := strings.Repeat("x", 40<<10)
	for _, kind := range []string{"text", "markdown", "json"} {
		body, err := renderLayout(t, kind, `[{"kind":"field","field":`+fa+`}]`, lfixture(t, map[string]any{"a": large}))
		if err != nil || !strings.Contains(body, large) {
			t.Fatalf("%s: a 40 KiB field did not render: %v", kind, err)
		}
	}
	tooLarge := strings.Repeat("x", 70<<10)
	for _, kind := range []string{"text", "markdown", "json"} {
		_, err := renderLayout(t, kind, `[{"kind":"field","field":`+fa+`}]`, lfixture(t, map[string]any{"a": tooLarge}))
		if platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("%s: a 70 KiB field: %v", kind, err)
		}
	}
}
