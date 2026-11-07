package borrowed_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

// rematchFixture drives Gateway mapping re-resolution through the production intake service,
// Gateway administration and correction runner. Agency A owns POCSAG Network networks[0], Agency B
// owns POCSAG networks[1] and FLEX networks[2]; the Gateway starts with "default" → networks[0].
type rematchFixture struct {
	*restampFixture
	service *ingest.Service
	admin   *ingest.GatewayAdministration
	bearer  string
	unitB   uuid.UUID
	// observed is the Observation time of every Submission, so a retry has the same fingerprint.
	observed string
}

func newRematchFixture(t *testing.T) *rematchFixture {
	t.Helper()
	f := &rematchFixture{restampFixture: newRestampFixture(t),
		observed: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)}
	var err error
	if f.service, err = ingest.NewService(f.store, f.store, f.intake, f.rules); err != nil {
		t.Fatal(err)
	}
	if f.admin, err = ingest.NewGatewayAdministration(f.store, f.resolver, f.store, f.intake, f.intake,
		f.receipts, f.facts, f.ruleStore); err != nil {
		t.Fatal(err)
	}
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	f.bearer = secret
	f.exec(t, `INSERT INTO gateway_credentials(id,gateway_id,secret_hash,pending,created_at)
		VALUES($1,$2,$3,false,now())`, uuid.New(), f.gateway, hash[:])
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(),
		units.UnitInput{AgencyID: f.agencies[1], Name: "Rematch B", CollectionIDs: []uuid.UUID{}}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	f.unitB = unit.ID
	return f
}

// submit accepts one POCSAG Submission through intake with a distinct text Payload, the optional
// declaration and one target per address (function 1).
func (f *rematchFixture) submit(t *testing.T, id, declared string, addresses ...uint64) ingest.Receipt {
	t.Helper()
	function := uint8(1)
	targets := make([]protocols.Target, 0, len(addresses))
	for _, address := range addresses {
		targets = append(targets, protocols.Target{Address: protocols.NumericAddress(address), Function: &function})
	}
	payload, err := json.Marshal(map[string]string{"form": "text", "value": id})
	if err != nil {
		t.Fatal(err)
	}
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
		ObservationTime: f.observed, GatewayNetworkIdentifier: declared,
		Payload: payload, TransmissionTargets: targets}
	encoded, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := f.service.Submit(t.Context(), f.bearer, submission, len(encoded))
	if err != nil {
		t.Fatalf("submit %s: %v (%s)", id, err, platform.FailureCode(err))
	}
	return receipt
}

// replace sets the Gateway's mapping at its current revision.
func (f *rematchFixture) replace(t *testing.T, mapping ...ingest.GatewayNetworkInput) ingest.GatewayNetworksResult {
	t.Helper()
	var revision int64
	f.queryRow(t, `SELECT revision FROM gateways WHERE id=$1`, []any{f.gateway}, &revision)
	result, err := f.admin.ConfigureNetworks(t.Context(), f.proof, f.gateway, revision, mapping, checkedMutation)
	if err != nil {
		t.Fatalf("replace mapping: %v", err)
	}
	return result
}

// rematchState is the stored Network state of one Transmission and its first target.
type rematchState struct {
	network, targetNetwork uuid.UUID
	resolution             string
	unresolved             bool
	sourceRevision         int64
	unit                   *uuid.UUID
}

func (f *rematchFixture) state(t *testing.T, transmission uuid.UUID) rematchState {
	t.Helper()
	var value rematchState
	var unit *string
	f.queryRow(t, `SELECT t.network_id,COALESCE(target.network_id,t.network_id),t.network_resolution,
		t.network_unresolved,t.source_revision,target.unit_id::text
		FROM transmissions t LEFT JOIN transmission_targets target ON target.transmission_id=t.id AND target.position=0
		WHERE t.id=$1`, []any{transmission}, &value.network, &value.targetNetwork, &value.resolution,
		&value.unresolved, &value.sourceRevision, &unit)
	if unit != nil {
		id := uuid.MustParse(*unit)
		value.unit = &id
	}
	return value
}

