package borrowed_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

func narrowedScope(kind string, agencies []uuid.UUID, references ...uuid.UUID) processing.WorkflowScope {
	return processing.WorkflowScope{Agencies: agencies,
		Narrowing: &processing.WorkflowNarrowing{Kind: kind, References: references}}
}

func TestProcessingWorkflowScopeValidationAgainstMasterData(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	both := []uuid.UUID{f.agencies[0], f.agencies[1]}
	cases := map[string]processing.WorkflowScope{
		"unknown Agency":                    {Agencies: []uuid.UUID{uuid.New()}},
		"reference outside assigned Agency": narrowedScope("network", []uuid.UUID{f.agencies[0]}, f.networks[1]),
		"assigned Agency without reference": narrowedScope("network", both, f.networks[0]),
		"unknown Network":                   narrowedScope("network", []uuid.UUID{f.agencies[0]}, uuid.New()),
		"unknown Unit":                      narrowedScope("unit", []uuid.UUID{f.agencies[0]}, uuid.New()),
	}
	for name, scope := range cases {
		_, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), processingMetadataFixture(name, scope), checkedMutation)
		if !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: err=%v, want invalid input", name, err)
		}
	}
	valid := narrowedScope("network", both, f.networks[0], f.networks[1])
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), processingMetadataFixture("valid", valid), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := service.ReadWorkflow(t.Context(), f.proof, created.ID)
	if err != nil || len(stored.Scope.Agencies) != 2 || stored.Scope.Narrowing == nil ||
		stored.Scope.Narrowing.Kind != "network" || len(stored.Scope.Narrowing.References) != 2 {
		t.Fatalf("scope did not round-trip: %+v %v", stored.Scope, err)
	}
}

