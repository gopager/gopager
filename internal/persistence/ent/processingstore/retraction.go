package processingstore

import (
	"bytes"
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// RetractPublished withdraws every published result of the Workflow for the Transmission except
// the one named keep (uuid.Nil keeps none): it stamps retracted_at, detaches the result from its
// Message and deletes its projected attribute values, so no reader sees it any more and it can
// never be published again. Each retraction advances the result's revision, so a retraction fact
// continues the aggregate of the result's completion fact. It returns the retracted results,
// ordered by ID. The caller's transaction must
// also carry the reason for the retraction.
func (s *Store) RetractPublished(ctx context.Context, tx platform.TransactionContext,
	transmission, workflow, keep uuid.UUID) ([]processing.RetractedResult, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `UPDATE processing_results r
		SET retracted_at=$4, published_at=NULL, message_id=NULL, revision=r.revision+1
		FROM processing_candidates c
		WHERE c.id = r.candidate_id AND c.transmission_id=$1 AND c.workflow_id=$2
			AND r.published_at IS NOT NULL AND r.id <> $3
		RETURNING r.id, r.revision`, transmission, workflow, keep, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	retracted := []processing.RetractedResult{}
	for rows.Next() {
		var result processing.RetractedResult
		if err := rows.Scan(&result.ID, &result.Revision); err != nil {
			return nil, err
		}
		retracted = append(retracted, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(retracted) == 0 {
		return retracted, nil
	}
	slices.SortFunc(retracted, func(a, b processing.RetractedResult) int { return bytes.Compare(a.ID[:], b.ID[:]) })
	_, err = executor.ExecContext(ctx, `DELETE FROM message_attribute_values WHERE result_id = ANY($1::uuid[])`, processing.RetractedIDs(retracted))
	return retracted, err
}