// cacheAgencies lists the Message's message_units rows as "agency/unit" ("agency/-" without Unit).
func (f *rematchFixture) cacheAgencies(t *testing.T, message uuid.UUID) map[string]bool {
	t.Helper()
	rows := make(map[string]bool)
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		result, err := executor.QueryContext(tx.Context(), `SELECT agency_id::text,COALESCE(unit_id::text,'-'),
			correction_status FROM message_units WHERE message_id=$1`, message)
		if err != nil {
			return err
		}
		defer result.Close()
		for result.Next() {
			var agency, unit, status string
			if err := result.Scan(&agency, &unit, &status); err != nil {
				return err
			}
			rows[agency+"/"+unit+"/"+status] = true
		}
		return result.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return rows
}

// movedFacts returns the corrected assignment.changed facts of a Transmission.
func (f *rematchFixture) movedFacts(t *testing.T, transmission uuid.UUID) []map[string]any {
	t.Helper()
	var facts []map[string]any
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), `SELECT envelope->'data' FROM domain_events
			WHERE envelope->>'type'='assignment.changed' AND envelope->'data'->>'reason'='corrected'
			AND envelope->'data'->>'transmissionId'=$1 ORDER BY seq`, transmission.String())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var encoded []byte
			if err := rows.Scan(&encoded); err != nil {
				return err
			}
			var data map[string]any
			if err := json.Unmarshal(encoded, &data); err != nil {
				return err
			}
			facts = append(facts, data)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return facts
}

func (f *rematchFixture) correction(t *testing.T, id uuid.UUID) assignments.CorrectionStatus {
	t.Helper()
	status, err := f.rules.ReadCorrection(t.Context(), f.proof, id)
	if err != nil {
		t.Fatalf("read correction: %v", err)
	}
	return status
}

func mapping(identifier string, network uuid.UUID, priority int) ingest.GatewayNetworkInput {
	return ingest.GatewayNetworkInput{Identifier: identifier, NetworkID: network, Priority: priority}
}

