package readmodel

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

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
		"&unit="+unit.String()+"&network="+network.String()+"&protocol=pocsag&protocol=flex&derived=level%3D7&derived=level%3D8&withoutUnit=true")
	if err != nil {
		t.Fatal(err)
	}
	if filters.From.UTC().Hour() != 11 || filters.Text != "Straße" || len(filters.Units) != 1 || len(filters.Derived) != 2 ||
		!filters.WithoutUnit || len(filters.Protocols) != 2 {
		t.Fatalf("unexpected filters %+v", filters)
	}
	if got := filters.RequiredFieldGroups(); strings.Join(got, ",") != "payload,unit,gateway,derived" {
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
		"derived without equal": "derived=level",
		"derived empty name":    "derived=%3D7",
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
	if len(first.RequiredFieldGroups()) != 1 || first.Hash() != second.Hash() {
		t.Fatal("equal filter sets must share one canonical encoding, and empty text must need no group")
	}
	third, _ := parse(t, "unit="+a.String())
	if third.Hash() == first.Hash() {
		t.Fatal("different filters must differ")
	}
}

func TestDerivedMatcherInterpretations(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]DerivedMatcher{
		"true": {Text: "true", Boolean: "true"}, "12": {Text: "12", Number: "12"}, "-1.5": {Text: "-1.5", Number: "-1.5"},
		"007": {Text: "007"}, "abc": {Text: "abc"}, "": {Text: ""},
	} {
		if got := (DerivedCondition{Name: "x", Value: value}).Matcher(); got != want {
			t.Errorf("%q: %+v, want %+v", value, got, want)
		}
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
