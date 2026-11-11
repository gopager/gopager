package readstore_test

import (
	"github.com/gopager/gopager/internal/authz"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/readmodel"
)

// workflow inserts a Workflow with one definition version and returns both IDs.
func (w *world) workflow() (workflow, version uuid.UUID) {
	workflow, version = uuid.New(), uuid.New()
	w.exec(`INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', $2, $2)`, workflow, base)
	w.exec(`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) SELECT $1, id FROM agencies`, workflow)
	w.exec(`INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, '{}', $3, $4)`,
		version, workflow, base, uuid.New())
	return workflow, version
}

// result publishes a Processing result of a Transmission with its stored field groups, a JSON
// array; evaluation must be a valid result document. The Workflow version's definition is
// deliberately not a compilable one: visibility must not depend on it.
func (w *world) result(transmission, workflow, version uuid.UUID, message uuid.UUID, published time.Time, groups, evaluation string) uuid.UUID {
	candidate, result := uuid.New(), uuid.New()
	w.serial++
	w.exec(`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision,
		configuration_revision, source_revision, context_revision, state, selected, due_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, 1, $5, 1, 'completed', true, $6, $6, $6)`, candidate, transmission, workflow, version, w.serial, base)
	w.exec(`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
		VALUES ($1, $2, 1, $3::jsonb, $4::jsonb, $5, $5, $6)`, result, candidate, evaluation, groups, published, message)
	return result
}

func field(kind, value string) string {
	return `{"state":"present","value":{"kind":"` + kind + `","value":` + value + `}}`
}

