package events

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestConfigurationFactsKeepSeparateTypedProducerAndNonsecretEnvelope(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 12, 0, 0, 123000, time.UTC)
	for _, aggregate := range []string{"unit", "unitCollection", "addressRule"} {
		fact := ConfigurationFact{ID: uuid.New(), Aggregate: EntityRef{Type: aggregate, ID: uuid.New()},
			Revision: 1, OccurredAt: now, ChangeKind: "created"}
		envelope, err := fact.Envelope(now)
		if err != nil {
			t.Fatal(err)
		}
		domain, version := "masterdata", "master-data-core1.0"
		if aggregate == "addressRule" {
			domain, version = "assignment", "current-matcher1.0"
		}
		if envelope.Producer.Domain != domain || envelope.Producer.Version != version ||
			envelope.Type != ConfigurationChanged || envelope.Classification != "configuration" || envelope.ReplayMode != "normal" {
			t.Fatal("configuration fact forged another producer or event family")
		}
		data, err := json.Marshal(envelope)
		if err != nil || !strings.Contains(string(data), `"aggregateRevision":"1"`) || !strings.Contains(string(data), `"data":{"changeKind":"created"}`) {
			t.Fatalf("configuration envelope changed its typed shape: %v", err)
		}
		fact.Aggregate.Type = "account"
		if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatal("configuration producer accepted an Identity aggregate")
		}
		fact.Aggregate.Type = aggregate
		fact.ChangeKind = "secret-changed"
		if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
			t.Fatal("undeclared configuration change kind accepted")
		}
	}
}

func TestMasterDataFactAcceptsAttributeSchemaAggregate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 28, 12, 0, 0, 123000, time.UTC)
	fact := MasterDataFact{ID: uuid.New(), Aggregate: EntityRef{Type: "attributeSchema", ID: uuid.New()},
		Revision: 2, OccurredAt: now, ChangeKind: "updated"}
	envelope, err := fact.Envelope(now)
	if err != nil || envelope.Type != ConfigurationChanged || envelope.Classification != "configuration" ||
		envelope.AggregateRef.Type != "attributeSchema" || envelope.AggregateRevision != 2 {
		t.Fatalf("envelope = %+v, %v", envelope, err)
	}
	fact.Aggregate.Type = "schema"
	if _, err := fact.Envelope(now); !errors.Is(err, ErrInvalidFact) {
		t.Fatal("master-data fact accepted an unknown aggregate")
	}
}
