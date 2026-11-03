package processing

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SourceDeclarationOwner supplies trusted, current source and target schemas.
type SourceDeclarationOwner interface {
	ProcessingDeclarations(context.Context) (Declarations, error)
}

// MissingSourceDeclarationOwner blocks preparation until the schema owner is wired.
type MissingSourceDeclarationOwner struct{}

// ProcessingDeclarations never substitutes caller-provided declarations.
func (MissingSourceDeclarationOwner) ProcessingDeclarations(context.Context) (Declarations, error) {
	return nil, capabilityUnavailable(errors.New("processing declaration owner unavailable"))
}

// WorkflowDefinitionPreparer compiles immutable definitions against trusted declarations.
type WorkflowDefinitionPreparer struct {
	declarations SourceDeclarationOwner
	regex        *WASMHost
	artifacts    InstalledArtifacts
}

// NewWorkflowDefinitionPreparer requires the real declaration owner and the approved regex host.
func NewWorkflowDefinitionPreparer(declarations SourceDeclarationOwner, regex *WASMHost,
	artifacts InstalledArtifacts) (*WorkflowDefinitionPreparer, error) {
	if declarations == nil || regex == nil {
		return nil, invalid(errors.New("missing Workflow preparation dependency"))
	}
	return &WorkflowDefinitionPreparer{declarations: declarations, regex: regex, artifacts: artifacts}, nil
}

// CompileWorkflow prepares one Workflow using fresh trusted declarations and run-owned artifacts.
func (p *WorkflowDefinitionPreparer) CompileWorkflow(ctx context.Context, definition Definition,
	artifacts InstalledArtifacts) (*WorkflowProgram, error) {
	if p == nil || p.declarations == nil || p.regex == nil || ctx == nil {
		return nil, invalid(errors.New("missing Workflow preparation participant"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	declarations, err := p.declarations.ProcessingDeclarations(ctx)
	if err != nil {
		return nil, err
	}
	if declarations == nil {
		return nil, capabilityUnavailable(errors.New("processing declaration owner returned no declarations"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if artifacts == nil {
		return CompileWorkflow(ctx, definition, declarations, p.regex)
	}
	return CompileWorkflowInstalled(ctx, definition, declarations, p.regex, artifacts)
}

type preparationArtifactTracker struct {
	owner     InstalledArtifacts
	mu        sync.Mutex
	artifacts []*InstalledArtifact
}

type preparationPreviewArtifactTracker struct {
	*preparationArtifactTracker
}

func (t *preparationArtifactTracker) PrepareInstalled(ctx context.Context, id uuid.UUID, revision int64) (*InstalledArtifact, error) {
	artifact, err := t.owner.PrepareInstalled(ctx, id, revision)
	if artifact != nil {
		t.mu.Lock()
		t.artifacts = append(t.artifacts, artifact)
		t.mu.Unlock()
	}
	return artifact, err
}

func (t preparationPreviewArtifactTracker) PrepareInstalledPreview(ctx context.Context, id uuid.UUID,
	revision int64) (*InstalledArtifact, bool, error) {
	owner, ok := t.owner.(InstalledPreviewArtifacts)
	if !ok {
		return nil, false, capabilityUnavailable(errors.New("dedicated preview artifact owner unavailable"))
	}
	artifact, configured, err := owner.PrepareInstalledPreview(ctx, id, revision)
	if artifact != nil {
		t.mu.Lock()
		t.artifacts = append(t.artifacts, artifact)
		t.mu.Unlock()
	}
	return artifact, configured, err
}

func (t *preparationArtifactTracker) close(ctx context.Context) error {
	t.mu.Lock()
	artifacts := append([]*InstalledArtifact(nil), t.artifacts...)
	t.artifacts = nil
	t.mu.Unlock()
	var failure error
	seen := make(map[*InstalledArtifact]bool, len(artifacts))
	for _, artifact := range artifacts {
		if artifact == nil || seen[artifact] {
			continue
		}
		seen[artifact] = true
		failure = errors.Join(failure, artifact.Close(ctx))
	}
	return failure
}

// PrepareWorkflow implements management's trusted definition-preparation port.
func (p *WorkflowDefinitionPreparer) PrepareWorkflow(ctx context.Context, definition Definition) (*WorkflowProgram, error) {
	if p == nil || ctx == nil {
		return nil, invalid(errors.New("missing Workflow preparation context"))
	}
	if p.artifacts == nil {
		return p.CompileWorkflow(ctx, definition, nil)
	}
	tracker := &preparationArtifactTracker{owner: p.artifacts}
	var artifacts InstalledArtifacts = tracker
	if _, ok := p.artifacts.(InstalledPreviewArtifacts); ok {
		artifacts = preparationPreviewArtifactTracker{tracker}
	}
	program, err := p.CompileWorkflow(ctx, definition, artifacts)
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		return nil, errors.Join(err, tracker.close(closeCtx))
	}
	program.preparationArtifacts = tracker
	return program, nil
}

// ClosePreparedWorkflow releases artifacts loaded only to validate an immutable definition.
func (p *WorkflowDefinitionPreparer) ClosePreparedWorkflow(ctx context.Context, program *WorkflowProgram) error {
	if ctx == nil {
		return invalid(errors.New("missing Workflow preparation close context"))
	}
	if program == nil || program.preparationArtifacts == nil {
		return nil
	}
	tracker, ok := program.preparationArtifacts.(*preparationArtifactTracker)
	if !ok {
		return invalid(errors.New("invalid Workflow preparation artifact owner"))
	}
	program.preparationArtifacts = nil
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	return tracker.close(closeCtx)
}
