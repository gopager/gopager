package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
)

// hiddenLocationLayout prefers the incident location and falls back to the Message text.
const hiddenLocationLayout = `{"schemaVersion":1,"outputKind":"json","blocks":[{"kind":"fallback","blocks":[
	{"kind":"field","field":{"namespace":"attributes","path":["incident","location"]},"label":"Location"},
	{"kind":"field","field":{"namespace":"message","path":["text"]},"label":"Text"}]}]}`

// TestMessageCardFallbackNeverShowsAHiddenMembersLocation proves that a card falls back to the
// readable original text when only a hidden member of the Message carries a location: the
// hidden Transmission's result never reaches the card of a caller who cannot see it, while a
// caller who sees both members gets the location.
func TestMessageCardFallbackNeverShowsAHiddenMembersLocation(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	visible := f.unit("Engine 1", nil)
	hiddenAgency := seedAgency(t, f.store)
	hidden := uuid.New()
	f.exec(`INSERT INTO units (id, name, agency_id) VALUES ($1, 'Hidden', $2)`, hidden, hiddenAgency)
	first := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	message := f.message(first, "FIRE", visible)

	// The hidden member: same Message, assigned only to a Unit of another Agency.
	member := uuid.New()
	f.exec(`INSERT INTO transmissions (id, gateway_id, network_id, submission_id, submission_fingerprint, network_resolution, protocol,
		payload, observed_at, observation_offset_seconds, accepted_at, source_revision, message_id)
		VALUES ($1, $2, $3, $4, $5, 'declared', 'pocsag', '{"form":"text","value":"SECRET"}'::json, $6, 0, $6, 1, $7)`,
		member, f.gateway, f.network, uuid.NewString(), make([]byte, 32), first.Add(time.Second), message)
	f.exec(`INSERT INTO transmission_targets (transmission_id, position, original, network_id, protocol, observed_at, accepted_at,
		address_number, function, resolution_status, unit_id, assignment_revision)
		VALUES ($1, 0, '{}'::json, $2, 'pocsag', $3, $3, 2000, 0, 'assigned', $4, 1)`, member, f.network, first.Add(time.Second), hidden)
	f.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor) VALUES ($1, $2, $3, $4, $4)`,
		message, hidden, hiddenAgency, first.Add(time.Second))

	f.exec(`INSERT INTO attribute_schemas (key, revision, label, subject, fields, created_by, created_at, updated_at)
		VALUES ('incident', 1, 'Incident', 'message', '{"location":{"kind":"string"}}'::jsonb, $1, now(), now())`, uuid.New())
	f.exec(`INSERT INTO attribute_schema_agencies (schema_key, agency_id) SELECT 'incident', id FROM agencies`)
	f.exec(`UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
	workflow, version, candidate, result := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec(`INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
	f.exec(`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) SELECT $1, id FROM agencies`, workflow)
	f.exec(`INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by)
		VALUES ($1, $2, '{"schemaVersion":1}'::jsonb, now(), $3)`, version, workflow, uuid.New())
	f.exec(`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision, configuration_revision,
		source_revision, context_revision, state, selected, due_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 1, 1, 1, 1, 'completed', true, now(), now(), now())`, candidate, member, workflow, version)
	f.exec(`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
		VALUES ($1, $2, 1, '{"selected":true,"fields":{"incident":{"location":{"state":"present","value":{"kind":"string","value":"Main Street 1"}}}}}'::jsonb,
		'["derived","payload"]'::jsonb, now(), now(), $3)`, result, candidate, message)
	f.exec(processingstore.ProjectAttributeValuesSQL, []uuid.UUID{result})

	layout := uuid.New()
	f.exec(`INSERT INTO layout_definitions (id, revision, name, output_kind, created_by, created_at, updated_at)
		VALUES ($1, 1, 'Card', 'json', $2, now(), now())`, layout, uuid.New())
	f.exec(`INSERT INTO layout_definition_versions (layout_id, version, definition, created_by, created_at)
		VALUES ($1, 1, $2::jsonb, $3, now())`, layout, hiddenLocationLayout, uuid.New())
	f.exec(`INSERT INTO layout_definition_agencies (layout_id, agency_id) SELECT $1, id FROM agencies`, layout)
	f.exec(`UPDATE installation_settings SET default_layout_id = $1`, layout)

	groups := []string{"payload", "unit", "derived"}
	restricted, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "unit", References: []string{visible.String()}}, groups)
	everyone, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, groups)
	items := func(bearer string) string {
		page := f.feed(bearer, "", http.StatusOK)
		if len(page.Data.Items) != 1 {
			t.Fatalf("items = %v", page.Data.Items)
		}
		card := cardOf(t, page.Data.Items[0])
		if card["layoutId"] != layout.String() || card["fallback"] != false {
			t.Fatalf("card = %v, want the installation default %s", card, layout)
		}
		encoded, _ := json.Marshal(card["items"])
		return string(encoded)
	}
	if got := items(restricted); strings.Contains(got, "Main Street") || strings.Contains(got, "SECRET") || !strings.Contains(got, "FIRE") {
		t.Fatalf("restricted card = %s, want the readable text only", got)
	}
	if got := items(everyone); !strings.Contains(got, "Main Street 1") {
		t.Fatalf("unrestricted card = %s, want the location", got)
	}
}
