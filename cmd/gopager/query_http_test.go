package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// queryFixture is a composed API with seeded master data and Messages.
type queryFixture struct {
	t       *testing.T
	store   *postgres.Store
	origin  string
	client  *http.Client
	gateway uuid.UUID
	network uuid.UUID
	agency  uuid.UUID
}

func (f *queryFixture) exec(statement string, args ...any) {
	f.t.Helper()
	err := f.store.WithinTransaction(context.Background(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), statement, args...)
		return err
	})
	if err != nil {
		f.t.Fatalf("%s: %v", statement, err)
	}
}

func newQueryFixture(t *testing.T) *queryFixture {
	t.Helper()
	store := newManagementStore(t)
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), mustComposeCore(t, store), nil)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	f := &queryFixture{t: t, store: store, origin: origin, client: backend.Client(), gateway: uuid.New(),
		network: uuid.New(), agency: seedAgency(t, store)}
	f.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, f.gateway)
	f.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Net', 'pocsag', $2)`, f.network, f.agency)
	return f
}

func (f *queryFixture) unit(name string, organization *uuid.UUID) uuid.UUID {
	id := uuid.New()
	f.exec(`INSERT INTO units (id, name, agency_id, organization_id) VALUES ($1, $2, $3, $4)`, id, name, f.agency, organization)
	return id
}

// message inserts a Message with one Transmission whose targets are assigned to the given Units.
func (f *queryFixture) message(first time.Time, text string, units ...uuid.UUID) uuid.UUID {
	message, transmission := uuid.New(), uuid.New()
	f.exec(`INSERT INTO messages (id, identity_form, identity_value, first_observed_at, last_observed_at) VALUES ($1, 'text', $2, $3, $3)`,
		message, []byte(text), first)
	f.exec(`INSERT INTO transmissions (id, gateway_id, network_id, submission_id, submission_fingerprint, network_resolution, protocol,
		payload, observed_at, observation_offset_seconds, accepted_at, source_revision, message_id)
		VALUES ($1, $2, $3, $4, $5, 'declared', 'pocsag', $6::json, $7, 0, $7, 1, $8)`,
		transmission, f.gateway, f.network, uuid.NewString(), make([]byte, 32), `{"form":"text","value":"`+text+`"}`, first, message)
	for position, unit := range units {
		f.exec(`INSERT INTO transmission_targets (transmission_id, position, original, network_id, protocol, observed_at, accepted_at,
			address_number, function, resolution_status, unit_id, assignment_revision)
			VALUES ($1, $2, '{}'::json, $3, 'pocsag', $4, $4, $5, 0, 'assigned', $6, 1)`, transmission, position, f.network, first, 1000+position, unit)
		f.exec(`INSERT INTO message_units (message_id, unit_id, agency_id, first_observed_at, age_anchor) VALUES ($1, $2, $3, $4, $4)`,
			message, unit, f.agency, first)
	}
	return message
}

// caller seeds a technical account with the grants, field permission and an access key, and
// returns the bearer header value and the Principal.
func (f *queryFixture) caller(actions []string, scope authz.Scope, fields []string) (string, uuid.UUID) {
	f.t.Helper()
	principal := uuid.New()
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		f.t.Fatal(err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(secret))
	scopeJSON, _ := json.Marshal(scope)
	actionsJSON, _ := json.Marshal(actions)
	fieldsJSON, _ := json.Marshal(fields)
	restrictions, _ := json.Marshal(authz.Restrictions{Actions: actions, Scopes: []authz.Scope{{Kind: "all"}}, Fields: fields})
	f.exec(`INSERT INTO identity_accounts (id, kind, state, created_at) VALUES ($1, 'technical', 'active', now())`, principal)
	f.exec(`INSERT INTO identity_grants (id, subject_id, subject_kind, actions, scope) VALUES ($1, $2, 'principal', $3::jsonb, $4::jsonb)`,
		uuid.New(), principal, actionsJSON, scopeJSON)
	f.exec(`INSERT INTO identity_field_permissions (id, subject_id, subject_kind, field_names) VALUES ($1, $2, 'principal', $3::jsonb)`,
		uuid.New(), principal, fieldsJSON)
	f.exec(`INSERT INTO identity_credentials (id, account_id, kind, secret_hash, restrictions, created_at, last_used_at)
		VALUES ($1, $2, 'accessKey', $3, $4::jsonb, now(), now())`, uuid.New(), principal, hash[:], restrictions)
	return "Bearer " + secret, principal
}

