package readmodel

import (
	"encoding/json"
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

func TestDerivedHiddenWithoutClassificationAndTargetsFiltered(t *testing.T) {
	t.Parallel()
	readable, hidden := uuid.New(), uuid.New()
	record := sampleRecord()
	evaluation := json.RawMessage(`{"selected":true,"targetResults":[
		{"targetPosition":0,"fields":{"a":{"state":"present","value":{"kind":"string","value":"x"}}}},
		{"targetPosition":1,"fields":{"a":{"state":"present","value":{"kind":"string","value":"y"}}}}]}`)
	record.Derived = []DerivedRecord{
		{WorkflowID: uuid.New(), ResultID: uuid.New(), VersionID: readable, Evaluation: evaluation, VisibleTargets: []int{1}},
		{WorkflowID: uuid.New(), ResultID: uuid.New(), VersionID: hidden, Evaluation: evaluation, VisibleTargets: []int{0, 1}},
	}
	v := viewer{groups: map[string]bool{"derived": true}, versions: map[uuid.UUID]bool{readable: true}}
	item := v.feedItem(record, false)
	if item.Derived == nil || len(*item.Derived) != 1 {
		t.Fatalf("derived = %+v", item.Derived)
	}
	results := (*item.Derived)[0].TargetResults
	if len(results) != 1 || results[0].TargetPosition != 1 {
		t.Fatalf("target results = %+v", results)
	}
	if (viewer{groups: map[string]bool{}, versions: map[uuid.UUID]bool{readable: true}}).feedItem(record, false).Derived != nil {
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
