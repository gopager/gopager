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
	"github.com/gopager/gopager/internal/events"
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
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
)

type noTargetMatcher struct{}

type intakeCommitAcknowledger struct {
	identity.Transactions
	loseNextAcknowledgment bool
}

func (a *intakeCommitAcknowledger) WithinTransaction(ctx context.Context, callback platform.TxFunc) error {
	if err := a.Transactions.WithinTransaction(ctx, callback); err != nil {
		return err
	}
	if a.loseNextAcknowledgment {
		a.loseNextAcknowledgment = false
		return platform.NewFailure(platform.CodeCommitUncertain, errors.New("injected lost commit acknowledgment"))
	}
	return nil
}

type intakeFailAfterAccept struct {
	ingest.Repository
	failNext bool
}

func (r *intakeFailAfterAccept) AcceptTransmission(ctx context.Context, tx platform.TransactionContext,
	gateway uuid.UUID, submission ingest.ValidatedSubmission, accepted time.Time,
	match assignments.MatchResult,
) (ingest.Receipt, error) {
	receipt, err := r.Repository.AcceptTransmission(ctx, tx, gateway, submission, accepted, match)
	if err != nil {
		return ingest.Receipt{}, err
	}
	if r.failNext {
		r.failNext = false
		return ingest.Receipt{}, errors.New("injected failure after accepted Transmission")
	}
	return receipt, nil
}

func (noTargetMatcher) NetworksWithMatches(context.Context, platform.TransactionContext, []uuid.UUID,
	protocols.Protocol, time.Time, []protocols.Target) (assignments.NetworkCandidates, error) {
	return assignments.NetworkCandidates{}, nil
}

func (noTargetMatcher) MatchTargets(context.Context, platform.TransactionContext, uuid.UUID,
	protocols.Protocol, time.Time, []protocols.Target) (assignments.MatchResult, error) {
	return assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}}, nil
}

type ingestTestScopeReferences struct{}

func (ingestTestScopeReferences) ScopeExists(context.Context, platform.TransactionContext, authz.Scope) (bool, error) {
	return true, nil
}

type ingestTestPrincipalResolver struct{}

func (ingestTestPrincipalResolver) ResolvePrincipalAuthority(context.Context,
	platform.TransactionContext, uuid.UUID,
) (authz.Authority, error) {
	return authz.Authority{}, nil
}

type ingestTestArtifacts struct{}

func (ingestTestArtifacts) PrepareInstalled(context.Context, uuid.UUID, int64) (*processing.InstalledArtifact, error) {
	return nil, errors.New("unexpected installed artifact request")
}

func (ingestTestArtifacts) LoadInstalled(context.Context, processing.InstanceConfiguration) (*processing.InstalledArtifact, error) {
	return nil, errors.New("unexpected installed artifact request")
}

type ingestTestProcessingAppender struct {
	events.ProcessingAppender
	fail bool
}

func (a *ingestTestProcessingAppender) AppendProcessingCompletion(ctx context.Context,
	tx platform.TransactionContext, fact events.ProcessingCompletionFact,
) (uuid.UUID, error) {
	id, err := a.ProcessingAppender.AppendProcessingCompletion(ctx, tx, fact)
	if err != nil {
		return uuid.Nil, err
	}
	if a.fail {
		return uuid.Nil, errors.New("injected failure after completion fact insert")
	}
	return id, nil
}

