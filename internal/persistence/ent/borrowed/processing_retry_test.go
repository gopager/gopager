package borrowed_test

import (
	"context"
	"errors"
	"testing"

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
	"github.com/gopager/gopager/internal/processing/management"
)

type processingRetrySourceFixture struct {
	binding   processing.CandidateBinding
	failure   error
	calls     int
	published int
}

// Publish records the status refresh request; the fixture owns no Transmission row.
func (f *processingRetrySourceFixture) Publish(context.Context, platform.TransactionContext, uuid.UUID) error {
	f.published++
	return nil
}

// CheckRetrySource is explicit synthetic eligibility, not a production source/publication owner.
func (f *processingRetrySourceFixture) CheckRetrySource(_ context.Context, _ platform.TransactionContext, candidate processing.Candidate) error {
	f.calls++
	if candidate.CandidateBinding != f.binding {
		return authz.ErrRevisionConflict
	}
	return f.failure
}

func TestProcessingRetryRechecksProofAndRequiresCurrentSourceOwner(t *testing.T) {
	f := newMatcherFixture(t)
	service, _ := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("retry", authz.Scope{Kind: "all"}), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	version, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1,
		preparedProcessingDefinition(t), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.ActivateVersion(t.Context(), f.proof, created.ID, version.ID, 2, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	store, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	source := &processingRetrySourceFixture{binding: processing.CandidateBinding{
		TransmissionID: uuid.New(), WorkflowID: created.ID, DefinitionVersionID: version.ID,
		SourceRevision: 1, ContextRevision: 1, WorkflowRevision: active.Revision,
	}}
	var candidate processing.Candidate
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		revision, err := store.LockRevision(tx.Context(), tx)
		if err != nil {
			return err
		}
		source.binding.ConfigurationRevision = revision
		candidate, err = store.AdmitCandidate(tx.Context(), tx, source.binding)
		if err != nil {
			return err
		}
		driver, err := borrowed.New(f.store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Predicates: map[string]entql.P{"ProcessingCandidate": entql.FieldEQ("id", candidate.ID)}})
		return client.ProcessingCandidate.UpdateOneID(candidate.ID).
			SetState(processingcandidate.StateFailed).SetAttempts(5).SetAutomaticAttempts(5).Exec(scope)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := management.NewRetry(service, store, nil); err == nil {
		t.Fatal("missing source owner accepted")
	}
	retries, err := management.NewRetry(service, store, source)
	if err != nil {
		t.Fatal(err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Scope: authz.Scope{Kind: "all"}, Actions: []string{"workflow.read", "workflow.update"}}})
	if _, err := retries.RetryFailed(t.Context(), f.proof, created.ID, candidate.ID, candidate.Revision,
		checkedMutation); !errors.Is(err, authz.ErrForbidden) || source.calls != 0 {
		t.Fatalf("current CRUD proof bypassed retry action: %v calls=%d", err, source.calls)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Scope: authz.Scope{Kind: "all"}, Actions: []string{"workflow.reprocess"}}})
	if _, err := retries.RetryFailed(t.Context(), f.proof, created.ID, candidate.ID, candidate.Revision,
		nil); err == nil || source.calls != 0 {
		t.Fatalf("missing mutation/CSRF participant accepted: %v calls=%d", err, source.calls)
	}
	unavailable := platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("synthetic source unavailable"))
	source.failure = unavailable
	if _, err := retries.RetryFailed(t.Context(), f.proof, created.ID, candidate.ID, candidate.Revision,
		checkedMutation); !errors.Is(err, unavailable) {
		t.Fatalf("unavailable source became authorized retry: %v", err)
	}
	source.failure = nil
	result, err := retries.RetryFailed(t.Context(), f.proof, created.ID, candidate.ID, candidate.Revision, checkedMutation)
	if err != nil || result.Revision != candidate.Revision+1 || result.State != "pending" ||
		!result.ManualRetryPending || result.Attempts != 5 || result.AutomaticAttempts != 5 {
		t.Fatalf("retry after explicit source recovery: %+v err=%v", result, err)
	}
	if _, err := retries.RetryFailed(t.Context(), f.proof, created.ID, candidate.ID, candidate.Revision,
		checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale retry repeated: %v", err)
	}
}
