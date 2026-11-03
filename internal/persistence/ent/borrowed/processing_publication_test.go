package borrowed_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/publication"
)

// publicationSourceFixture owns a real SQL test barrier, not a production AP-06 Original or Message.
type publicationSourceFixture struct {
	primary    *postgres.Store
	repository *processingstore.Store
	binding    processing.PublicationBinding
	candidates []processing.CandidateBinding
	deny       bool
	failReady  bool
	mutate     bool
	staleFence bool
	supersede  bool
}

func (s *publicationSourceFixture) CapturePublication(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (processing.PublicationBinding, []processing.CandidateBinding, error) {
	if _, err := s.repository.LockRevision(ctx, tx); err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	var ready bool
	binding := s.binding
	if err := executor.QueryRowContext(ctx,
		`SELECT ready, fence, message_id, network_id, source_revision, context_revision
		 FROM processing_publication_fixture WHERE id=$1 FOR UPDATE`, id).
		Scan(&ready, &binding.SourceFence, &binding.MessageID, &binding.Source.NetworkID,
			&binding.Source.SourceRevision, &binding.Source.ContextRevision); err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	if ready || id != s.binding.Source.TransmissionID {
		return processing.PublicationBinding{}, nil, authz.ErrRevisionConflict
	}
	if s.staleFence {
		binding.SourceFence++
	}
	return binding, s.candidates, nil
}

func (s *publicationSourceFixture) CheckItem(ctx context.Context, tx platform.TransactionContext, binding processing.PublicationBinding, item processing.PublicationItem) error {
	if s.deny || binding.Source.NetworkID != s.binding.Source.NetworkID {
		return authz.ErrRevisionConflict
	}
	if item.Result != nil {
		if !item.Result.Evaluation.Selected || len(item.Result.Evaluation.TargetResults) != 0 {
			return authz.ErrInvalidInput
		}
		if s.supersede {
			_, err := s.repository.SupersedeCandidate(ctx, tx, item.Candidate.ID, item.Candidate.Revision)
			return err
		}
		if s.mutate {
			item.Result.Evaluation.Fields["injected"] = processing.FieldValue{State: processing.FieldAbsent}
			item.Definition.OutputSchema["injected"] = processing.FieldSchema{Kind: processing.StringValue, Optional: true}
		}
	}
	return nil
}

func (s *publicationSourceFixture) ApplyReadiness(ctx context.Context, tx platform.TransactionContext, binding processing.PublicationBinding) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `UPDATE processing_publication_fixture
		SET ready=true WHERE id=$1 AND fence=$2 AND message_id=$3 AND network_id=$4
			AND source_revision=$5 AND context_revision=$6 AND ready=false`,
		binding.Source.TransmissionID, binding.SourceFence, binding.MessageID,
		binding.Source.NetworkID, binding.Source.SourceRevision, binding.Source.ContextRevision)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 || s.failReady {
		return authz.ErrRevisionConflict
	}
	return nil
}

type failingPublicationFacts struct{ events.ProcessingAppender }

func (f failingPublicationFacts) AppendProcessingCompletion(ctx context.Context, tx platform.TransactionContext, fact events.ProcessingCompletionFact) (uuid.UUID, error) {
	id, err := f.ProcessingAppender.AppendProcessingCompletion(ctx, tx, fact)
	if err != nil {
		return uuid.Nil, err
	}
	return id, platform.NewFailure(platform.CodeStorageUnavailable, errors.New("fixture failure after real fact insert"))
}

