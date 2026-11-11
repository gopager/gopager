package borrowed_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/protocols"
)

var errRollbackFixture = errors.New("roll back source owner fixture")

func TestProcessingSourceOwnerMapsMissingMessageToNotFound(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	intakeRepository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	processingRepository, err := processingstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	identityRepository, err := identitystore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	assignmentRepository, err := assignmentstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := commandstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := identity.NewResolver(identityRepository, store, ingestTestScopeReferences{})
	if err != nil {
		t.Fatal(err)
	}
	matcher, err := assignments.NewService(store, resolver, store, assignmentRepository,
		unitRepository, intakeRepository, receipts, facts)
	if err != nil {
		t.Fatal(err)
	}
	host := processing.NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	principals := ingestTestPrincipalResolver{}
	executions, err := processing.NewInstanceExecutions(host, nil, store, processingRepository, principals)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executions.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	owner, err := ingeststore.NewProcessingSourceOwner(intakeRepository, matcher,
		processingRepository, facts, store, ingestTestArtifacts{}, executions, principals)
	if err != nil {
		t.Fatal(err)
	}

	network, gateway, workflow, version := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocolValue, err := processing.Literal(processing.StringValue, string(protocols.POCSAG))
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &protocolField, Value: &protocolValue},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
	encodedDefinition, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		for _, statement := range []struct {
			query string
			args  []any
		}{
			{`WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id) INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'owner network','pocsag',a.id FROM a`, []any{network}},
			{`INSERT INTO gateways(id,state) VALUES($1,'active')`, []any{gateway}},
			{`INSERT INTO gateway_networks(id,gateway_id,gateway_network_identifier,network_id,priority)
				VALUES($1,$2,'default',$3,0)`, []any{uuid.New(), gateway, network}},
			{`INSERT INTO processing_workflows(id,name,enabled,created_at,updated_at)
				VALUES($1,'owner workflow',true,$2,$2)`, []any{workflow, now}},
			{`INSERT INTO processing_workflow_agencies(workflow_id,agency_id) SELECT $1, id FROM agencies`, []any{workflow}},
			{`INSERT INTO processing_workflow_versions(id,workflow_id,definition,created_at,created_by)
				VALUES($1,$2,$3,$4,$5)`, []any{version, workflow, encodedDefinition, now, uuid.New()}},
			{`UPDATE processing_workflows SET active_version_id=$2 WHERE id=$1`, []any{workflow, version}},
		} {
			if _, err := executor.ExecContext(tx.Context(), statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "missing-message",
		Protocol: protocols.POCSAG, ObservationTime: now.Format(time.RFC3339),
		GatewayNetworkIdentifier: "default", Payload: json.RawMessage(`{"form":"tone-only"}`),
		TransmissionTargets: []protocols.Target{}}
	encoded, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	validated, err := submission.Validate(len(encoded), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	validated.ResolvedNetwork, validated.Resolution = network, "declared"
	var receipt ingest.Receipt
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		receipt, err = intakeRepository.AcceptTransmission(tx.Context(), tx, gateway, validated,
			now.Add(time.Second), assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// withoutMessage captures the publication binding, then removes the bound Message in the
	// same transaction (temporarily dropping the foreign key) and rolls everything back.
	withoutMessage := func(check func(platform.TransactionContext, processing.PublicationBinding,
		[]processing.CandidateBinding) error) error {
		t.Helper()
		var checked error
		err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			binding, candidates, err := owner.CapturePublication(tx.Context(), tx, receipt.TransmissionID)
			if err != nil {
				return err
			}
			if binding.MessageID != receipt.MessageID || len(candidates) != 1 {
				t.Fatalf("publication binding = %+v candidates = %+v", binding, candidates)
			}
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(),
				`ALTER TABLE transmissions DROP CONSTRAINT transmissions_message_id_fkey`); err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `DELETE FROM messages WHERE id=$1`,
				receipt.MessageID); err != nil {
				return err
			}
			checked = check(tx, binding, candidates)
			return errRollbackFixture
		})
		if !errors.Is(err, errRollbackFixture) {
			t.Fatal(err)
		}
		return checked
	}
	if err := withoutMessage(func(tx platform.TransactionContext, binding processing.PublicationBinding,
		candidates []processing.CandidateBinding) error {
		return owner.CheckItem(tx.Context(), tx, binding,
			processing.PublicationItem{Candidate: processing.Candidate{CandidateBinding: candidates[0]}, Definition: definition})
	}); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("CheckItem with missing Message = %v, want not found", err)
	}
	if err := withoutMessage(func(tx platform.TransactionContext, binding processing.PublicationBinding,
		_ []processing.CandidateBinding) error {
		return owner.ApplyProcessingStatus(tx.Context(), tx, binding)
	}); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("ApplyProcessingStatus with missing Message = %v, want not found", err)
	}
}

func TestIntakeStoreReadsSyntheticNetwork(t *testing.T) {
	t.Parallel()
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	intakeRepository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	live, deleted, agency := uuid.New(), uuid.New(), uuid.New()
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES($3,'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,deleted,agency_id) SELECT v.id,v.name,v.protocol,v.deleted,a.id FROM (VALUES ($1::uuid,'Live FLEX','flex',false),($2::uuid,'Deleted ZVEI','zvei',true)) AS v(id,name,protocol,deleted), a`, live, deleted, agency); err != nil {
			return err
		}
		got, err := intakeRepository.ReadSyntheticNetwork(tx.Context(), tx, live)
		if err != nil || got != (synthetic.Network{ID: live, Name: "Live FLEX", Protocol: protocols.FLEX, AgencyID: agency}) {
			t.Fatalf("live Network = %+v, err = %v", got, err)
		}
		got, err = intakeRepository.ReadSyntheticNetwork(tx.Context(), tx, deleted)
		if err != nil || got != (synthetic.Network{ID: deleted, Name: "Deleted ZVEI", Protocol: protocols.ZVEI, AgencyID: agency, Deleted: true}) {
			t.Fatalf("deleted Network = %+v, err = %v", got, err)
		}
		if _, err := intakeRepository.ReadSyntheticNetwork(tx.Context(), tx, uuid.New()); !errors.Is(err, authz.ErrNotFound) {
			t.Fatalf("missing Network error = %v, want not found", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
