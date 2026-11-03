package processingstore

import (
	"context"
	"errors"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.CandidateRepository = (*Store)(nil)
var _ processing.CandidateRetryRepository = (*Store)(nil)
var _ processing.CandidateFailureRepository = (*Store)(nil)
var _ processing.CandidateRecoveryRepository = (*Store)(nil)

// candidateState detaches the Processing-owned row from its borrowed Ent client.
func candidateState(row *ent.ProcessingCandidate) (processing.Candidate, error) {
	value := processing.Candidate{
		CandidateBinding: processing.CandidateBinding{
			TransmissionID: row.TransmissionID, WorkflowID: row.WorkflowID,
			DefinitionVersionID: row.DefinitionVersionID, SourceRevision: row.SourceRevision,
			ContextRevision: row.ContextRevision, WorkflowRevision: row.WorkflowRevision,
			ConfigurationRevision: row.ConfigurationRevision,
		},
		ID: row.ID, Revision: row.Revision, State: string(row.State), Selected: row.Selected,
		Attempts: row.Attempts, AutomaticAttempts: row.AutomaticAttempts,
		ManualRetryPending: row.ManualRetryPending, FencingToken: row.FencingToken,
		WorkerID: row.WorkerID, LeaseUntil: row.LeaseUntil, DueAt: row.DueAt,
		FailureCode: row.FailureCode, FailureStep: row.FailureStep,
		BarrierAppliedAt: row.BarrierAppliedAt, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	return value, value.Validate()
}

// AdmitCandidate confirms current active configuration and atomically deduplicates or reselects work.
// Current source/scope authority and unpublished-source exclusion remain the caller owner's obligation.
func (s *Store) AdmitCandidate(ctx context.Context, tx platform.TransactionContext, binding processing.CandidateBinding) (processing.Candidate, error) {
	if err := binding.Validate(); err != nil {
		return processing.Candidate{}, err
	}
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	if revision != binding.ConfigurationRevision {
		return processing.Candidate{}, authz.ErrRevisionConflict
	}
	key := entql.And(entql.FieldEQ("transmission_id", binding.TransmissionID),
		entql.FieldEQ("workflow_id", binding.WorkflowID),
		entql.FieldEQ("definition_version_id", binding.DefinitionVersionID),
		entql.FieldEQ("source_revision", binding.SourceRevision),
		entql.FieldEQ("context_revision", binding.ContextRevision))
	active := entql.And(entql.FieldEQ("id", binding.WorkflowID),
		entql.FieldEQ("revision", binding.WorkflowRevision), entql.FieldEQ("enabled", true),
		entql.FieldEQ("deleted", false), entql.FieldEQ("active_version_id", binding.DefinitionVersionID))
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Workflow": active, "ProcessingCandidate": key,
	}, map[string]bool{"ProcessingCandidate": true})
	if err != nil {
		return processing.Candidate{}, err
	}
	if _, err := client.Workflow.Query().Only(scope); err != nil {
		if ent.IsNotFound(err) {
			return processing.Candidate{}, authz.ErrRevisionConflict
		}
		return processing.Candidate{}, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	row, err := client.ProcessingCandidate.Query().ForUpdate().Only(scope)
	if ent.IsNotFound(err) {
		row, err = client.ProcessingCandidate.Create().SetID(uuid.New()).
			SetTransmissionID(binding.TransmissionID).SetWorkflowID(binding.WorkflowID).
			SetDefinitionVersionID(binding.DefinitionVersionID).
			SetSourceRevision(binding.SourceRevision).SetContextRevision(binding.ContextRevision).
			SetWorkflowRevision(binding.WorkflowRevision).SetConfigurationRevision(binding.ConfigurationRevision).
			SetDueAt(now).SetCreatedAt(now).SetUpdatedAt(now).Save(scope)
		if err != nil {
			return processing.Candidate{}, err
		}
		return candidateState(row)
	}
	if err != nil {
		return processing.Candidate{}, err
	}
	current, err := candidateState(row)
	if err != nil {
		return processing.Candidate{}, err
	}
	next, changed, err := current.Reselect(binding, now)
	if err != nil || !changed {
		return next, err
	}
	update := client.ProcessingCandidate.Update().
		Where(processingcandidate.ID(current.ID), processingcandidate.Revision(current.Revision),
			processingcandidate.FencingToken(current.FencingToken), processingcandidate.BarrierAppliedAtIsNil()).
		SetRevision(next.Revision).SetWorkflowRevision(next.WorkflowRevision).
		SetConfigurationRevision(next.ConfigurationRevision).SetFencingToken(next.FencingToken).
		SetState(processingcandidate.State(next.State)).SetDueAt(now).SetUpdatedAt(now).
		ClearSelected().ClearWorkerID().ClearLeaseUntil().ClearFailureStep().
		SetManualRetryPending(false)
	if next.FailureCode != nil {
		update.SetFailureCode(*next.FailureCode)
	} else {
		update.ClearFailureCode()
	}
	updated, err := update.Save(scope)
	if err != nil {
		return processing.Candidate{}, err
	}
	if updated != 1 {
		return processing.Candidate{}, platform.NewFailure(platform.CodeRevisionConflict,
			errors.New("candidate changed before reselection"))
	}
	return next, nil
}

// ClaimCandidate reserves a due attempt after the caller captures current source/Principal authority.
func (s *Store) ClaimCandidate(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64, worker uuid.UUID) (processing.Candidate, error) {
	return s.changeClaim(ctx, tx, id, false, func(current processing.Candidate, now time.Time) (processing.Candidate, error) {
		return current.Claim(expected, worker, now)
	})
}

// RenewCandidate extends only the matching current unexpired claim using the primary clock.
func (s *Store) RenewCandidate(ctx context.Context, tx platform.TransactionContext, lease processing.CandidateLease) (processing.Candidate, error) {
	return s.changeClaim(ctx, tx, lease.CandidateID, false, func(current processing.Candidate, now time.Time) (processing.Candidate, error) {
		return current.Renew(lease, now)
	})
}

// changeClaim serializes a checked lease transition with current active configuration.
func (s *Store) changeClaim(ctx context.Context, tx platform.TransactionContext, id uuid.UUID,
	requireExpiredLease bool, change func(processing.Candidate, time.Time) (processing.Candidate, error)) (processing.Candidate, error) {
	if id == uuid.Nil {
		return processing.Candidate{}, authz.ErrInvalidInput
	}
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	client, scope, err := s.client(ctx, tx,
		map[string]entql.P{"ProcessingCandidate": entql.FieldEQ("id", id)}, nil)
	if err != nil {
		return processing.Candidate{}, err
	}
	row, err := client.ProcessingCandidate.Query().ForUpdate().Only(scope)
	if ent.IsNotFound(err) {
		return processing.Candidate{}, authz.ErrNotFound
	}
	if err != nil {
		return processing.Candidate{}, err
	}
	current, err := candidateState(row)
	if err != nil {
		return processing.Candidate{}, err
	}
	if current.ConfigurationRevision != revision {
		return processing.Candidate{}, authz.ErrRevisionConflict
	}
	active := entql.And(entql.FieldEQ("id", current.WorkflowID),
		entql.FieldEQ("revision", current.WorkflowRevision), entql.FieldEQ("enabled", true),
		entql.FieldEQ("deleted", false), entql.FieldEQ("active_version_id", current.DefinitionVersionID))
	workflowClient, workflowScope, err := s.client(ctx, tx, map[string]entql.P{"Workflow": active}, nil)
	if err != nil {
		return processing.Candidate{}, err
	}
	if _, err := workflowClient.Workflow.Query().Only(workflowScope); err != nil {
		if ent.IsNotFound(err) {
			return processing.Candidate{}, authz.ErrRevisionConflict
		}
		return processing.Candidate{}, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	next, err := change(current, now)
	if err != nil {
		return processing.Candidate{}, err
	}
	update := client.ProcessingCandidate.Update().
		Where(processingcandidate.Revision(current.Revision), processingcandidate.FencingToken(current.FencingToken)).
		SetRevision(next.Revision).SetState(processingcandidate.State(next.State)).
		SetFencingToken(next.FencingToken).SetAttempts(next.Attempts).SetAutomaticAttempts(next.AutomaticAttempts).
		SetManualRetryPending(next.ManualRetryPending).SetDueAt(next.DueAt).SetUpdatedAt(now)
	if current.State == "running" {
		update.Where(func(selector *sql.Selector) {
			if requireExpiredLease {
				selector.Where(sql.ExprP(`clock_timestamp() >= "lease_until"`))
			} else {
				selector.Where(sql.ExprP(`clock_timestamp() < "lease_until"`))
			}
		})
	}
	if next.WorkerID == nil {
		update.ClearWorkerID().ClearLeaseUntil()
	} else {
		update.SetWorkerID(*next.WorkerID).SetLeaseUntil(*next.LeaseUntil)
	}
	if next.Selected == nil {
		update.ClearSelected()
	} else {
		update.SetSelected(*next.Selected)
	}
	if next.FailureCode == nil {
		update.ClearFailureCode()
	} else {
		update.SetFailureCode(*next.FailureCode)
	}
	if next.FailureStep == nil {
		update.ClearFailureStep()
	} else {
		update.SetFailureStep(*next.FailureStep)
	}
	updated, err := update.Save(scope)
	if err != nil {
		return processing.Candidate{}, err
	}
	if updated != 1 {
		return processing.Candidate{}, authz.ErrRevisionConflict
	}
	return next, nil
}

// FailCandidate persists a safe classified failure and remaining retry schedule under current fencing.
func (s *Store) FailCandidate(ctx context.Context, tx platform.TransactionContext, lease processing.CandidateLease,
	code platform.ErrorCode, retryable bool) (processing.Candidate, error) {
	return s.changeClaim(ctx, tx, lease.CandidateID, false, func(current processing.Candidate, now time.Time) (processing.Candidate, error) {
		return current.Fail(lease, code, retryable, now)
	})
}

// RecoverCandidate fences expired pure-compute ownership without resetting consumed attempts.
// The source owner checks current unpublished eligibility before this private transition.
func (s *Store) RecoverCandidate(ctx context.Context, tx platform.TransactionContext, lease processing.CandidateLease) (processing.Candidate, error) {
	return s.changeClaim(ctx, tx, lease.CandidateID, true, func(current processing.Candidate, now time.Time) (processing.Candidate, error) {
		return current.RecoverExpired(lease, now)
	})
}

// ReadRetryCandidate correlates private work with a natively rule.reprocess-authorized Workflow.
func (s *Store) ReadRetryCandidate(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	parent, id uuid.UUID, now time.Time) (processing.Candidate, error) {
	if parent == uuid.Nil || id == uuid.Nil {
		return processing.Candidate{}, authz.ErrInvalidInput
	}
	if _, err := s.ReadWorkflow(ctx, tx, authority, "rule.reprocess", parent, now); err != nil {
		return processing.Candidate{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"ProcessingCandidate": entql.And(
		entql.FieldEQ("id", id), entql.FieldEQ("workflow_id", parent))}, nil)
	if err != nil {
		return processing.Candidate{}, err
	}
	row, err := client.ProcessingCandidate.Query().Only(scope)
	if ent.IsNotFound(err) {
		return processing.Candidate{}, authz.ErrNotFound
	}
	if err != nil {
		return processing.Candidate{}, err
	}
	return candidateState(row)
}

// RetryCandidate rechecks scoped action authority and active pins, then records a safe audit atomically.
func (s *Store) RetryCandidate(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	parent, id uuid.UUID, expected int64) (processing.Candidate, error) {
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return processing.Candidate{}, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	if _, err := s.ReadRetryCandidate(ctx, tx, authority, parent, id, now); err != nil {
		return processing.Candidate{}, err
	}
	next, err := s.changeClaim(ctx, tx, id, false, func(current processing.Candidate, now time.Time) (processing.Candidate, error) {
		return current.RetryFailed(expected, now)
	})
	if err != nil {
		return processing.Candidate{}, err
	}
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil {
		return processing.Candidate{}, authz.ErrInvalidInput
	}
	client, scope, err := s.client(ctx, tx, nil, map[string]bool{"AuditRecord": true})
	if err != nil {
		return processing.Candidate{}, err
	}
	if err := client.AuditRecord.Create().SetActorID(actor).SetTargetID(id).SetAction("rule.reprocess").
		SetOutcome("completed").SetRecordedAt(next.UpdatedAt).Exec(scope); err != nil {
		return processing.Candidate{}, err
	}
	return next, nil
}