// TestDerivedResultsLatestPerWorkflowAndFilter covers the derived projection, the classification
// hiding and the derived filter.
func TestDerivedResultsLatestPerWorkflowAndFilter(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, hiddenUnit := w.unit(agency, "A", nil), w.unit(agency, "H", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	visible := w.assigned(message, network, a, agency, base, "")
	foreign := w.assigned(message, network, hiddenUnit, agency, base, "")
	workflowOne, versionOne := w.workflow()
	workflowTwo, versionTwo := w.workflow()
	const groupsOne, groupsTwo = `["derived","payload"]`, `["derived","gateway"]`
	// Workflow one: an older and a newer result; only the newer counts.
	w.result(visible, workflowOne, versionOne, message, base.Add(time.Minute), groupsOne,
		`{"selected":true,"fields":{"level":`+field("integer", `"3"`)+`}}`)
	newest := w.result(visible, workflowOne, versionOne, message, base.Add(2*time.Minute), groupsOne,
		`{"selected":true,"fields":{"level":`+field("integer", `"7"`)+`,"urgent":`+field("boolean", `true`)+`,"name":`+field("string", `"Brand"`)+`,"ratio":`+field("number", `1.5`)+`}}`)
	// Workflow two: a target-level result of the visible and of the hidden Transmission.
	w.result(visible, workflowTwo, versionTwo, message, base.Add(time.Minute), groupsTwo,
		`{"selected":true,"targetResults":[{"targetPosition":0,"fields":{"city":`+field("string", `"Bonn"`)+`}}]}`)
	w.result(foreign, workflowTwo, versionTwo, message, base.Add(time.Minute-time.Second), groupsTwo,
		`{"selected":true,"fields":{"city":`+field("string", `"Köln"`)+`}}`)
	caller := principalForUnits(a)
	now := base.Add(time.Hour)
	both := []string{"derived", "gateway", "payload"}
	item := w.feed(caller, now, readmodel.FeedQuery{DerivedGroups: both}).Items[0]
	if len(item.Derived) != 2 {
		t.Fatalf("derived = %d results", len(item.Derived))
	}
	byWorkflow := map[uuid.UUID]readmodel.DerivedRecord{}
	for _, d := range item.Derived {
		byWorkflow[d.WorkflowID] = d
	}
	if byWorkflow[workflowOne].ResultID != newest {
		t.Fatalf("latest result of workflow one = %v, want %v", byWorkflow[workflowOne].ResultID, newest)
	}
	if !slices.Equal(byWorkflow[workflowTwo].VisibleTargets, []int{0}) {
		t.Fatalf("visible targets = %v", byWorkflow[workflowTwo].VisibleTargets)
	}
	// Results with a stored group the caller cannot read are neither shown nor matched.
	only := w.feed(caller, now, readmodel.FeedQuery{DerivedGroups: []string{"derived", "gateway"}}).Items[0]
	if len(only.Derived) != 1 || only.Derived[0].WorkflowID != workflowTwo {
		t.Fatalf("version filter: %+v", only.Derived)
	}
	if got := w.feed(caller, now, readmodel.FeedQuery{}).Items[0]; len(got.Derived) != 0 {
		t.Fatalf("no derived group shows no derived result: %+v", got.Derived)
	}
	for name, test := range map[string]struct {
		conditions []readmodel.DerivedCondition
		groups     []string
		want       int
	}{
		"integer as number":      {[]readmodel.DerivedCondition{{Name: "level", Value: "7"}}, both, 1},
		"integer decimal form":   {[]readmodel.DerivedCondition{{Name: "level", Value: "7.0"}}, both, 1},
		"older value is stale":   {[]readmodel.DerivedCondition{{Name: "level", Value: "3"}}, both, 0},
		"boolean":                {[]readmodel.DerivedCondition{{Name: "urgent", Value: "true"}}, both, 1},
		"boolean false":          {[]readmodel.DerivedCondition{{Name: "urgent", Value: "false"}}, both, 0},
		"string":                 {[]readmodel.DerivedCondition{{Name: "name", Value: "Brand"}}, both, 1},
		"string case sensitive":  {[]readmodel.DerivedCondition{{Name: "name", Value: "brand"}}, both, 0},
		"number":                 {[]readmodel.DerivedCondition{{Name: "ratio", Value: "1.5"}}, both, 1},
		"same name alternatives": {[]readmodel.DerivedCondition{{Name: "level", Value: "1"}, {Name: "level", Value: "7"}}, both, 1},
		"different names AND":    {[]readmodel.DerivedCondition{{Name: "level", Value: "7"}, {Name: "name", Value: "Feuer"}}, both, 0},
		"target result":          {[]readmodel.DerivedCondition{{Name: "city", Value: "Bonn"}}, both, 1},
		"hidden transmission":    {[]readmodel.DerivedCondition{{Name: "city", Value: "Köln"}}, both, 0},
		"hidden group":           {[]readmodel.DerivedCondition{{Name: "level", Value: "7"}}, []string{"derived", "gateway"}, 0},
		"no derived group":       {[]readmodel.DerivedCondition{{Name: "level", Value: "7"}}, nil, 0},
	} {
		result := w.feed(caller, now, readmodel.FeedQuery{Filters: readmodel.Filters{Derived: test.conditions}, DerivedGroups: test.groups})
		if len(result.Items) != test.want {
			t.Errorf("%s: %d rows, want %d", name, len(result.Items), test.want)
		}
	}
}

// TestDerivedVisibilityFollowsStoredGroupsOnly shows that a result needs nothing but its stored
// groups: the Workflow version row holds no usable definition, and a caller reading only
// `derived` sees exactly the results classified with `derived` alone.
func TestDerivedVisibilityFollowsStoredGroupsOnly(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a := w.unit(agency, "A", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	transmission := w.assigned(message, network, a, agency, base, "")
	workflowOne, versionOne := w.workflow()
	workflowTwo, versionTwo := w.workflow()
	plain := w.result(transmission, workflowOne, versionOne, message, base.Add(time.Minute), `["derived"]`,
		`{"selected":true,"fields":{"level":`+field("integer", `"1"`)+`}}`)
	w.result(transmission, workflowTwo, versionTwo, message, base.Add(time.Minute), `["derived","payload"]`,
		`{"selected":true,"fields":{"level":`+field("integer", `"2"`)+`}}`)
	got := w.feed(principalForUnits(a), base.Add(time.Hour), readmodel.FeedQuery{DerivedGroups: []string{"derived"}}).Items[0]
	if len(got.Derived) != 1 || got.Derived[0].ResultID != plain {
		t.Fatalf("derived-only caller sees %+v", got.Derived)
	}
	detail, err := w.detail(principalForUnits(a), base.Add(time.Hour), message, "derived")
	if err != nil || len(detail.Derived) != 1 || detail.Derived[0].ResultID != plain {
		t.Fatalf("derived-only detail = %+v, %v", detail.Derived, err)
	}
}

func principalForUnits(units ...uuid.UUID) authz.Authority {
	return principal(grant(scope("unit", units...), nil, "message.read", "message.detail"))
}
