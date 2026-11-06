package borrowed_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
)

type workerSourceFixture struct {
	capture worker.Capture
	primary *postgres.Store
	last    platform.TransactionContext
	deny    bool
	checks  []processing.Candidate
	onCheck func()
}

// ReconcileCandidate supplies this test's explicit unchanged source pins, not production eligibility.
func (s *workerSourceFixture) ReconcileCandidate(_ context.Context, _ platform.TransactionContext, candidate processing.Candidate) (processing.Candidate, error) {
	if s.deny {
		return processing.Candidate{}, authz.ErrRevisionConflict
	}
	return candidate, nil
}

func (s *workerSourceFixture) CaptureCandidate(_ context.Context, tx platform.TransactionContext, _ processing.Candidate) (worker.Capture, error) {
	s.last = tx
	return s.capture, nil
}

func (s *workerSourceFixture) CheckCandidate(_ context.Context, _ platform.TransactionContext, candidate processing.Candidate, _ worker.Capture) error {
	s.checks = append(s.checks, candidate)
	if s.onCheck != nil {
		s.onCheck()
	}
	if _, err := s.primary.Resolve(s.last); err == nil {
		return errors.New("worker retained capture transaction during evaluation")
	}
	if s.deny {
		return authz.ErrRevisionConflict
	}
	return nil
}

type blockingWorkerArtifactsFixture struct {
	release <-chan struct{}
	started func()
	done    chan struct{}
}

func (a blockingWorkerArtifactsFixture) PrepareInstalled(ctx context.Context, _ uuid.UUID, _ int64) (*processing.InstalledArtifact, error) {
	defer close(a.done)
	if a.started != nil {
		a.started()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.release:
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("fixture provisioned module unavailable"))
	}
}

