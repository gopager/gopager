package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

// incidentFields declares the Message attribute schema "incident" that the Delivery tests use.
const incidentFields = `{"level":{"kind":"integer"},"city":{"kind":"string"},
	"owner":{"kind":"object","properties":{"name":{"kind":"string"}}}}`

// attributeSchema stores a message Attribute schema and bumps the catalog revision, so that the
// field registry sees it.
func (f *deliveryFixture) attributeSchema(key, fields string) {
	f.t.Helper()
	f.exec(`INSERT INTO attribute_schemas (key, revision, label, subject, fields, created_by, created_at, updated_at)
		VALUES ($1, 1, 'Incident', 'message', $2::jsonb, $3, now(), now())`, key, fields, uuid.New())
	f.exec(`UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
}

// replaceAttributeFields changes the field declarations of a schema and bumps the catalog revision.
func (f *deliveryFixture) replaceAttributeFields(key, fields string) {
	f.t.Helper()
	f.exec(`UPDATE attribute_schemas SET fields = $2::jsonb, revision = revision + 1 WHERE key = $1`, key, fields)
	f.exec(`UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
}

// resultWorkflow stores a Workflow with an active version, the producer of published results.
func (f *deliveryFixture) resultWorkflow() uuid.UUID {
	f.t.Helper()
	workflow, version := uuid.New(), uuid.New()
	f.exec(`INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
	f.exec(`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) SELECT $1, id FROM agencies`, workflow)
	f.exec(`INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, '{}'::jsonb, now(), $3)`,
		version, workflow, uuid.New())
	f.exec(`UPDATE processing_workflows SET active_version_id = $2 WHERE id = $1`, workflow, version)
	return workflow
}

// attributeEntry renders one present field of the schema "incident" in a result document.
func attributeEntry(field, kind, raw string) string {
	return `"` + field + `":{"state":"present","value":{"kind":"` + kind + `","value":` + raw + `}}`
}

// untargetedResult renders a Transmission-level result document for the schema "incident".
func untargetedResult(entries ...string) string {
	return `{"selected":true,"fields":{"incident":{` + strings.Join(entries, ",") + `}}}`
}

// targetedResult renders a result document with one target-level value set for the schema "incident".
func targetedResult(position int, entries ...string) string {
	encoded, _ := json.Marshal(position)
	return `{"selected":true,"targetResults":[{"targetPosition":` + string(encoded) + `,"fields":{"incident":{` +
		strings.Join(entries, ",") + `}}}]}`
}

// publishResult publishes a result with the evaluation for the Message and returns its ID.
// groups are the stored field groups the read side requires of a reader.
func (f *deliveryFixture) publishResult(workflow, message uuid.UUID, evaluation, groups string) uuid.UUID {
	f.t.Helper()
	candidate, result := uuid.New(), uuid.New()
	f.exec(`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision, configuration_revision,
		source_revision, context_revision, state, selected, due_at, created_at, updated_at)
		SELECT $1, t.id, w.id, w.active_version_id, 1, 1, 1, 1, 'completed', true, now(), now(), now()
		FROM transmissions t, processing_workflows w WHERE t.message_id = $3 AND w.id = $2`, candidate, workflow, message)
	f.exec(`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
		VALUES ($1, $2, 1, $3::jsonb, $5::jsonb, now(), now(), $4)`, result, candidate, evaluation, message, groups)
	return result
}

// publishLevel publishes an untargeted incident level readable with the groups derived and payload.
func (f *deliveryFixture) publishLevel(workflow, message uuid.UUID, level string) uuid.UUID {
	f.t.Helper()
	return f.publishResult(workflow, message, untargetedResult(attributeEntry("level", "integer", `"`+level+`"`)),
		`["derived","payload"]`)
}

// attributeRef is the field reference of an incident attribute.
func attributeRef(path ...string) map[string]any {
	return map[string]any{"namespace": "attributes", "path": append([]string{"incident"}, path...)}
}

// levelAtLeast is the typed predicate "attributes.incident.level >= minimum".
func levelAtLeast(minimum string) map[string]any {
	return map[string]any{"op": "gte", "field": attributeRef("level"),
		"value": map[string]any{"kind": "integer", "value": minimum}}
}

// cityIs is the typed predicate "attributes.incident.city equals city".
func cityIs(city string) map[string]any {
	return map[string]any{"op": "eq", "field": attributeRef("city"), "value": map[string]any{"kind": "string", "value": city}}
}

// minuteAgo returns an instant a minute in the past, when test Messages happened.
func (f *deliveryFixture) minuteAgo() time.Time { return f.now().Add(-time.Minute) }
