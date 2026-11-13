package processingstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/rowcheck"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ReplacePublished retracts the other published results of the item's Transmission and Workflow
// and clears the candidate's Reprocess mark. The item's own result, if any, stays published.
// It returns the retracted results, each with the revision the retraction gave it, and fails with
// a revision conflict when the candidate no longer carries the item's mark.
func (s *Store) ReplacePublished(ctx context.Context, tx platform.TransactionContext, item processing.PublicationItem) ([]processing.RetractedResult, error) {
	if !item.ReplacesPublished() {
		return nil, authz.ErrRevisionConflict
	}
	keep := uuid.Nil
	if item.Result != nil {
		keep = item.Result.ID
	}
	retracted, err := s.RetractPublished(ctx, tx, item.Candidate.TransmissionID, item.Candidate.WorkflowID, keep)
	if err != nil {
		return nil, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	cleared, err := executor.ExecContext(ctx, `UPDATE processing_candidates SET reprocess_id=NULL
		WHERE id=$1 AND reprocess_id=$2`, item.Candidate.ID, *item.ReprocessID)
	return retracted, rowcheck.RequireOne(cleared, err)
}
