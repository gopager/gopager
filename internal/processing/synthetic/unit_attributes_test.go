package synthetic

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// testUnitAttributes declares the one Unit attribute the previewed definition reads, as the
// production reader does, and records the definition it was asked about.
type testUnitAttributes struct {
	unit  uuid.UUID
	value processing.Value
	// reads are the Unit attribute references the previewed definition reads.
	reads []processing.FieldRef
	asked *processing.Definition
}

func (a testUnitAttributes) ReadUnitAttributes(_ context.Context, _ platform.TransactionContext,
	definition processing.Definition, ids []uuid.UUID,
) (declarations.UnitAttributes, error) {
	if a.asked != nil {
		*a.asked = definition
	}
	registry := fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{{
		Key: "station", Subject: attributes.SubjectUnit,
		Fields: map[string]attributes.Field{"town": {Kind: processing.StringValue}, "region": {Kind: processing.StringValue}},
	}}, 1))
	declared, err := declarations.UnitAttributeDeclarations(registry, a.reads)
	if err != nil {
		return declarations.UnitAttributes{}, err
	}
	result := declarations.UnitAttributes{Declared: declared,
		Effective: map[uuid.UUID]units.EffectiveAttributes{}}
	for _, id := range ids {
		if id == a.unit {
			result.Effective[id] = units.EffectiveAttributes{"station": {
				"town": {Value: &a.value, Source: units.SourceUnit}}}
		}
	}
	return result, nil
}

func TestSyntheticOwnerCapturesUnitAttributes(t *testing.T) {
	t.Parallel()
	unit := uuid.New()
	town, err := processing.Literal(processing.StringValue, "A")
	if err != nil {
		t.Fatal(err)
	}
	network := Network{ID: uuid.New(), Name: "Dispatch", Protocol: protocols.POCSAG, AgencyID: testAgency}
	var asked processing.Definition
	definition := processing.Definition{SchemaVersion: 1}
	owner, err := NewOwner(testMatcher{unitID: &unit}, testNetworks{network: network},
		testClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}, &testScope{},
		WithUnitAttributes(testUnitAttributes{unit: unit, value: town, asked: &asked,
			reads: []processing.FieldRef{{Namespace: "target", Path: []string{"unit", "attributes", "station", "town"}}}}))
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ReplaceAll(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK",`+
		`"payload":{"value":"hello"},"targets":[{"address":123,"function":1}]}`, "NETWORK", network.ID.String())
	projection, _ := captureDefinition(t, owner, testTx{ctx: context.Background()},
		testAuthority("payload", "gateway", "address", "unit"), testWorkflow(nil), definition, raw)
	if asked.SchemaVersion != definition.SchemaVersion {
		t.Fatalf("reader was not asked about the previewed definition: %+v", asked)
	}
	declared := 0
	for _, declaration := range projection.TargetDeclarations {
		if len(declaration.Reference.Path) > 1 && declaration.Reference.Path[1] == "attributes" {
			declared++
		}
	}
	if declared != 1 {
		t.Fatalf("preview declared %d Unit attributes, want only the one read", declared)
	}
	for _, field := range projection.Targets[0].Fields {
		if strings.Join(field.Reference.Path, ".") != "unit.attributes.station.town" {
			continue
		}
		if field.Field.State != processing.FieldPresent || field.Field.Value == nil || string(field.Field.Value.Value) != `"A"` {
			t.Fatalf("field=%+v", field.Field)
		}
		return
	}
	t.Fatal("preview did not capture the Unit attribute")
}