// TestGatewayRemapMovesDeclaredTransmission covers F-10e: a remapped declaration moves the
// Transmission, its targets, cache rows and Processing to the new Network, leaving evidence alone.
func TestGatewayRemapMovesDeclaredTransmission(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	f.seedWorkflow(t, protocolSelectorDefinition(t))
	for network, unit := range map[uuid.UUID]uuid.UUID{f.networks[0]: f.unit, f.networks[1]: f.unitB} {
		if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(network, unit, 100, 100, 1),
			checkedMutation); err != nil {
			t.Fatal(err)
		}
	}
	receipt := f.submit(t, "declared", "default", 100)
	f.runDue(t)
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] || got.unit == nil || *got.unit != f.unit {
		t.Fatalf("setup: %+v", got)
	}
	var original string
	f.queryRow(t, `SELECT original::text FROM transmission_targets WHERE transmission_id=$1`,
		[]any{receipt.TransmissionID}, &original)

	result := f.replace(t, mapping("default", f.networks[1], 0))
	if len(result.Corrections) != 1 {
		t.Fatalf("corrections=%v, want one gateway scope", result.Corrections)
	}
	pending := f.correction(t, result.Corrections[0])
	if pending.Scope.Kind() != "gateway" || pending.Remaining != 1 ||
		!f.cacheAgencies(t, receipt.MessageID)[f.agencies[0].String()+"/"+f.unit.String()+"/correcting"] {
		t.Fatalf("queued: %+v cache=%v", pending, f.cacheAgencies(t, receipt.MessageID))
	}
	if drainCorrections(t, f.runner) == 0 {
		t.Fatal("correction did not run")
	}
	got := f.state(t, receipt.TransmissionID)
	if got.network != f.networks[1] || got.targetNetwork != f.networks[1] || got.resolution != "declared" ||
		got.unresolved || got.sourceRevision != 2 || got.unit == nil || *got.unit != f.unitB {
		t.Fatalf("after move: %+v", got)
	}
	if cache := f.cacheAgencies(t, receipt.MessageID); len(cache) != 1 ||
		!cache[f.agencies[1].String()+"/"+f.unitB.String()+"/current"] {
		t.Fatalf("visibility did not follow the Network: %v", cache)
	}
	if candidates := f.candidates(t, receipt.TransmissionID); candidates.status != "pending" || candidates.pending != 1 {
		t.Fatalf("Processing not re-admitted: %+v", candidates)
	}
	facts := f.movedFacts(t, receipt.TransmissionID)
	if len(facts) != 1 || facts[0]["networkId"] != f.networks[1].String() ||
		facts[0]["previousNetworkId"] != f.networks[0].String() || len(facts[0]["targets"].([]any)) != 1 {
		t.Fatalf("facts=%v", facts)
	}
	var after, submissionID string
	f.queryRow(t, `SELECT target.original::text,t.submission_id FROM transmission_targets target
		JOIN transmissions t ON t.id=target.transmission_id WHERE t.id=$1`, []any{receipt.TransmissionID},
		&after, &submissionID)
	if after != original || submissionID != "declared" {
		t.Fatalf("evidence changed: %s -> %s, submission %s", original, after, submissionID)
	}
	replay := f.submit(t, "declared", "default", 100)
	if replay.TransmissionID != receipt.TransmissionID || !replay.AcceptanceTime.Equal(receipt.AcceptanceTime) ||
		replay.MessageID != receipt.MessageID {
		t.Fatalf("receipt changed: %+v -> %+v", receipt, replay)
	}
	if done := f.correction(t, result.Corrections[0]); done.State != "completed" || done.Remaining != 0 ||
		done.Processed != 1 || done.Unresolved != 0 {
		t.Fatalf("completed status: %+v", done)
	}
}

// TestGatewayRemapKeepsUnresolvableTransmission covers F-97: an unmapped declaration keeps the
// Network and is reported; a later mapping that resolves it again clears the report.
func TestGatewayRemapKeepsUnresolvableTransmission(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[0], f.unit, 100, 100, 1),
		checkedMutation); err != nil {
		t.Fatal(err)
	}
	receipt := f.submit(t, "unresolvable", "default", 100)
	result := f.replace(t, mapping("other", f.networks[1], 0))
	drainCorrections(t, f.runner)
	got := f.state(t, receipt.TransmissionID)
	if got.network != f.networks[0] || got.resolution != "declared" || !got.unresolved || got.sourceRevision != 1 ||
		got.unit == nil || *got.unit != f.unit {
		t.Fatalf("unresolvable Transmission changed: %+v", got)
	}
	if status := f.correction(t, result.Corrections[0]); status.State != "completed" || status.Unresolved != 1 {
		t.Fatalf("status: %+v", status)
	}
	if facts := f.movedFacts(t, receipt.TransmissionID); len(facts) != 0 {
		t.Fatalf("unresolvable Transmission emitted facts: %v", facts)
	}
	// Without any POCSAG Network the Transmission stays unresolved as well.
	f.replace(t, mapping("flex", f.networks[2], 0))
	drainCorrections(t, f.runner)
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] || !got.unresolved {
		t.Fatalf("without a POCSAG Network: %+v", got)
	}
	again := f.replace(t, mapping("default", f.networks[0], 0))
	drainCorrections(t, f.runner)
	if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] || got.unresolved {
		t.Fatalf("flag not cleared: %+v", got)
	}
	if status := f.correction(t, again.Corrections[0]); status.Unresolved != 0 {
		t.Fatalf("status after recovery: %+v", status)
	}
}

