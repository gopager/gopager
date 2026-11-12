package readstore_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
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
// deliberately not a compilable one: visibility must not depend on it. Its attribute values are
// projected as publication does.
func (w *world) result(transmission, workflow, version uuid.UUID, message uuid.UUID, published time.Time, groups, evaluation string) uuid.UUID {
	candidate, result := uuid.New(), uuid.New()
	w.serial++
	w.exec(`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision,
		configuration_revision, source_revision, context_revision, state, selected, due_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, 1, $5, 1, 'completed', true, $6, $6, $6)`, candidate, transmission, workflow, version, w.serial, base)
	w.exec(`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
		VALUES ($1, $2, 1, $3::jsonb, $4::jsonb, $5, $5, $6)`, result, candidate, evaluation, groups, published, message)
	w.exec(processingstore.ProjectAttributeValuesSQL, []uuid.UUID{result})
	return result
}

// entry renders one present field value of a result.
func entry(name, kind, raw string) string {
	return `"` + name + `":{"state":"present","value":` + value(kind, raw) + `}`
}

// fieldsOf renders the `fields` of a result: the entries of one schema key.
func fieldsOf(key string, entries ...string) string {
	return `"` + key + `":{` + strings.Join(entries, ",") + `}`
}

// untargeted renders a Transmission-level result document.
func untargeted(groups ...string) string {
	return `{"selected":true,"fields":{` + strings.Join(groups, ",") + `}}`
}

// targeted renders a target-level result document with one target position.
func targeted(position int, groups ...string) string {
	return fmt.Sprintf(`{"selected":true,"targetResults":[{"targetPosition":%d,"fields":{%s}}]}`, position, strings.Join(groups, ","))
}

// attributeKeys indexes effective attribute values by "key.field" or "key.field@position".
func attributeKeys(records []readmodel.AttributeRecord) map[string]readmodel.AttributeRecord {
	out := map[string]readmodel.AttributeRecord{}
	for _, record := range records {
		name := record.SchemaKey + "." + record.Field
		if record.TargetPosition != nil {
			name += fmt.Sprintf("@%d", *record.TargetPosition)
		}
		out[name] = record
	}
	return out
}

