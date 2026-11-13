package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
)

// storedAddresses hands out distinct address ranges to the stored Transmissions of parallel tests.
var storedAddresses atomic.Int64

// previewHTTPFixture is the composed API with a Workflow editor session, one POCSAG Network and
// the helpers to store a Workflow version, seed stored Transmissions and preview.
type previewHTTPFixture struct {
	t          *testing.T
	store      *postgres.Store
	client     *http.Client
	origin     string
	csrf       string
	seed       *queryFixture
	agency     uuid.UUID
	workflowID uuid.UUID
	network    synthetic.Network
}

// newPreviewHTTPFixture composes the production runtime over a database whose operator reads the
// given field groups.
func newPreviewHTTPFixture(t *testing.T, groups []string) *previewHTTPFixture {
	t.Helper()
	store := newManagementStoreWithFields(t, groups)
	identityRepository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	agency := seedAgency(t, store)
	seed := &queryFixture{t: t, store: store, gateway: uuid.New(), network: uuid.New(), agency: agency}
	seed.exec(`INSERT INTO gateways (id, name) VALUES ($1, 'gw')`, seed.gateway)
	seed.exec(`INSERT INTO networks (id, name, protocol, agency_id) VALUES ($1, 'Net', 'pocsag', $2)`, seed.network, agency)
	network := synthetic.Network{ID: seed.network, Name: "Net", Protocol: protocols.POCSAG, AgencyID: agency}
	networks := previewFixtureNetworks{values: map[uuid.UUID]synthetic.Network{network.ID: network},
		identity: identityRepository, units: unitRepository}
	shared := mustComposeCore(t, store)
	runtime, err := composeProcessingAPIRuntime(t.Context(), shared, "", networks)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(t.Context()); err != nil {
			t.Error(err)
		}
	})
	backend := httptest.NewUnstartedServer(nil)
	t.Cleanup(backend.Close)
	origin := "http://" + backend.Listener.Addr().String()
	handler, err := composeApplicationAPI(origin, identity.DefaultLoginRateLimits(), shared, networks, runtime.routes)
	if err != nil {
		t.Fatal(err)
	}
	backend.Config.Handler = handler
	backend.Start()
	client := backend.Client()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Jar = jar
	seed.origin, seed.client = origin, client
	login := managementRequest(t, client, origin, http.MethodPost, "/api/v1/auth/login",
		`{"schemaVersion":1,"username":"operator","password":"initial-password"}`, "", "", http.StatusOK)
	workflow := managementRequest(t, client, origin, http.MethodPost, "/api/v1/workflows",
		`{"schemaVersion":1,"commandId":"`+uuid.NewString()+`","name":"Preview","scope":`+workflowScopeJSON(agency)+`,"enabled":true}`,
		login.CSRFToken, "", http.StatusCreated)
	workflowID, _ := managementResultID(t, workflow)
	return &previewHTTPFixture{t: t, store: store, client: client, origin: origin, csrf: login.CSRFToken, seed: seed,
		agency: agency, workflowID: workflowID, network: network}
}

// saveVersion stores the definition as a Workflow version and returns its ID.
func (f *previewHTTPFixture) saveVersion(definition processing.Definition) uuid.UUID {
	f.t.Helper()
	input, err := json.Marshal(workflowVersionMutation{SchemaVersion: 1, CommandID: uuid.New(), ExpectedRevision: 1,
		Definition: definition})
	if err != nil {
		f.t.Fatal(err)
	}
	saved := managementRequest(f.t, f.client, f.origin, http.MethodPost, "/api/v1/workflows/"+f.workflowID.String()+"/versions",
		string(input), f.csrf, "", http.StatusCreated)
	versionID, _ := managementResultID(f.t, saved)
	return versionID
}

// preview posts a preview body for the Workflow and expects the status.
func (f *previewHTTPFixture) preview(body string, status int) managementResponse {
	f.t.Helper()
	return managementRequest(f.t, f.client, f.origin, http.MethodPost, "/api/v1/workflows/"+f.workflowID.String()+"/preview",
		body, f.csrf, "", status)
}

// storedTransmission seeds a Message with one Transmission and returns the Transmission's ID.
func (f *previewHTTPFixture) storedTransmission(text string, units ...uuid.UUID) uuid.UUID {
	f.t.Helper()
	message := f.seed.message(time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond), text, units...)
	var id uuid.UUID
	err := f.store.WithinTransaction(f.t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT id FROM transmissions WHERE message_id = $1`, message).Scan(&id)
	})
	if err != nil {
		f.t.Fatal(err)
	}
	// Give each target an address of its own, the rule that assigns it to its Unit and the original
	// document intake would have stored, so that matching the target again resolves to the same Unit.
	base := 100000 + int(storedAddresses.Add(1))*100
	for position, unit := range units {
		f.seed.exec(`UPDATE transmission_targets SET address_number = $3 WHERE transmission_id = $1 AND position = $2`,
			id, position, base+position)
		f.seed.exec(`INSERT INTO address_rules (id, network_id, unit_id, protocol, selector)
			VALUES ($1, $2, $3, 'pocsag', $4::jsonb)`, uuid.New(), f.network.ID, unit,
			fmt.Sprintf(`{"from":%d,"to":%d,"mode":"function-bit"}`, base+position, base+position))
	}
	f.seed.exec(`UPDATE transmission_targets SET original = json_build_object('address', address_number, 'function', function)
		WHERE transmission_id = $1`, id)
	return id
}
