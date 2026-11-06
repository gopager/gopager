package management

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/units"
)

type previewRecheckRepository struct {
	processing.ConfigurationRepository
	workflow processing.WorkflowConfiguration
}

// ReadWorkflow returns the current fixture revision for the final read-only preview fence.
func (r *previewRecheckRepository) ReadWorkflow(context.Context, platform.TransactionContext,
	authz.Authority, string, uuid.UUID, time.Time) (processing.WorkflowConfiguration, error) {
	return r.workflow, nil
}

type previewScopes struct{}

// ReadNetwork represents an existing fixture Network of a nil Agency; `all` grants cover it.
func (previewScopes) ReadNetwork(context.Context, platform.TransactionContext, uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	return protocols.POCSAG, uuid.Nil, nil
}

// ReadUnitContext represents an existing fixture Unit of a nil Agency.
func (previewScopes) ReadUnitContext(_ context.Context, _ platform.TransactionContext, id uuid.UUID) (units.Context, error) {
	return units.Context{ID: id, Revision: 1, CollectionIDs: []uuid.UUID{}}, nil
}

type previewRecheckSource struct {
	PreviewSourceOwner
	calls int
	err   error
}

// CheckInstalledSource observes the final synthetic identity and separate current editor authority.
func (s *previewRecheckSource) CheckInstalledSource(_ context.Context, _ platform.TransactionContext,
	binding processing.ExecutionBinding, _ authz.Authority, call processing.InvocationContext,
	_ []processing.Dependency, editor bool) error {
	if !editor || call.Source != binding.Source || !binding.Source.Synthetic {
		return errors.New("final preview source/editor binding missing")
	}
	s.calls++
	return s.err
}

func TestPreviewFinalFenceRechecksSourceAndRejectsChangedWorkflowOrAuthority(t *testing.T) {
	principal := uuid.NewString()
	authority := authz.Authority{PrincipalID: principal, Revision: 1, Active: true,
		Grants: []authz.Grant{{ID: uuid.NewString(), SubjectID: principal, SubjectKind: "principal",
			Actions: []string{"workflow.preview"}, Scope: authz.Scope{Kind: "all"}}}}
	workflow := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1, Scope: authz.Scope{Kind: "all"}}
	repository := &previewRecheckRepository{workflow: workflow}
	source := &previewRecheckSource{}
	service := &PreviewService{management: &Service{repository: repository, scopes: previewScopes{}}, sourceOwner: source}
	binding := processing.ExecutionBinding{WorkflowID: workflow.ID, Source: processing.SourceIdentity{
		TransmissionID: uuid.New(), NetworkID: uuid.New(), SourceRevision: 1, ContextRevision: 1, Synthetic: true}}
	check := func(authority authz.Authority) error {
		return service.checkPreviewContext(t.Context(), nil, authority, time.Now(), workflow, binding, nil)
	}
	if err := check(authority); err != nil || source.calls != 1 {
		t.Fatalf("final source fence not called: calls=%d err=%v", source.calls, err)
	}
	source.err = platform.NewFailure(platform.CodeRevisionConflict, errors.New("source changed"))
	if err := check(authority); platform.FailureCode(err) != platform.CodeRevisionConflict {
		t.Fatalf("changed source accepted after evaluation: %v", err)
	}
	source.err = nil
	repository.workflow.Revision++
	if err := check(authority); platform.FailureCode(err) != platform.CodeRevisionConflict {
		t.Fatalf("changed Workflow accepted: %v", err)
	}
	repository.workflow.Revision--
	authority.Grants = nil
	if err := check(authority); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("revoked editor accepted: %v", err)
	}
}