// get sends an authenticated GET and returns the status and body.
func (f *queryFixture) get(bearer, path string, headers ...string) (int, []byte) {
	f.t.Helper()
	request, err := http.NewRequestWithContext(f.t.Context(), http.MethodGet, f.origin+path, nil)
	if err != nil {
		f.t.Fatal(err)
	}
	if bearer != "" {
		request.Header.Set("Authorization", bearer)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		request.Header.Set(headers[i], headers[i+1])
	}
	response, err := f.client.Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		f.t.Fatal(err)
	}
	return response.StatusCode, body
}

type feedBody struct {
	Data struct {
		Items         []map[string]any `json:"items"`
		NextPageToken string           `json:"nextPageToken"`
		Position      string           `json:"position"`
	} `json:"data"`
	Code string `json:"code"`
}

func (f *queryFixture) feed(bearer, query string, status int) feedBody {
	f.t.Helper()
	got, body := f.get(bearer, "/api/v1/messages"+query)
	if got != status {
		f.t.Fatalf("GET /messages%s = %d, want %d: %s", query, got, status, body)
	}
	var parsed feedBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		f.t.Fatal(err)
	}
	return parsed
}

func itemIDs(items []map[string]any) []string {
	var out []string
	for _, item := range items {
		out = append(out, item["messageId"].(string))
	}
	return out
}

