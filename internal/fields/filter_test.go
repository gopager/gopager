package fields

import (
	"encoding/json"
	"testing"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func ref(namespace string, path ...string) processing.FieldRef {
	return processing.FieldRef{Namespace: namespace, Path: path}
}

func lit(t *testing.T, kind processing.ValueKind, value any) processing.Value {
	t.Helper()
	v, err := processing.Literal(kind, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func str(t *testing.T, s string) processing.Value  { return lit(t, processing.StringValue, s) }
func num(t *testing.T, n int64) processing.Value   { return lit(t, processing.IntegerValue, itoa(n)) }
func flt(t *testing.T, f float64) processing.Value { return lit(t, processing.NumberValue, f) }
func ts(t *testing.T, s string) processing.Value   { return lit(t, processing.TimestampValue, s) }
func boolean(t *testing.T, b bool) processing.Value {
	return lit(t, processing.BooleanValue, b)
}
func null() processing.Value {
	return processing.Value{Kind: processing.NullValue, Value: json.RawMessage("null")}
}
func list(t *testing.T, values ...processing.Value) processing.Value {
	return lit(t, processing.ListValue, values)
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

// testProvider declares fields of every filterable kind plus unfilterable and special ones.
func testProvider() Provider {
	opt := func(kind processing.ValueKind) processing.FieldSchema {
		return processing.FieldSchema{Kind: kind, Optional: true}
	}
	listOfKind := func(kind processing.ValueKind) processing.FieldSchema {
		return processing.FieldSchema{Kind: processing.ListValue, Optional: true, Element: &processing.FieldSchema{Kind: kind}}
	}
	mk := func(name string, schema processing.FieldSchema, group string) Field {
		return Field{Ref: ref("t", name), Type: schema, Group: group, Filterable: FilterableType(schema),
			Label: name, Scopes: []Scope{ScopeMessage}}
	}
	nullable := opt(processing.StringValue)
	nullable.Nullable = true
	old := mk("old", opt(processing.StringValue), "")
	old.Deprecated = true
	other := mk("other", opt(processing.StringValue), "")
	other.Scopes = []Scope{ScopeDelivery}
	return NewStaticProvider(3, []Field{
		mk("s", opt(processing.StringValue), "payload"),
		mk("i", opt(processing.IntegerValue), "gateway"),
		mk("n", opt(processing.NumberValue), ""),
		mk("b", opt(processing.BooleanValue), ""),
		mk("ts", opt(processing.TimestampValue), ""),
		mk("ns", nullable, ""),
		mk("ls", listOfKind(processing.StringValue), "unit"),
		mk("bytes", opt(processing.BytesValue), ""),
		mk("obj", processing.FieldSchema{Kind: processing.ObjectValue, Optional: true,
			Properties: map[string]processing.FieldSchema{}}, ""),
		old, other,
	})
}

type source map[string][]processing.Value

func (s source) Values(r processing.FieldRef) []processing.Value { return s[r.Path[0]] }

func leaf(op, field string, value *processing.Value, values ...processing.Value) processing.Predicate {
	r := ref("t", field)
	return processing.Predicate{Op: op, Field: &r, Value: value, Values: values}
}

func TestMatchSemantics(t *testing.T) {
	t.Parallel()
	registry := New(testProvider())
	declarations := registry.Declarations(ScopeMessage, true)
	p := func(v processing.Value) *processing.Value { return &v }
	tests := []struct {
		name      string
		predicate processing.Predicate
		source    source
		want      bool
	}{
		{"eq match", leaf("eq", "s", p(str(t, "a"))), source{"s": {str(t, "a")}}, true},
		{"eq mismatch", leaf("eq", "s", p(str(t, "a"))), source{"s": {str(t, "b")}}, false},
		{"eq absent", leaf("eq", "s", p(str(t, "a"))), source{}, false},
		{"eq null equals null", leaf("eq", "ns", p(null())), source{"ns": {null()}}, true},
		{"eq null vs string", leaf("eq", "ns", p(null())), source{"ns": {str(t, "a")}}, false},
		{"eq string vs null candidate", leaf("eq", "ns", p(str(t, "a"))), source{"ns": {null()}}, false},
		{"eq bool", leaf("eq", "b", p(boolean(t, true))), source{"b": {boolean(t, true)}}, true},
		{"eq timestamp by instant", leaf("eq", "ts", p(ts(t, "2026-01-01T01:00:00+01:00"))),
			source{"ts": {ts(t, "2026-01-01T00:00:00Z")}}, true},
		{"eq undecodable candidate ignored", leaf("eq", "i", p(num(t, 1))),
			source{"i": {str(t, "1"), num(t, 1)}}, true},
		{"eq only undecodable", leaf("eq", "i", p(num(t, 1))), source{"i": {str(t, "1")}}, false},
		{"in match", leaf("in", "s", nil, str(t, "x"), str(t, "a")), source{"s": {str(t, "a")}}, true},
		{"in mismatch", leaf("in", "s", nil, str(t, "x")), source{"s": {str(t, "a")}}, false},
		{"in null", leaf("in", "ns", nil, null(), str(t, "x")), source{"ns": {null()}}, true},
		{"lt integer", leaf("lt", "i", p(num(t, 5))), source{"i": {num(t, 4)}}, true},
		{"lt integer equal", leaf("lt", "i", p(num(t, 5))), source{"i": {num(t, 5)}}, false},
		{"lte integer equal", leaf("lte", "i", p(num(t, 5))), source{"i": {num(t, 5)}}, true},
		{"gt integer", leaf("gt", "i", p(num(t, 5))), source{"i": {num(t, 6)}}, true},
		{"gte integer", leaf("gte", "i", p(num(t, 5))), source{"i": {num(t, 4)}}, false},
		{"gt number", leaf("gt", "n", p(flt(t, 1.5))), source{"n": {flt(t, 2.5)}}, true},
		{"eq timestamp ignores sub-microseconds", leaf("eq", "ts", p(ts(t, "2026-01-01T00:00:00.000000100Z"))),
			source{"ts": {ts(t, "2026-01-01T00:00:00.000000200Z")}}, true},
		{"eq timestamp differs by a microsecond", leaf("eq", "ts", p(ts(t, "2026-01-01T00:00:00.000001000Z"))),
			source{"ts": {ts(t, "2026-01-01T00:00:00.000000999Z")}}, false},
		{"in timestamp ignores sub-microseconds", leaf("in", "ts", nil, ts(t, "2026-01-01T00:00:00.000000900Z")),
			source{"ts": {ts(t, "2026-01-01T00:00:00.000000100Z")}}, true},
		{"gt timestamp ignores sub-microseconds", leaf("gt", "ts", p(ts(t, "2026-01-01T00:00:00.000000100Z"))),
			source{"ts": {ts(t, "2026-01-01T00:00:00.000000900Z")}}, false},
		{"lt timestamp", leaf("lt", "ts", p(ts(t, "2026-01-02T00:00:00Z"))),
			source{"ts": {ts(t, "2026-01-01T00:00:00Z")}}, true},
		{"contains", leaf("contains", "s", p(str(t, "ell"))), source{"s": {str(t, "hello")}}, true},
		{"contains case sensitive", leaf("contains", "s", p(str(t, "ELL"))), source{"s": {str(t, "hello")}}, false},
		{"contains null candidate", leaf("contains", "ns", p(str(t, "a"))), source{"ns": {null()}}, false},
		{"present true", leaf("present", "s", p(boolean(t, true))), source{"s": {str(t, "a")}}, true},
		{"present true absent", leaf("present", "s", p(boolean(t, true))), source{}, false},
		{"present false absent", leaf("present", "s", p(boolean(t, false))), source{}, true},
		{"present false present", leaf("present", "s", p(boolean(t, false))), source{"s": {str(t, "a")}}, false},
		{"present true null is present", leaf("present", "ns", p(boolean(t, true))), source{"ns": {null()}}, true},
		{"present true undecodable", leaf("present", "i", p(boolean(t, true))), source{"i": {str(t, "x")}}, false},
		{"list any element", leaf("eq", "ls", p(str(t, "b"))), source{"ls": {list(t, str(t, "a"), str(t, "b"))}}, true},
		{"list no element", leaf("eq", "ls", p(str(t, "c"))), source{"ls": {list(t, str(t, "a"))}}, false},
		{"list empty not present", leaf("present", "ls", p(boolean(t, true))), source{"ls": {list(t)}}, false},
		{"list empty present false", leaf("present", "ls", p(boolean(t, false))), source{"ls": {list(t)}}, true},
		{"list contains", leaf("contains", "ls", p(str(t, "ar"))), source{"ls": {list(t, str(t, "bar"))}}, true},
		{"list in", leaf("in", "ls", nil, str(t, "z"), str(t, "a")), source{"ls": {list(t, str(t, "a"))}}, true},
		{"several candidates", leaf("eq", "s", p(str(t, "b"))), source{"s": {str(t, "a"), str(t, "b")}}, true},
		{"and", processing.Predicate{Op: "and", Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "a"))), leaf("eq", "i", p(num(t, 1)))}},
			source{"s": {str(t, "a")}, "i": {num(t, 1)}}, true},
		{"and fails", processing.Predicate{Op: "and", Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "a"))), leaf("eq", "i", p(num(t, 1)))}},
			source{"s": {str(t, "a")}}, false},
		{"or", processing.Predicate{Op: "or", Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "x"))), leaf("eq", "i", p(num(t, 1)))}},
			source{"i": {num(t, 1)}}, true},
		{"or fails", processing.Predicate{Op: "or", Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "x")))}}, source{}, false},
		{"not of absent comparison is true", processing.Predicate{Op: "not", Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "x")))}}, source{}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			filter, err := CompileFilter(test.predicate, declarations)
			if err != nil {
				t.Fatalf("CompileFilter: %v", err)
			}
			if got := filter.Match(test.source); got != test.want {
				t.Fatalf("Match = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMatchOrderedIgnoresNullCandidates(t *testing.T) {
	t.Parallel()
	nullableInt := processing.FieldSchema{Kind: processing.IntegerValue, Optional: true, Nullable: true}
	provider := NewStaticProvider(0, []Field{{Ref: ref("t", "ni"), Type: nullableInt, Filterable: true,
		Scopes: []Scope{ScopeMessage}}})
	declarations := New(provider).Declarations(ScopeMessage, false)
	operand := num(t, 5)
	predicate := leaf("lt", "ni", &operand)
	predicate.Field = &processing.FieldRef{Namespace: "t", Path: []string{"ni"}}
	filter, err := CompileFilter(predicate, declarations)
	if err != nil {
		t.Fatal(err)
	}
	if filter.Match(source{"ni": {null()}}) {
		t.Fatal("a null candidate must not satisfy an ordered comparison")
	}
}

func TestCompileFilterRejects(t *testing.T) {
	t.Parallel()
	declarations := New(testProvider()).Declarations(ScopeMessage, true)
	p := func(v processing.Value) *processing.Value { return &v }
	deep := leaf("eq", "s", p(str(t, "a")))
	for i := 0; i < processing.MaxDepth; i++ {
		deep = processing.Predicate{Op: "not", Args: []processing.Predicate{deep}}
	}
	tooMany := make([]processing.Value, processing.MaxArguments+1)
	for i := range tooMany {
		tooMany[i] = str(t, "a")
	}
	manyArgs := make([]processing.Predicate, processing.MaxArguments+1)
	for i := range manyArgs {
		manyArgs[i] = leaf("eq", "s", p(str(t, "a")))
	}
	unknown := processing.FieldRef{Namespace: "t", Path: []string{"missing"}}
	emptyPath := processing.FieldRef{Namespace: "t"}
	tests := []struct {
		name      string
		predicate processing.Predicate
		code      platform.ErrorCode
	}{
		{"unknown operator", leaf("like", "s", p(str(t, "a"))), platform.CodeInvalidInput},
		{"unknown field", processing.Predicate{Op: "eq", Field: &unknown, Value: p(str(t, "a"))}, platform.CodeInvalidInput},
		{"empty path", processing.Predicate{Op: "eq", Field: &emptyPath, Value: p(str(t, "a"))}, platform.CodeInvalidInput},
		{"missing field", processing.Predicate{Op: "eq", Value: p(str(t, "a"))}, platform.CodeInvalidInput},
		{"bytes not filterable", leaf("present", "bytes", p(boolean(t, true))), platform.CodeInvalidInput},
		{"object not filterable", leaf("present", "obj", p(boolean(t, true))), platform.CodeInvalidInput},
		{"out of scope", leaf("eq", "other", p(str(t, "a"))), platform.CodeInvalidInput},
		{"deprecated strict", leaf("eq", "old", p(str(t, "a"))), platform.CodeInvalidInput},
		{"kind mismatch", leaf("eq", "s", p(num(t, 1))), platform.CodeInvalidInput},
		{"list element mismatch", leaf("eq", "ls", p(num(t, 1))), platform.CodeInvalidInput},
		{"null for non-nullable", leaf("eq", "s", p(null())), platform.CodeInvalidInput},
		{"ordered on string", leaf("lt", "s", p(str(t, "a"))), platform.CodeInvalidInput},
		{"ordered on bool", leaf("gt", "b", p(boolean(t, true))), platform.CodeInvalidInput},
		{"ordered null", leaf("lt", "ns", p(null())), platform.CodeInvalidInput},
		{"contains on integer", leaf("contains", "i", p(num(t, 1))), platform.CodeInvalidInput},
		{"contains null", leaf("contains", "ns", p(null())), platform.CodeInvalidInput},
		{"present non-boolean", leaf("present", "s", p(str(t, "a"))), platform.CodeInvalidInput},
		{"present with values", leaf("present", "s", p(boolean(t, true)), boolean(t, true)), platform.CodeInvalidInput},
		{"eq with values", leaf("eq", "s", p(str(t, "a")), str(t, "b")), platform.CodeInvalidInput},
		{"eq without value", leaf("eq", "s", nil), platform.CodeInvalidInput},
		{"in empty", leaf("in", "s", nil), platform.CodeInvalidInput},
		{"in with value", leaf("in", "s", p(str(t, "a")), str(t, "a")), platform.CodeInvalidInput},
		{"in too many", leaf("in", "s", nil, tooMany...), platform.CodeResourceLimit},
		{"and empty", processing.Predicate{Op: "and"}, platform.CodeInvalidInput},
		{"not two args", processing.Predicate{Op: "not", Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "a"))), leaf("eq", "s", p(str(t, "a")))}}, platform.CodeInvalidInput},
		{"logical with field", processing.Predicate{Op: "and", Field: &unknown, Args: []processing.Predicate{
			leaf("eq", "s", p(str(t, "a")))}}, platform.CodeInvalidInput},
		{"leaf with args", processing.Predicate{Op: "eq", Field: &unknown, Value: p(str(t, "a")),
			Args: []processing.Predicate{leaf("eq", "s", p(str(t, "a")))}}, platform.CodeInvalidInput},
		{"too many args", processing.Predicate{Op: "or", Args: manyArgs}, platform.CodeResourceLimit},
		{"too deep", deep, platform.CodeResourceLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := CompileFilter(test.predicate, declarations)
			if err == nil {
				t.Fatal("expected an error")
			}
			if got := platform.FailureCode(err); got != test.code {
				t.Fatalf("code = %v, want %v (%v)", got, test.code, err)
			}
		})
	}
}

