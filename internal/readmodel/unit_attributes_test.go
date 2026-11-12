package readmodel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

const townFilter = `{"op":"eq","field":{"namespace":"unit","path":["attributes","station","town"]},"value":{"kind":"string","value":"A"}}`

func unitRegistry() *fields.Registry {
	return fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{{Key: "station",
		Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{"town": {Kind: processing.StringValue}}}}, 1))
}

func TestUnitAttributePredicateNeedsTheUnitGroup(t *testing.T) {
	t.Parallel()
	filters, err := parse(t, "filter="+strings.ReplaceAll(townFilter, `"`, "%22"))
	if err != nil {
		t.Fatal(err)
	}
	selection, err := CompileSelection(unitRegistry(), filters.Filter, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := filters.RequiredFieldGroups(selection); strings.Join(got, ",") != "unit" {
		t.Fatalf("required groups %v", got)
	}
	if err := (access{viewer: viewer{groups: map[string]bool{"derived": true}}}).requireGroups(filters.RequiredFieldGroups(selection)); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden without the unit group", err)
	}
	if err := (access{viewer: viewer{groups: map[string]bool{"unit": true}}}).requireGroups(filters.RequiredFieldGroups(selection)); err != nil {
		t.Fatal(err)
	}
}

func TestUnitItemCarriesAttributes(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	record.Units[0].Attributes = []UnitAttribute{
		{SchemaKey: "station", Field: "town", Value: processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"A"`)}, Source: "unit"},
	}
	record.Units = append(record.Units, UnitRecord{UnitID: uuid.New(), Name: "B"})
	out, err := json.Marshal(viewer{groups: map[string]bool{"unit": true}}.feedItem(record, false))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"attributes":[{"schemaKey":"station","field":"town","value":{"kind":"string","value":"A"},"source":"unit"}]`) ||
		!strings.Contains(string(out), `"attributes":[]`) {
		t.Fatalf("units = %s", out)
	}
	hidden, err := json.Marshal(viewer{groups: map[string]bool{"payload": true}}.feedItem(record, false))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(hidden), "station") {
		t.Fatalf("unit attributes leak without the unit group: %s", hidden)
	}
}

// feedStub answers every Feed with one fixed result.
type feedStub struct {
	Store
	result FeedResult
}

func (s feedStub) Feed(context.Context, platform.TransactionContext, FeedQuery) (FeedResult, error) {
	return s.result, nil
}

type stubTx struct{}

func (stubTx) Context() context.Context { return context.Background() }

func TestDeliveryProjectionCarriesUnitAttributes(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	record.Units[0].Attributes = []UnitAttribute{{SchemaKey: "station", Field: "town",
		Value: processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"A"`)}, Source: "organization"}}
	service := &Service{store: feedStub{result: FeedResult{Items: []FeedRecord{record}}}}
	id := uuid.NewString()
	authority := authz.Authority{PrincipalID: id, Revision: 1, Active: true, Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: id,
		SubjectKind: "principal", Actions: []string{"message.read"}, Scope: authz.Scope{Kind: "all"}}}}
	projection, ok, err := service.ProjectForDelivery(context.Background(), stubTx{}, authority, time.Now(), record.MessageID, uuid.Nil)
	if err != nil || !ok {
		t.Fatalf("ok = %v, err = %v", ok, err)
	}
	if len(projection.Units) != 1 || len(projection.Units[0].Attributes) != 1 || projection.Units[0].Attributes[0].Source != "organization" {
		t.Fatalf("units = %+v", projection.Units)
	}
}
