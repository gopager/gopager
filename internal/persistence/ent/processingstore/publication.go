package processingstore

import (
	"context"
	"errors"

	"entgo.io/ent/entql"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/persistence/ent/processingresult"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.PublicationRepository = (*Store)(nil)

// ReadPublicationItem locks completed evidence with the exact current active Workflow/configuration pins.
// Incomplete, superseded or already applied evidence cannot become a replacement publication.
func (s *Store) ReadPublicationItem(ctx context.Context, tx platform.TransactionContext,
	binding processing.CandidateBinding) (processing.PublicationItem, error) {
	if err := binding.Validate(); err != nil {
		return processing.PublicationItem{}, err
	}
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	if revision != binding.ConfigurationRevision {
		return processing.PublicationItem{}, authz.ErrRevisionConflict
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"Workflow": entql.And(entql.FieldEQ("id", binding.WorkflowID), entql.FieldEQ("revision", binding.WorkflowRevision),
			entql.FieldEQ("enabled", true), entql.FieldEQ("deleted", false),
			entql.FieldEQ("active_version_id", binding.DefinitionVersionID)),
		"WorkflowVersion": entql.And(entql.FieldEQ("id", binding.DefinitionVersionID),
			entql.FieldEQ("workflow_id", binding.WorkflowID)),
		"ProcessingCandidate": entql.And(entql.FieldEQ("transmission_id", binding.TransmissionID),
			entql.FieldEQ("workflow_id", binding.WorkflowID), entql.FieldEQ("definition_version_id", binding.DefinitionVersionID),
			entql.FieldEQ("source_revision", binding.SourceRevision), entql.FieldEQ("context_revision", binding.ContextRevision)),
	}, nil)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	if _, err := client.Workflow.Query().Only(scope); err != nil {
		if ent.IsNotFound(err) {
			return processing.PublicationItem{}, authz.ErrRevisionConflict
		}
		return processing.PublicationItem{}, err
	}
	row, err := client.ProcessingCandidate.Query().ForUpdate().Only(scope)
	if ent.IsNotFound(err) {
		return processing.PublicationItem{}, authz.ErrRevisionConflict
	}
	if err != nil {
		return processing.PublicationItem{}, err
	}
	candidate, err := candidateState(row)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	if candidate.CandidateBinding != binding || candidate.State != "completed" ||
		candidate.Selected == nil || candidate.BarrierAppliedAt != nil {
		return processing.PublicationItem{}, authz.ErrRevisionConflict
	}
	version, err := client.WorkflowVersion.Query().Only(scope)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	item := processing.PublicationItem{Candidate: candidate, Definition: version.Definition}
	if !*candidate.Selected {
		return item, nil
	}
	resultClient, resultScope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingResult": entql.And(entql.FieldEQ("candidate_id", candidate.ID),
			entql.FieldEQ("revision", candidate.Revision), entql.FieldEQ("producing_fence", candidate.FencingToken),
			entql.FieldNil("published_at"), entql.FieldNil("message_id")),
	}, nil)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	result, err := resultClient.ProcessingResult.Query().ForUpdate().Only(resultScope)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	item.Result = &processing.PrivateResult{ID: result.ID, CandidateID: result.CandidateID,
		Revision: result.Revision, ProducingFence: result.ProducingFence,
		Evaluation: result.Evaluation, CompletedAt: result.CompletedAt}
	return item, nil
}

// ApplyPublicationItem re-reads current evidence and applies only matching candidate/result fences.
// Source readiness and required facts must commit in this same transaction or all writes roll back.
func (s *Store) ApplyPublicationItem(ctx context.Context, tx platform.TransactionContext,
	binding processing.PublicationBinding, expected processing.PublicationItem) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	item, err := s.ReadPublicationItem(ctx, tx, expected.Candidate.CandidateBinding)
	if err != nil {
		return err
	}
	if item.Candidate.ID != expected.Candidate.ID || item.Candidate.Revision != expected.Candidate.Revision ||
		item.Candidate.FencingToken != expected.Candidate.FencingToken ||
		item.Candidate.TransmissionID != binding.Source.TransmissionID ||
		item.Candidate.SourceRevision != binding.Source.SourceRevision ||
		item.Candidate.ContextRevision != binding.Source.ContextRevision ||
		(item.Result == nil) != (expected.Result == nil) {
		return authz.ErrRevisionConflict
	}
	if item.Result != nil && (item.Result.ID != expected.Result.ID || item.Result.Revision != expected.Result.Revision ||
		item.Result.ProducingFence != expected.Result.ProducingFence) {
		return authz.ErrRevisionConflict
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	next, err := commands.NextRevision(item.Candidate.Revision, item.Candidate.Revision)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingCandidate": entql.FieldEQ("id", item.Candidate.ID),
		"ProcessingResult":    entql.FieldEQ("candidate_id", item.Candidate.ID),
	}, nil)
	if err != nil {
		return err
	}
	changed, err := client.ProcessingCandidate.Update().
		Where(processingcandidate.Revision(item.Candidate.Revision),
			processingcandidate.FencingToken(item.Candidate.FencingToken), processingcandidate.BarrierAppliedAtIsNil()).
		SetRevision(next).SetBarrierAppliedAt(now).SetUpdatedAt(now).Save(scope)
	if err != nil {
		return err
	}
	if changed != 1 {
		return authz.ErrRevisionConflict
	}
	if item.Result == nil {
		return nil
	}
	if err := processing.ValidateEvaluation(item.Result.Evaluation, item.Definition.OutputSchema, processing.EvaluationBinding{
		TransmissionID: binding.Source.TransmissionID, SourceRevision: binding.Source.SourceRevision,
		ContextRevision: binding.Source.ContextRevision, TargetLimit: binding.Source.OriginalTargetCount,
	}); err != nil {
		return err
	}
	changed, err = client.ProcessingResult.Update().Where(processingresult.ID(item.Result.ID),
		processingresult.Revision(item.Result.Revision), processingresult.ProducingFence(item.Result.ProducingFence),
		processingresult.PublishedAtIsNil(), processingresult.MessageIDIsNil()).
		SetPublishedAt(now).SetMessageID(binding.MessageID).Save(scope)
	if err != nil {
		return err
	}
	if changed != 1 {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("private result changed before source publication"))
	}
	return nil
}
