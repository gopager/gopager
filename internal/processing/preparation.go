package processing

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DeclarationSet is what a Workflow definition is compiled against.
type DeclarationSet struct {
	// Source declares the source and target fields a definition reads.
	Source Declarations
	// Outputs declares the Attribute fields a definition writes.
	Outputs Declarations
}

// SourceDeclarationOwner supplies trusted, current declarations for compiling a definition.
// Whether deprecated fields are accepted is the owner's configuration: the owner behind the
// preparer rejects them, so a new version never references a deprecated field.
type SourceDeclarationOwner interface {
	ProcessingDeclarations(context.Context) (DeclarationSet, error)
}

// MissingSourceDeclarationOwner blocks preparation until the schema owner is wired.
type MissingSourceDeclarationOwner struct{}

// ProcessingDeclarations never substitutes caller-provided declarations.
func (MissingSourceDeclarationOwner) ProcessingDeclarations(context.Context) (DeclarationSet, error) {
	return DeclarationSet{}, capabilityUnavailable(errors.New("processing declaration owner unavailable"))
}

// WorkflowDefinitionPreparer compiles immutable definitions against trusted declarations.
type WorkflowDefinitionPreparer struct {
	declarations SourceDeclarationOwner
	regex        RegexEvaluator
	artifacts    InstalledArtifacts
}

// NewWorkflowDefinitionPreparer requires the real declaration owner and the approved regex host.
func NewWorkflowDefinitionPreparer(declarations SourceDeclarationOwner, regex RegexEvaluator,
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
	if declarations.Source == nil || declarations.Outputs == nil {
		return nil, capabilityUnavailable(errors.New("processing declaration owner returned no declarations"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if artifacts == nil {
		return CompileWorkflow(ctx, definition, declarations.Source, declarations.Outputs, p.regex)
	}
	return CompileWorkflowInstalled(ctx, definition, declarations.Source, declarations.Outputs, p.regex, artifacts)
}

type preparationArtifactTracker struct {
	owner     InstalledArtifacts
	mu        sync.Mutex
	artifacts []*InstalledArtifact
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
	program, err := p.CompileWorkflow(ctx, definition, tracker)
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
