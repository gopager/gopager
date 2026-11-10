package readmodel

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
)

func ptr[T any](value T) *T { return &value }

func record(t *testing.T, factType string, version int, aggregate string, data map[string]any) events.Record {
	t.Helper()
	envelope, err := json.Marshal(map[string]any{"schemaVersion": version, "type": factType,
		"aggregateRef": map[string]any{"type": aggregate, "id": uuid.NewString()}, "data": data})
	if err != nil {
		t.Fatal(err)
	}
	return events.Record{Seq: 7, TxID: 3, Type: factType, Envelope: envelope}
}

func TestIntersectsRange(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	from, until := base, base.Add(time.Hour)
	tests := []struct {
		name     string
		time     *time.Time
		previous *time.Time
		until    *time.Time
		want     bool
	}{
		{"inside", ptr(base.Add(time.Minute)), nil, &until, true},
		{"at from", ptr(base), nil, &until, true},
		{"before from", ptr(base.Add(-time.Minute)), nil, &until, false},
		{"at until is excluded", ptr(until), nil, &until, false},
		{"late join interval reaches in", ptr(base.Add(-time.Hour)), ptr(base.Add(time.Minute)), &until, true},
		{"interval ends before from", ptr(base.Add(-2 * time.Hour)), ptr(base.Add(-time.Hour)), &until, false},
		{"moved out of the range later", ptr(until.Add(time.Minute)), nil, &until, false},
		{"interval starts after until", ptr(until.Add(time.Minute)), ptr(until.Add(time.Hour)), &until, false},
		{"no until", ptr(base.Add(24 * time.Hour)), nil, nil, true},
		{"missing time is relevant", nil, nil, &until, true},
	}
	for _, test := range tests {
		if got := intersectsRange(test.time, test.previous, &from, test.until); got != test.want {
			t.Errorf("%s: got %v, want %v", test.name, got, test.want)
		}
	}
}

func TestDecodeLiveFactHandlesTypesAndVersions(t *testing.T) {
	t.Parallel()
	message := uuid.New()
	inRange := map[string]any{"messageId": message, "messageTime": "2026-01-01T12:30:00Z"}
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)

	if fact, outcome := decodeLiveFact(record(t, "message.created", 1, "message", inRange), &from, &until); outcome != outcomeFact ||
		fact.kind != kindAdd || fact.messageID != message || fact.position != (events.Position{TxID: 3, Seq: 7}) {
		t.Fatalf("message.created: %+v %v", fact, outcome)
	}
	for _, ignored := range []string{"gateway.state-changed", "operation.network-ambiguous", "correction.failed", "identity.login"} {
		if _, outcome := decodeLiveFact(record(t, ignored, 99, "gateway", inRange), &from, &until); outcome != outcomeIgnore {
			t.Errorf("%s must be ignored regardless of version, got %v", ignored, outcome)
		}
	}
	for _, handled := range []string{"message.created", "message.transmission-joined", "message.merged", "assignment.changed",
		"suppression.changed", "processing.completed", "configuration.changed"} {
		if _, outcome := decodeLiveFact(record(t, handled, 2, "unit", inRange), &from, &until); outcome != outcomeResync {
			t.Errorf("%s with an unknown schema version must resync, got %v", handled, outcome)
		}
	}
	outside := map[string]any{"messageId": message, "messageTime": "2026-01-02T12:30:00Z"}
	if _, outcome := decodeLiveFact(record(t, "message.created", 1, "message", outside), &from, &until); outcome != outcomeIgnore {
		t.Fatalf("a fact outside the loaded range must be ignored, got %v", outcome)
	}
}

func TestDecodeLiveFactConfigurationAggregates(t *testing.T) {
	t.Parallel()
	from := time.Now()
	for aggregate, want := range map[string]liveOutcome{"unit": outcomeResync, "unitCollection": outcomeResync,
		"organization": outcomeResync, "agency": outcomeResync, "addressRule": outcomeIgnore, "workflow": outcomeIgnore,
		"unitTemplate": outcomeIgnore} {
		if _, outcome := decodeLiveFact(record(t, "configuration.changed", 1, aggregate, map[string]any{}), &from, nil); outcome != want {
			t.Errorf("configuration.changed %s: got %v, want %v", aggregate, outcome, want)
		}
	}
}

