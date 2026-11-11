package readmodel

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// levelFilter selects Messages whose attribute incident.level is 7.
const levelFilter = `{"op":"eq","field":{"namespace":"attributes","path":["incident","level"]},"value":{"kind":"integer","value":"7"}}`

// testRegistry holds the built-in fields and the attribute schema the filter tests address.
func testRegistry() *fields.Registry {
	return fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{{Key: "incident",
		Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{"level": {Kind: processing.IntegerValue}}}}, 1))
}

func parse(t *testing.T, query string) (Filters, error) {
	t.Helper()
	values, err := url.ParseQuery(query)
	if err != nil {
		t.Fatal(err)
	}
	return ParseFilters(values)
}

func TestParseFiltersAcceptsTheVocabulary(t *testing.T) {
	t.Parallel()
	unit, network := uuid.New(), uuid.New()
	filters, err := parse(t, "from=2026-11-08T12:00:00%2B01:00&until=2026-11-08T13:00:00Z&text=Stra%C3%9Fe&unit="+unit.String()+
		"&unit="+unit.String()+"&network="+network.String()+"&protocol=pocsag&protocol=flex&filter="+url.QueryEscape(levelFilter)+"&withoutUnit=true")
	if err != nil {
		t.Fatal(err)
	}
	if filters.From.UTC().Hour() != 11 || filters.Text != "Straße" || len(filters.Units) != 1 || filters.Filter == nil ||
		!filters.WithoutUnit || len(filters.Protocols) != 2 {
		t.Fatalf("unexpected filters %+v", filters)
	}
	selection, err := CompileSelection(testRegistry(), filters.Filter, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := filters.RequiredFieldGroups(selection); strings.Join(got, ",") != "payload,unit,gateway,derived" {
		t.Fatalf("required groups %v", got)
	}
}

func TestParseFiltersRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	hundredOne := make([]string, 101)
	for i := range hundredOne {
		hundredOne[i] = "unit=" + uuid.NewString()
	}
	for name, query := range map[string]string{
		"unknown parameter":     "gateway=" + uuid.NewString(),
		"repeated scalar text":  "text=a&text=b",
		"repeated from":         "from=2026-11-08T12:00:00Z&from=2026-11-08T12:00:00Z",
		"bad time":              "from=yesterday",
		"empty from":            "from=",
		"bad uuid":              "unit=nope",
		"nil uuid":              "unit=" + uuid.Nil.String(),
		"invalid utf-8":         "text=%FF%FE",
		"nul byte":              "text=a%00b",
		"text over 1024 bytes":  "text=" + strings.Repeat("a", 1025),
		"101 values":            strings.Join(hundredOne, "&"),
		"unknown protocol":      "protocol=tetra",
		"derived parameter":     "derived=level%3D7",
		"filter twice":          "filter=" + url.QueryEscape(levelFilter) + "&filter=" + url.QueryEscape(levelFilter),
		"filter empty":          "filter=",
		"filter not JSON":       "filter=nope",
		"filter unknown member": "filter=" + url.QueryEscape(`{"op":"and","args":[],"extra":1}`),
		"filter over 32 KiB":    "filter=" + url.QueryEscape(strings.Repeat(" ", MaxFilterBytes+1)+levelFilter),
		"bad withoutUnit":       "withoutUnit=yes",
		"repeated withoutUnit":  "withoutUnit=true&withoutUnit=true",
	} {
		if _, err := parse(t, query); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v, want INVALID_INPUT", name, err)
		}
	}
	if _, err := parse(t, "text="+strings.Repeat("a", 1024)); err != nil {
		t.Errorf("text of exactly 1024 bytes: %v", err)
	}
	var hundred []string
	for range 100 {
		hundred = append(hundred, "unit="+uuid.NewString())
	}
	if _, err := parse(t, strings.Join(hundred, "&")); err != nil {
		t.Errorf("100 values: %v", err)
	}
}

func TestEmptyTextIsNoFilterAndCanonicalFormIgnoresOrder(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	first, err := parse(t, "text=&unit="+a.String()+"&unit="+b.String())
	if err != nil {
		t.Fatal(err)
	}
	second, err := parse(t, "unit="+b.String()+"&unit="+a.String()+"&unit="+b.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.RequiredFieldGroups(nil)) != 1 || first.Hash() != second.Hash() {
		t.Fatal("equal filter sets must share one canonical encoding, and empty text must need no group")
	}
	third, _ := parse(t, "unit="+a.String())
	if third.Hash() == first.Hash() {
		t.Fatal("different filters must differ")
	}
}

func TestCanonicalFormBindsThePredicate(t *testing.T) {
	t.Parallel()
	spaced := `{ "op" : "eq", "field": {"namespace":"attributes","path":["incident","level"]},
		"value": {"kind":"integer","value":"7"} }`
	first, err := parse(t, "filter="+url.QueryEscape(levelFilter))
	if err != nil {
		t.Fatal(err)
	}
	second, err := parse(t, "filter="+url.QueryEscape(spaced))
	if err != nil {
		t.Fatal(err)
	}
	other, err := parse(t, "filter="+url.QueryEscape(strings.Replace(levelFilter, `"7"`, `"8"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if first.Hash() != second.Hash() || first.Hash() == other.Hash() || first.Hash() == (Filters{}).Hash() {
		t.Fatal("the canonical form must ignore the layout of the predicate and nothing else")
	}
	if !strings.HasPrefix(string(first.Canonical()), `{"v":2,`) {
		t.Fatalf("canonical = %s", first.Canonical())
	}
}

func TestCompileSelectionChecksTheRegistry(t *testing.T) {
	t.Parallel()
	for name, predicate := range map[string]string{
		"unknown field":  strings.Replace(levelFilter, `"level"`, `"missing"`, 1),
		"unknown schema": strings.Replace(levelFilter, `"incident"`, `"other"`, 1),
		"wrong operand":  strings.Replace(levelFilter, `"integer"`, `"string"`, 1),
		"workflow field": `{"op":"eq","field":{"namespace":"source","path":["protocol"]},"value":{"kind":"string","value":"x"}}`,
	} {
		filters, err := parse(t, "filter="+url.QueryEscape(predicate))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := CompileSelection(testRegistry(), filters.Filter, true); err == nil {
			t.Errorf("%s: want a failure", name)
		}
	}
	if selection, err := CompileSelection(testRegistry(), nil, true); selection != nil || err != nil {
		t.Errorf("no predicate: %v, %v", selection, err)
	}
}

func TestParseFeedRequestPageSize(t *testing.T) {
	t.Parallel()
	for query, want := range map[string]int{"": 50, "pageSize=1": 1, "pageSize=200": 200} {
		values, _ := url.ParseQuery(query)
		request, err := ParseFeedRequest(values)
		if err != nil || request.PageSize != want {
			t.Errorf("%q: %+v, %v", query, request, err)
		}
	}
	for _, query := range []string{"pageSize=0", "pageSize=201", "pageSize=x", "pageSize=1&pageSize=2", "pageToken=", "offset=1"} {
		values, _ := url.ParseQuery(query)
		if _, err := ParseFeedRequest(values); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%q: %v", query, err)
		}
	}
}
