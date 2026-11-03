package borrowed_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func TestProcessingCandidateNativeReselectionRetainsResultsAndAttempts(t *testing.T) {
	primary := newStore(t)
	store, err := processingstore.New(primary)
	if err != nil {
		t.Fatal(err)
	}
	actor := uuid.New()
	authority := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
	configuration := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Name: "candidate",
		Scope: authz.Scope{Kind: "all"}, Enabled: true}
	version := processing.Version{ID: uuid.New(), WorkflowID: configuration.ID, CreatedBy: actor,
		Definition: preparedProcessingDefinition(t)}
	binding := processing.CandidateBinding{TransmissionID: uuid.New(), WorkflowID: configuration.ID,
		DefinitionVersionID: version.ID, SourceRevision: 1, ContextRevision: 1, WorkflowRevision: 2}
	var initial processing.Candidate
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		now, err := primary.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		configuration.CreatedAt, configuration.UpdatedAt, version.CreatedAt = now, now, now
		if err := store.SaveWorkflow(tx.Context(), tx, authority, "rule.create", configuration, 0, now); err != nil {
			return err
		}
		if err := store.SaveVersion(tx.Context(), tx, version); err != nil {
			return err
		}
		configuration.Revision, configuration.ActiveVersionID = 2, &version.ID
		if err := store.SaveWorkflow(tx.Context(), tx, authority, "rule.activate", configuration, 1, now); err != nil {
			return err
		}
		if err := store.RecordChange(tx.Context(), tx, actor, configuration.ID, "rule.activate", now); err != nil {
			return err
		}
		binding.ConfigurationRevision, err = store.LockRevision(tx.Context(), tx)
		if err != nil {
			return err
		}
		initial, err = store.AdmitCandidate(tx.Context(), tx, binding)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The synthetic source reference is not a production AP-06 eligibility/readiness proof.
	resultID := uuid.New()
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(primary, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{"ProcessingCandidate": entql.FieldEQ("id", initial.ID)},
			Create:     map[string]bool{"ProcessingResult": true}})
		if err := client.ProcessingCandidate.UpdateOneID(initial.ID).
			SetState(processingcandidate.StateSuperseded).SetAttempts(7).SetAutomaticAttempts(5).
			SetFencingToken(9).SetFailureCode("INVALID_INPUT").Exec(scope); err != nil {
			return err
		}
		return client.ProcessingResult.Create().SetID(resultID).SetCandidateID(initial.ID).
			SetRevision(1).SetProducingFence(8).SetCompletedAt(initial.CreatedAt).
			SetEvaluation(processing.Evaluation{Selected: true, Fields: map[string]processing.FieldValue{}}).Exec(scope)
	}); err != nil {
		t.Fatal(err)
	}
	admit := func(value processing.CandidateBinding) (processing.Candidate, error) {
		var result processing.Candidate
		err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			result, err = store.AdmitCandidate(tx.Context(), tx, value)
			return err
		})
		return result, err
	}
	unchanged, err := admit(binding)
	if err != nil || unchanged.State != "superseded" || unchanged.Revision != 1 {
		t.Fatalf("unchanged pins revived work: %+v err=%v", unchanged, err)
	}
	update := func(enabled bool) {
		t.Helper()
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			if _, err := store.LockRevision(tx.Context(), tx); err != nil {
				return err
			}
			now, err := primary.Now(tx.Context(), tx)
			if err != nil {
				return err
			}
			expected := configuration.Revision
			configuration.Revision++
			configuration.Enabled, configuration.UpdatedAt = enabled, now
			if err := store.SaveWorkflow(tx.Context(), tx, authority, "rule.update", configuration, expected, now); err != nil {
				return err
			}
			if err := store.RecordChange(tx.Context(), tx, actor, configuration.ID, "rule.update", now); err != nil {
				return err
			}
			binding.WorkflowRevision = configuration.Revision
			binding.ConfigurationRevision, err = store.LockRevision(tx.Context(), tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	verifyClaims := func() {
		t.Helper()
		claimBinding := binding
		claimBinding.TransmissionID = uuid.New()
		due, err := admit(claimBinding)
		if err != nil {
			t.Fatal(err)
		}
		worker := uuid.New()
		var claimed, renewed processing.Candidate
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			claimed, err = store.ClaimCandidate(tx.Context(), tx, due.ID, due.Revision, worker)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if claimed.Attempts != 1 || claimed.AutomaticAttempts != 1 || claimed.FencingToken != 1 ||
			claimed.LeaseUntil.Sub(claimed.UpdatedAt) != processing.CandidateLeaseDuration {
			t.Fatalf("native claim violated attempt/lease contract: %+v", claimed)
		}
		lease, err := claimed.Lease()
		if err != nil {
			t.Fatal(err)
		}
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			var err error
			renewed, err = store.RenewCandidate(tx.Context(), tx, lease)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if renewed.Revision != claimed.Revision+1 || renewed.FencingToken != claimed.FencingToken ||
			renewed.Attempts != claimed.Attempts || renewed.LeaseUntil.Sub(renewed.UpdatedAt) != processing.CandidateLeaseDuration {
			t.Fatalf("native renewal changed ownership/attempt history: %+v", renewed)
		}
		for _, operation := range []string{"stale-renew", "duplicate-claim"} {
			err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				if operation == "stale-renew" {
					_, err := store.RenewCandidate(tx.Context(), tx, lease)
					return err
				}
				_, err := store.ClaimCandidate(tx.Context(), tx, due.ID, due.Revision, uuid.New())
				return err
			})
			if !errors.Is(err, authz.ErrRevisionConflict) {
				t.Fatalf("%s retained ownership: %v", operation, err)
			}
		}
		latest, err := renewed.Lease()
		if err != nil {
			t.Fatal(err)
		}
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			_, err := store.RecoverCandidate(tx.Context(), tx, latest)
			return err
		}); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("unexpired native lease recovered: %v", err)
		}
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			driver, err := borrowed.New(primary, tx)
			if err != nil {
				return err
			}
			client := ent.NewClient(ent.Driver(driver))
			scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Predicates: map[string]entql.P{"ProcessingCandidate": entql.FieldEQ("id", due.ID)}})
			// A valid synthetic past lease exercises the real primary clock without a minute-long sleep.
			return client.ProcessingCandidate.UpdateOneID(due.ID).
				SetUpdatedAt(due.CreatedAt).SetLeaseUntil(due.CreatedAt.Add(time.Microsecond)).Exec(scope)
		}); err != nil {
			t.Fatal(err)
		}
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			_, err := store.RenewCandidate(tx.Context(), tx, latest)
			return err
		}); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("expired primary-clock lease renewed: %v", err)
		}
		recoverClaim := func() (processing.Candidate, error) {
			var result processing.Candidate
			err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				var err error
				result, err = store.RecoverCandidate(tx.Context(), tx, latest)
				return err
			})
			return result, err
		}
		var recovered processing.Candidate
		var recoveryError error
		var competitors sync.WaitGroup
		var winner sync.Mutex
		successes := 0
		for range 2 {
			competitors.Go(func() {
				result, err := recoverClaim()
				winner.Lock()
				defer winner.Unlock()
				if err == nil {
					successes++
					recovered = result
				} else if !errors.Is(err, authz.ErrRevisionConflict) {
					recoveryError = err
				}
			})
		}
		competitors.Wait()
		if recoveryError != nil || successes != 1 || recovered.State != "pending" ||
			recovered.Revision != renewed.Revision+1 || recovered.FencingToken != renewed.FencingToken+1 ||
			recovered.Attempts != renewed.Attempts || recovered.AutomaticAttempts != renewed.AutomaticAttempts ||
			recovered.WorkerID != nil || recovered.LeaseUntil != nil ||
			recovered.DueAt.Sub(recovered.UpdatedAt) < 24*time.Second ||
			recovered.DueAt.Sub(recovered.UpdatedAt) > 36*time.Second {
			t.Fatalf("native recovery race/history: successes=%d result=%+v err=%v", successes, recovered, recoveryError)
		}
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			_, err := store.FailCandidate(tx.Context(), tx, latest, platform.CodeInternalError, false)
			return err
		}); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("lost worker wrote over native recovery: %v", err)
		}
		retryExpected := recovered.Revision
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			driver, err := borrowed.New(primary, tx)
			if err != nil {
				return err
			}
			client := ent.NewClient(ent.Driver(driver))
			scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Predicates: map[string]entql.P{"ProcessingCandidate": entql.FieldEQ("id", due.ID)}})
			return client.ProcessingCandidate.UpdateOneID(due.ID).
				SetState(processingcandidate.StateFailed).ClearWorkerID().ClearLeaseUntil().
				SetAttempts(5).SetAutomaticAttempts(5).Exec(scope)
		}); err != nil {
			t.Fatal(err)
		}
		abortRetry := errors.New("rollback retry and audit")
		retry := func(access authz.Authority, expected int64, rollback bool) (processing.Candidate, error) {
			var result processing.Candidate
			err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
				var err error
				result, err = store.RetryCandidate(tx.Context(), tx, access, binding.WorkflowID, due.ID, expected)
				if err != nil {
					return err
				}
				if rollback {
					return abortRetry
				}
				return nil
			})
			return result, err
		}
		if _, err := retry(authority, retryExpected, false); !errors.Is(err, authz.ErrForbidden) {
			t.Fatalf("CRUD/read rights implicitly permitted retry: %v", err)
		}
		allowed := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
		allowed.Grants[0].Actions = []string{"rule.reprocess"}
		if result, err := retry(allowed, retryExpected, true); !errors.Is(err, abortRetry) ||
			result.State != "pending" || !result.ManualRetryPending {
			t.Fatalf("retry/audit participants were not reached before rollback: %+v err=%v", result, err)
		}
		retried, err := retry(allowed, retryExpected, false)
		if err != nil || retried.State != "pending" || !retried.ManualRetryPending ||
			retried.AutomaticAttempts != 5 || retried.Attempts != 5 || retried.Revision != retryExpected+1 {
			t.Fatalf("authorized manual retry: %+v err=%v", retried, err)
		}
		if _, err := retry(allowed, retryExpected, false); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("stale manual retry duplicated work: %v", err)
		}
		if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			driver, err := borrowed.New(primary, tx)
			if err != nil {
				return err
			}
			client := ent.NewClient(ent.Driver(driver))
			scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Predicates: map[string]entql.P{"AuditRecord": entql.And(
					entql.FieldEQ("target_id", due.ID), entql.FieldEQ("action", "rule.reprocess"))}})
			count, err := client.AuditRecord.Query().Count(scope)
			if err != nil {
				return err
			}
			if count != 1 {
				t.Fatalf("retry audit duplicated or rollback leaked: %d", count)
			}
			revision, err := store.LockRevision(tx.Context(), tx)
			if err != nil {
				return err
			}
			if revision != binding.ConfigurationRevision {
				t.Fatal("run retry spuriously invalidated configuration")
			}
			manual, err := store.ClaimCandidate(tx.Context(), tx, due.ID, retried.Revision, worker)
			if err != nil {
				return err
			}
			if manual.Attempts != 6 || manual.AutomaticAttempts != 5 || manual.ManualRetryPending {
				t.Fatalf("manual claim reset automatic cycle: %+v", manual)
			}
			lease, err := manual.Lease()
			if err != nil {
				return err
			}
			completed, result, err := store.CompleteCandidate(tx.Context(), tx, lease, 0,
				processing.Evaluation{Selected: true, Fields: map[string]processing.FieldValue{}})
			if err != nil {
				return err
			}
			if completed.State != "completed" || completed.BarrierAppliedAt != nil || result == nil ||
				result.Revision != completed.Revision || result.ProducingFence != manual.FencingToken ||
				result.CandidateID != manual.ID || completed.WorkerID != nil {
				t.Fatalf("private completion fabricated readiness or lost fencing: %+v %+v", completed, result)
			}
			if _, _, err := store.CompleteCandidate(tx.Context(), tx, lease, 0,
				processing.Evaluation{Selected: true, Fields: map[string]processing.FieldValue{}}); !errors.Is(err, authz.ErrRevisionConflict) {
				t.Fatalf("completed lease duplicated private evidence: %v", err)
			}
			falseBinding := binding
			falseBinding.TransmissionID = uuid.New()
			notSelected, err := store.AdmitCandidate(tx.Context(), tx, falseBinding)
			if err != nil {
				return err
			}
			notSelected, err = store.ClaimCandidate(tx.Context(), tx, notSelected.ID, notSelected.Revision, worker)
			if err != nil {
				return err
			}
			falseLease, err := notSelected.Lease()
			if err != nil {
				return err
			}
			notSelected, private, err := store.CompleteCandidate(tx.Context(), tx, falseLease, 0, processing.Evaluation{})
			if err != nil {
				return err
			}
			if private != nil || notSelected.Selected == nil || *notSelected.Selected ||
				notSelected.State != "completed" || notSelected.BarrierAppliedAt != nil {
				t.Fatalf("not-selected completion made a namespace/readiness: %+v %+v", notSelected, private)
			}
			resultScope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Predicates: map[string]entql.P{"ProcessingResult": entql.FieldEQ("candidate_id", notSelected.ID)}})
			count, err = client.ProcessingResult.Query().Count(resultScope)
			if err != nil {
				return err
			}
			if count != 0 {
				t.Fatal("not-selected outcome persisted a private result")
			}
			failureBinding := binding
			failureBinding.TransmissionID = uuid.New()
			failed, err := store.AdmitCandidate(tx.Context(), tx, failureBinding)
			if err != nil {
				return err
			}
			failed, err = store.ClaimCandidate(tx.Context(), tx, failed.ID, failed.Revision, worker)
			if err != nil {
				return err
			}
			failureLease, err := failed.Lease()
			if err != nil {
				return err
			}
			failed, err = store.FailCandidate(tx.Context(), tx, failureLease, platform.CodeAuthorityUnavailable, true)
			if err != nil {
				return err
			}
			if failed.State != "pending" || failed.Attempts != 1 || failed.AutomaticAttempts != 1 ||
				failed.WorkerID != nil || failed.LeaseUntil != nil || failed.FailureCode == nil ||
				failed.DueAt.Sub(failed.UpdatedAt) < 24*time.Second || failed.DueAt.Sub(failed.UpdatedAt) > 36*time.Second {
				t.Fatalf("native failure lost safe status/schedule: %+v", failed)
			}
			if _, err := store.FailCandidate(tx.Context(), tx, failureLease, platform.CodeInternalError, false); !errors.Is(err, authz.ErrRevisionConflict) {
				t.Fatalf("old failed lease overwrote pending retry: %v", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	oldBinding := binding
	update(false)
	if _, err := admit(binding); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("disabled Workflow admitted work: %v", err)
	}
	update(true)
	if _, err := admit(oldBinding); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("obsolete configuration admitted work: %v", err)
	}
	abort := errors.New("abort after reselection")
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := store.AdmitCandidate(tx.Context(), tx, binding); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatalf("rollback fixture: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan processing.Candidate, 2)
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() {
			value, err := admit(binding)
			results <- value
			failures <- err
		})
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	for value := range results {
		if value.ID != initial.ID || value.Revision != 2 || value.FencingToken != 10 ||
			value.State != "failed" || value.Attempts != 7 || value.AutomaticAttempts != 5 ||
			value.FailureCode == nil || *value.FailureCode != string(platform.CodeResourceLimit) {
			t.Fatalf("reselection lost history or duplicated state: %+v", value)
		}
	}
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		driver, err := borrowed.New(primary, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{
				"ProcessingCandidate": entql.FieldEQ("id", initial.ID),
				"ProcessingResult":    entql.FieldEQ("id", resultID),
			}})
		count, err := client.ProcessingCandidate.Query().Count(scope)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("duplicate candidate rows: %d", count)
		}
		count, err = client.ProcessingCandidate.Update().Where(processingcandidate.FencingToken(9),
			processingcandidate.Revision(1)).SetState(processingcandidate.StateFailed).Save(scope)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("old claim overwrote reselected work")
		}
		row, err := client.ProcessingResult.Get(scope, resultID)
		if err != nil {
			return err
		}
		if row.CandidateID != initial.ID || row.ProducingFence != 8 || row.Revision != 1 ||
			row.PublishedAt != nil || !row.Evaluation.Selected {
			t.Fatalf("prior private evidence changed: %+v", row)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		work, err := store.ListCandidateWork(tx.Context(), tx, processing.PageInput{Limit: 1})
		if err != nil {
			return err
		}
		if work.Total != 0 {
			t.Fatalf("exhausted reselection entered automatic dispatch: %+v", work)
		}
		allowed := processingAuthorityFixture(actor, authz.Scope{Kind: "all"})
		allowed.Grants[0].Actions = []string{"rule.reprocess"}
		retried, err := store.RetryCandidate(tx.Context(), tx, allowed, binding.WorkflowID, initial.ID, 2)
		if err != nil {
			return err
		}
		claimed, err := store.ClaimCandidate(tx.Context(), tx, retried.ID, retried.Revision, uuid.New())
		if err != nil {
			return err
		}
		if claimed.State != "running" || claimed.Attempts != 8 || claimed.AutomaticAttempts != 5 ||
			claimed.FencingToken != 11 || claimed.ManualRetryPending {
			t.Fatalf("exhausted reselection retry reset counters/fences: %+v", claimed)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	verifyClaims()
}