// TestMessageQueryHTTP covers field-group refinements, eligibility before
// paging, token binding, locale independence and the detail 404.
func TestMessageQueryHTTP(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	organization := uuid.New()
	f.exec(`INSERT INTO organizations (id, name, defaults) VALUES ($1, 'O', '{}')`, organization)
	a, b, c := f.unit("A", &organization), f.unit("B", nil), f.unit("C", nil)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	m4 := f.message(base, "m4", a, c)
	m5 := f.message(base.Add(time.Minute), "m5", c)
	read := []string{"message.read", "message.detail"}
	scopeAB := authz.Scope{Kind: "unit", References: []string{a.String(), b.String()}}
	s, _ := f.caller(read, scopeAB, []string{"payload", "unit"})
	w, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})

	t.Run("S sees only A of M4 and not M5", func(t *testing.T) {
		t.Parallel()
		page := f.feed(s, "", http.StatusOK)
		if got := itemIDs(page.Data.Items); len(got) != 1 || got[0] != m4.String() {
			t.Fatalf("items = %v", got)
		}
		units := page.Data.Items[0]["units"].([]any)
		if len(units) != 1 || units[0].(map[string]any)["name"] != "A" {
			t.Fatalf("units = %v", units)
		}
		if _, present := page.Data.Items[0]["networkUnresolved"]; present || page.Data.Position == "" {
			t.Fatalf("item = %v position %q", page.Data.Items[0], page.Data.Position)
		}
		status, body := f.get(s, "/api/v1/messages/"+m4.String())
		if status != http.StatusOK || strings.Contains(string(body), c.String()) {
			t.Fatalf("detail %d leaks C: %s", status, body)
		}
		if status, _ := f.get(s, "/api/v1/messages/"+m5.String()); status != http.StatusNotFound {
			t.Fatalf("hidden detail = %d", status)
		}
		if status, _ := f.get(s, "/api/v1/messages/"+uuid.NewString()); status != http.StatusNotFound {
			t.Fatalf("unknown detail = %d", status)
		}
	})
	t.Run("S network filter needs gateway", func(t *testing.T) {
		t.Parallel()
		f.feed(s, "?network="+f.network.String(), http.StatusForbidden)
		got := f.feed(s, "?organization="+organization.String(), http.StatusOK)
		if ids := itemIDs(got.Data.Items); len(ids) != 1 || ids[0] != m4.String() {
			t.Fatalf("organization filter = %v", ids)
		}
	})
	t.Run("S filters Messages by a predicate", func(t *testing.T) {
		t.Parallel()
		got := f.feed(s, "?filter="+url.QueryEscape(textFilter("eq", "m4")), http.StatusOK)
		if ids := itemIDs(got.Data.Items); len(ids) != 1 || ids[0] != m4.String() {
			t.Fatalf("filter = %v", ids)
		}
		if got := f.feed(s, "?filter="+url.QueryEscape(textFilter("eq", "other")), http.StatusOK); len(got.Data.Items) != 0 {
			t.Fatalf("non-matching filter = %v", itemIDs(got.Data.Items))
		}
		unit := `{"op":"eq","field":{"namespace":"message","path":["unitNames"]},"value":{"kind":"string","value":"C"}}`
		if got := f.feed(s, "?filter="+url.QueryEscape(unit), http.StatusOK); len(got.Data.Items) != 0 {
			t.Fatalf("a Unit the caller cannot see must not match: %v", itemIDs(got.Data.Items))
		}
	})
	t.Run("filter errors", func(t *testing.T) {
		t.Parallel()
		for name, query := range map[string]string{
			"unknown attribute": "?filter=" + url.QueryEscape(`{"op":"present","field":{"namespace":"attributes","path":["nope","x"]},"value":{"kind":"boolean","value":true}}`),
			"workflow field":    "?filter=" + url.QueryEscape(`{"op":"present","field":{"namespace":"source","path":["protocol"]},"value":{"kind":"boolean","value":true}}`),
			"not JSON":          "?filter=nope",
			"twice":             "?filter=" + url.QueryEscape(textFilter("eq", "a")) + "&filter=" + url.QueryEscape(textFilter("eq", "a")),
		} {
			if got := f.feed(s, query, http.StatusBadRequest); got.Code != "INVALID_INPUT" {
				t.Errorf("%s: code %q", name, got.Code)
			}
		}
	})
	t.Run("S options", func(t *testing.T) {
		t.Parallel()
		status, body := f.get(s, "/api/v1/messages/filter-options")
		if status != http.StatusOK {
			t.Fatalf("options = %d %s", status, body)
		}
		var options struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &options); err != nil {
			t.Fatal(err)
		}
		if _, present := options.Data["networks"]; present {
			t.Fatalf("networks offered without the gateway group: %s", body)
		}
		var units []map[string]any
		if err := json.Unmarshal(options.Data["units"], &units); err != nil || len(units) != 2 {
			t.Fatalf("units = %s, %v", options.Data["units"], err)
		}
	})
	t.Run("W sees payload without units and only protocol options", func(t *testing.T) {
		t.Parallel()
		page := f.feed(w, "", http.StatusOK)
		if len(page.Data.Items) != 2 {
			t.Fatalf("items = %v", itemIDs(page.Data.Items))
		}
		for _, item := range page.Data.Items {
			if item["payload"] == nil || item["messageTime"] == nil {
				t.Fatalf("item lacks payload or time: %v", item)
			}
			for _, property := range []string{"units", "withoutUnit", "attributes"} {
				if _, present := item[property]; present {
					t.Fatalf("item exposes %s: %v", property, item)
				}
			}
		}
		unitFilter := `{"op":"present","field":{"namespace":"message","path":["unitNames"]},"value":{"kind":"boolean","value":true}}`
		for _, query := range []string{"?unit=" + a.String(), "?organization=" + organization.String(), "?agency=" + f.agency.String(), "?withoutUnit=true",
			"?filter=" + url.QueryEscape(unitFilter)} {
			f.feed(w, query, http.StatusForbidden)
		}
		f.feed(w, "?text=m4", http.StatusOK)
		_, body := f.get(w, "/api/v1/messages/filter-options")
		if strings.Contains(string(body), `"units"`) || !strings.Contains(string(body), `"protocols":["pocsag","flex","zvei","eas"]`) {
			t.Fatalf("options = %s", body)
		}
		if status, _ := f.get(w, "/api/v1/messages/"+m4.String()); status != http.StatusForbidden {
			t.Fatalf("detail without message.detail = %d", status)
		}
	})
	t.Run("guest without grants", func(t *testing.T) {
		t.Parallel()
		f.feed("", "", http.StatusForbidden)
	})
	t.Run("invalid parameters", func(t *testing.T) {
		t.Parallel()
		for _, query := range []string{"?bogus=1", "?pageSize=0", "?pageSize=201", "?from=a", "?text=a&text=b", "?unit=x",
			"?text=%FF", "?pageToken=zzz", "?gateway=" + f.gateway.String()} {
			if got := f.feed(s, query, http.StatusBadRequest); got.Code != "INVALID_INPUT" {
				t.Errorf("%s: code %q", query, got.Code)
			}
		}
		if status, _ := f.get(s, "/api/v1/messages/not-a-uuid"); status != http.StatusBadRequest {
			t.Errorf("bad id = %d", status)
		}
		if status, _ := f.get(s, "/api/v1/messages/filter-options?x=1"); status != http.StatusBadRequest {
			t.Errorf("options with parameter = %d", status)
		}
	})
	t.Run("locale changes nothing", func(t *testing.T) {
		t.Parallel()
		_, english := f.get(w, "/api/v1/messages?pageSize=1")
		_, german := f.get(w, "/api/v1/messages?pageSize=1", "Accept-Language", "de")
		var first, second feedBody
		if json.Unmarshal(english, &first) != nil || json.Unmarshal(german, &second) != nil {
			t.Fatal("undecodable responses")
		}
		if first.Data.NextPageToken == "" || first.Data.NextPageToken != second.Data.NextPageToken ||
			strings.Join(itemIDs(first.Data.Items), ",") != strings.Join(itemIDs(second.Data.Items), ",") {
			t.Fatalf("responses differ: %s / %s", english, german)
		}
	})
}

