package processingstore

import (
	"context"
	"errors"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/processingcandidate"
	"github.com/gopager/gopager/internal/persistence/ent/processingresult"
	"github.com/gopager/gopager/internal/persistence/ent/workflow"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
)

var _ processingmanagement.ProcessingReadRepository = (*Store)(nil)

type candidateStatusProjection struct {
	ID          uuid.UUID `sql:"id"`
	Revision    int64     `sql:"revision"`
	State       string    `sql:"state"`
	Selected    *bool     `sql:"selected"`
	FailureCode *string   `sql:"failure_code"`
	UpdatedAt   time.Time `sql:"updated_at"`
}

type privateResultMetadataProjection struct {
	ID          uuid.UUID `sql:"id"`
	Revision    int64     `sql:"revision"`
	CandidateID uuid.UUID `sql:"candidate_id"`
	CompletedAt time.Time `sql:"completed_at"`
}

func (s *Store) processingReadClient(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, workflowID uuid.UUID, now time.Time,
	candidateID uuid.UUID) (*ent.Client, context.Context, error) {
	if workflowID == uuid.Nil {
		return nil, nil, authz.ErrInvalidInput
	}
	filter, err := workflowScope(authority, "workflow.read", now)
	if err != nil {
		return nil, nil, err
	}
	predicates := map[string]entql.P{
		"Workflow":               filter,
		"WorkflowVersion":        entql.HasEdgeWith("workflow", filter),
		"WorkflowScopeReference": entql.HasEdgeWith("workflow", filter),
		"ProcessingCandidate":    entql.FieldEQ("workflow_id", workflowID),
	}
	if candidateID != uuid.Nil {
		predicates["ProcessingCandidate"] = entql.And(
			entql.FieldEQ("workflow_id", workflowID),
			entql.FieldEQ("id", candidateID),
		)
		predicates["ProcessingResult"] = entql.FieldEQ("candidate_id", candidateID)
	}
	client, scope, err := s.client(ctx, tx, predicates, nil)
	if err != nil {
		return nil, nil, err
	}
	if _, err := client.Workflow.Query().Where(workflow.ID(workflowID)).Only(scope); err != nil {
		if ent.IsNotFound(err) {
			return nil, nil, authz.ErrNotFound
		}
		return nil, nil, err
	}
	return client, scope, nil
}

func candidateStatus(value candidateStatusProjection) processingmanagement.CandidateStatus {
	status := processingmanagement.CandidateStatus{
		ID: value.ID, Revision: value.Revision, State: value.State,
		Selected: value.Selected, UpdatedAt: value.UpdatedAt,
	}
	if value.FailureCode != nil {
		code := platform.ErrorCode(*value.FailureCode)
		status.FailureCode = &code
	}
	return status
}

// ListCandidateStatuses applies the current readable Workflow and parent key before count and paging.
func (s *Store) ListCandidateStatuses(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, workflowID uuid.UUID, input processing.PageInput, now time.Time,
) (processing.Page[processingmanagement.CandidateStatus], error) {
	input, err := page(input)
	if err != nil {
		return processing.Page[processingmanagement.CandidateStatus]{}, err
	}
	client, scope, err := s.processingReadClient(ctx, tx, authority, workflowID, now, uuid.Nil)
	if err != nil {
		return processing.Page[processingmanagement.CandidateStatus]{}, err
	}
	query := client.ProcessingCandidate.Query().Where(processingcandidate.WorkflowID(workflowID))
	total, err := query.Clone().Count(scope)
	if err != nil {
		return processing.Page[processingmanagement.CandidateStatus]{}, err
	}
	var rows []candidateStatusProjection
	err = query.Order(processingcandidate.ByUpdatedAt(), processingcandidate.ByID()).
		Offset(input.Offset).Limit(input.Limit).
		Select(processingcandidate.FieldID, processingcandidate.FieldRevision, processingcandidate.FieldState,
			processingcandidate.FieldSelected, processingcandidate.FieldFailureCode, processingcandidate.FieldUpdatedAt).
		Scan(scope, &rows)
	if err != nil {
		return processing.Page[processingmanagement.CandidateStatus]{}, err
	}
	result := processing.Page[processingmanagement.CandidateStatus]{
		Total: total, Items: make([]processingmanagement.CandidateStatus, 0, len(rows)),
	}
	for _, row := range rows {
		result.Items = append(result.Items, candidateStatus(row))
	}
	return result, nil
}