func TestCompileFilterAcceptsDeprecatedWhenNotStrict(t *testing.T) {
	t.Parallel()
	declarations := New(testProvider()).Declarations(ScopeMessage, false)
	value := str(t, "a")
	if _, err := CompileFilter(leaf("eq", "old", &value), declarations); err != nil {
		t.Fatal(err)
	}
}

func TestCompileFilterWithoutRegistry(t *testing.T) {
	t.Parallel()
	value := str(t, "a")
	if _, err := CompileFilter(leaf("eq", "s", &value), Declarations{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFilterFieldsGroupsAndRoot(t *testing.T) {
	t.Parallel()
	declarations := New(testProvider()).Declarations(ScopeMessage, true)
	a, b, c := str(t, "a"), num(t, 1), str(t, "c")
	filter, err := CompileFilter(processing.Predicate{Op: "or", Args: []processing.Predicate{
		leaf("eq", "s", &a), leaf("eq", "i", &b), leaf("eq", "s", &c), leaf("in", "ls", nil, str(t, "z")),
		leaf("present", "n", &processing.Value{Kind: processing.BooleanValue, Value: json.RawMessage("true")}),
	}}, declarations)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range filter.Fields() {
		paths = append(paths, f.Path())
	}
	if want := []string{"t.i", "t.ls", "t.n", "t.s"}; !equalStrings(paths, want) {
		t.Fatalf("Fields = %v, want %v", paths, want)
	}
	if want := []string{"gateway", "payload", "unit"}; !equalStrings(filter.Groups(), want) {
		t.Fatalf("Groups = %v, want %v", filter.Groups(), want)
	}
	root := filter.Root()
	if root.Op != "or" || len(root.Args) != 5 || root.Args[0].Field.Path() != "t.s" ||
		root.Args[0].Values[0] != "a" || root.Args[4].Values[0] != true {
		t.Fatalf("unexpected root %+v", root)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