// TestMessagePageTokensOverHTTP covers 120 Messages, pages of 50, a Message arriving
// between pages and every way a token can be refused.
func TestMessagePageTokensOverHTTP(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	unit := f.unit("A", nil)
	base := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	for i := range 120 {
		f.message(base.Add(time.Duration(i)*time.Second), "m", unit)
	}
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	other, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload"})
	var seen []string
	var sizes []int
	token := ""
	for page := 0; page < 3; page++ {
		query := "?pageSize=50"
		if token != "" {
			query += "&pageToken=" + token
		}
		got := f.feed(bearer, query, http.StatusOK)
		sizes = append(sizes, len(got.Data.Items))
		seen = append(seen, itemIDs(got.Data.Items)...)
		token = got.Data.NextPageToken
		if page == 0 {
			f.message(time.Now().UTC(), "new", unit)
			if token == "" {
				t.Fatal("no next page token")
			}
			f.feed(bearer, "?pageSize=50&text=zz&pageToken="+token, http.StatusBadRequest)
			f.feed(other, query, http.StatusOK)
			f.feed(other, "?pageSize=50&pageToken="+token, http.StatusBadRequest)
			flipped := []byte(token)
			if flipped[12] == 'A' {
				flipped[12] = 'B'
			} else {
				flipped[12] = 'A'
			}
			f.feed(bearer, "?pageSize=50&pageToken="+string(flipped), http.StatusBadRequest)
		}
	}
	if sizes[0] != 50 || sizes[1] != 50 || sizes[2] != 20 || token != "" {
		t.Fatalf("page sizes %v, last token %q", sizes, token)
	}
	unique := map[string]bool{}
	for _, id := range seen {
		unique[id] = true
	}
	if len(unique) != 120 {
		t.Fatalf("%d distinct of %d", len(unique), len(seen))
	}
	_, raw := f.get(bearer, "/api/v1/messages?pageSize=1")
	if strings.Contains(string(raw), `"total"`) {
		t.Fatalf("feed must not carry total: %s", raw)
	}
	fresh := f.feed(bearer, "?pageSize=1", http.StatusOK)
	if fresh.Data.Items[0]["messageTime"] == nil {
		t.Fatal("fresh first page lacks the new Message")
	}
}

