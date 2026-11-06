package processingstore

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.CandidateWorkRepository = (*Store)(nil)

// SelectDueWork returns up to limit due or lease-expired candidates ordered by due time and id,
// judged on the primary clock. It locks the selected rows with FOR UPDATE SKIP LOCKED instead of
// the Processing-control row, so concurrent dispatchers receive disjoint rows and never wait on
// each other or on configuration changes. Selection is not a claim; each worker must recheck
// current owner authority before reserving work.
func (s *Store) SelectDueWork(ctx context.Context, tx platform.TransactionContext, limit int) ([]processing.Candidate, error) {
	if limit < 1 {
		return nil, authz.ErrInvalidInput
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingCandidate": entql.Or(
			entql.And(entql.FieldEQ("state", "pending"), entql.FieldLTE("due_at", now)),
			entql.And(entql.FieldEQ("state", "running"), entql.FieldLTE("lease_until", now))),
	}, nil)
	if err != nil {
		return nil, err
	}
	rows, err := client.ProcessingCandidate.Query().Order(processingcandidate.ByDueAt(), processingcandidate.ByID()).
		Limit(limit).ForUpdate(sql.WithLockAction(sql.SkipLocked)).All(scope)
	if err != nil {
		return nil, err
	}
	result := make([]processing.Candidate, 0, len(rows))
	for _, row := range rows {
		value, err := candidateState(row)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

// SupersedeCandidate ends owner-confirmed obsolete work even when its configuration is old. It
// retains immutable result rows (published ones stay attached to their Message) and invalidates
// current ownership rather than using an expired lease.
func (s *Store) SupersedeCandidate(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) (processing.Candidate, error) {
	if id == uuid.Nil || expected <= 0 {
		return processing.Candidate{}, authz.ErrInvalidInput
	}
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return processing.Candidate{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingCandidate": entql.FieldEQ("id", id),
	}, nil)
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
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	next, err := current.Supersede(expected, now)
	if err != nil || current.State == "superseded" {
		return next, err
	}
	changed, err := client.ProcessingCandidate.Update().Where(processingcandidate.Revision(expected),
		processingcandidate.FencingToken(current.FencingToken)).
		SetRevision(next.Revision).SetFencingToken(next.FencingToken).SetState(processingcandidate.StateSuperseded).
		SetUpdatedAt(now).ClearWorkerID().ClearLeaseUntil().ClearSelected().SetManualRetryPending(false).Save(scope)
	if err != nil {
		return processing.Candidate{}, err
	}
	if changed != 1 {
		return processing.Candidate{}, authz.ErrRevisionConflict
	}
	return next, nil
}
