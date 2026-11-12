package ingeststore

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// manyUnitFields returns a registry whose Unit schema declares count string fields, next to the
// Message schema `routing` a Workflow writes to.
func manyUnitFields(count int) *fields.Registry {
	declared := make(map[string]attributes.Field, count)
	for i := 0; i < count; i++ {
		declared[fmt.Sprintf("field%05d", i)] = attributes.Field{Kind: processing.StringValue}
	}
	return fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{
		{Key: "station", Subject: attributes.SubjectUnit, Fields: declared},
		{Key: "routing", Subject: attributes.SubjectMessage,
			Fields: map[string]attributes.Field{"town": {Kind: processing.StringValue}}},
	}, 1))
}

func unitField(name string) processing.FieldRef {
	return processing.FieldRef{Namespace: "target", Path: []string{"unit", "attributes", "station", name}}
}

// definitionReading returns a Workflow that selects POCSAG and writes the given Unit attributes
// to the message attribute `routing`; with no names it reads no Unit attribute.
func definitionReading(t *testing.T, names ...string) processing.Definition {
	t.Helper()
	protocol := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	pocsag, err := processing.Literal(processing.StringValue, "pocsag")
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocol, Value: &pocsag}}
	for _, name := range names {
		ref := unitField(name)
		if definition.Outputs == nil {
			definition.Outputs = map[string]map[string]processing.Assignment{"routing": {}}
		}
		definition.Outputs["routing"]["town"] = processing.Assignment{Field: &ref}
	}
	return definition
}

func referencedUnitFields(t *testing.T, _ *fields.Registry, definition processing.Definition) []processing.FieldRef {
	t.Helper()
	refs, err := declarations.DefinitionUnitAttributeRefs(definition)
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

// TestOnlyReadUnitAttributesAreCaptured shows that a catalog too large to capture completely
// still processes a Workflow that reads no Unit attribute, and that a Workflow reading one
// captures exactly that one.
func TestOnlyReadUnitAttributesAreCaptured(t *testing.T) {
	t.Parallel()
	const catalog = 25000
	registry := manyUnitFields(catalog)
	var payload protocols.TransmissionPayload
	if err := json.Unmarshal([]byte(`{"form":"numeric","value":"123"}`), &payload); err != nil {
		t.Fatal(err)
	}
	function := uint8(2)
	unit := uuid.New()
	targets := []protocols.Target{{Address: protocols.NumericAddress(1), Function: &function}}
	matched := assignments.MatchResult{Revision: 7, Resolutions: []assignments.Resolution{
		{TargetPosition: 0, Status: "assigned", UnitID: &unit}},
		UnitContext: map[uuid.UUID]units.Context{unit: {ID: unit, Revision: 1}}}
	capture := func(source declarations.UnitAttributes) error {
		t.Helper()
		projection, err := projectProcessingSource(context.Background(), uuid.New(), uuid.New(), "Network",
			protocols.POCSAG, payload, time.Date(2026, time.November, 4, 12, 0, 0, 0, time.UTC), 1, matched, targets, source)
		if err != nil {
			t.Fatal(err)
		}
		_, err = processing.CaptureSource(context.Background(), projection)
		return err
	}
	declare := func(refs []processing.FieldRef) declarations.UnitAttributes {
		t.Helper()
		declared, err := declarations.UnitAttributeDeclarations(registry, refs)
		if err != nil {
			t.Fatal(err)
		}
		return declarations.UnitAttributes{Declared: declared, Effective: map[uuid.UUID]units.EffectiveAttributes{}}
	}

	var every []processing.FieldRef
	for i := 0; i < catalog; i++ {
		every = append(every, unitField(fmt.Sprintf("field%05d", i)))
	}
	if err := capture(declare(every)); err == nil {
		t.Fatal("capturing the whole catalog fits the input limit; enlarge the catalog of this test")
	}

	none := referencedUnitFields(t, registry, definitionReading(t))
	if len(none) != 0 {
		t.Fatalf("a Workflow reading no Unit attribute references %v", none)
	}
	if err := capture(declare(none)); err != nil {
		t.Fatalf("a Workflow reading no Unit attribute does not process: %v", err)
	}

	one := referencedUnitFields(t, registry, definitionReading(t, "field00042"))
	if len(one) != 1 || one[0].Path[3] != "field00042" {
		t.Fatalf("references=%v, want only field00042", one)
	}
}

// TestReferencesOfAStoredDefinition collects the references of a definition decoded from its
// stored JSON form, which is what the owner reads.
func TestReferencesOfAStoredDefinition(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(definitionReading(t, "field00001"))
	if err != nil {
		t.Fatal(err)
	}
	var stored processing.Definition
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	refs := referencedUnitFields(t, manyUnitFields(3), stored)
	if len(refs) != 1 || refs[0].Path[3] != "field00001" {
		t.Fatalf("references=%v", refs)
	}
}