// TestMessageDerivedClassificationOverHTTP stores a Workflow definition version and a result with
// its field groups and shows that the result needs the derived group and every stored group.
func TestMessageDerivedClassificationOverHTTP(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	unit := f.unit("A", nil)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	message := f.message(base, "alarm", unit)
	protocol := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	name := processing.FieldRef{Namespace: "source", Path: []string{"network", "name"}}
	pocsag := processing.Value{Kind: processing.StringValue, Value: json.RawMessage(`"pocsag"`)}
	definition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocol, Value: &pocsag},
		Outputs:  map[string]map[string]processing.Assignment{"incident": {"network": {Field: &name}}}})
	if err != nil {
		t.Fatal(err)
	}
	const evaluation = `{"selected":true,"fields":{"incident":{"network":{"state":"present","value":{"kind":"string","value":"Net"}}}}}`
	workflow, version, candidate := uuid.New(), uuid.New(), uuid.New()
	f.exec(`INSERT INTO processing_workflows (id, name, created_at, updated_at) VALUES ($1, 'w', now(), now())`, workflow)
	f.exec(`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) SELECT $1, id FROM agencies`, workflow)
	f.exec(`INSERT INTO processing_workflow_versions (id, workflow_id, definition, created_at, created_by) VALUES ($1, $2, $3::jsonb, now(), $4)`,
		version, workflow, definition, uuid.New())
	f.exec(`INSERT INTO processing_candidates (id, transmission_id, workflow_id, definition_version_id, workflow_revision, configuration_revision,
		source_revision, context_revision, state, selected, due_at, created_at, updated_at)
		SELECT $1, t.id, $2, $3, 1, 1, 1, 1, 'completed', true, now(), now(), now() FROM transmissions t WHERE t.message_id = $4`,
		candidate, workflow, version, message)
	result := uuid.New()
	f.exec(`INSERT INTO processing_results (id, candidate_id, producing_fence, evaluation, field_groups, completed_at, published_at, message_id)
		VALUES ($1, $2, 1, $3::jsonb, '["derived","gateway","payload"]'::jsonb, now(), now(), $4)`, result, candidate,
		evaluation, message)
	read := []string{"message.read"}
	everything := authz.Scope{Kind: "all"}
	complete, _ := f.caller(read, everything, []string{"payload", "gateway", "derived"})
	missingGateway, _ := f.caller(read, everything, []string{"payload", "derived"})
	noDerived, _ := f.caller(read, everything, []string{"payload", "gateway"})
	noPayload, _ := f.caller(read, everything, []string{"gateway", "derived"})
	attributes := func(bearer string) []any {
		page := f.feed(bearer, "", http.StatusOK)
		if len(page.Data.Items) == 0 {
			return nil
		}
		value, _ := page.Data.Items[0]["attributes"].([]any)
		return value
	}
	got := attributes(complete)
	if len(got) != 1 {
		t.Fatalf("complete caller attributes = %v", got)
	}
	attribute := got[0].(map[string]any)
	if attribute["schemaKey"] != "incident" || attribute["field"] != "network" || attribute["workflowId"] != workflow.String() ||
		attribute["definitionVersionId"] != version.String() || attribute["resultId"] != result.String() ||
		attribute["publishedAt"] == nil || !strings.Contains(fmt.Sprint(attribute["value"]), "Net") {
		t.Fatalf("attribute = %v", attribute)
	}
	if got := attributes(missingGateway); len(got) != 0 {
		t.Fatalf("a result reading gateway fields must be hidden without that group: %v", got)
	}
	page := f.feed(noDerived, "", http.StatusOK)
	if _, present := page.Data.Items[0]["attributes"]; present {
		t.Fatalf("attributes appear without the derived group: %v", page.Data.Items[0])
	}
	if got := f.feed(complete, "?filter="+url.QueryEscape(textFilter("eq", "alarm")), http.StatusOK); len(got.Data.Items) != 1 {
		t.Fatalf("filter = %v", itemIDs(got.Data.Items))
	}
	f.feed(noPayload, "?filter="+url.QueryEscape(textFilter("eq", "alarm")), http.StatusForbidden)
}