func TestDecodeLiveFactKindsAndBefore(t *testing.T) {
	t.Parallel()
	message, unitA, unitB, previousNetwork, network := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	decode := func(factType string, data map[string]any) liveFact {
		t.Helper()
		data["messageId"], data["messageTime"] = message, "2026-01-01T12:00:00Z"
		fact, outcome := decodeLiveFact(record(t, factType, 1, "message", data), &from, nil)
		if outcome != outcomeFact {
			t.Fatalf("%s: outcome %v", factType, outcome)
		}
		return fact
	}
	assigned := func(unit uuid.UUID) map[string]any { return map[string]any{"status": "assigned", "unitId": unit} }
	unassigned := map[string]any{"status": "unassigned"}

	if fact := decode("assignment.changed", map[string]any{"reason": "created"}); fact.kind != kindAdd {
		t.Errorf("created assignment: kind %v", fact.kind)
	}
	if fact := decode("suppression.changed", map[string]any{"changeKind": "lifted"}); fact.kind != kindAdd {
		t.Errorf("lifted suppression: kind %v", fact.kind)
	}
	corrected := decode("assignment.changed", map[string]any{"reason": "corrected", "networkId": network, "targets": []any{
		map[string]any{"position": 0, "before": assigned(unitA), "after": assigned(unitB)},
		map[string]any{"position": 1, "before": unassigned, "after": assigned(unitB)},
		map[string]any{"position": 2, "before": map[string]any{"status": "pending"}, "after": unassigned},
	}})
	if corrected.kind != kindChange || len(corrected.before.units) != 1 || corrected.before.units[0] != unitA ||
		len(corrected.before.networks) != 1 || corrected.before.networks[0] != network {
		t.Errorf("corrected assignment before basis: %+v", corrected)
	}
	moved := decode("assignment.changed", map[string]any{"reason": "corrected", "networkId": network, "previousNetworkId": previousNetwork,
		"targets": []any{map[string]any{"position": 0, "before": unassigned, "after": assigned(unitB)}}})
	if len(moved.before.networks) != 1 || moved.before.networks[0] != previousNetwork {
		t.Errorf("a moved Transmission's Unit-less context is the previous Network: %+v", moved.before)
	}
	targetless := decode("assignment.changed", map[string]any{"reason": "corrected", "networkId": network, "previousNetworkId": previousNetwork})
	if len(targetless.before.networks) != 1 || targetless.before.networks[0] != previousNetwork {
		t.Errorf("a move without target changes: %+v", targetless.before)
	}
	pendingOnly := decode("assignment.changed", map[string]any{"reason": "corrected", "networkId": network, "targets": []any{
		map[string]any{"position": 0, "before": map[string]any{"status": "pending"}, "after": assigned(unitB)}}})
	if len(pendingOnly.before.units)+len(pendingOnly.before.networks) != 0 {
		t.Errorf("a pending before target names no appearance context: %+v", pendingOnly.before)
	}
	suppressedUnit := decode("suppression.changed", map[string]any{"changeKind": "created", "unitId": unitA, "networkId": network})
	if suppressedUnit.kind != kindChange || len(suppressedUnit.before.units) != 1 || len(suppressedUnit.before.networks) != 0 {
		t.Errorf("suppression of an assigned target: %+v", suppressedUnit)
	}
	suppressedLess := decode("suppression.changed", map[string]any{"changeKind": "created", "networkId": network})
	if len(suppressedLess.before.networks) != 1 || len(suppressedLess.before.units) != 0 {
		t.Errorf("suppression of a Unit-less target: %+v", suppressedLess)
	}
	absorbed := []uuid.UUID{uuid.New(), uuid.New()}
	joining := uuid.New()
	merged := decode("message.merged", map[string]any{"absorbedMessageIds": absorbed, "transmissionId": joining})
	if merged.kind != kindMerge || fmt.Sprint(merged.absorbed) != fmt.Sprint(absorbed) || merged.transmissionID != joining {
		t.Errorf("merged: %+v", merged)
	}
	if fact := decode("processing.completed", map[string]any{}); fact.kind != kindChange {
		t.Errorf("processing.completed: kind %v", fact.kind)
	}
}