func TestGatewayCredentialCutoverStateAndNetworkReplacement(t *testing.T) {
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	network, gateway := uuid.New(), uuid.New()
	_, currentHash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	_, pendingHash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'ingest network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
			VALUES($1,'stopped')`, gateway); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_credentials
			(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,false,$4)`,
			uuid.New(), gateway, currentHash[:], now); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_credentials
			(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,true,$4)`,
			uuid.New(), gateway, pendingHash[:], now.Add(time.Microsecond))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	var authenticated identity.GatewayAuthentication
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		authenticated, err = repository.AuthenticateGatewayCredential(tx.Context(), tx, pendingHash)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if authenticated.GatewayID != gateway {
		t.Fatalf("authenticated Gateway=%s want %s", authenticated.GatewayID, gateway)
	}
	report := ingest.GatewayStateReport{SchemaVersion: 1, Operation: "heartbeat"}
	for _, expectedState := range []string{"active", "active"} {
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			now, err := store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			return repository.ReportGatewayState(tx.Context(), tx, gateway, report, now)
		}); err != nil {
			t.Fatal(err)
		}
		var actual string
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT state FROM gateways WHERE id=$1`, gateway).Scan(&actual)
		}); err != nil {
			t.Fatal(err)
		}
		if actual != expectedState {
			t.Fatalf("state=%q want %q", actual, expectedState)
		}
	}
	var revision int64
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		revision, err = repository.ReplaceGatewayNetworks(tx.Context(), tx, gateway, 3,
			[]ingest.GatewayNetwork{{Identifier: "replacement", NetworkID: network,
				Protocol: protocols.POCSAG, Default: true}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if revision != 4 {
		t.Fatalf("replacement revision=%d want 4", revision)
	}
	var currentRevoked, pendingActive bool
	var eventCount int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT revoked_at IS NOT NULL FROM gateway_credentials
			WHERE secret_hash=$1`, currentHash[:]).Scan(&currentRevoked); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT pending=false AND revoked_at IS NULL
			FROM gateway_credentials WHERE secret_hash=$1`, pendingHash[:]).Scan(&pendingActive); err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events`).Scan(&eventCount)
	}); err != nil {
		t.Fatal(err)
	}
	if !currentRevoked || !pendingActive || eventCount != 2 {
		t.Fatalf("cutover currentRevoked=%t pendingActive=%t events=%d", currentRevoked, pendingActive, eventCount)
	}
}

func TestTransmissionGroupingMergeRollbackAndOffsetPersistence(t *testing.T) {
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	network, gateway := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'grouping network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
			VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	base := now.Add(-time.Hour).Truncate(time.Second)
	observationZone := time.FixedZone("gateway-offset", 5*60*60+30*60)
	submit := func(id string, observed time.Time) ingest.Receipt {
		t.Helper()
		wire := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: observed.In(observationZone).Format(time.RFC3339), GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(`{"form":"tone-only"}`), TransmissionTargets: []protocols.Target{}}
		encoded, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		var receipt ingest.Receipt
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			accepted, err := store.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			validated, err := wire.Validate(len(encoded), accepted)
			if err != nil {
				return err
			}
			validated.ResolvedNetwork, validated.Resolution = network, "declared"
			receipt, err = repository.AcceptTransmission(tx.Context(), tx, gateway, validated, accepted,
				assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return receipt
	}
	first := submit("first", base)
	second := submit("second", base.Add(50*time.Second))
	third := submit("bridge", base.Add(25*time.Second))
	if first.MessageID == second.MessageID || third.MessageID != minMessageID(first.MessageID, second.MessageID) ||
		first.ProcessingStatus != "none" || third.ProcessingStatus != "none" {
		t.Fatalf("grouping receipts first=%+v second=%+v bridge=%+v", first, second, third)
	}
	var transmissionCount, messageCount, mergedFacts, unprocessedTransmissions, contributions, offset int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions`).Scan(&transmissionCount); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM messages`).Scan(&messageCount); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
			WHERE envelope->>'type'='message.merged'`).Scan(&mergedFacts); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions
			WHERE processing_status='none'`).Scan(&unprocessedTransmissions); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM message_contributions`).Scan(&contributions); err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT observation_offset_seconds FROM transmissions
			WHERE id=$1`, third.TransmissionID).Scan(&offset)
	}); err != nil {
		t.Fatal(err)
	}
	if transmissionCount != 3 || messageCount != 1 || mergedFacts != 1 || unprocessedTransmissions != 3 ||
		contributions != 1 || offset != 19800 {
		t.Fatalf("rows transmissions=%d messages=%d merged=%d unprocessed=%d contributions=%d offset=%d",
			transmissionCount, messageCount, mergedFacts, unprocessedTransmissions, contributions, offset)
	}
	receipt, fingerprint, found := receiptInStore(t, store, repository, gateway, "bridge")
	if !found || receipt.TransmissionID != third.TransmissionID || fingerprint == [32]byte{} {
		t.Fatalf("retry lookup receipt=%+v found=%t", receipt, found)
	}

	beforeEvents := 0
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events`).Scan(&beforeEvents)
	}); err != nil {
		t.Fatal(err)
	}
	abort := errors.New("rollback accepted Transmission")
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		accepted, err := store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		wire := ingest.Submission{SchemaVersion: 1, SubmissionID: "rollback", Protocol: protocols.POCSAG,
			ObservationTime: base.Add(26 * time.Second).Format(time.RFC3339), Payload: json.RawMessage(`{"form":"tone-only"}`),
			TransmissionTargets: []protocols.Target{}}
		encoded, err := json.Marshal(wire)
		if err != nil {
			return err
		}
		validated, err := wire.Validate(len(encoded), accepted)
		if err != nil {
			return err
		}
		validated.ResolvedNetwork, validated.Resolution = network, "inferred-single"
		if _, err := repository.AcceptTransmission(tx.Context(), tx, gateway, validated, accepted,
			assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}}); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("rollback error=%v", err)
	}
	var afterTransmissions, afterMessages, afterEvents, afterContributions int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions`).Scan(&afterTransmissions); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM messages`).Scan(&afterMessages); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events`).Scan(&afterEvents); err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM message_contributions`).Scan(&afterContributions)
	}); err != nil {
		t.Fatal(err)
	}
	if afterTransmissions != 3 || afterMessages != messageCount || afterEvents != beforeEvents ||
		afterContributions != contributions {
		t.Fatalf("rollback altered accepted evidence: transmissions/messages/events/contributions=%d/%d/%d/%d, expected 3/%d/%d/%d",
			afterTransmissions, afterMessages, afterEvents, afterContributions,
			messageCount, beforeEvents, contributions)
	}
}