// TestGatewayPrioritySwapMovesInferredAndTargetlessTransmissions covers F-98: inferred and
// targetless Transmissions move with the priority order, and only changed Protocols are queued.
func TestGatewayPrioritySwapMovesInferredAndTargetlessTransmissions(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	f.replace(t, mapping("n1", f.networks[0], 0), mapping("n2", f.networks[1], 1))
	drainCorrections(t, f.runner)
	inferred := f.submit(t, "inferred", "", 200)
	targetless := f.submit(t, "targetless", "")
	for _, receipt := range []ingest.Receipt{inferred, targetless} {
		if got := f.state(t, receipt.TransmissionID); got.network != f.networks[0] || got.resolution != "priority" {
			t.Fatalf("setup: %+v", got)
		}
	}
	flexOnly := f.replace(t, mapping("n1", f.networks[0], 0), mapping("n2", f.networks[1], 1),
		mapping("flex", f.networks[2], 2))
	if len(flexOnly.Corrections) != 1 || f.correction(t, flexOnly.Corrections[0]).Scope.Protocol != protocols.FLEX {
		t.Fatalf("a FLEX-only change queued %v", flexOnly.Corrections)
	}
	var stamp int64
	f.queryRow(t, `SELECT network_revision FROM transmissions WHERE id=$1`, []any{inferred.TransmissionID}, &stamp)
	drainCorrections(t, f.runner)
	var unchanged int64
	f.queryRow(t, `SELECT network_revision FROM transmissions WHERE id=$1`, []any{inferred.TransmissionID}, &unchanged)
	if unchanged != stamp {
		t.Fatalf("FLEX scope touched a POCSAG Transmission: stamp %d -> %d", stamp, unchanged)
	}

	swapped := f.replace(t, mapping("n1", f.networks[0], 1), mapping("n2", f.networks[1], 0),
		mapping("flex", f.networks[2], 2))
	if len(swapped.Corrections) != 1 || f.correction(t, swapped.Corrections[0]).Scope.Protocol != protocols.POCSAG {
		t.Fatalf("priority swap queued %v", swapped.Corrections)
	}
	drainCorrections(t, f.runner)
	for _, receipt := range []ingest.Receipt{inferred, targetless} {
		if got := f.state(t, receipt.TransmissionID); got.network != f.networks[1] || got.resolution != "priority" {
			t.Fatalf("not moved: %+v", got)
		}
		if cache := f.cacheAgencies(t, receipt.MessageID); len(cache) != 1 || !cache[f.agencies[1].String()+"/-/current"] {
			t.Fatalf("unassigned visibility did not move to Agency B: %v", cache)
		}
	}
	facts := f.movedFacts(t, targetless.TransmissionID)
	if len(facts) != 1 || facts[0]["previousNetworkId"] != f.networks[0].String() || facts[0]["targets"] != nil {
		t.Fatalf("targetless move fact=%v", facts)
	}
}

// failingReadmission fails every readmission after the first allowed ones, so a later batch of a
// correction rolls back as a crash would.
type failingReadmission struct {
	assignments.ProcessingReadmission
	allowed *int
}

func (r failingReadmission) ReadmitProcessing(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	if *r.allowed == 0 {
		return errors.New("simulated crash")
	}
	*r.allowed--
	return r.ProcessingReadmission.ReadmitProcessing(ctx, tx, id)
}

