package management

import (
	"slices"
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

// TestReadSchemaKeysNamesTheSchemasOfReadUnitAttributes expects only `target.unit.attributes`
// references to name a read schema, each key once.
func TestReadSchemaKeysNamesTheSchemasOfReadUnitAttributes(t *testing.T) {
	t.Parallel()
	references := []processing.SourceReference{
		{Field: processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "town"}}},
		{Field: processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", "site", "name"}}},
		{Field: processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "radio", "channel"}}},
		{Field: processing.FieldRef{Namespace: "target", Path: []string{"unit", "id"}}},
		{Field: processing.FieldRef{Namespace: "source", Path: []string{"unit", "attributes", "other"}}},
	}
	if got := readSchemaKeys(references); !slices.Equal(got, []string{"station", "radio"}) {
		t.Fatalf("readSchemaKeys = %v, want [station radio]", got)
	}
}
