package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

// storedTextDefinition selects the Transmissions whose text is the given one, so that a selected
// preview proves the stored Payload was evaluated.
func storedTextDefinition(t *testing.T, text string) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"payload", "text", "value"}}
	expected, err := processing.Literal(processing.StringValue, text)
	if err != nil {
		t.Fatal(err)
	}
	return processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &field, Value: &expected},
		Outputs:  map[string]map[string]processing.Assignment{}}
}

func storedBody(version, transmission uuid.UUID) string {
	return `{"schemaVersion":1,"versionId":"` + version.String() + `","storedTransmissionId":"` + transmission.String() + `"}`
}

// TestStoredPreviewEvaluatesAReadableTransmissionAndWritesNothing proves that a stored
// Transmission the editor may read is evaluated with its stored Payload and that no candidate,
// result or fact is written.
func TestStoredPreviewEvaluatesAReadableTransmissionAndWritesNothing(t *testing.T) {
	t.Parallel()
	f := newPreviewHTTPFixture(t, []string{"unit", "address", "payload", "gateway", "derived"})
	version := f.saveVersion(storedTextDefinition(t, "stored alarm"))
	stored := f.storedTransmission("stored alarm", f.seed.unit("Engine 1", nil))
	other := f.storedTransmission("another alarm", f.seed.unit("Engine 2", nil))

	before := processingPreviewSideEffects(t, f.store)
	response := f.preview(storedBody(version, stored), http.StatusOK)
	var evaluation processing.Evaluation
	if err := json.Unmarshal(response.Data, &evaluation); err != nil || !evaluation.Selected {
		t.Fatalf("stored preview = %s, %v; want the stored text selected", response.Data, err)
	}
	var unselected processing.Evaluation
	if err := json.Unmarshal(f.preview(storedBody(version, other), http.StatusOK).Data, &unselected); err != nil || unselected.Selected {
		t.Fatalf("a different stored text was selected: %+v, %v", unselected, err)
	}
	if after := processingPreviewSideEffects(t, f.store); after != before {
		t.Fatalf("stored preview wrote candidates/results/facts: before=%v after=%v", before, after)
	}
}

// TestStoredPreviewHidesUnreadableTransmissionsAndDeniesUnreadableGroups proves that a
// Transmission the editor cannot read is not found and that a readable one whose projection
// carries an unreadable field group is forbidden.
func TestStoredPreviewHidesUnreadableTransmissionsAndDeniesUnreadableGroups(t *testing.T) {
	t.Parallel()
	t.Run("unknown Transmission", func(t *testing.T) {
		t.Parallel()
		f := newPreviewHTTPFixture(t, []string{"unit", "address", "payload", "gateway", "derived"})
		version := f.saveVersion(storedTextDefinition(t, "x"))
		f.preview(storedBody(version, uuid.New()), http.StatusNotFound)
	})
	t.Run("hidden Transmission", func(t *testing.T) {
		t.Parallel()
		f := newPreviewHTTPFixture(t, []string{"unit", "address", "payload", "gateway", "derived"})
		version := f.saveVersion(storedTextDefinition(t, "x"))
		unit := f.seed.unit("Engine 1", nil)
		stored := f.storedTransmission("x", unit)
		// Without message.read no appearance of the Message is eligible, so the Transmission is hidden.
		revokeManagementAction(t, f.store, managementOperatorID(t, f.store), "message.read")
		f.preview(storedBody(version, stored), http.StatusNotFound)
	})
	t.Run("unreadable group", func(t *testing.T) {
		t.Parallel()
		f := newPreviewHTTPFixture(t, []string{"unit", "payload", "gateway", "derived"})
		version := f.saveVersion(storedTextDefinition(t, "x"))
		stored := f.storedTransmission("x", f.seed.unit("Engine 1", nil))
		before := processingPreviewSideEffects(t, f.store)
		response := f.preview(storedBody(version, stored), http.StatusForbidden)
		if response.Code != "FORBIDDEN" {
			t.Fatalf("code = %q", response.Code)
		}
		if after := processingPreviewSideEffects(t, f.store); after != before {
			t.Fatalf("forbidden preview wrote rows: before=%v after=%v", before, after)
		}
	})
}

// TestStoredPreviewRefusesATransmissionWithAnInvisibleTarget proves that a readable Transmission
// one of whose original targets the editor cannot see (here a suppressed one) is not evaluated, so
// the preview cannot expose the hidden target through an extracted output.
func TestStoredPreviewRefusesATransmissionWithAnInvisibleTarget(t *testing.T) {
	t.Parallel()
	f := newPreviewHTTPFixture(t, []string{"unit", "address", "payload", "gateway", "derived"})
	version := f.saveVersion(storedTextDefinition(t, "x"))
	stored := f.storedTransmission("x", f.seed.unit("Engine 1", nil), f.seed.unit("Engine 2", nil))
	f.preview(storedBody(version, stored), http.StatusOK)

	f.seed.exec(`INSERT INTO target_suppressions (id, transmission_id, target_position, created_at, created_by)
		VALUES ($1, $2, 1, now(), $3)`, uuid.New(), stored, managementOperatorID(t, f.store))
	before := processingPreviewSideEffects(t, f.store)
	f.preview(storedBody(version, stored), http.StatusForbidden)
	if after := processingPreviewSideEffects(t, f.store); after != before {
		t.Fatalf("refused preview wrote rows: before=%v after=%v", before, after)
	}
}