func TestProcessingWorkflowScopeAuthorizationAndVisibility(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	both := []uuid.UUID{f.agencies[0], f.agencies[1]}
	shared, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("both", processing.WorkflowScope{Agencies: both}), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	own, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("own", f.agencyScope()), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("foreign", processing.WorkflowScope{Agencies: []uuid.UUID{f.agencies[1]}}), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agencies[0], Name: "narrowed"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	narrowed, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("unit", narrowedScope("unit", []uuid.UUID{f.agencies[0]}, unit.ID)), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	agencyGrant := func(actions ...string) []authz.Grant {
		return []authz.Grant{{Actions: actions,
			Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}}}
	}
	replaceManagementGrants(t, f, agencyGrant("workflow.read", "workflow.create", "workflow.update", "workflow.delete"))

	page, err := service.ListWorkflows(t.Context(), f.proof, processing.PageInput{})
	if err != nil {
		t.Fatal(err)
	}
	var visible []uuid.UUID
	for _, item := range page.Items {
		visible = append(visible, item.ID)
	}
	for id, want := range map[uuid.UUID]bool{shared.ID: true, own.ID: true, narrowed.ID: true, foreign.ID: false} {
		if slices.Contains(visible, id) != want || page.Total != 3 {
			t.Fatalf("Agency reader list=%v total=%d, workflow %s want visible=%v", visible, page.Total, id, want)
		}
	}
	// Seeing a Workflow of two Agencies does not allow changing it: every assigned Agency is needed.
	if _, err := service.ReadWorkflow(t.Context(), f.proof, shared.ID); err != nil {
		t.Fatalf("reader of one assigned Agency cannot read: %v", err)
	}
	if _, err := service.UpdateWorkflow(t.Context(), f.proof, shared.ID, 1,
		processingMetadataFixture("renamed", processing.WorkflowScope{Agencies: both}), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update of a two-Agency Workflow by a one-Agency manager: %v", err)
	}
	if _, err := service.DeleteWorkflow(t.Context(), f.proof, shared.ID, 1, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("delete of a two-Agency Workflow by a one-Agency manager: %v", err)
	}
	// Create needs every assigned Agency.
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("wider", processing.WorkflowScope{Agencies: both}), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("create over a foreign Agency: %v", err)
	}
	// Update checks stored and new scope: widening an own Workflow to a foreign Agency is refused.
	if _, err := service.UpdateWorkflow(t.Context(), f.proof, own.ID, 1,
		processingMetadataFixture("widened", processing.WorkflowScope{Agencies: both}), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("widening update accepted: %v", err)
	}
	if _, err := service.UpdateWorkflow(t.Context(), f.proof, own.ID, 1,
		processingMetadataFixture("renamed", f.agencyScope()), checkedMutation); err != nil {
		t.Fatalf("own Workflow update: %v", err)
	}

	// A Unit grant manages only Workflows narrowed to its Unit.
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"workflow.read", "workflow.create", "workflow.update"},
		Scope: authz.Scope{Kind: "unit", References: []string{unit.ID.String()}}}})
	page, err = service.ListWorkflows(t.Context(), f.proof, processing.PageInput{})
	if err != nil || page.Total != 1 || page.Items[0].ID != narrowed.ID {
		t.Fatalf("Unit reader list: %+v %v", page, err)
	}
	if _, err := service.UpdateWorkflow(t.Context(), f.proof, narrowed.ID, 1,
		processingMetadataFixture("unit renamed", narrowedScope("unit", []uuid.UUID{f.agencies[0]}, unit.ID)), checkedMutation); err != nil {
		t.Fatalf("Unit manager cannot update its narrowed Workflow: %v", err)
	}
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("agency wide", f.agencyScope()), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Unit grant created an Agency-wide Workflow: %v", err)
	}
	// A narrowed Unit that moved to another Agency no longer lets its Unit grants manage the
	// Workflow: the assigned Agency is then required as well.
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE units SET agency_id = $2 WHERE id = $1`, unit.ID, f.agencies[1])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateWorkflow(t.Context(), f.proof, narrowed.ID, 2,
		processingMetadataFixture("drifted", narrowedScope("unit", []uuid.UUID{f.agencies[0]}, unit.ID)), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Unit grant managed a Workflow whose Unit moved to another Agency: %v", err)
	}
}

func TestAdmissionSelectsWorkflowsOfTheNetworkAgencyAndNarrowing(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	agencyA, agencyB := uuid.New(), uuid.New()
	network, otherNetwork, gateway, unit := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocolValue, err := processing.Literal(processing.StringValue, string(protocols.POCSAG))
	if err != nil {
		t.Fatal(err)
	}
	definition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocolField, Value: &protocolValue},
		Outputs:  map[string]map[string]processing.Assignment{}})
	if err != nil {
		t.Fatal(err)
	}
	// name -> expected to be selected for a Transmission on `network` of Agency A.
	type fixture struct {
		id        uuid.UUID
		agencies  []uuid.UUID
		narrowing string
		network   *uuid.UUID
		unit      *uuid.UUID
		selected  bool
	}
	fixtures := []fixture{
		{id: uuid.New(), agencies: []uuid.UUID{agencyA}, selected: true},
		{id: uuid.New(), agencies: []uuid.UUID{agencyA, agencyB}, selected: true},
		{id: uuid.New(), agencies: []uuid.UUID{agencyB}},
		{id: uuid.New(), agencies: []uuid.UUID{agencyA}, narrowing: "network", network: &network, selected: true},
		{id: uuid.New(), agencies: []uuid.UUID{agencyA}, narrowing: "network", network: &otherNetwork},
		{id: uuid.New(), agencies: []uuid.UUID{agencyA}, narrowing: "unit", unit: &unit},
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		statements := []struct {
			query string
			args  []any
		}{
			{`INSERT INTO agencies(id,name) VALUES($1,'Agency A'),($2,'Agency B')`, []any{agencyA, agencyB}},
			{`INSERT INTO networks(id,name,protocol,agency_id) VALUES($1,'network','pocsag',$3),($2,'other','pocsag',$4)`,
				[]any{network, otherNetwork, agencyA, agencyB}},
			{`INSERT INTO units(id,name,agency_id) VALUES($1,'narrowing unit',$2)`, []any{unit, agencyA}},
			{`INSERT INTO gateways(id,state) VALUES($1,'active')`, []any{gateway}},
			{`INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
				VALUES($1,$2,'default',$3,0)`, []any{uuid.New(), gateway, network}},
		}
		for _, w := range fixtures {
			version := uuid.New()
			var kind any
			if w.narrowing != "" {
				kind = w.narrowing
			}
			statements = append(statements, []struct {
				query string
				args  []any
			}{
				{`INSERT INTO processing_workflows(id,name,narrowing_kind,enabled,created_at,updated_at)
					VALUES($1,'admission workflow',$2,true,$3,$3)`, []any{w.id, kind, now}},
				{`INSERT INTO processing_workflow_versions(id,workflow_id,definition,created_at,created_by)
					VALUES($1,$2,$3,$4,$5)`, []any{version, w.id, definition, now, uuid.New()}},
				{`UPDATE processing_workflows SET active_version_id=$2 WHERE id=$1`, []any{w.id, version}},
			}...)
			for _, agency := range w.agencies {
				statements = append(statements, struct {
					query string
					args  []any
				}{`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) VALUES($1,$2)`, []any{w.id, agency}})
			}
			for _, reference := range []struct {
				column string
				id     *uuid.UUID
			}{{"network_id", w.network}, {"unit_id", w.unit}} {
				if reference.id != nil {
					statements = append(statements, struct {
						query string
						args  []any
					}{`INSERT INTO processing_workflow_scope_references(id,workflow_id,` + reference.column + `) VALUES($1,$2,$3)`,
						[]any{uuid.New(), w.id, *reference.id}})
				}
			}
		}
		for _, statement := range statements {
			if _, err := executor.ExecContext(tx.Context(), statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	acceptedAt := now.Add(time.Second)
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "scope-admission",
		Protocol: protocols.POCSAG, ObservationTime: now.Format(time.RFC3339),
		GatewayNetworkIdentifier: "default", Payload: json.RawMessage(`{"form":"tone-only"}`),
		TransmissionTargets: []protocols.Target{}}
	encoded, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := submission.Validate(len(encoded), acceptedAt)
	if err != nil {
		t.Fatal(err)
	}
	validated.ResolvedNetwork, validated.Resolution = network, "declared"
	var receipt ingest.Receipt
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		receipt, err = repository.AcceptTransmission(tx.Context(), tx, gateway, validated, acceptedAt,
			assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	selected := map[uuid.UUID]bool{}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		rows, err := executor.QueryContext(tx.Context(), `SELECT workflow_id FROM processing_candidates WHERE transmission_id=$1`, receipt.TransmissionID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return err
			}
			selected[id] = true
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	for i, w := range fixtures {
		if selected[w.id] != w.selected {
			t.Errorf("fixture %d (agencies=%d narrowing=%q): selected=%v want %v", i, len(w.agencies), w.narrowing, selected[w.id], w.selected)
		}
	}
}
