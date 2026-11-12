package processingstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// RetractPublished withdraws every published result of the Workflow for the Transmission except
// the one named keep (uuid.Nil keeps none): it stamps retracted_at, detaches the result from its
// Message and deletes its projected attribute values, so no reader sees it any more and it can
// never be published again. It returns the retracted result IDs. The caller's transaction must
// also carry the reason for the retraction.
func (s *Store) RetractPublished(ctx context.Context, tx platform.TransactionContext,
	transmission, workflow, keep uuid.UUID) ([]uuid.UUID, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `UPDATE processing_results r
		SET retracted_at=$4, published_at=NULL, message_id=NULL
		FROM processing_candidates c
		WHERE c.id = r.candidate_id AND c.transmission_id=$1 AND c.workflow_id=$2
			AND r.published_at IS NOT NULL AND r.id <> $3
		RETURNING r.id`, transmission, workflow, keep, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ids, nil
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM message_attribute_values WHERE result_id = ANY($1::uuid[])`, ids)
	return ids, err
}