// TestEffectiveAttributesTakeTheLatestPresentValuePerPosition covers the projection: the latest
// result per schema field wins whichever Workflow wrote it, a position shows only for a visible
// target, and a value carries the result it comes from.
func TestEffectiveAttributesTakeTheLatestPresentValuePerPosition(t *testing.T) {
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
	const groups = `["derived","payload"]`
	w.result(visible, workflowOne, versionOne, message, base.Add(time.Minute), groups,
		untargeted(fieldsOf("incident", entry("level", "integer", `"3"`))))
	w.result(visible, workflowOne, versionOne, message, base.Add(2*time.Minute), groups,
		untargeted(fieldsOf("incident", entry("name", "string", `"Alt"`))))
	newest := w.result(visible, workflowOne, versionOne, message, base.Add(3*time.Minute), groups,
		untargeted(fieldsOf("incident", entry("level", "integer", `"7"`), entry("urgent", "boolean", `true`),
			entry("ratio", "number", `1.5`))))
	renamed := w.result(visible, workflowTwo, versionTwo, message, base.Add(4*time.Minute), groups,
		untargeted(fieldsOf("incident", entry("name", "string", `"Brand"`))))
	w.result(visible, workflowTwo, versionTwo, message, base.Add(time.Minute), groups,
		targeted(0, fieldsOf("incident", entry("city", "string", `"Alt"`))))
	city := w.result(visible, workflowTwo, versionTwo, message, base.Add(2*time.Minute), groups,
		targeted(0, fieldsOf("incident", entry("city", "string", `"Bonn"`))))
	w.result(visible, workflowTwo, versionTwo, message, base.Add(time.Minute), groups,
		targeted(5, fieldsOf("incident", entry("city", "string", `"Nowhere"`))))
	w.result(foreign, workflowTwo, versionTwo, message, base.Add(time.Minute), groups,
		untargeted(fieldsOf("incident", entry("note", "string", `"hidden"`))))
	w.result(visible, workflowTwo, versionTwo, message, base.Add(time.Minute), `["derived","gateway"]`,
		untargeted(fieldsOf("incident", entry("note", "string", `"classified"`))))
	caller := principalForUnits(a)
	now := base.Add(time.Hour)
	item := w.feed(caller, now, readmodel.FeedQuery{DerivedGroups: []string{"derived", "payload"}}).Items[0]
	got := attributeKeys(item.Attributes)
	if len(got) != 5 {
		t.Fatalf("attributes = %+v", got)
	}
	for name, want := range map[string]struct {
		result, workflow, version uuid.UUID
		raw                       string
	}{
		"incident.level":  {newest, workflowOne, versionOne, `{"kind":"integer","value":"7"}`},
		"incident.urgent": {newest, workflowOne, versionOne, `{"kind":"boolean","value":true}`},
		"incident.ratio":  {newest, workflowOne, versionOne, `{"kind":"number","value":1.5}`},
		"incident.name":   {renamed, workflowTwo, versionTwo, `{"kind":"string","value":"Brand"}`},
		"incident.city@0": {city, workflowTwo, versionTwo, `{"kind":"string","value":"Bonn"}`},
	} {
		record, ok := got[name]
		if !ok {
			t.Errorf("%s is missing", name)
			continue
		}
		if record.ResultID != want.result || record.WorkflowID != want.workflow || record.DefinitionVersionID != want.version ||
			!strings.EqualFold(strings.ReplaceAll(string(record.Value), " ", ""), want.raw) {
			t.Errorf("%s = %+v, want result %v workflow %v value %s", name, record, want.result, want.workflow, want.raw)
		}
	}
	if got["incident.city@0"].UnitID == nil || *got["incident.city@0"].UnitID != a ||
		!got["incident.city@0"].PublishedAt.Equal(base.Add(2*time.Minute)) {
		t.Errorf("target value = %+v", got["incident.city@0"])
	}
	if _, exists := got["incident.city@5"]; exists {
		t.Error("a position without a visible target must not show")
	}
	if got["incident.note"].ResultID != uuid.Nil {
		t.Error("a value of a hidden Transmission or an unreadable group must not show")
	}
	if none := w.feed(caller, now, readmodel.FeedQuery{}).Items[0]; len(none.Attributes) != 0 {
		t.Fatalf("no readable derived group shows no attributes: %+v", none.Attributes)
	}
	detail, err := w.detail(caller, now, message, "derived", "payload")
	if err != nil || len(detail.Attributes) != 5 {
		t.Fatalf("detail = %+v, %v", detail.Attributes, err)
	}
}

// TestAttributeVisibilityFollowsStoredGroupsOnly shows that a result needs nothing but its stored
// groups: the Workflow version row holds no usable definition, and a caller reading only
// `derived` sees exactly the results classified with `derived` alone.
func TestAttributeVisibilityFollowsStoredGroupsOnly(t *testing.T) {
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
		untargeted(fieldsOf("incident", entry("level", "integer", `"1"`))))
	w.result(transmission, workflowTwo, versionTwo, message, base.Add(time.Minute), `["derived","payload"]`,
		untargeted(fieldsOf("incident", entry("name", "string", `"classified"`))))
	got := w.feed(principalForUnits(a), base.Add(time.Hour), readmodel.FeedQuery{DerivedGroups: []string{"derived"}}).Items[0]
	if len(got.Attributes) != 1 || got.Attributes[0].ResultID != plain {
		t.Fatalf("derived-only caller sees %+v", got.Attributes)
	}
	detail, err := w.detail(principalForUnits(a), base.Add(time.Hour), message, "derived")
	if err != nil || len(detail.Attributes) != 1 || detail.Attributes[0].ResultID != plain {
		t.Fatalf("derived-only detail = %+v, %v", detail.Attributes, err)
	}
}