func TestProcessingPublicationAtomicBarrierAndFences(t *testing.T) {
	primary := newStore(t)
	repository, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := eventstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	var bindings []processing.CandidateBinding
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := repository.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		for range 2 {
			configuration := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1,
				Name: uuid.NewString(), Scope: authz.Scope{Kind: "all"}, Enabled: true,
				CreatedAt: now, UpdatedAt: now}
			version := processing.Version{ID: uuid.New(), WorkflowID: configuration.ID,
				CreatedAt: now, CreatedBy: actor, Definition: preparedProcessingDefinition(t)}
			if err := repository.SaveWorkflow(tx.Context(), tx, authority, "rule.create", configuration, 0, now); err != nil {
				return err
			}
			if err := repository.SaveVersion(tx.Context(), tx, version); err != nil {
				return err
			}
			configuration.Revision, configuration.ActiveVersionID = 2, &version.ID
			if err := repository.SaveWorkflow(tx.Context(), tx, authority, "rule.activate", configuration, 1, now); err != nil {
				return err
			}
			bindings = append(bindings, processing.CandidateBinding{WorkflowID: configuration.ID,
				WorkflowRevision: configuration.Revision, DefinitionVersionID: version.ID,
				SourceRevision: 1, ContextRevision: 1})
		}
		executor, err := primary.Resolve(tx)
		if err != nil {
			return err
		}
		_, err = executor.ExecContext(tx.Context(), `CREATE TABLE processing_publication_fixture
			(id uuid PRIMARY KEY, message_id uuid NOT NULL, network_id uuid NOT NULL,
			source_revision bigint NOT NULL, context_revision bigint NOT NULL,
			fence bigint NOT NULL, ready boolean NOT NULL DEFAULT false)`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"selected", "false", "no-candidate", "pending", "stale-config",
		"stale-fence", "principal-revoked", "fact-failure", "readiness-failure", "duplicate-set",
		"source-changed", "context-changed", "network-changed", "superseded", "detached-check", "concurrent"} {
		t.Run(name, func(t *testing.T) {
			source := processing.PublicationBinding{Source: processing.SourceIdentity{
				TransmissionID: uuid.New(), NetworkID: uuid.New(), SourceRevision: 1, ContextRevision: 1},
				MessageID: uuid.New(), SourceFence: 1}
			owner := &publicationSourceFixture{primary: primary, repository: repository, binding: source,
				deny: name == "principal-revoked", failReady: name == "readiness-failure",
				mutate: name == "detached-check", staleFence: name == "stale-fence", supersede: name == "superseded"}
			var completed []processing.Candidate
			if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				configurationRevision, err := repository.LockRevision(tx.Context(), tx)
				if err != nil {
					return err
				}
				if name != "no-candidate" {
					for i, binding := range bindings {
						binding.TransmissionID, binding.ConfigurationRevision = source.Source.TransmissionID, configurationRevision
						candidate, err := repository.AdmitCandidate(tx.Context(), tx, binding)
						if err != nil {
							return err
						}
						owner.candidates = append(owner.candidates, binding)
						if name == "pending" && i == 1 {
							continue
						}
						candidate, err = repository.ClaimCandidate(tx.Context(), tx, candidate.ID, candidate.Revision, uuid.New())
						if err != nil {
							return err
						}
						lease, err := candidate.Lease()
						if err != nil {
							return err
						}
						selected := name != "false" && i == 0
						evaluation := processing.Evaluation{Selected: selected}
						if selected {
							evaluation.Fields = map[string]processing.FieldValue{}
						}
						candidate, _, err = repository.CompleteCandidate(tx.Context(), tx, lease, 0, evaluation)
						if err != nil {
							return err
						}
						completed = append(completed, candidate)
					}
				}
				executor, err := primary.Resolve(tx)
				if err != nil {
					return err
				}
				_, err = executor.ExecContext(tx.Context(),
					`INSERT INTO processing_publication_fixture(id,message_id,fence,network_id,source_revision,context_revision)
					 VALUES($1,$2,$3,$4,$5,$6)`,
					source.Source.TransmissionID, source.MessageID, source.SourceFence,
					source.Source.NetworkID, source.Source.SourceRevision, source.Source.ContextRevision)
				if err != nil {
					return err
				}
				switch name {
				case "source-changed":
					_, err = executor.ExecContext(tx.Context(),
						`UPDATE processing_publication_fixture SET source_revision=2 WHERE id=$1`, source.Source.TransmissionID)
				case "context-changed":
					_, err = executor.ExecContext(tx.Context(),
						`UPDATE processing_publication_fixture SET context_revision=2 WHERE id=$1`, source.Source.TransmissionID)
				case "network-changed":
					_, err = executor.ExecContext(tx.Context(),
						`UPDATE processing_publication_fixture SET network_id=$2 WHERE id=$1`, source.Source.TransmissionID, uuid.New())
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if name == "stale-config" {
				owner.candidates[0].ConfigurationRevision++
			}
			if name == "duplicate-set" {
				owner.candidates = append(owner.candidates, owner.candidates[0])
			}
			var appender events.ProcessingAppender = facts
			if name == "fact-failure" {
				appender = failingPublicationFacts{facts}
			}
			service, err := publication.New(repository, owner, appender, primary)
			if err != nil {
				t.Fatal(err)
			}
			publish := func() error {
				return primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					return service.Publish(tx.Context(), tx, source.Source.TransmissionID)
				})
			}
			success := name == "selected" || name == "false" || name == "no-candidate" || name == "detached-check" || name == "concurrent"
			if name == "concurrent" {
				var wait sync.WaitGroup
				results := make(chan error, 2)
				for range 2 {
					wait.Go(func() { results <- publish() })
				}
				wait.Wait()
				close(results)
				winners := 0
				for err := range results {
					if err == nil {
						winners++
					} else if !errors.Is(err, authz.ErrRevisionConflict) {
						t.Fatalf("unexpected concurrent publication failure: %v", err)
					}
				}
				if winners != 1 {
					t.Fatalf("publication winners=%d", winners)
				}
			} else if err := publish(); (err == nil) != success {
				t.Fatalf("publication success=%t err=%v", success, err)
			}
			if success {
				if err := publish(); !errors.Is(err, authz.ErrRevisionConflict) {
					t.Fatalf("repeated source publication was not fenced: %v", err)
				}
			}
			if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				executor, err := primary.Resolve(tx)
				if err != nil {
					return err
				}
				var ready bool
				var applied, published, factCount, injected int
				if err := executor.QueryRowContext(tx.Context(), `SELECT ready,
					(SELECT count(*) FROM processing_candidates WHERE transmission_id=$1 AND barrier_applied_at IS NOT NULL),
					(SELECT count(*) FROM processing_results WHERE message_id=$2 AND published_at IS NOT NULL),
					(SELECT count(*) FROM domain_events WHERE envelope->'data'->>'transmissionId'=$3),
					(SELECT count(*) FROM domain_events WHERE envelope->'data'->>'transmissionId'=$3
						AND envelope->'data'->'result'->'fields' ? 'injected')
					FROM processing_publication_fixture WHERE id=$1`,
					source.Source.TransmissionID, source.MessageID, source.Source.TransmissionID.String()).
					Scan(&ready, &applied, &published, &factCount, &injected); err != nil {
					return err
				}
				wantApplied, wantFacts := 0, 0
				if success {
					wantApplied = len(completed)
					if name != "false" && name != "no-candidate" {
						wantFacts = 1
					}
				}
				if ready != success || applied != wantApplied || published != wantFacts || factCount != wantFacts || injected != 0 {
					t.Fatalf("partial/substituted publication: ready=%t applied=%d published=%d facts=%d injected=%d",
						ready, applied, published, factCount, injected)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !success && len(completed) > 0 {
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					item, err := repository.ReadPublicationItem(tx.Context(), tx, completed[0].CandidateBinding)
					if err != nil {
						return err
					}
					if item.Candidate.Revision != completed[0].Revision || item.Candidate.FencingToken != completed[0].FencingToken {
						t.Fatal("rolled-back publication advanced private candidate revision/fence")
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			if name == "selected" {
				reception := source
				reception.Source.TransmissionID = uuid.New()
				pendingOwner := &publicationSourceFixture{primary: primary, repository: repository, binding: reception}
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					for _, binding := range owner.candidates {
						binding.TransmissionID = reception.Source.TransmissionID
						if _, err := repository.AdmitCandidate(tx.Context(), tx, binding); err != nil {
							return err
						}
						pendingOwner.candidates = append(pendingOwner.candidates, binding)
					}
					executor, err := primary.Resolve(tx)
					if err != nil {
						return err
					}
					_, err = executor.ExecContext(tx.Context(),
						`INSERT INTO processing_publication_fixture(id,message_id,fence,network_id,source_revision,context_revision)
						 VALUES($1,$2,$3,$4,$5,$6)`, reception.Source.TransmissionID, reception.MessageID,
						reception.SourceFence, reception.Source.NetworkID, reception.Source.SourceRevision, reception.Source.ContextRevision)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				pendingService, err := publication.New(repository, pendingOwner, facts, primary)
				if err != nil {
					t.Fatal(err)
				}
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					return pendingService.Publish(tx.Context(), tx, reception.Source.TransmissionID)
				}); !errors.Is(err, authz.ErrRevisionConflict) {
					t.Fatalf("pending reception bypassed completed evidence: %v", err)
				}
				if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					executor, err := primary.Resolve(tx)
					if err != nil {
						return err
					}
					var oldReady, newReady bool
					var published int
					if err := executor.QueryRowContext(tx.Context(),
						`SELECT (SELECT ready FROM processing_publication_fixture WHERE id=$1),
							(SELECT ready FROM processing_publication_fixture WHERE id=$2),
							(SELECT count(*) FROM processing_results WHERE message_id=$3 AND published_at IS NOT NULL)`,
						source.Source.TransmissionID, reception.Source.TransmissionID, source.MessageID).
						Scan(&oldReady, &newReady, &published); err != nil {
						return err
					}
					if !oldReady || newReady || published != 1 {
						t.Fatalf("pending reception removed or replaced ready evidence: old=%t new=%t results=%d", oldReady, newReady, published)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
