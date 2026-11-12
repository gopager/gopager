package management

import (
	"context"
	"errors"
	"math"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// CandidateStatus is the bounded management projection of private work state.
// It deliberately excludes source identity, execution pins, lease data and private results.
type CandidateStatus struct {
	ID          uuid.UUID           `json:"id"`
	Revision    int64               `json:"revision,string"`
	State       string              `json:"state"`
	Selected    *bool               `json:"selected,omitempty"`
	FailureCode *platform.ErrorCode `json:"failureCode,omitempty"`
	// FailureReason explains FailureCode (`resource-limit`, `capability-unavailable`,
	// `conversion-failed` or `evaluation-failed`); it is derived on read, never stored.
	FailureReason string `json:"failureReason,omitempty"`
	// FailureStep is the ID of the Workflow step that failed, when a step did.
	FailureStep *string   `json:"failureStep,omitempty"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// PrivateResultMetadata omits classified Evaluation fields and all source identity.
type PrivateResultMetadata struct {
	ID          uuid.UUID `json:"id"`
	Revision    int64     `json:"revision,string"`
	CompletedAt time.Time `json:"completedAt"`
	CandidateID uuid.UUID `json:"-"`
}

// ProcessingReadRepository provides only Workflow-bound management projections.
// Implementations must enforce current workflow.read permissions in the authoritative SQL query,
// including permission-filtered paging; worker dispatch queries are not suitable.
type ProcessingReadRepository interface {
	ListCandidateStatuses(context.Context, platform.TransactionContext, authz.Authority,
		uuid.UUID, processing.PageInput, time.Time) (processing.Page[CandidateStatus], error)
	ReadCandidateStatus(context.Context, platform.TransactionContext, authz.Authority,
		uuid.UUID, uuid.UUID, time.Time) (CandidateStatus, error)
	ReadPrivateResultMetadata(context.Context, platform.TransactionContext, authz.Authority,
		uuid.UUID, uuid.UUID, time.Time) (PrivateResultMetadata, error)
}

// ProcessingReads adds protected, Workflow-bound private status reads without exposing raw work rows.
type ProcessingReads struct {
	management *Service
	repository ProcessingReadRepository
}

// NewProcessingReads requires the native permission-filtered status query owner.
func NewProcessingReads(management *Service, repository ProcessingReadRepository) (*ProcessingReads, error) {
	if management == nil || repository == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing read dependency"))
	}
	return &ProcessingReads{management: management, repository: repository}, nil
}

func validateCandidateStatus(status CandidateStatus) error {
	if status.ID == uuid.Nil || status.Revision <= 0 ||
		!slices.Contains([]string{"pending", "running", "completed", "failed", "superseded"}, status.State) ||
		status.UpdatedAt.IsZero() {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid candidate status projection"))
	}
	if status.State == "completed" && status.Selected == nil ||
		status.State != "completed" && status.Selected != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid candidate selection projection"))
	}
	if status.FailureCode != nil && !slices.Contains([]platform.ErrorCode{
		platform.CodeInvalidInput, platform.CodeIncompatibleVersion, platform.CodeResourceLimit,
		platform.CodeStorageUnavailable, platform.CodeAuthorityUnavailable, platform.CodeInternalError,
	}, *status.FailureCode) {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid candidate failure projection"))
	}
	if status.State != "failed" && status.FailureCode != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("unexpected candidate failure projection"))
	}
	if status.FailureStep != nil && (status.FailureCode == nil || len(*status.FailureStep) > processing.MaxFailureStepBytes) {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid candidate failure step projection"))
	}
	return nil
}

func validatePageInput(input processing.PageInput) error {
	if input.Offset < 0 || input.Limit <= 0 || input.Limit > processing.MaxCollection ||
		input.Offset > math.MaxInt-input.Limit {
		return authz.ErrInvalidInput
	}
	return nil
}

// detachCandidateStatus copies the mutable fields of a validated status and explains its failure
// code with a reason.
func detachCandidateStatus(status CandidateStatus) CandidateStatus {
	if status.Selected != nil {
		selected := *status.Selected
		status.Selected = &selected
	}
	if status.FailureCode != nil {
		code := *status.FailureCode
		status.FailureCode = &code
		status.FailureReason = processing.FailureReason(string(code))
	}
	if status.FailureStep != nil {
		step := *status.FailureStep
		status.FailureStep = &step
	}
	return status
}

func (s *ProcessingReads) withWorkflowRead(ctx context.Context, proof identity.Proof, workflowID uuid.UUID,
	read func(platform.TransactionContext, authz.Authority, time.Time) error) error {
	if s == nil || s.management == nil || s.repository == nil || workflowID == uuid.Nil || ctx == nil {
		return authz.ErrInvalidInput
	}
	return s.management.within(ctx, proof, false, nil, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		workflow, err := s.management.repository.ReadWorkflow(tx.Context(), tx, authority,
			"workflow.read", workflowID, now)
		if err != nil {
			return err
		}
		if err := s.management.authorizeScope(tx.Context(), tx, authority, "workflow.read", workflow.Scope, now); err != nil {
			return err
		}
		return read(tx, authority, now)
	})
}

// ListCandidateStatuses returns a bounded status page for one currently readable Workflow.
func (s *ProcessingReads) ListCandidateStatuses(ctx context.Context, proof identity.Proof,
	workflowID uuid.UUID, input processing.PageInput) (processing.Page[CandidateStatus], error) {
	if err := validatePageInput(input); err != nil {
		return processing.Page[CandidateStatus]{}, err
	}
	var result processing.Page[CandidateStatus]
	err := s.withWorkflowRead(ctx, proof, workflowID, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListCandidateStatuses(tx.Context(), tx, authority, workflowID, input, now)
		if err != nil {
			return err
		}
		if result.Total < len(result.Items) || len(result.Items) > input.Limit {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid candidate status page"))
		}
		result.Items = append([]CandidateStatus(nil), result.Items...)
		for i, status := range result.Items {
			if err := validateCandidateStatus(status); err != nil {
				return err
			}
			result.Items[i] = detachCandidateStatus(status)
		}
		return nil
	})
	if err != nil {
		return processing.Page[CandidateStatus]{}, err
	}
	return result, nil
}

// ReadCandidateStatus returns one current status only through its readable Workflow.
func (s *ProcessingReads) ReadCandidateStatus(ctx context.Context, proof identity.Proof,
	workflowID, candidateID uuid.UUID) (CandidateStatus, error) {
	if candidateID == uuid.Nil {
		return CandidateStatus{}, authz.ErrInvalidInput
	}
	var result CandidateStatus
	err := s.withWorkflowRead(ctx, proof, workflowID, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadCandidateStatus(tx.Context(), tx, authority, workflowID, candidateID, now)
		if err != nil {
			return err
		}
		if result.ID != candidateID {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("candidate status query returned a different candidate"))
		}
		if err := validateCandidateStatus(result); err != nil {
			return err
		}
		result = detachCandidateStatus(result)
		return nil
	})
	if err != nil {
		return CandidateStatus{}, err
	}
	return result, nil
}

// ReadPrivateResultMetadata returns identifiers/timing only; Evaluation requires a separate authorized projection.
func (s *ProcessingReads) ReadPrivateResultMetadata(ctx context.Context, proof identity.Proof,
	workflowID, candidateID uuid.UUID) (PrivateResultMetadata, error) {
	if candidateID == uuid.Nil {
		return PrivateResultMetadata{}, authz.ErrInvalidInput
	}
	var result PrivateResultMetadata
	err := s.withWorkflowRead(ctx, proof, workflowID, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		status, err := s.repository.ReadCandidateStatus(tx.Context(), tx, authority, workflowID, candidateID, now)
		if err != nil {
			return err
		}
		if status.ID != candidateID {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("candidate status query returned a different candidate"))
		}
		if err := validateCandidateStatus(status); err != nil {
			return err
		}
		if status.State != "completed" || status.Selected == nil || !*status.Selected {
			return authz.ErrNotFound
		}
		result, err = s.repository.ReadPrivateResultMetadata(tx.Context(), tx, authority,
			workflowID, candidateID, now)
		if err != nil {
			return err
		}
		if result.ID == uuid.Nil || result.CandidateID != candidateID || result.Revision <= 0 || result.CompletedAt.IsZero() ||
			result.CompletedAt.After(now) {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid private result metadata"))
		}
		return nil
	})
	if err != nil {
		return PrivateResultMetadata{}, err
	}
	return result, nil
}
