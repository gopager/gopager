package readmodel

import (
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/processing"
)

func TestParseDefinitionIsStrict(t *testing.T) {
	t.Parallel()
	unit := uuid.NewString()
	for name, raw := range map[string]string{
		"unknown top-level field": `{"schemaVersion":1,"filters":{},"extra":1}`,
		"unknown filter field":    `{"schemaVersion":1,"filters":{"gateway":["` + unit + `"]}}`,
		"wrong schema version":    `{"schemaVersion":2,"filters":{}}`,
		"missing schema version":  `{"filters":{}}`,
		"trailing data":           `{"schemaVersion":1,"filters":{}} {}`,
		"malformed unit":          `{"schemaVersion":1,"filters":{"units":["x"]}}`,
		"bad time":                `{"schemaVersion":1,"filters":{},"from":"yesterday"}`,
	} {
		if _, err := ParseDefinition([]byte(raw)); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := ParseDefinition([]byte(`{"schemaVersion":1,"filters":{"text":"a","units":["` + unit + `"],"withoutUnit":true,"filter":` + levelFilter + `}}`)); err != nil {
		t.Fatalf("valid definition: %v", err)
	}
}

func TestNormalizeAppliesFeedFilterLimits(t *testing.T) {
	t.Parallel()
	a, b := uuid.MustParse("00000000-0000-0000-0000-00000000000a"), uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	normalized, filters, err := Definition{SchemaVersion: 1, Filters: DefinitionFilters{Units: []uuid.UUID{b, a, b},
		Protocols: []string{"flex", "pocsag", "flex"}}}.Normalize()
	if err != nil || !slices.Equal(normalized.Filters.Units, []uuid.UUID{a, b}) ||
		!slices.Equal(filters.Protocols, []string{"flex", "pocsag"}) {
		t.Fatalf("normalized = %+v, %v", normalized, err)
	}
	many := make([]uuid.UUID, MaxFilterValues+1)
	for i := range many {
		many[i] = uuid.New()
	}
	invalid := map[string]Definition{
		"too many units":   {SchemaVersion: 1, Filters: DefinitionFilters{Units: many}},
		"nil unit":         {SchemaVersion: 1, Filters: DefinitionFilters{Units: []uuid.UUID{uuid.Nil}}},
		"unknown protocol": {SchemaVersion: 1, Filters: DefinitionFilters{Protocols: []string{"morse"}}},
		"oversized text":   {SchemaVersion: 1, Filters: DefinitionFilters{Text: strings.Repeat("a", MaxTextBytes+1)}},
		"invalid UTF-8":    {SchemaVersion: 1, Filters: DefinitionFilters{Text: "\xff"}},
		"schema version":   {SchemaVersion: 0},
		"filter too large": {SchemaVersion: 1, Filters: DefinitionFilters{Filter: oversizedPredicate()}},
	}
	for name, definition := range invalid {
		if _, _, err := definition.Normalize(); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestValidateViewName(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"a", strings.Repeat("ä", 256), " padded "} {
		if err := ValidateViewName(name); err != nil {
			t.Errorf("%q: %v", name, err)
		}
	}
	for _, name := range []string{"", "  \t", strings.Repeat("ä", 257), "\xff", "a\x00b"} {
		if err := ValidateViewName(name); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%q: err = %v", name, err)
		}
	}
}

func TestUnavailableDimensionsNamesDimensionsOfUnreadableGroups(t *testing.T) {
	t.Parallel()
	filters := Filters{Text: "Brand", Units: []uuid.UUID{uuid.New()}, Networks: []uuid.UUID{uuid.New()}, WithoutUnit: true}
	readable := func(groups ...string) func(string) bool {
		return func(group string) bool { return slices.Contains(groups, group) }
	}
	for _, test := range []struct {
		groups []string
		want   []string
	}{
		{[]string{"payload"}, []string{"network", "unit", "withoutUnit"}},
		{[]string{"payload", "unit"}, []string{"network"}},
		{[]string{"payload", "unit", "gateway"}, []string{}},
		{nil, []string{"network", "text", "unit", "withoutUnit"}},
	} {
		if got := filters.UnavailableDimensions(readable(test.groups...), nil); !slices.Equal(got, test.want) {
			t.Errorf("groups %v: unavailable = %v, want %v", test.groups, got, test.want)
		}
	}
	if got := (Filters{}).UnavailableDimensions(readable(), nil); got == nil || len(got) != 0 {
		t.Errorf("no filters: %v, want an empty non-nil list", got)
	}
}

// oversizedPredicate is a valid predicate whose encoding exceeds MaxFilterBytes.
func oversizedPredicate() *processing.Predicate {
	return &processing.Predicate{Op: "eq", Field: &processing.FieldRef{Namespace: "message", Path: []string{"text"}},
		Value: &processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"` + strings.Repeat("a", MaxFilterBytes) + `"`)}}
}

func TestUnavailableDimensionsReportsTheFilterByItsFieldGroups(t *testing.T) {
	t.Parallel()
	filters, err := ParseFilters(url.Values{"filter": {levelFilter}})
	if err != nil {
		t.Fatal(err)
	}
	selection, err := CompileSelection(testRegistry(), filters.Filter, true)
	if err != nil {
		t.Fatal(err)
	}
	readable := func(groups ...string) func(string) bool {
		return func(group string) bool { return slices.Contains(groups, group) }
	}
	if got := filters.UnavailableDimensions(readable("derived"), selection); len(got) != 0 {
		t.Errorf("readable: %v", got)
	}
	if got := filters.UnavailableDimensions(readable("payload"), selection); !slices.Equal(got, []string{"filter"}) {
		t.Errorf("unreadable group: %v", got)
	}
	if got := filters.UnavailableDimensions(readable("derived"), nil); !slices.Equal(got, []string{"filter"}) {
		t.Errorf("a predicate the registry no longer accepts: %v", got)
	}
}
