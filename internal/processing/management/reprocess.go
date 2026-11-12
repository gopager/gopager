package management

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ReprocessView is the wire projection of a Reprocess request; it carries no lease or actor data.
type ReprocessView struct {
	ID              uuid.UUID `json:"id"`
	WorkflowID      uuid.UUID `json:"workflowId"`
	VersionID       uuid.UUID `json:"versionId"`
	MessageTimeFrom time.Time `json:"messageTimeFrom"`
	MessageTimeTo   time.Time `json:"messageTimeTo"`
	CutoffAt        time.Time `json:"cutoffAt"`
	State           string    `json:"state"`
	Scanned         int64     `json:"scanned"`
	Readmitted      int64     `json:"readmitted"`
	Retracted       int64     `json:"retracted"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func reprocessView(request processing.ReprocessRequest) ReprocessView {
	return ReprocessView{ID: request.ID, WorkflowID: request.WorkflowID, VersionID: request.DefinitionVersionID,
		MessageTimeFrom: request.MessageTimeFrom.UTC(), MessageTimeTo: request.MessageTimeTo.UTC(),
		CutoffAt: request.CutoffAt.UTC(), State: request.State, Scanned: request.Scanned,
		Readmitted: request.Readmitted, Retracted: request.Retracted,
		CreatedAt: request.CreatedAt.UTC(), UpdatedAt: request.UpdatedAt.UTC()}
}

// ReprocessService requests and reports explicit historical Reprocess runs of a Workflow.
type ReprocessService struct {
	management *Service
	repository processing.ReprocessRepository
}

// NewReprocess composes the Workflow management service with the request store.
func NewReprocess(management *Service, repository processing.ReprocessRepository) (*ReprocessService, error) {
	if management == nil || repository == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing reprocess dependency"))
	}
	return &ReprocessService{management: management, repository: repository}, nil
}

// Request records a Reprocess of the Workflow's active version over the Message-time range. The
// caller needs workflow.reprocess over the whole Workflow scope. In one transaction it bumps the
// Workflow revision, which makes every existing candidate of the Workflow reselect on readmission
// and fences the request against later Workflow changes, appends the same configuration fact as a
// Workflow update and stores the pending request with the primary clock as its scan cutoff.
// A version other than the active one, a stale revision or a Workflow that is disabled is a
// revision conflict.
func (s *ReprocessService) Request(ctx context.Context, proof identity.Proof, workflowID uuid.UUID,
	input processing.ReprocessInput, check func(identity.Authentication) error) (ReprocessView, error) {
	if workflowID == uuid.Nil {
		return ReprocessView{}, authz.ErrInvalidInput
	}
	if err := input.Validate(); err != nil {
		return ReprocessView{}, err
	}
	var result processing.ReprocessRequest
	err := s.management.within(ctx, proof, true, check, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		current, err := s.management.current(tx, authority, "workflow.reprocess", workflowID, input.ExpectedRevision, now)
		if err != nil {
			return err
		}
		if !current.Enabled || current.ActiveVersionID == nil || *current.ActiveVersionID != input.VersionID {
			return authz.ErrRevisionConflict
		}
		if err := s.management.repository.SaveWorkflow(tx.Context(), tx, authority, "workflow.reprocess",
			current, input.ExpectedRevision, now); err != nil {
			return err
		}
		if err := s.management.record(tx, authority, current, "workflow.reprocess", "updated", nil, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		result = processing.ReprocessRequest{ID: uuid.New(), WorkflowID: workflowID,
			DefinitionVersionID: input.VersionID, WorkflowRevision: current.Revision,
			MessageTimeFrom: input.MessageTimeFrom.UTC().Truncate(time.Microsecond),
			MessageTimeTo:   input.MessageTimeTo.UTC().Truncate(time.Microsecond),
			CutoffAt:        now, State: processing.ReprocessPending, RequestedBy: actor, DueAt: now,
			CreatedAt: now, UpdatedAt: now}
		return s.repository.CreateReprocess(tx.Context(), tx, result)
	})
	if err != nil {
		return ReprocessView{}, err
	}
	return reprocessView(result), nil
}

// withWorkflowRead runs read once the caller's workflow.read over the whole Workflow scope is proven.
func (s *ReprocessService) withWorkflowRead(ctx context.Context, proof identity.Proof, workflowID uuid.UUID,
	read func(platform.TransactionContext) error) error {
	if workflowID == uuid.Nil {
		return authz.ErrInvalidInput
	}
	return s.management.within(ctx, proof, false, nil, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		workflow, err := s.management.repository.ReadWorkflow(tx.Context(), tx, authority, "workflow.read", workflowID, now)
		if err != nil {
			return err
		}
		if err := s.management.authorizeScope(tx.Context(), tx, authority, "workflow.read", workflow.Scope, now); err != nil {
			return err
		}
		return read(tx)
	})
}

// List returns one page of the Workflow's Reprocess requests, newest first.
func (s *ReprocessService) List(ctx context.Context, proof identity.Proof, workflowID uuid.UUID,
	input processing.PageInput) (processing.Page[ReprocessView], error) {
	if err := validatePageInput(input); err != nil {
		return processing.Page[ReprocessView]{}, err
	}
	result := processing.Page[ReprocessView]{Items: []ReprocessView{}}
	err := s.withWorkflowRead(ctx, proof, workflowID, func(tx platform.TransactionContext) error {
		page, err := s.repository.ListReprocesses(tx.Context(), tx, workflowID, input)
		if err != nil {
			return err
		}
		result.Total = page.Total
		for _, request := range page.Items {
			result.Items = append(result.Items, reprocessView(request))
		}
		return nil
	})
	return result, err
}

// Read returns one Reprocess request of the Workflow.
func (s *ReprocessService) Read(ctx context.Context, proof identity.Proof, workflowID, id uuid.UUID) (ReprocessView, error) {
	if id == uuid.Nil {
		return ReprocessView{}, authz.ErrInvalidInput
	}
	var result ReprocessView
	err := s.withWorkflowRead(ctx, proof, workflowID, func(tx platform.TransactionContext) error {
		request, err := s.repository.ReadReprocess(tx.Context(), tx, workflowID, id)
		result = reprocessView(request)
		return err
	})
	return result, err
}
