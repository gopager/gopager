package attributes

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

type fakeSource struct {
	schemas []Schema
	err     error
	locked  bool
}

func (f *fakeSource) Catalog(context.Context, platform.TransactionContext) ([]Schema, int64, error) {
	return f.schemas, 1, f.err
}

func (f *fakeSource) CatalogRevision(context.Context, platform.TransactionContext) (int64, error) {
	return 1, f.err
}

func (f *fakeSource) LockCatalog(context.Context, platform.TransactionContext) error {
	f.locked = true
	return f.err
}

func TestUnitCatalogUnitSchemas(t *testing.T) {
	t.Parallel()
	agency := uuid.New()
	source := &fakeSource{schemas: []Schema{
		{Key: "msg", Subject: SubjectMessage, Fields: map[string]Field{"a": {Kind: processing.StringValue}}},
		{Key: "unitdata", Subject: SubjectUnit, Agencies: []uuid.UUID{agency}, Fields: map[string]Field{
			"badge": {Kind: processing.StringValue, Label: "Badge"},
			"tags":  {Kind: processing.ListValue, Element: &Field{Kind: processing.IntegerValue}},
		}},
	}}
	schemas, err := NewUnitCatalog(source).UnitSchemas(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(schemas) != 1 {
		t.Fatalf("schemas = %v, want only the unit schema", schemas)
	}
	schema := schemas["unitdata"]
	if !schema.AssignedTo(agency) || schema.AssignedTo(uuid.New()) {
		t.Fatalf("agencies = %v", schema.Agencies)
	}
	if schema.Fields["badge"].Kind != processing.StringValue {
		t.Fatalf("badge = %+v", schema.Fields["badge"])
	}
	tags := schema.Fields["tags"]
	if tags.Kind != processing.ListValue || tags.Element == nil || tags.Element.Kind != processing.IntegerValue {
		t.Fatalf("tags = %+v", tags)
	}
}

func TestUnitCatalogPropagatesSourceErrors(t *testing.T) {
	t.Parallel()
	failure := errors.New("boom")
	source := &fakeSource{err: failure}
	catalog := NewUnitCatalog(source)
	if _, err := catalog.UnitSchemas(context.Background(), nil); !errors.Is(err, failure) {
		t.Fatalf("UnitSchemas error = %v", err)
	}
	if err := catalog.LockCatalog(context.Background(), nil); !errors.Is(err, failure) || !source.locked {
		t.Fatalf("LockCatalog error = %v, locked = %v", err, source.locked)
	}
}
