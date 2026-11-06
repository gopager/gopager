package management

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/execution"
)

// PreviewCapture is returned only by the trusted Protocol/source owner.
type PreviewCapture struct {
	Projection processing.SourceProjection
	Scope      processing.EvaluationScope
}

// PreviewSourceOwner captures current trusted source projections and rechecks them for installed calls.
type PreviewSourceOwner interface {
	processing.InvocationSourceOwner
	CaptureSyntheticPreview(context.Context, platform.TransactionContext, authz.Authority,
		processing.WorkflowConfiguration, json.RawMessage) (PreviewCapture, error)
	CaptureStoredPreview(context.Context, platform.TransactionContext, authz.Authority,
		processing.WorkflowConfiguration, uuid.UUID) (PreviewCapture, error)
}

// SyntheticPreviewReleaser is optionally implemented by a PreviewSourceOwner that holds
// captured synthetic sources in memory. The preview service calls ReleaseSyntheticPreview
// with the captured TransmissionID once the preview completes.
type SyntheticPreviewReleaser interface {
	ReleaseSyntheticPreview(uuid.UUID)
}

// WorkflowCompiler compiles against trusted declarations and the current per-run installed artifacts.
type WorkflowCompiler interface {
	CompileWorkflow(context.Context, processing.Definition, processing.InstalledArtifacts) (*processing.WorkflowProgram, error)
}

// PreviewInput selects one immutable saved version or one unsaved definition and exactly one source.
type PreviewInput struct {
	WorkflowID           uuid.UUID              `json:"workflowId"`
	VersionID            uuid.UUID              `json:"versionId,omitempty"`
	Definition           *processing.Definition `json:"definition,omitempty"`
	SyntheticJSON        json.RawMessage        `json:"syntheticJson,omitempty"`
	StoredTransmissionID uuid.UUID              `json:"storedTransmissionId,omitempty"`
}

// PreviewService coordinates read-only, isolated evaluation without publishing or persisting results.
type PreviewService struct {
	management  *Service
	compiler    WorkflowCompiler
	executions  *processing.InstanceExecutions
	sourceOwner PreviewSourceOwner
}

// NewPreview requires management, trusted compilation and per-run execution lifetimes.
// A nil source owner is permitted so deployments can expose an explicit unavailable capability.
func NewPreview(management *Service, compiler WorkflowCompiler, executions *processing.InstanceExecutions,
	sourceOwner PreviewSourceOwner) (*PreviewService, error) {
	if management == nil || compiler == nil || executions == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing preview dependency"))
	}
	return &PreviewService{management: management, compiler: compiler, executions: executions,
		sourceOwner: sourceOwner}, nil
}

type previewSourceMode uint8

const (
	previewSynthetic previewSourceMode = iota + 1
	previewStored
)

func (input PreviewInput) validate() (previewSourceMode, error) {
	if input.WorkflowID == uuid.Nil || (input.VersionID == uuid.Nil) == (input.Definition == nil) {
		return 0, authz.ErrInvalidInput
	}
	if len(input.SyntheticJSON) != 0 {
		if input.StoredTransmissionID != uuid.Nil || len(input.SyntheticJSON) > processing.MaxInputBytes ||
			!json.Valid(input.SyntheticJSON) {
			return 0, authz.ErrInvalidInput
		}
		return previewSynthetic, nil
	}
	if input.StoredTransmissionID != uuid.Nil {
		return previewStored, nil
	}
	return 0, authz.ErrInvalidInput
}