// ReadCandidateStatus returns only selected status columns for a candidate owned by the readable Workflow.
func (s *Store) ReadCandidateStatus(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, workflowID, candidateID uuid.UUID, now time.Time,
) (processingmanagement.CandidateStatus, error) {
	if candidateID == uuid.Nil {
		return processingmanagement.CandidateStatus{}, authz.ErrInvalidInput
	}
	client, scope, err := s.processingReadClient(ctx, tx, authority, workflowID, now, candidateID)
	if err != nil {
		return processingmanagement.CandidateStatus{}, err
	}
	var rows []candidateStatusProjection
	err = client.ProcessingCandidate.Query().
		Where(processingcandidate.ID(candidateID), processingcandidate.WorkflowID(workflowID)).
		Select(processingcandidate.FieldID, processingcandidate.FieldRevision, processingcandidate.FieldState,
			processingcandidate.FieldSelected, processingcandidate.FieldFailureCode, processingcandidate.FieldUpdatedAt).
		Scan(scope, &rows)
	if err != nil {
		return processingmanagement.CandidateStatus{}, err
	}
	if len(rows) == 0 {
		return processingmanagement.CandidateStatus{}, authz.ErrNotFound
	}
	if len(rows) != 1 {
		return processingmanagement.CandidateStatus{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("candidate status query returned multiple rows"))
	}
	return candidateStatus(rows[0]), nil
}

// ReadPrivateResultMetadata returns only the current completed result's identity and completion time.
func (s *Store) ReadPrivateResultMetadata(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, workflowID, candidateID uuid.UUID, now time.Time,
) (processingmanagement.PrivateResultMetadata, error) {
	if candidateID == uuid.Nil {
		return processingmanagement.PrivateResultMetadata{}, authz.ErrInvalidInput
	}
	client, scope, err := s.processingReadClient(ctx, tx, authority, workflowID, now, candidateID)
	if err != nil {
		return processingmanagement.PrivateResultMetadata{}, err
	}
	var candidates []candidateStatusProjection
	err = client.ProcessingCandidate.Query().
		Where(processingcandidate.ID(candidateID), processingcandidate.WorkflowID(workflowID)).
		Select(processingcandidate.FieldID, processingcandidate.FieldRevision, processingcandidate.FieldState,
			processingcandidate.FieldSelected, processingcandidate.FieldFailureCode, processingcandidate.FieldUpdatedAt).
		Scan(scope, &candidates)
	if err != nil {
		return processingmanagement.PrivateResultMetadata{}, err
	}
	if len(candidates) == 0 {
		return processingmanagement.PrivateResultMetadata{}, authz.ErrNotFound
	}
	if len(candidates) != 1 {
		return processingmanagement.PrivateResultMetadata{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("candidate status query returned multiple rows"))
	}
	status := candidateStatus(candidates[0])
	if status.State != "completed" || status.Selected == nil || !*status.Selected {
		return processingmanagement.PrivateResultMetadata{}, authz.ErrNotFound
	}

	var rows []privateResultMetadataProjection
	err = client.ProcessingResult.Query().
		Where(processingresult.CandidateID(candidateID), processingresult.Revision(status.Revision)).
		Select(processingresult.FieldID, processingresult.FieldRevision, processingresult.FieldCandidateID,
			processingresult.FieldCompletedAt).
		Scan(scope, &rows)
	if err != nil {
		return processingmanagement.PrivateResultMetadata{}, err
	}
	if len(rows) == 0 {
		return processingmanagement.PrivateResultMetadata{}, authz.ErrNotFound
	}
	if len(rows) != 1 {
		return processingmanagement.PrivateResultMetadata{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("private result query returned multiple rows"))
	}
	row := rows[0]
	return processingmanagement.PrivateResultMetadata{
		ID: row.ID, Revision: row.Revision, CandidateID: row.CandidateID, CompletedAt: row.CompletedAt,
	}, nil
}