func TestTransmissionAcceptanceRollsBackAtEachParticipant(t *testing.T) {
	tests := []struct {
		name, table, timing string
		selectedWorkflow    bool
	}{
		{"original and receipt", "transmissions", "BEFORE", false},
		{"grouping membership", "transmissions", "AFTER", false},
		{"target resolution", "transmission_targets", "BEFORE", false},
		{"contribution", "message_contributions", "BEFORE", false},
		{"required facts", "domain_events", "BEFORE", false},
		{"selected Processing work", "processing_candidates", "BEFORE", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newStore(t)
			facts, err := eventstore.New(store)
			if err != nil {
				t.Fatal(err)
			}
			repository, err := ingeststore.New(store, facts, facts)
			if err != nil {
				t.Fatal(err)
			}
			network, gateway, unit := uuid.New(), uuid.New(), uuid.New()
			workflow, version := uuid.New(), uuid.New()
			now := time.Now().UTC().Truncate(time.Microsecond)
			var workflowDefinition []byte
			if test.selectedWorkflow {
				workflowDefinition, err = json.Marshal(preparedProcessingDefinition(t))
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				executor, err := store.Resolve(tx)
				if err != nil {
					return err
				}
				if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
					INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'atomicity network','pocsag',a.id FROM a`, network); err != nil {
					return err
				}
				if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
					VALUES($1,'active')`, gateway); err != nil {
					return err
				}
				if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
					(id,gateway_id,gateway_network_identifier,network_id,is_default)
					VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network); err != nil {
					return err
				}
				if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
					INSERT INTO units(id,name,agency_id) SELECT $1,'atomicity unit',a.id FROM a`, unit); err != nil {
					return err
				}
				if test.selectedWorkflow {
					if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflows
						(id,name,scope_kind,enabled,created_at,updated_at)
						VALUES($1,'atomicity workflow','all',true,$2,$2)`, workflow, now); err != nil {
						return err
					}
					if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflow_versions
						(id,workflow_id,definition,created_at,created_by)
						VALUES($1,$2,$3,$4,$5)`, version, workflow, workflowDefinition, now, uuid.New()); err != nil {
						return err
					}
					if _, err := executor.ExecContext(tx.Context(), `UPDATE processing_workflows
						SET active_version_id=$2 WHERE id=$1`, workflow, version); err != nil {
						return err
					}
				}
				_, err = executor.ExecContext(tx.Context(), `CREATE FUNCTION test_intake_failpoint()
					RETURNS trigger LANGUAGE plpgsql AS $$
					BEGIN
						RAISE EXCEPTION 'injected intake participant failure' USING ERRCODE='P0001';
					END;
					$$`)
				if err != nil {
					return err
				}
				_, err = executor.ExecContext(tx.Context(), `CREATE TRIGGER test_intake_failpoint
					`+test.timing+` INSERT ON `+test.table+`
					FOR EACH ROW EXECUTE FUNCTION test_intake_failpoint()`)
				return err
			}); err != nil {
				t.Fatal(err)
			}

			function := uint8(0)
			submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "participant-failure",
				Protocol: protocols.POCSAG, ObservationTime: now.Format(time.RFC3339),
				GatewayNetworkIdentifier: "default", Payload: json.RawMessage(`{"form":"tone-only"}`),
				TransmissionTargets: []protocols.Target{{Address: protocols.NumericAddress(123), Function: &function}}}
			encoded, err := json.Marshal(submission)
			if err != nil {
				t.Fatal(err)
			}
			validated, err := submission.Validate(len(encoded), now.Add(time.Second))
			if err != nil {
				t.Fatal(err)
			}
			validated.ResolvedNetwork, validated.Resolution = network, "declared"
			match := assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{{
				TargetPosition: 0, Status: "assigned", UnitID: &unit,
			}}}
			err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				_, err := repository.AcceptTransmission(tx.Context(), tx, gateway, validated,
					now.Add(time.Second), match)
				return err
			})
			if err == nil {
				t.Fatal("injected participant failure was not returned")
			}

			var transmissions, messages, targets, contributions, domainEvents, candidates int
			var lastTransmissionIsNull bool
			if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				executor, err := store.Resolve(tx)
				if err != nil {
					return err
				}
				for _, count := range []struct {
					query string
					dest  *int
				}{
					{`SELECT count(*) FROM transmissions`, &transmissions},
					{`SELECT count(*) FROM messages`, &messages},
					{`SELECT count(*) FROM transmission_targets`, &targets},
					{`SELECT count(*) FROM message_contributions`, &contributions},
					{`SELECT count(*) FROM domain_events`, &domainEvents},
					{`SELECT count(*) FROM processing_candidates`, &candidates},
				} {
					if err := executor.QueryRowContext(tx.Context(), count.query).Scan(count.dest); err != nil {
						return err
					}
				}
				return executor.QueryRowContext(tx.Context(), `SELECT last_transmission_at IS NULL
					FROM gateways WHERE id=$1`, gateway).Scan(&lastTransmissionIsNull)
			}); err != nil {
				t.Fatal(err)
			}
			if transmissions != 0 || messages != 0 || targets != 0 || contributions != 0 ||
				domainEvents != 0 || candidates != 0 || !lastTransmissionIsNull {
				t.Fatalf("participant failure leaked transmission/message/target/contribution/fact/candidate=%d/%d/%d/%d/%d/%d, lastTransmissionIsNull=%t",
					transmissions, messages, targets, contributions, domainEvents, candidates, lastTransmissionIsNull)
			}
		})
	}
}

func TestConcurrentTransmissionReceiptsAreIdempotent(t *testing.T) {
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	service, err := ingest.NewService(store, store, repository, noTargetMatcher{})
	if err != nil {
		t.Fatal(err)
	}
	network, gateway := uuid.New(), uuid.New()
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'concurrent network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
			VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_credentials
			(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,false,$4)`,
			uuid.New(), gateway, hash[:], now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	observation := now.Format(time.RFC3339)
	submission := func(id, payload string) ingest.Submission {
		return ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: observation, GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(payload), TransmissionTargets: []protocols.Target{}}
	}
	submit := func(input ingest.Submission) (ingest.Receipt, error) {
		encoded, err := json.Marshal(input)
		if err != nil {
			return ingest.Receipt{}, err
		}
		return service.Submit(t.Context(), secret, input, len(encoded))
	}

	const retryCount = 8
	start := make(chan struct{})
	results := make(chan ingest.Receipt, retryCount)
	errs := make(chan error, retryCount)
	for range retryCount {
		go func() {
			<-start
			receipt, err := submit(submission("same", `{"form":"tone-only"}`))
			results <- receipt
			errs <- err
		}()
	}
	close(start)
	var first ingest.Receipt
	for range retryCount {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		receipt := <-results
		if first.TransmissionID == uuid.Nil {
			first = receipt
		} else if receipt.TransmissionID != first.TransmissionID || receipt.MessageID != first.MessageID {
			t.Fatalf("concurrent retry returned a different receipt: %+v != %+v", receipt, first)
		}
	}
	if first.TransmissionID == uuid.Nil {
		t.Fatal("concurrent submissions returned an empty receipt")
	}
	if _, err := submit(submission("same", `{"form":"text","value":"different"}`)); !errors.Is(err, ingest.ErrSubmissionConflict) {
		t.Fatalf("changed body retry error=%v want idempotency conflict", err)
	}
	type submitResult struct {
		receipt ingest.Receipt
		err     error
	}
	racingStart := make(chan struct{})
	racingResults := make(chan submitResult, 2)
	for _, payload := range []string{`{"form":"tone-only"}`, `{"form":"text","value":"racing"}`} {
		payload := payload
		go func() {
			<-racingStart
			receipt, err := submit(submission("different-race", payload))
			racingResults <- submitResult{receipt: receipt, err: err}
		}()
	}
	close(racingStart)
	var accepted, conflicted int
	for range 2 {
		result := <-racingResults
		switch {
		case result.err == nil && result.receipt.TransmissionID != uuid.Nil:
			accepted++
		case errors.Is(result.err, ingest.ErrSubmissionConflict):
			conflicted++
		default:
			t.Fatalf("different-body race result=%+v", result)
		}
	}
	if accepted != 1 || conflicted != 1 {
		t.Fatalf("different-body race accepted=%d conflicted=%d, want one each", accepted, conflicted)
	}
	const groupedCount = 8
	groupStart := make(chan struct{})
	groupResults := make(chan submitResult, groupedCount)
	for range groupedCount {
		id := uuid.NewString()
		go func() {
			<-groupStart
			receipt, err := submit(submission(id, `{"form":"tone-only"}`))
			groupResults <- submitResult{receipt: receipt, err: err}
		}()
	}
	close(groupStart)
	for range groupedCount {
		result := <-groupResults
		if result.err != nil || result.receipt.TransmissionID == uuid.Nil {
			t.Fatalf("distinct concurrent Submission failed: %+v", result)
		}
		if result.receipt.MessageID != first.MessageID {
			t.Fatalf("concurrent Payload-identical Transmission joined Message %s, want %s",
				result.receipt.MessageID, first.MessageID)
		}
	}
	var count int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions
			WHERE gateway_id=$1 AND submission_id='same'`, gateway).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent retries created %d Transmissions, want 1", count)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions
			WHERE gateway_id=$1`, gateway).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != groupedCount+2 {
		t.Fatalf("concurrent operations created %d Transmissions, want %d",
			count, groupedCount+2)
	}
}