func TestProcessingWorkerPrivateEvaluationAndCurrentSourceRecheck(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"selected", "false", "dispatch", "dispatch-recovery", "invalid-definition", "renewed-preparation", "source-changed", "cancelled-preparation"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Every case selects the single due candidate of its database, so each owns one.
			primary := newStore(t)
			repository, err := processingstore.New(primary)
			if err != nil {
				t.Fatal(err)
			}
			actor := uuid.New()
			authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
			configuration := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Name: "worker",
				Scope: authz.Scope{Kind: "all"}, Enabled: true}
			version := processing.Version{ID: uuid.New(), WorkflowID: configuration.ID,
				CreatedBy: actor, Definition: preparedProcessingDefinition(t)}
			if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				if _, err := repository.LockRevision(tx.Context(), tx); err != nil {
					return err
				}
				now, err := primary.Now(tx.Context(), tx)
				if err != nil {
					return err
				}
				configuration.CreatedAt, configuration.UpdatedAt, version.CreatedAt = now, now, now
				if err := repository.SaveWorkflow(tx.Context(), tx, authority, "workflow.create", configuration, 0, now); err != nil {
					return err
				}
				if err := repository.SaveVersion(tx.Context(), tx, version); err != nil {
					return err
				}
				configuration.Revision, configuration.ActiveVersionID = 2, &version.ID
				return repository.SaveWorkflow(tx.Context(), tx, authority, "workflow.activate", configuration, 1, now)
			}); err != nil {
				t.Fatal(err)
			}
			var pending processing.Candidate
			binding := processing.CandidateBinding{TransmissionID: uuid.New(),
				WorkflowID: configuration.ID, WorkflowRevision: configuration.Revision,
				DefinitionVersionID: version.ID, SourceRevision: 1, ContextRevision: 1}
			if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				var err error
				binding.ConfigurationRevision, err = repository.LockRevision(tx.Context(), tx)
				if err != nil {
					return err
				}
				pending, err = repository.AdmitCandidate(tx.Context(), tx, binding)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				work, err := repository.SelectDueWork(tx.Context(), tx, 1)
				if err != nil {
					return err
				}
				if len(work) != 1 || work[0].ID != pending.ID {
					t.Fatalf("private due selection included terminal/unexpired work: %+v", work)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			flag, err := processing.Literal(processing.BooleanValue, name != "false")
			if err != nil {
				t.Fatal(err)
			}
			network := uuid.New()
			source, err := processing.CaptureSource(t.Context(), processing.SourceProjection{
				TransmissionID: binding.TransmissionID, NetworkID: network, Protocol: protocols.POCSAG,
				Form: "text", ObservedAt: pending.CreatedAt, SourceRevision: 1, ContextRevision: 1,
				Fields: []processing.CapturedField{{Reference: processing.FieldRef{Namespace: "source", Path: []string{"flag"}},
					Schema: processing.FieldSchema{Kind: processing.BooleanValue},
					Field: processing.FieldValue{State: processing.FieldPresent, Value: &flag,
						Dependencies: []processing.Dependency{{TransmissionID: binding.TransmissionID,
							NetworkID: network, SourceRevision: 1, ContextRevision: 1, Groups: []string{"payload"}}}}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			owner := &workerSourceFixture{primary: primary, deny: name == "source-changed",
				capture: worker.Capture{Definition: version.Definition, Source: source,
					Scope: processing.EvaluationScope{SourceEligible: true}}}
			if name == "invalid-definition" {
				owner.capture.Definition.Selector.Op = "unavailable"
			}
			runContext, cancel := context.WithCancel(t.Context())
			defer cancel()
			var preparationDone chan struct{}
			if name == "renewed-preparation" || name == "cancelled-preparation" {
				release := make(chan struct{})
				preparationDone = make(chan struct{})
				artifacts := blockingWorkerArtifactsFixture{release: release, done: preparationDone}
				if name == "cancelled-preparation" {
					artifacts.started = cancel
				} else {
					owner.onCheck = func() {
						if len(owner.checks) == 1 {
							close(release)
						}
					}
				}
				owner.capture.Artifacts = artifacts
				owner.capture.Definition.Steps = []processing.Step{{
					ID: "installed", Kind: "installed", OutputSchema: map[string]processing.FieldSchema{},
					Installed: &processing.InstalledStep{InstanceID: uuid.New(), ConfigurationRevision: 1},
				}}
			}
			workerID := uuid.New()
			runner, err := worker.New(primary, repository, owner, processing.CoreRegex{}, workerID)
			if err != nil {
				t.Fatal(err)
			}
			if name == "dispatch-recovery" {
				var claimed processing.Candidate
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					var err error
					claimed, err = repository.ClaimCandidate(tx.Context(), tx, pending.ID, pending.Revision, workerID)
					if err != nil {
						return err
					}
					executor, err := primary.Resolve(tx)
					if err != nil {
						return err
					}
					_, err = executor.ExecContext(tx.Context(),
						`UPDATE processing_candidates SET lease_until=updated_at+interval '1 microsecond' WHERE id=$1`, claimed.ID)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if err := runner.ExecuteWork(runContext, claimed); err != nil {
					t.Fatal(err)
				}
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					executor, err := primary.Resolve(tx)
					if err != nil {
						return err
					}
					var state string
					var revision, fence, attempts int64
					var due bool
					if err := executor.QueryRowContext(tx.Context(),
						`SELECT state,revision,fencing_token,attempts,due_at<=clock_timestamp()
						 FROM processing_candidates WHERE id=$1`, pending.ID).Scan(&state, &revision, &fence, &attempts, &due); err != nil {
						return err
					}
					if state != "pending" || revision != claimed.Revision+1 || fence != claimed.FencingToken+1 || attempts != 1 || due {
						t.Fatalf("dispatch recovery recomputed or reset work: state=%s revision=%d fence=%d attempts=%d due=%t",
							state, revision, fence, attempts, due)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				return
			}
			var completed processing.Candidate
			var result *processing.PrivateResult
			if name == "dispatch" {
				err = runner.ExecuteWork(runContext, pending)
				if err == nil {
					err = primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
						item, readErr := repository.ReadPublicationItem(tx.Context(), tx, binding)
						completed, result = item.Candidate, item.Result
						return readErr
					})
				}
			} else {
				completed, result, err = runner.Run(runContext, pending)
			}
			switch name {
			case "cancelled-preparation":
				if !errors.Is(err, context.Canceled) || result != nil {
					t.Fatalf("cancelled worker reported a result: %+v err=%v", result, err)
				}
				select {
				case <-preparationDone:
				default:
					t.Fatal("worker returned without joining preparation")
				}
			case "renewed-preparation":
				if platform.FailureCode(err) != platform.CodeAuthorityUnavailable || result != nil ||
					completed.State != "pending" || completed.Revision != pending.Revision+3 ||
					completed.FencingToken != 1 || completed.Attempts != 1 || completed.AutomaticAttempts != 1 ||
					len(owner.checks) != 2 || owner.checks[1].Revision != owner.checks[0].Revision+1 {
					t.Fatalf("periodic renewal lost latest lease/attempt history: %+v checks=%+v err=%v", completed, owner.checks, err)
				}
			case "source-changed":
				if !errors.Is(err, authz.ErrRevisionConflict) || result != nil {
					t.Fatalf("changed source committed private result: %+v err=%v", result, err)
				}
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					value, err := repository.SupersedeCandidate(tx.Context(), tx, pending.ID, pending.Revision+1)
					if err != nil {
						return err
					}
					if value.State != "superseded" || value.Attempts != 1 || value.AutomaticAttempts != 1 ||
						value.FencingToken != 2 || value.Revision != pending.Revision+2 {
						t.Fatalf("obsolete source lost history during supersession: %+v", value)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					_, err := repository.RenewCandidate(tx.Context(), tx, processing.CandidateLease{
						CandidateID: pending.ID, Revision: pending.Revision + 1, Fence: 1, WorkerID: workerID,
					})
					return err
				}); !errors.Is(err, authz.ErrRevisionConflict) {
					t.Fatalf("superseded primary claim renewed: %v", err)
				}
			case "invalid-definition":
				if err == nil || completed.State != "failed" || completed.AutomaticAttempts != 1 ||
					completed.FailureCode == nil || *completed.FailureCode != string(platform.CodeInvalidInput) || result != nil {
					t.Fatalf("compile failure not durably recorded: %+v err=%v", completed, err)
				}
			default:
				if err != nil || completed.State != "completed" || completed.Selected == nil ||
					*completed.Selected != (name == "selected" || name == "dispatch") || (result != nil) != (name == "selected" || name == "dispatch") ||
					completed.Attempts != 1 {
					t.Fatalf("incorrect private worker outcome: %+v result=%+v err=%v", completed, result, err)
				}
			}
		})
	}
}
