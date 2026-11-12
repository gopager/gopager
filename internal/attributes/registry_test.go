package attributes

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

type countingCatalog struct {
	schemas  []Schema
	revision int64
	loads    int
}

func (c *countingCatalog) Catalog(context.Context, platform.TransactionContext) ([]Schema, int64, error) {
	c.loads++
	return c.schemas, c.revision, nil
}

func (c *countingCatalog) CatalogRevision(context.Context, platform.TransactionContext) (int64, error) {
	return c.revision, nil
}

func (c *countingCatalog) LockCatalog(context.Context, platform.TransactionContext) error { return nil }

func TestRegistrySourceCachesByRevision(t *testing.T) {
	t.Parallel()
	catalog := &countingCatalog{schemas: []Schema{alarmSchema()}, revision: 1}
	source, err := NewRegistrySource(catalog)
	if err != nil {
		t.Fatal(err)
	}
	ref := processing.FieldRef{Namespace: "attributes", Path: []string{"alarm", "keyword"}}
	for range 3 {
		registry, err := source.Registry(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.Lookup(fields.ScopeMessage, ref, true); err != nil {
			t.Fatalf("lookup: %v", err)
		}
	}
	if catalog.loads != 1 {
		t.Fatalf("loads = %d, want 1", catalog.loads)
	}
	next := alarmSchema()
	next.Key = "weather"
	next.Agencies = []uuid.UUID{uuid.New()}
	catalog.schemas, catalog.revision = append(catalog.schemas, next), 2
	registry, err := source.Registry(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.loads != 2 {
		t.Fatalf("loads = %d, want 2", catalog.loads)
	}
	if _, err := registry.Lookup(fields.ScopeMessage,
		processing.FieldRef{Namespace: "attributes", Path: []string{"weather", "level"}}, true); err != nil {
		t.Fatalf("new schema not visible: %v", err)
	}
}