// Preview authorizes the editor, captures trusted input, compiles/evaluates outside SQL, and always drains the run.
func (s *PreviewService) Preview(ctx context.Context, proof identity.Proof, input PreviewInput,
	checks ...func(identity.Authentication) error) (result processing.Evaluation, err error) {
	if s == nil || s.management == nil || s.compiler == nil || s.executions == nil || ctx == nil {
		return processing.Evaluation{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing preview participant"))
	}
	if len(checks) > 1 || (len(checks) == 1 && checks[0] == nil) {
		return processing.Evaluation{}, authz.ErrInvalidInput
	}
	var transportCheck func(identity.Authentication) error
	if len(checks) == 1 {
		transportCheck = checks[0]
	}
	mode, err := input.validate()
	if err != nil {
		return processing.Evaluation{}, err
	}
	input.SyntheticJSON = append(json.RawMessage(nil), input.SyntheticJSON...)
	if input.Definition != nil {
		encoded, encodeErr := json.Marshal(*input.Definition)
		if encodeErr != nil {
			return processing.Evaluation{}, encodeErr
		}
		var detached processing.Definition
		if decodeErr := processing.Decode(encoded, &detached); decodeErr != nil {
			return processing.Evaluation{}, decodeErr
		}
		input.Definition = &detached
	}
	if err := ctx.Err(); err != nil {
		return processing.Evaluation{}, err
	}
	var workflow processing.WorkflowConfiguration
	var definition processing.Definition
	var versionID uuid.UUID
	var capture PreviewCapture
	var editorAuthority authz.Authority
	if mode == previewSynthetic {
		if releaser, ok := s.sourceOwner.(SyntheticPreviewReleaser); ok {
			defer func() { releaser.ReleaseSyntheticPreview(capture.Projection.TransmissionID) }()
		}
	}
	err = s.management.within(ctx, proof, false, transportCheck, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var readErr error
		editorAuthority = authority
		workflow, readErr = s.management.repository.ReadWorkflow(tx.Context(), tx, authority, "rule.preview", input.WorkflowID, now)
		if readErr != nil {
			return readErr
		}
		if readErr = s.management.authorizeScope(tx.Context(), tx, authority, "rule.preview", workflow.Scope, now); readErr != nil {
			return readErr
		}
		if input.VersionID != uuid.Nil {
			version, versionErr := s.management.repository.ReadVersionForAction(tx.Context(), tx, authority,
				"rule.preview", input.WorkflowID, input.VersionID, now)
			if versionErr != nil {
				return versionErr
			}
			definition, versionID = version.Definition, version.ID
		} else {
			definition = *input.Definition
			versionID = uuid.New()
		}
		if s.sourceOwner == nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Processing preview source owner unavailable"))
		}
		switch mode {
		case previewSynthetic:
			capture, readErr = s.sourceOwner.CaptureSyntheticPreview(tx.Context(), tx, authority,
				workflow, append(json.RawMessage(nil), input.SyntheticJSON...))
		case previewStored:
			capture, readErr = s.sourceOwner.CaptureStoredPreview(tx.Context(), tx, authority,
				workflow, input.StoredTransmissionID)
		}
		return readErr
	})
	if err != nil {
		return processing.Evaluation{}, err
	}
	source, err := processing.CaptureSource(ctx, capture.Projection)
	if err != nil {
		return processing.Evaluation{}, err
	}
	sourceIdentity, err := source.Identity()
	if err != nil {
		return processing.Evaluation{}, err
	}
	editor, err := execution.NewPreviewAuthority(s.management.resolver, proof)
	if err != nil {
		return processing.Evaluation{}, err
	}
	binding := processing.ExecutionBinding{WorkflowID: workflow.ID,
		DefinitionVersionID: versionID, RunID: uuid.New(), Source: sourceIdentity,
		PreviewEditor: editor}
	run, err := s.executions.Begin(ctx, binding, s.sourceOwner)
	if err != nil {
		return processing.Evaluation{}, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		err = errors.Join(err, run.Close(closeCtx))
		if err != nil {
			result = processing.Evaluation{}
		}
	}()
	program, err := s.compiler.CompileWorkflow(run.Context(), definition, run)
	if err != nil {
		return processing.Evaluation{}, err
	}
	// A saved version was validated when it was saved; an unsaved draft has not been, and its
	// result is shown to the editor, so it is judged by the editor's current field permissions
	// exactly like a save.
	if input.Definition != nil {
		if err = authorizeReferences(editorAuthority, program.SourceReferences()); err != nil {
			return processing.Evaluation{}, err
		}
	}
	result, err = program.EvaluateScoped(run.Context(), source, capture.Scope, run)
	if err != nil {
		return processing.Evaluation{}, err
	}
	if err = s.recheckPreview(ctx, proof, workflow, binding, result); err != nil {
		return processing.Evaluation{}, err
	}
	return result, nil
}

// recheckPreview fences a detached result with current editor, Workflow and source authority.
func (s *PreviewService) recheckPreview(ctx context.Context, proof identity.Proof,
	workflow processing.WorkflowConfiguration, binding processing.ExecutionBinding, result processing.Evaluation) error {
	dependencies, err := result.Dependencies()
	if err != nil {
		return err
	}
	return s.management.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		return s.checkPreviewContext(tx.Context(), tx, authority, now, workflow, binding, dependencies)
	})
}

// checkPreviewContext validates the captured result while the caller holds current-authority locks.
func (s *PreviewService) checkPreviewContext(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, now time.Time, workflow processing.WorkflowConfiguration,
	binding processing.ExecutionBinding, dependencies []processing.Dependency) error {
	current, err := s.management.repository.ReadWorkflow(ctx, tx, authority, "rule.preview", workflow.ID, now)
	if err != nil {
		return err
	}
	if current.Revision != workflow.Revision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("preview Workflow changed during evaluation"))
	}
	if err := s.management.authorizeScope(ctx, tx, authority, "rule.preview", current.Scope, now); err != nil {
		return err
	}
	return s.sourceOwner.CheckInstalledSource(ctx, tx, binding, authority,
		processing.InvocationContext{Source: binding.Source}, dependencies, true)
}
