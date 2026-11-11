package processingstore

import (
	"context"
	"encoding/json"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.ResultRepository = (*Store)(nil)

// CompleteCandidate validates the saved schema/current pins and atomically stores private fenced evidence.
// It does not append a completion fact, invent a Message or mark the source barrier ready.
func (s *Store) CompleteCandidate(ctx context.Context, tx platform.TransactionContext, lease processing.CandidateLease,
	originalTargetCount uint32, evaluation processing.Evaluation) (processing.Candidate, *processing.PrivateResult, error) {
	var result *processing.PrivateResult
	next, err := s.changeClaim(ctx, tx, lease.CandidateID, false, func(current processing.Candidate, now time.Time) (processing.Candidate, error) {
		completed, err := current.Complete(lease, evaluation.Selected, now)
		if err != nil {
			return processing.Candidate{}, err
		}
		client, scope, err := s.client(ctx, tx, map[string]entql.P{
			"WorkflowVersion": entql.And(entql.FieldEQ("id", current.DefinitionVersionID),
				entql.FieldEQ("workflow_id", current.WorkflowID)),
		}, map[string]bool{"ProcessingResult": true})
		if err != nil {
			return processing.Candidate{}, err
		}
		if _, err := client.WorkflowVersion.Query().Only(scope); err != nil {
			return processing.Candidate{}, err
		}
		outputs, err := s.outputDeclarations(ctx, tx)
		if err != nil {
			return processing.Candidate{}, err
		}
		if err := processing.ValidateEvaluation(evaluation, outputs, processing.EvaluationBinding{
			TransmissionID: current.TransmissionID, SourceRevision: current.SourceRevision,
			ContextRevision: current.ContextRevision, TargetLimit: originalTargetCount,
		}); err != nil {
			return processing.Candidate{}, err
		}
		if !evaluation.Selected {
			return completed, nil
		}
		// Classified before the round-trip below, which drops the unexported dependencies.
		fieldGroups, err := evaluation.FieldGroups()
		if err != nil {
			return processing.Candidate{}, err
		}
		encoded, err := json.Marshal(evaluation)
		if err != nil {
			return processing.Candidate{}, platform.NewFailure(platform.CodeInvalidInput, err)
		}
		var detached processing.Evaluation
		if err := processing.Decode(encoded, &detached); err != nil {
			return processing.Candidate{}, err
		}
		row, err := client.ProcessingResult.Create().SetID(uuid.New()).SetCandidateID(current.ID).
			SetRevision(completed.Revision).SetProducingFence(current.FencingToken).
			SetEvaluation(detached).SetFieldGroups(fieldGroups).SetCompletedAt(now).Save(scope)
		if err != nil {
			return processing.Candidate{}, err
		}
		result = &processing.PrivateResult{ID: row.ID, CandidateID: row.CandidateID, Revision: row.Revision,
			ProducingFence: row.ProducingFence, Evaluation: row.Evaluation, CompletedAt: row.CompletedAt}
		return completed, nil
	})
	if err != nil {
		return processing.Candidate{}, nil, err
	}
	if next.Selected == nil {
		return processing.Candidate{}, nil, authz.ErrRevisionConflict
	}
	return next, result, nil
}
