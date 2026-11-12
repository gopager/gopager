package processingstore

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// terminalState reports whether a candidate has finished a run, successfully or not. Re-pending
// such a candidate re-runs work whose earlier outcome may already be published.
func terminalState(state string) bool { return state == "completed" || state == "failed" }

// otherCandidates lists the non-superseded candidates the Transmission has for Workflows other
// than the given one.
func (s *Store) otherCandidates(ctx context.Context, tx platform.TransactionContext,
	transmission, workflow uuid.UUID) ([]processing.ReprocessCandidateState, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id, state, revision FROM processing_candidates
		WHERE transmission_id=$1 AND workflow_id <> $2 AND state <> 'superseded'`, transmission, workflow)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var states []processing.ReprocessCandidateState
	for rows.Next() {
		var state processing.ReprocessCandidateState
		if err := rows.Scan(&state.ID, &state.State, &state.Revision); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

// markCompanionReruns gives the request's mark to every unmarked candidate of another Workflow
// that the readmission of the Transmission created or re-pended from a terminal state, when its
// Workflow already has history for the Transmission. Such a re-run is historical like the
// Reprocessed Workflow's own: it must replace the older results and must not reach Delivery.
// before is the state of those candidates ahead of the readmission.
func (s *Store) markCompanionReruns(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, transmission uuid.UUID, before []processing.ReprocessCandidateState) error {
	after, err := s.otherCandidates(ctx, tx, transmission, request.WorkflowID)
	if err != nil {
		return err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	earlier := make(map[uuid.UUID]processing.ReprocessCandidateState, len(before))
	for _, state := range before {
		earlier[state.ID] = state
	}
	for _, state := range after {
		previous, existed := earlier[state.ID]
		repended := existed && terminalState(previous.State) && state.Revision != previous.Revision
		if !repended && existed {
			continue
		}
		if _, err := executor.ExecContext(ctx, `UPDATE processing_candidates c SET reprocess_id=$2
			WHERE c.id=$1 AND c.reprocess_id IS NULL AND ($3 OR `+workflowHistory+`)`,
			state.ID, request.ID, repended); err != nil {
			return err
		}
	}
	return nil
}

// workflowHistory is a predicate over the candidate c: its Workflow has published or retracted
// results for its Transmission, or another of its candidates for the Transmission finished a run.
const workflowHistory = `(EXISTS (SELECT 1 FROM processing_results r JOIN processing_candidates h ON h.id = r.candidate_id
		WHERE h.transmission_id = c.transmission_id AND h.workflow_id = c.workflow_id
			AND (r.published_at IS NOT NULL OR r.retracted_at IS NOT NULL))
	OR EXISTS (SELECT 1 FROM processing_candidates h WHERE h.transmission_id = c.transmission_id
		AND h.workflow_id = c.workflow_id AND h.id <> c.id AND h.state IN ('completed', 'failed')))`

// transferOutstandingMark moves the mark of another candidate of the same Transmission and
// Workflow, typically one just superseded by a new version or context revision before it could
// publish, onto the given candidate and clears it on the donor, in one statement. The
// replacement thereby still retracts the earlier results and stays historical.
func (s *Store) transferOutstandingMark(ctx context.Context, tx platform.TransactionContext, candidate uuid.UUID) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var donor, mark uuid.UUID
	err = executor.QueryRowContext(ctx, `SELECT d.id, d.reprocess_id FROM processing_candidates d
		JOIN processing_candidates c ON c.transmission_id = d.transmission_id AND c.workflow_id = d.workflow_id
		WHERE c.id = $1 AND d.id <> c.id AND d.reprocess_id IS NOT NULL AND c.reprocess_id IS NULL
		ORDER BY d.created_at LIMIT 1 FOR UPDATE OF d`, candidate).Scan(&donor, &mark)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE processing_candidates SET reprocess_id = NULL WHERE id = $1`, donor); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `UPDATE processing_candidates SET reprocess_id = $2 WHERE id = $1`, candidate, mark)
	return err
}

// inheritReprocessMark gives a candidate that AdmitCandidate just created or re-pended the
// outstanding mark of its own Workflow's replaced candidate or, failing that, the mark of a
// Reprocess its Transmission is under, when its Workflow has history for the Transmission
// or the candidate was re-pended from a terminal state. The Transmission is under Reprocess while
// a non-superseded candidate of another Workflow carries a mark; the candidate itself is then a
// historical re-run, whereas a first run (fresh intake) stays unmarked.
func (s *Store) inheritReprocessMark(ctx context.Context, tx platform.TransactionContext,
	candidate uuid.UUID, repended bool) error {
	if err := s.transferOutstandingMark(ctx, tx, candidate); err != nil {
		return err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if _, err = executor.ExecContext(ctx, `UPDATE processing_candidates c SET reprocess_id = m.reprocess_id
		FROM (SELECT o.reprocess_id, o.transmission_id, o.workflow_id FROM processing_candidates o
			WHERE o.id <> $1 AND o.reprocess_id IS NOT NULL AND o.state <> 'superseded'
				AND o.transmission_id = (SELECT transmission_id FROM processing_candidates WHERE id = $1)
				AND o.workflow_id <> (SELECT workflow_id FROM processing_candidates WHERE id = $1)
			LIMIT 1) m
		WHERE c.id = $1 AND c.reprocess_id IS NULL AND ($2 OR `+workflowHistory+`)`, candidate, repended); err != nil {
		return err
	}
	return s.markCoveredByRequest(ctx, tx, candidate, repended)
}

// markCoveredByRequest marks a candidate that was just created or re-pended with the newest
// active Reprocess request of its Workflow that covers its Transmission, when the Workflow has
// history for the Transmission. The runner may not have reached the Transmission yet, so without
// this a readmission by a correction or by another Workflow's dispatch would make the re-run
// look like fresh intake. A request covers a Transmission accepted by its cutoff whose Message
// time lies in its range.
func (s *Store) markCoveredByRequest(ctx context.Context, tx platform.TransactionContext, candidate uuid.UUID, repended bool) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `UPDATE processing_candidates c SET reprocess_id = (
			SELECT r.id FROM processing_reprocess_requests r
				JOIN transmissions t ON t.id = c.transmission_id JOIN messages m ON m.id = t.message_id
			WHERE r.workflow_id = c.workflow_id AND r.state IN ('pending', 'running')
				AND t.accepted_at <= r.cutoff_at
				AND m.first_observed_at >= r.message_time_from AND m.first_observed_at < r.message_time_to
			ORDER BY r.created_at DESC, r.id DESC LIMIT 1)
		WHERE c.id = $1 AND c.reprocess_id IS NULL AND ($2 OR `+workflowHistory+`)`, candidate, repended)
	return err
}
