package processing

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func ids(n int) []uuid.UUID {
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = uuid.New()
	}
	return out
}

func TestWorkflowScopeCanonicalRejectsInvalidShapes(t *testing.T) {
	t.Parallel()
	agency := uuid.New()
	duplicate := uuid.New()
	cases := map[string]WorkflowScope{
		"no agencies":         {},
		"nil agency":          {Agencies: []uuid.UUID{uuid.Nil}},
		"duplicate agency":    {Agencies: []uuid.UUID{agency, agency}},
		"too many agencies":   {Agencies: ids(257)},
		"unknown kind":        {Agencies: []uuid.UUID{agency}, Narrowing: &WorkflowNarrowing{Kind: "agency", References: ids(1)}},
		"empty references":    {Agencies: []uuid.UUID{agency}, Narrowing: &WorkflowNarrowing{Kind: "unit"}},
		"nil reference":       {Agencies: []uuid.UUID{agency}, Narrowing: &WorkflowNarrowing{Kind: "unit", References: []uuid.UUID{uuid.Nil}}},
		"duplicate reference": {Agencies: []uuid.UUID{agency}, Narrowing: &WorkflowNarrowing{Kind: "network", References: []uuid.UUID{duplicate, duplicate}}},
		"too many references": {Agencies: []uuid.UUID{agency}, Narrowing: &WorkflowNarrowing{Kind: "network", References: ids(1025)}},
	}
	for name, scope := range cases {
		if _, err := scope.Canonical(); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestWorkflowScopeCanonicalSortsAndDetaches(t *testing.T) {
	t.Parallel()
	agencies := ids(3)
	references := ids(3)
	scope := WorkflowScope{Agencies: agencies, Narrowing: &WorkflowNarrowing{Kind: "unit", References: references}}
	canonical, err := scope.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 3; i++ {
		if canonical.Agencies[i-1].String() >= canonical.Agencies[i].String() ||
			canonical.Narrowing.References[i-1].String() >= canonical.Narrowing.References[i].String() {
			t.Fatalf("not sorted: %v", canonical)
		}
	}
	canonical.Agencies[0] = uuid.Nil
	if agencies[0] == uuid.Nil {
		t.Fatal("canonical scope aliases the input")
	}
}

func TestWorkflowScopeJSONShape(t *testing.T) {
	t.Parallel()
	agency := uuid.New()
	encoded, err := json.Marshal(WorkflowScope{Agencies: []uuid.UUID{agency}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"agencies":["` + agency.String() + `"],"narrowing":null}`; string(encoded) != want {
		t.Fatalf("got %s want %s", encoded, want)
	}
	var decoded WorkflowScope
	if err := Decode([]byte(`{"agencies":["`+agency.String()+`"],"narrowing":{"kind":"unit","references":["`+agency.String()+`"]},"extra":1}`), &decoded); err == nil {
		t.Fatal("undeclared property accepted")
	}
}
