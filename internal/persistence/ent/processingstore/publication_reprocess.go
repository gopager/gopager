package processingstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ReplacePublished retracts the other published results of the item's Transmission and Workflow
// and clears the candidate's Reprocess mark. The item's own result, if any, stays published.
// It fails with a revision conflict when the candidate no longer carries the item's mark.
func (s *Store) ReplacePublished(ctx context.Context, tx platform.TransactionContext, item processing.PublicationItem) error {
	if !item.ReplacesPublished() {
		return authz.ErrRevisionConflict
	}
	keep := uuid.Nil
	if item.Result != nil {
		keep = item.Result.ID
	}
	if _, err := s.RetractPublished(ctx, tx, item.Candidate.TransmissionID, item.Candidate.WorkflowID, keep); err != nil {
		return err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	cleared, err := executor.ExecContext(ctx, `UPDATE processing_candidates SET reprocess_id=NULL
		WHERE id=$1 AND reprocess_id=$2`, item.Candidate.ID, *item.ReprocessID)
	return requireOneRow(cleared, err)
}