// TestAttributeFiltersMatchTheEffectiveValue covers the filter against effective values: a
// superseded value is stale, a hidden Transmission or group never matches, a property path
// descends into objects and a list matches on any element.
func TestAttributeFiltersMatchTheEffectiveValue(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	agency := w.agency("county")
	a, hiddenUnit := w.unit(agency, "A", nil), w.unit(agency, "H", nil)
	network := w.network(agency, "N", "pocsag")
	message := w.message(base, "text", "alarm")
	visible := w.assigned(message, network, a, agency, base, "")
	foreign := w.assigned(message, network, hiddenUnit, agency, base, "")
	workflow, version := w.workflow()
	const groups = `["derived","payload"]`
	w.result(visible, workflow, version, message, base.Add(time.Minute), groups,
		untargeted(fieldsOf("incident", entry("level", "integer", `"3"`))))
	w.result(visible, workflow, version, message, base.Add(2*time.Minute), groups,
		untargeted(fieldsOf("incident", entry("level", "integer", `"7"`), entry("name", "string", `"Brand"`),
			entry("tags", "list", `[`+value("string", `"x"`)+`,`+value("string", `"y"`)+`]`),
			entry("owner", "object", `{"name":`+value("string", `"Ada"`)+`}`))))
	w.result(visible, workflow, version, message, base.Add(2*time.Minute), groups,
		targeted(0, fieldsOf("incident", entry("city", "string", `"Bonn"`))))
	w.result(foreign, workflow, version, message, base.Add(time.Minute), groups,
		untargeted(fieldsOf("incident", entry("city", "string", `"Köln"`))))
	w.result(visible, workflow, version, message, base.Add(time.Minute), `["derived","gateway"]`,
		untargeted(fieldsOf("incident", entry("ratio", "number", `2.5`))))
	caller := principalForUnits(a)
	now := base.Add(time.Hour)
	readable := []string{"derived", "payload"}
	for name, test := range map[string]struct {
		predicate string
		groups    []string
		want      int
	}{
		"integer equals":        {compare("eq", "attributes.incident.level", "integer", `"7"`), readable, 1},
		"superseded is stale":   {compare("eq", "attributes.incident.level", "integer", `"3"`), readable, 0},
		"integer range":         {compare("gte", "attributes.incident.level", "integer", `"7"`), readable, 1},
		"string case sensitive": {compare("eq", "attributes.incident.name", "string", `"brand"`), readable, 0},
		"contains":              {compare("contains", "attributes.incident.name", "string", `"ran"`), readable, 1},
		"list any element":      {compare("eq", "attributes.incident.tags", "string", `"y"`), readable, 1},
		"property path":         {compare("eq", "attributes.incident.owner.name", "string", `"Ada"`), readable, 1},
		"absent property":       {present("attributes.incident.owner.age", true), readable, 0},
		"target value":          {compare("eq", "attributes.incident.city", "string", `"Bonn"`), readable, 1},
		"hidden transmission":   {compare("eq", "attributes.incident.city", "string", `"Köln"`), readable, 0},
		"hidden group":          {compare("eq", "attributes.incident.ratio", "number", `2.5`), readable, 0},
		"not present":           {present("attributes.incident.ratio", false), readable, 1},
		"no derived group":      {compare("eq", "attributes.incident.level", "integer", `"7"`), nil, 0},
	} {
		result := w.feed(caller, now, readmodel.FeedQuery{Selection: compile(t, test.predicate), DerivedGroups: test.groups})
		if len(result.Items) != test.want {
			t.Errorf("%s: %d rows, want %d", name, len(result.Items), test.want)
		}
	}
}

func principalForUnits(units ...uuid.UUID) authz.Authority {
	return principal(grant(scope("unit", units...), nil, "message.read", "message.detail"))
}
