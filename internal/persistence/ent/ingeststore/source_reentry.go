package ingeststore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// PublicationReentries lists unpublished Transmissions with no unfinished Processing candidate and no
// overlapping unfinished correction, oldest first (C-05 post-correction publication re-entry).
func (o *ProcessingSourceOwner) PublicationReentries(ctx context.Context, tx platform.TransactionContext,
	limit int,
) ([]uuid.UUID, error) {
	if limit <= 0 || limit > 100 {
		return nil, authz.ErrInvalidInput
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT t.id FROM transmissions t
		JOIN networks n ON n.id=t.network_id AND n.deleted=false
		WHERE t.processing_status='pending' AND t.publication_status='pending'
		AND NOT EXISTS (SELECT 1 FROM processing_candidates candidate
			WHERE candidate.transmission_id=t.id AND candidate.state NOT IN ('completed','superseded'))
		AND NOT EXISTS (SELECT 1 FROM assignment_pending_scopes scope
			WHERE scope.state IN ('queued','running','failed')
			AND (jsonb_array_length(scope.network_ids)=0 OR scope.network_ids @> jsonb_build_array(t.network_id::text)
				OR EXISTS (SELECT 1 FROM transmission_targets affected
					WHERE affected.transmission_id=t.id AND affected.unit_id=scope.unit_id)))
		ORDER BY t.accepted_at,t.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

// ReenterPublication admits the current Workflow bindings for an unpublished Transmission, or publishes it
// through the existing atomic barrier when no Workflow is selected or every current binding has completed.
func (o *ProcessingSourceOwner) ReenterPublication(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID,
) (bool, error) {
	snapshot, err := o.readSource(ctx, tx, transmission)
	if err != nil {
		return false, err
	}
	if !snapshot.targetsCurrent {
		return false, nil
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return false, err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return false, err
	}
	candidates, err := o.readCandidates(ctx, tx, transmission)
	if err != nil {
		return false, err
	}
	expected := make(map[uuid.UUID]processing.CandidateBinding, len(workflows))
	for _, workflow := range workflows {
		expected[workflow.id] = processing.CandidateBinding{
			TransmissionID: transmission, WorkflowID: workflow.id, DefinitionVersionID: workflow.version,
			SourceRevision: snapshot.identity.SourceRevision, ContextRevision: snapshot.identity.ContextRevision,
			WorkflowRevision: workflow.revision, ConfigurationRevision: configurationRevision,
		}
	}
	completed := make(map[uuid.UUID]bool, len(workflows))
	for _, row := range candidates {
		if row.state == "superseded" {
			continue
		}
		binding, active := expected[row.binding.WorkflowID]
		if active && row.binding == binding && row.state == "completed" {
			completed[row.binding.WorkflowID] = true
			continue
		}
		if row.barrierAt.Valid {
			continue
		}
		if active && row.binding == binding {
			// Unfinished current work is owned by the Processing worker.
			return false, nil
		}
		if _, err := o.repository.SupersedeCandidate(ctx, tx, row.id, row.revision); err != nil {
			return false, err
		}
	}
	admitted := false
	for _, workflow := range workflows {
		if completed[workflow.id] {
			continue
		}
		if _, err := o.repository.AdmitCandidate(ctx, tx, expected[workflow.id]); err != nil {
			return false, err
		}
		admitted = true
	}
	if admitted {
		return true, nil
	}
	if err := o.publication.Publish(ctx, tx, transmission); err != nil {
		return false, err
	}
	return true, nil
}

var _ assignments.CorrectionPublication = (*ProcessingSourceOwner)(nil)
