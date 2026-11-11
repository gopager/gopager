package readmodel

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

func sampleRecord() FeedRecord {
	agency := uuid.New()
	return FeedRecord{MessageID: uuid.New(), Time: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		ProcessingStatus: "completed", CorrectionStatus: "current",
		Representative: &PayloadRecord{Payload: json.RawMessage(`{"form":"text","value":"x"}`), Protocol: "pocsag"},
		Units:          []UnitRecord{{UnitID: uuid.New(), Name: "A", AgencyID: agency}},
		WithoutUnit:    []WithoutUnitRecord{{AgencyID: agency}}, UnresolvedAgencies: []uuid.UUID{agency}}
}

func TestFeedItemOmitsUnreadableGroups(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	encode := func(v viewer) string {
		out, err := json.Marshal(v.feedItem(record, false))
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	minimal := encode(viewer{groups: map[string]bool{}})
	for _, property := range []string{"payload", "protocol", "incomplete", "units", "withoutUnit", "derived", "networkUnresolved"} {
		if strings.Contains(minimal, `"`+property+`"`) {
			t.Errorf("minimal projection leaks %s: %s", property, minimal)
		}
	}
	for _, property := range []string{"messageId", "messageTime", "processingStatus", "correctionStatus"} {
		if !strings.Contains(minimal, `"`+property+`"`) {
			t.Errorf("minimal projection lacks %s", property)
		}
	}
	payloadOnly := encode(viewer{groups: map[string]bool{"payload": true}})
	if !strings.Contains(payloadOnly, `"payload":{"form":"text","value":"x"}`) || strings.Contains(payloadOnly, `"units"`) {
		t.Errorf("payload-only projection: %s", payloadOnly)
	}
	full := encode(viewer{groups: map[string]bool{"unit": true}})
	if !strings.Contains(full, `"units":[`) || !strings.Contains(full, `"withoutUnit":true`) || strings.Contains(full, "firstObservedAt") {
		t.Errorf("unit projection: %s", full)
	}
}

func TestNetworkUnresolvedNeedsNetworkReach(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	reach := authz.AgencyReach{Agencies: record.UnresolvedAgencies}
	if item := (viewer{groups: map[string]bool{}, networkReach: reach}).feedItem(record, false); item.NetworkUnresolved == nil || !*item.NetworkUnresolved {
		t.Fatal("reachable Network agency must show the marker")
	}
	if item := (viewer{groups: map[string]bool{}}).feedItem(record, false); item.NetworkUnresolved != nil {
		t.Fatal("unreachable Network agency must not show the marker")
	}
}

func TestAttributesAreOrderedAndGated(t *testing.T) {
	t.Parallel()
	record := sampleRecord()
	zero, one, unit := 0, 1, uuid.New()
	value := json.RawMessage(`{"kind":"string","value":"x"}`)
	record.Attributes = []AttributeRecord{
		{SchemaKey: "b", Field: "f", Value: value},
		{SchemaKey: "a", Field: "g", Value: value},
		{SchemaKey: "a", Field: "f", TargetPosition: &one, UnitID: &unit, Value: value},
		{SchemaKey: "a", Field: "f", TargetPosition: &zero, Value: value},
		{SchemaKey: "a", Field: "f", Value: value},
		{SchemaKey: "a", Field: "broken", Value: json.RawMessage(`{`)},
	}
	item := viewer{groups: map[string]bool{"derived": true}}.feedItem(record, false)
	if item.Attributes == nil || len(*item.Attributes) != 5 {
		t.Fatalf("attributes = %+v", item.Attributes)
	}
	var order []string
	for _, attribute := range *item.Attributes {
		position := "-"
		if attribute.TargetPosition != nil {
			position = strconv.Itoa(*attribute.TargetPosition)
		}
		if attribute.UnitID != nil {
			t.Error("the Unit of a target needs the unit group")
		}
		order = append(order, attribute.SchemaKey+"."+attribute.Field+"@"+position)
	}
	if want := []string{"a.f@-", "a.f@0", "a.f@1", "a.g@-", "b.f@-"}; !slices.Equal(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	withUnits := viewer{groups: map[string]bool{"derived": true, "unit": true}}.feedItem(record, false)
	if got := (*withUnits.Attributes)[2].UnitID; got == nil || *got != unit {
		t.Fatalf("unit = %v", got)
	}
	if (viewer{groups: map[string]bool{}}).feedItem(record, false).Attributes != nil {
		t.Fatal("derived group must gate the property")
	}
}

func TestOptionsFamiliesFollowGroups(t *testing.T) {
	t.Parallel()
	record := OptionsRecord{Units: []UnitOption{{ID: uuid.New(), Name: "A"}}, Networks: []NamedOption{{ID: uuid.New(), Name: "N"}}}
	item := viewer{groups: map[string]bool{"payload": true}}.optionsItem(record)
	if item.Units != nil || item.Organizations != nil || item.Agencies != nil || item.Networks != nil || item.Protocols == nil {
		t.Fatalf("payload-only options = %+v", item)
	}
	item = viewer{groups: map[string]bool{"unit": true, "gateway": true}}.optionsItem(record)
	if item.Units == nil || item.Networks == nil || item.Protocols != nil {
		t.Fatalf("unit+gateway options = %+v", item)
	}
}
