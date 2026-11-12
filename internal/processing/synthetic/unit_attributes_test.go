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

type testUnitAttributes struct {
	unit  uuid.UUID
	value processing.Value
}

func (a testUnitAttributes) ReadUnitAttributes(_ context.Context, _ platform.TransactionContext, ids []uuid.UUID,
) (declarations.UnitAttributes, error) {
	registry := fields.New(fields.Builtin(), attributes.NewProvider([]attributes.Schema{{
		Key: "station", Subject: attributes.SubjectUnit,
		Fields: map[string]attributes.Field{"town": {Kind: processing.StringValue}},
	}}, 1))
	result := declarations.UnitAttributes{Declared: declarations.UnitAttributeDeclarations(registry),
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
	owner, err := NewOwner(testMatcher{unitID: &unit}, testNetworks{network: network},
		testClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}, &testScope{},
		WithUnitAttributes(testUnitAttributes{unit: unit, value: town}))
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ReplaceAll(`{"schemaVersion":1,"protocol":"pocsag","form":"text","networkId":"NETWORK",`+
		`"payload":{"value":"hello"},"targets":[{"address":123,"function":1}]}`, "NETWORK", network.ID.String())
	projection, _ := capture(t, owner, network, testTx{ctx: context.Background()}, testAuthority("payload", "gateway", "address", "unit"),
		testWorkflow(nil), raw)
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