// textFilter is a predicate comparing the text of a Message.
func textFilter(op, text string) string {
	return `{"op":"` + op + `","field":{"namespace":"message","path":["text"]},"value":{"kind":"string","value":"` + text + `"}}`
}

// TestMessageLiveHelpersReuseTheFeedRules checks the service entry points the live stream builds
// on: re-evaluating Messages with the feed filters minus their time, and plain eligibility.
func TestMessageLiveHelpersReuseTheFeedRules(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	a, b := f.unit("A", nil), f.unit("B", nil)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	visible := f.message(base, "one", a)
	elsewhere := f.message(base.Add(time.Minute), "two", b)
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "unit", References: []string{a.String()}}, []string{"payload", "unit"})
	core := mustComposeCore(t, f.store)
	service, err := readmodel.NewService(core.store, core.resolver, core.store, core.reads, core.reads, core.fields)
	if err != nil {
		t.Fatal(err)
	}
	proof := identity.Proof{Kind: identity.AccessKeyProof, Secret: strings.TrimPrefix(bearer, "Bearer ")}
	eligible, err := service.EligibleMessages(t.Context(), proof, []uuid.UUID{visible, elsewhere, uuid.New()})
	if err != nil || len(eligible) != 1 || eligible[0] != visible {
		t.Fatalf("eligible = %v, %v", eligible, err)
	}
	from := base.Add(24 * time.Hour)
	filters := readmodel.Filters{From: &from, Units: []uuid.UUID{a}}
	items, err := service.Evaluate(t.Context(), proof, filters, []uuid.UUID{visible, elsewhere})
	if err != nil || len(items) != 1 || items[0].MessageID != visible {
		t.Fatalf("a Message outside the client's range is still evaluated: %+v, %v", items, err)
	}
	filters.Units = []uuid.UUID{b}
	if items, err = service.Evaluate(t.Context(), proof, filters, []uuid.UUID{visible}); err != nil || len(items) != 0 {
		t.Fatalf("non-matching filter: %+v, %v", items, err)
	}
	for text, want := range map[string]int{"one": 1, "other": 0} {
		var predicate processing.Predicate
		if err := processing.Decode([]byte(textFilter("eq", text)), &predicate); err != nil {
			t.Fatal(err)
		}
		items, err := service.Evaluate(t.Context(), proof, readmodel.Filters{Filter: &predicate}, []uuid.UUID{visible, elsewhere})
		if err != nil || len(items) != want {
			t.Fatalf("predicate on %q: %+v, %v", text, items, err)
		}
	}
	if _, err = service.Evaluate(t.Context(), proof, readmodel.Filters{Networks: []uuid.UUID{f.network}}, []uuid.UUID{visible}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("filter without its group: %v", err)
	}
}

// TestPageTokenKeySurvivesAFailedFirstRead checks that the token secret created by a read that
// fails is still committed, so the tokens this process issues afterwards are accepted by any
// other process of the installation.
func TestPageTokenKeySurvivesAFailedFirstRead(t *testing.T) {
	t.Parallel()
	f := newQueryFixture(t)
	unit := f.unit("A", nil)
	base := time.Now().UTC().Add(-time.Hour)
	for i := range 3 {
		f.message(base.Add(time.Duration(i)*time.Minute), "alarm", unit)
	}
	bearer, _ := f.caller([]string{"message.read"}, authz.Scope{Kind: "all"}, []string{"payload", "unit"})
	f.feed(bearer, "?pageSize=1&pageToken=invalid", http.StatusBadRequest)
	token := f.feed(bearer, "?pageSize=1", http.StatusOK).Data.NextPageToken
	if token == "" {
		t.Fatal("no page token")
	}

	other := httptest.NewUnstartedServer(nil)
	t.Cleanup(other.Close)
	origin := "http://" + other.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), mustComposeCore(t, f.store), nil)
	if err != nil {
		t.Fatal(err)
	}
	other.Config.Handler = handler
	other.Start()
	second := *f
	second.origin, second.client = origin, other.Client()
	if got := second.feed(bearer, "?pageSize=1&pageToken="+url.QueryEscape(token), http.StatusOK); len(got.Data.Items) != 1 {
		t.Errorf("second process page = %v", got.Data.Items)
	}
}
