package processingstore

import (
	"context"
	"errors"

	"entgo.io/ent/entql"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/processingresult"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.PublicationRepository = (*Store)(nil)

// ReadPublicationItem locks the candidate of a binding and returns its current state with the saved
// definition and, for a selected completion, its result including a publication already applied.
// It returns authz.ErrNotFound when no candidate exists for the binding and a revision conflict
// when the Workflow is no longer active at the binding's version, the configuration revision moved
// or the candidate's revisions differ from the binding.
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
		return processing.PublicationItem{}, authz.ErrNotFound
	}
	if err != nil {
		return processing.PublicationItem{}, err
	}
	candidate, err := candidateState(row)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	if candidate.CandidateBinding != binding {
		return processing.PublicationItem{}, authz.ErrRevisionConflict
	}
	version, err := client.WorkflowVersion.Query().Only(scope)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	outputs, err := s.outputDeclarations(ctx, tx)
	if err != nil {
		return processing.PublicationItem{}, err
	}
	item := processing.PublicationItem{Candidate: candidate, Definition: version.Definition, Outputs: outputs}
	if candidate.State != "completed" || candidate.Selected == nil || !*candidate.Selected {
		return item, nil
	}
	resultClient, resultScope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingResult": entql.And(entql.FieldEQ("candidate_id", candidate.ID),
			entql.FieldEQ("revision", candidate.Revision), entql.FieldEQ("producing_fence", candidate.FencingToken)),
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
		Evaluation: result.Evaluation, CompletedAt: result.CompletedAt,
		PublishedAt: result.PublishedAt, MessageID: result.MessageID}
	return item, nil
}

// ApplyPublicationItem re-reads the candidate's current evidence, verifies that it still matches
// expected and attaches the result to the Message by setting its publication time and Message
// under the result's revision and producing fence; the candidate row is unchanged. The source
// status refresh and the completion fact must commit in this same transaction or all writes roll back.
func (s *Store) ApplyPublicationItem(ctx context.Context, tx platform.TransactionContext,
	binding processing.PublicationBinding, expected processing.PublicationItem) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	if !expected.Publishable() {
		return authz.ErrRevisionConflict
	}
	item, err := s.ReadPublicationItem(ctx, tx, expected.Candidate.CandidateBinding)
	if err != nil {
		return err
	}
	if !item.Publishable() || item.Candidate.ID != expected.Candidate.ID ||
		item.Candidate.Revision != expected.Candidate.Revision ||
		item.Candidate.FencingToken != expected.Candidate.FencingToken ||
		item.Candidate.TransmissionID != binding.Source.TransmissionID ||
		item.Candidate.SourceRevision != binding.Source.SourceRevision ||
		item.Candidate.ContextRevision != binding.Source.ContextRevision ||
		item.Result.ID != expected.Result.ID || item.Result.Revision != expected.Result.Revision ||
		item.Result.ProducingFence != expected.Result.ProducingFence {
		return authz.ErrRevisionConflict
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	if err := processing.ValidateEvaluation(item.Result.Evaluation, item.Outputs, processing.EvaluationBinding{
		TransmissionID: binding.Source.TransmissionID, SourceRevision: binding.Source.SourceRevision,
		ContextRevision: binding.Source.ContextRevision, TargetLimit: binding.Source.OriginalTargetCount,
	}); err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{
		"ProcessingResult": entql.FieldEQ("candidate_id", item.Candidate.ID),
	}, nil)
	if err != nil {
		return err
	}
	changed, err := client.ProcessingResult.Update().Where(processingresult.ID(item.Result.ID),
		processingresult.Revision(item.Result.Revision), processingresult.ProducingFence(item.Result.ProducingFence),
		processingresult.PublishedAtIsNil(), processingresult.MessageIDIsNil()).
		SetPublishedAt(now).SetMessageID(binding.MessageID).Save(scope)
	if err != nil {
		return err
	}
	if changed != 1 {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("private result changed before publication"))
	}
	return nil
}