func TestSubmissionRetryAfterKnownRollbackAndLostCommitAcknowledgment(t *testing.T) {
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	network, gateway := uuid.New(), uuid.New()
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'retry network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
			VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `INSERT INTO gateway_credentials
			(id,gateway_id,secret_hash,pending,created_at) VALUES($1,$2,$3,false,$4)`,
			uuid.New(), gateway, hash[:], now)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	newSubmission := func(id string) ingest.Submission {
		return ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
			ObservationTime: now.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
			Payload: json.RawMessage(`{"form":"tone-only"}`), TransmissionTargets: []protocols.Target{}}
	}
	submit := func(service *ingest.Service, input ingest.Submission) (ingest.Receipt, error) {
		encoded, err := json.Marshal(input)
		if err != nil {
			return ingest.Receipt{}, err
		}
		return service.Submit(t.Context(), secret, input, len(encoded))
	}

	failingRepository := &intakeFailAfterAccept{Repository: repository, failNext: true}
	rollbackService, err := ingest.NewService(store, store, failingRepository, noTargetMatcher{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := submit(rollbackService, newSubmission("known-rollback")); err == nil {
		t.Fatal("injected post-acceptance failure was not returned")
	}
	var transmissions int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions
			WHERE gateway_id=$1`, gateway).Scan(&transmissions)
	}); err != nil {
		t.Fatal(err)
	}
	if transmissions != 0 {
		t.Fatalf("known rollback left %d Transmissions", transmissions)
	}
	rollbackReceipt, err := submit(rollbackService, newSubmission("known-rollback"))
	if err != nil || rollbackReceipt.TransmissionID == uuid.Nil {
		t.Fatalf("retry after known rollback receipt=%+v error=%v", rollbackReceipt, err)
	}

	acknowledger := &intakeCommitAcknowledger{Transactions: store, loseNextAcknowledgment: true}
	uncertainService, err := ingest.NewService(acknowledger, store, repository, noTargetMatcher{})
	if err != nil {
		t.Fatal(err)
	}
	input := newSubmission("lost-acknowledgment")
	if _, err := submit(uncertainService, input); platform.FailureCode(err) != platform.CodeCommitUncertain {
		t.Fatalf("lost acknowledgment error=%v, want COMMIT_UNCERTAIN", err)
	}
	recoveredReceipt, err := submit(uncertainService, input)
	if err != nil || recoveredReceipt.TransmissionID == uuid.Nil {
		t.Fatalf("retry after uncertain commit receipt=%+v error=%v", recoveredReceipt, err)
	}
	repeatedReceipt, err := submit(uncertainService, input)
	if err != nil {
		t.Fatalf("repeat after receipt recovery: %v", err)
	}
	if repeatedReceipt.TransmissionID != recoveredReceipt.TransmissionID ||
		repeatedReceipt.MessageID != recoveredReceipt.MessageID {
		t.Fatalf("repeat after uncertain commit returned a different receipt: %+v != %+v",
			repeatedReceipt, recoveredReceipt)
	}
	if recoveredReceipt.TransmissionID == rollbackReceipt.TransmissionID {
		t.Fatal("independent Submissions returned the same Transmission")
	}
	var factCount, messageCount, candidateCount int
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM transmissions
			WHERE gateway_id=$1`, gateway).Scan(&transmissions); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events`).Scan(&factCount); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM messages`).Scan(&messageCount); err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM processing_candidates`).Scan(&candidateCount)
	}); err != nil {
		t.Fatal(err)
	}
	if transmissions != 2 || factCount != 2 || messageCount != 1 || candidateCount != 0 {
		t.Fatalf("known rollback and lost-ack retries left transmissions/facts/messages/candidates=%d/%d/%d/%d",
			transmissions, factCount, messageCount, candidateCount)
	}
}

func TestSelectedWorkflowKeepsProcessingPending(t *testing.T) {
	store := newStore(t)
	facts, err := eventstore.New(store)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		t.Fatal(err)
	}
	network, gateway := uuid.New(), uuid.New()
	workflow, version := uuid.New(), uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocolValue, err := processing.Literal(processing.StringValue, string(protocols.POCSAG))
	if err != nil {
		t.Fatal(err)
	}
	definition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &protocolField, Value: &protocolValue},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'workflow network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
			VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflows
			(id,name,scope_kind,enabled,created_at,updated_at)
			VALUES($1,'test workflow','all',true,$2,$2)`, workflow, now); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflow_versions
			(id,workflow_id,definition,created_at,created_by)
			VALUES($1,$2,$3,$4,$5)`, version, workflow, definition, now, uuid.New()); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE processing_workflows
			SET active_version_id=$2 WHERE id=$1`, workflow, version)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	acceptedAt := now.Add(time.Second)
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "selected-workflow",
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
	if receipt.ProcessingStatus != "pending" {
		t.Fatalf("selected Workflow receipt status=%q want pending", receipt.ProcessingStatus)
	}
	var candidateState, transmissionStatus string
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT state FROM processing_candidates
			WHERE transmission_id=$1 AND workflow_id=$2`, receipt.TransmissionID, workflow).Scan(&candidateState); err != nil {
			return err
		}
		return executor.QueryRowContext(tx.Context(), `SELECT processing_status
			FROM transmissions WHERE id=$1`, receipt.TransmissionID).Scan(&transmissionStatus)
	}); err != nil {
		t.Fatal(err)
	}
	if candidateState != "pending" || transmissionStatus != "pending" {
		t.Fatalf("candidate/transmission state=%s/%s, want pending/pending", candidateState, transmissionStatus)
	}
}

func TestSelectedWorkflowPublicationRollsBackAndRecovers(t *testing.T) {
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
	appender := &ingestTestProcessingAppender{ProcessingAppender: facts, fail: true}
	sourceOwner, err := ingeststore.NewProcessingSourceOwner(intakeRepository, matcher,
		processingRepository, appender, store, ingestTestArtifacts{}, executions, principals)
	if err != nil {
		t.Fatal(err)
	}

	network, gateway := uuid.New(), uuid.New()
	workflows, versions := []uuid.UUID{uuid.New(), uuid.New()}, []uuid.UUID{uuid.New(), uuid.New()}
	now := time.Now().UTC().Truncate(time.Microsecond)
	protocolField := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	protocolValue, err := processing.Literal(processing.StringValue, string(protocols.POCSAG))
	if err != nil {
		t.Fatal(err)
	}
	definition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &protocolField, Value: &protocolValue},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `WITH a AS (INSERT INTO agencies(id,name) VALUES(gen_random_uuid(),'Fixture agency') RETURNING id)
			INSERT INTO networks(id,name,protocol,agency_id) SELECT $1,'publication network','pocsag',a.id FROM a`, network); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateways(id,state)
			VALUES($1,'active')`, gateway); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO gateway_networks
			(id,gateway_id,gateway_network_identifier,network_id,is_default)
			VALUES($1,$2,'default',$3,true)`, uuid.New(), gateway, network); err != nil {
			return err
		}
		for i, workflow := range workflows {
			version := versions[i]
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflows
				(id,name,scope_kind,enabled,created_at,updated_at)
				VALUES($1,$2,'all',true,$3,$3)`, workflow, "publication workflow "+workflow.String(), now); err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflow_versions
				(id,workflow_id,definition,created_at,created_by)
				VALUES($1,$2,$3,$4,$5)`, version, workflow, definition, now, uuid.New()); err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `UPDATE processing_workflows
				SET active_version_id=$2 WHERE id=$1`, workflow, version); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: "publication-retry",
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
	var selected []processing.Candidate
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		page, err := processingRepository.ListCandidateWork(tx.Context(), tx, processing.PageInput{Limit: 10})
		if err != nil {
			return err
		}
		if page.Total != 2 || len(page.Items) != 2 {
			t.Fatalf("selected candidate page=%+v", page)
		}
		selected = page.Items
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service, err := worker.NewWithPublisher(store, processingRepository, sourceOwner, processing.CoreRegex{},
		uuid.New(), sourceOwner)
	if err != nil {
		t.Fatal(err)
	}
	firstCandidate, retryCandidate := selected[0], selected[1]
	// The injected fact failure rolls the first completion back together with its publication.
	if err := service.ExecuteWork(t.Context(), firstCandidate); err == nil {
		t.Fatal("injected publication failure was not returned")
	}
	var pending, running, completed int
	var processingStatus string
	var results, completionFacts int
	assertState := func(wantPending, wantRunning, wantCompleted int, wantProcessing string,
		wantResults, wantFacts int) {
		t.Helper()
		if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := store.Resolve(tx)
			if err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT
				count(*) FILTER (WHERE state='pending'),
				count(*) FILTER (WHERE state='running'),
				count(*) FILTER (WHERE state='completed')
				FROM processing_candidates WHERE transmission_id=$1`, receipt.TransmissionID).
				Scan(&pending, &running, &completed); err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT processing_status
				FROM transmissions WHERE id=$1`, receipt.TransmissionID).Scan(&processingStatus); err != nil {
				return err
			}
			if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM processing_results
				WHERE message_id=$1 AND published_at IS NOT NULL`, receipt.MessageID).Scan(&results); err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
				WHERE envelope->>'type'='processing.completed'
				AND envelope->'data'->>'transmissionId'=$1`, receipt.TransmissionID.String()).Scan(&completionFacts)
		}); err != nil {
			t.Fatal(err)
		}
		if pending != wantPending || running != wantRunning || completed != wantCompleted ||
			processingStatus != wantProcessing || results != wantResults || completionFacts != wantFacts {
			t.Fatalf("candidate states=%d/%d/%d transmission=%s results/facts=%d/%d; want %d/%d/%d %s %d/%d",
				pending, running, completed, processingStatus, results, completionFacts,
				wantPending, wantRunning, wantCompleted, wantProcessing, wantResults, wantFacts)
		}
	}
	assertState(1, 1, 0, "pending", 0, 0)

	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE processing_candidates
			SET updated_at=created_at, lease_until=created_at + interval '1 microsecond' WHERE id=$1`,
			firstCandidate.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	appender.fail = false
	if err := service.ExecuteWork(t.Context(), firstCandidate); err != nil {
		t.Fatal(err)
	}
	assertState(2, 0, 0, "pending", 0, 0)
	var recovered processing.Candidate
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `UPDATE processing_candidates
			SET due_at=clock_timestamp() WHERE id=$1`, firstCandidate.ID); err != nil {
			return err
		}
		page, err := processingRepository.ListCandidateWork(tx.Context(), tx, processing.PageInput{Limit: 10})
		if err != nil {
			return err
		}
		for _, candidate := range page.Items {
			if candidate.ID == firstCandidate.ID {
				recovered = candidate
				return nil
			}
		}
		t.Fatalf("recovered candidate was not selected for retry: %+v", page)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The recovered completion publishes its own result while the second Workflow is still pending.
	if err := service.ExecuteWork(t.Context(), recovered); err != nil {
		t.Fatal(err)
	}
	assertState(1, 0, 1, "pending", 1, 1)
	if err := service.ExecuteWork(t.Context(), retryCandidate); err != nil {
		t.Fatal(err)
	}
	assertState(0, 0, 2, "completed", 2, 2)
	// Dispatching stale candidate values of completed work is a no-op; a stale claim is fenced by the revision.
	if err := service.ExecuteWork(t.Context(), firstCandidate); err != nil {
		t.Fatalf("stale dispatch of completed work failed: %v", err)
	}
	if err := service.ExecuteWork(t.Context(), retryCandidate); err != nil {
		t.Fatalf("stale dispatch of completed work failed: %v", err)
	}
	if _, _, err := service.Run(t.Context(), retryCandidate); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale claim was not fenced: %s", causeChain(err))
	}
	assertState(0, 0, 2, "completed", 2, 2)

	failedWorkflow, failedVersion := uuid.New(), uuid.New()
	missingField := processing.FieldRef{Namespace: "source", Path: []string{"not-declared"}}
	missingValue, err := processing.Literal(processing.StringValue, "missing")
	if err != nil {
		t.Fatal(err)
	}
	failedDefinition, err := json.Marshal(processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &missingField, Value: &missingValue},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflows
			(id,name,scope_kind,enabled,created_at,updated_at)
			VALUES($1,'invalid selected workflow','all',true,$2,$2)`, failedWorkflow, now); err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `INSERT INTO processing_workflow_versions
			(id,workflow_id,definition,created_at,created_by)
			VALUES($1,$2,$3,$4,$5)`, failedVersion, failedWorkflow, failedDefinition, now, uuid.New()); err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `UPDATE processing_workflows
			SET active_version_id=$2 WHERE id=$1`, failedWorkflow, failedVersion)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	failedSubmission := ingest.Submission{SchemaVersion: 1, SubmissionID: "failed-workflow",
		Protocol: protocols.POCSAG, ObservationTime: now.Format(time.RFC3339),
		GatewayNetworkIdentifier: "default", Payload: json.RawMessage(`{"form":"text","value":"failure-case"}`),
		TransmissionTargets: []protocols.Target{}}
	encoded, err = json.Marshal(failedSubmission)
	if err != nil {
		t.Fatal(err)
	}
	validated, err = failedSubmission.Validate(len(encoded), now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	validated.ResolvedNetwork, validated.Resolution = network, "declared"
	var failedReceipt ingest.Receipt
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		failedReceipt, err = intakeRepository.AcceptTransmission(tx.Context(), tx, gateway, validated,
			now.Add(time.Second), assignments.MatchResult{Revision: 1, Resolutions: []assignments.Resolution{}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var invalidCandidate processing.Candidate
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		page, err := processingRepository.ListCandidateWork(tx.Context(), tx, processing.PageInput{Limit: 10})
		if err != nil {
			return err
		}
		for _, candidate := range page.Items {
			if candidate.TransmissionID == failedReceipt.TransmissionID && candidate.WorkflowID == failedWorkflow {
				invalidCandidate = candidate
				return nil
			}
		}
		return errors.New("invalid selected Workflow candidate was not admitted")
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.ExecuteWork(t.Context(), invalidCandidate); err == nil {
		t.Fatal("invalid selected Workflow did not fail")
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var pendingCandidates, failedCandidates int
		var processingStatus string
		var publishedResults, completionEvents int
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			count(*) FILTER (WHERE state='pending'),count(*) FILTER (WHERE state='failed')
			FROM processing_candidates WHERE transmission_id=$1`, failedReceipt.TransmissionID).
			Scan(&pendingCandidates, &failedCandidates); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT processing_status FROM transmissions WHERE id=$1`,
			failedReceipt.TransmissionID).Scan(&processingStatus); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM processing_results
			WHERE candidate_id IN (SELECT id FROM processing_candidates WHERE transmission_id=$1)
			AND published_at IS NOT NULL`, failedReceipt.TransmissionID).Scan(&publishedResults); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
			WHERE envelope->>'type'='processing.completed'
			AND envelope->'data'->>'transmissionId'=$1`, failedReceipt.TransmissionID.String()).
			Scan(&completionEvents); err != nil {
			return err
		}
		if pendingCandidates != 2 || failedCandidates != 1 || processingStatus != "pending" ||
			publishedResults != 0 || completionEvents != 0 {
			t.Fatalf("failed work leaked publication: candidates=%d/%d transmission=%s results/facts=%d/%d",
				pendingCandidates, failedCandidates, processingStatus, publishedResults, completionEvents)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		_, err := assignmentRepository.MarkPending(tx.Context(), tx, []uuid.UUID{network}, nil, "rule-change")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// A pending correction in the Network no longer gates a Transmission whose targets are current.
	err = store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return sourceOwner.Publish(tx.Context(), tx, receipt.TransmissionID)
	})
	if err != nil {
		t.Fatalf("publication during an unrelated pending correction failed: %v", err)
	}
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := store.Resolve(tx)
		if err != nil {
			return err
		}
		var processingStatus string
		var publishedResults, completionFacts int
		if err := executor.QueryRowContext(tx.Context(), `SELECT processing_status
			FROM transmissions WHERE id=$1`, receipt.TransmissionID).Scan(&processingStatus); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM processing_results
			WHERE message_id=$1 AND published_at IS NOT NULL`, receipt.MessageID).Scan(&publishedResults); err != nil {
			return err
		}
		if err := executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
			WHERE envelope->>'type'='processing.completed'
			AND envelope->'data'->>'transmissionId'=$1`, receipt.TransmissionID.String()).
			Scan(&completionFacts); err != nil {
			return err
		}
		if processingStatus != "completed" || publishedResults != 2 || completionFacts != 2 {
			t.Fatalf("publication changed completed evidence: transmission=%s results/facts=%d/%d",
				processingStatus, publishedResults, completionFacts)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func receiptInStore(t *testing.T, store *postgres.Store, repository *ingeststore.Store,
	gateway uuid.UUID, id string) (ingest.Receipt, [32]byte, bool) {
	t.Helper()
	var receipt ingest.Receipt
	var fingerprint [32]byte
	var found bool
	if err := store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		var err error
		receipt, fingerprint, found, err = repository.SubmissionReceipt(tx.Context(), tx, gateway, id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return receipt, fingerprint, found
}

func minMessageID(left, right uuid.UUID) uuid.UUID {
	if left.String() < right.String() {
		return left
	}
	return right
}