// TestGatewayRemapResumesAfterRestartWithConcurrentChanges covers F-99: a scope stopped after its
// first batch resumes under a new runner and re-resolves against the mapping and rules current
// then; a newer mapping scope and a rule scope finish without leaving a half-moved Transmission.
func TestGatewayRemapResumesAfterRestartWithConcurrentChanges(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	receipts := make([]ingest.Receipt, 150)
	for i := range receipts {
		receipts[i] = f.submit(t, fmt.Sprintf("bulk-%03d", i), "", 100)
	}
	first := f.replace(t, mapping("a", f.networks[1], 0))
	allowed := 100
	networks, err := ingest.NewNetworkReResolver(f.intake, f.rules)
	if err != nil {
		t.Fatal(err)
	}
	crashing, err := assignments.NewCorrectionRunner(f.store, f.store, f.rules, networks, f.ruleStore, f.facts,
		failingReadmission{ProcessingReadmission: f.owner, allowed: &allowed})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := crashing.RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	var moved, halfMoved int
	f.queryRow(t, `SELECT count(*) FILTER (WHERE t.network_id=$2),
		count(*) FILTER (WHERE EXISTS (SELECT 1 FROM transmission_targets target
			WHERE target.transmission_id=t.id AND target.network_id<>t.network_id))
		FROM transmissions t WHERE t.gateway_id=$1`, []any{f.gateway, f.networks[1]}, &moved, &halfMoved)
	if moved != 100 || halfMoved != 0 {
		t.Fatalf("after the crashed second batch: moved=%d half-moved=%d, want 100 and 0", moved, halfMoved)
	}
	second := f.replace(t, mapping("a", f.networks[0], 0), mapping("b", f.networks[1], 1))
	if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(f.networks[1], f.unitB, 100, 100, 1),
		checkedMutation); err != nil {
		t.Fatal(err)
	}
	// The restarted runner need not wait for the failed attempt's backoff.
	f.exec(t, `UPDATE assignment_pending_scopes SET lease_until=NULL WHERE state='queued'`)
	if drainCorrections(t, f.runner) == 0 {
		t.Fatal("restarted runner did no work")
	}
	for _, id := range []uuid.UUID{first.Corrections[0], second.Corrections[0]} {
		if status := f.correction(t, id); status.State != "completed" || status.Remaining != 0 {
			t.Fatalf("scope %s: %+v", id, status)
		}
	}
	for _, receipt := range receipts {
		got := f.state(t, receipt.TransmissionID)
		if got.network != f.networks[1] || got.targetNetwork != f.networks[1] || got.resolution != "inferred-address" ||
			got.unit == nil || *got.unit != f.unitB {
			t.Fatalf("final state of %s: %+v", receipt.TransmissionID, got)
		}
	}
}

// TestGatewayCorrectionNeedsEveryAffectedAgency covers F-100: a gateway correction is readable
// only with assignment-correction.read on every Agency it affects.
func TestGatewayCorrectionNeedsEveryAffectedAgency(t *testing.T) {
	t.Parallel()
	f := newRematchFixture(t)
	f.submit(t, "authorized", "default", 100)
	result := f.replace(t, mapping("default", f.networks[1], 0))
	drainCorrections(t, f.runner)
	grant := func(agencies ...uuid.UUID) authz.Grant {
		references := make([]string, len(agencies))
		for i, agency := range agencies {
			references[i] = agency.String()
		}
		return authz.Grant{Actions: []string{"assignment-correction.read"},
			Scope: authz.Scope{Kind: "agency", References: references}}
	}
	replaceManagementGrants(t, f.matcherFixture, []authz.Grant{grant(f.agencies[0])})
	if _, err := f.rules.ReadCorrection(t.Context(), f.proof, result.Corrections[0]); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("read with Agency A only = %v, want not found", err)
	}
	replaceManagementGrants(t, f.matcherFixture, []authz.Grant{grant(f.agencies[0], f.agencies[1])})
	status := f.correction(t, result.Corrections[0])
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Scope struct {
			Kind      string `json:"kind"`
			GatewayID string `json:"gatewayId"`
			Protocol  string `json:"protocol"`
		} `json:"scope"`
		Processed  int `json:"processed"`
		Unresolved int `json:"unresolved"`
	}
	if err := json.Unmarshal(encoded, &view); err != nil {
		t.Fatal(err)
	}
	if view.Scope.Kind != "gateway" || view.Scope.GatewayID != f.gateway.String() || view.Scope.Protocol != "pocsag" ||
		view.Processed != 1 {
		t.Fatalf("view=%s", encoded)
	}
}
