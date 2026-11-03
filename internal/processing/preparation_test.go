package processing

import (
	"context"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

type fixedDeclarationOwner struct {
	declarations Declarations
	err          error
}

func (o fixedDeclarationOwner) ProcessingDeclarations(context.Context) (Declarations, error) {
	return o.declarations, o.err
}

type fixedDeclarations map[string]FieldSchema

func (d fixedDeclarations) FieldSchema(ref FieldRef) (FieldSchema, error) {
	schema, exists := d[ref.key()]
	if !exists {
		return FieldSchema{}, invalid(errors.New("undeclared test field"))
	}
	return schema, nil
}

func TestWorkflowDefinitionPreparerUsesOnlyTrustedDeclarations(t *testing.T) {
	ctx := t.Context()
	host, err := NewWASMHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	flag := FieldRef{Namespace: "source", Path: []string{"flag"}}
	preparer, err := NewWorkflowDefinitionPreparer(fixedDeclarationOwner{
		declarations: fixedDeclarations{flag.key(): {Kind: BooleanValue}},
	}, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preparer.PrepareWorkflow(ctx, selectedDefinition()); err != nil {
		t.Fatalf("trusted source declaration rejected: %v", err)
	}
	definition := selectedDefinition()
	definition.Selector.Field = &FieldRef{Namespace: "source", Path: []string{"callerInvented"}}
	if _, err := preparer.PrepareWorkflow(ctx, definition); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("undeclared caller field accepted: %v", err)
	}
}

func TestWorkflowDefinitionPreparerFailsClosedWithoutDeclarationOwner(t *testing.T) {
	ctx := t.Context()
	host, err := NewWASMHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	preparer, err := NewWorkflowDefinitionPreparer(MissingSourceDeclarationOwner{}, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preparer.PrepareWorkflow(ctx, selectedDefinition()); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("missing trusted owner did not fail closed: %v", err)
	}
}

func TestMissingDeclarationOwnerDoesNotMaskCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	preparer, err := NewWorkflowDefinitionPreparer(MissingSourceDeclarationOwner{}, &WASMHost{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := preparer.CompileWorkflow(ctx, Definition{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation consulted missing owner: %v", err)
	}
}

func TestPreparationTrackerPreservesOptionalPreviewCapability(t *testing.T) {
	var mainOnly InstalledArtifacts = &preparationArtifactTracker{}
	if _, ok := mainOnly.(InstalledPreviewArtifacts); ok {
		t.Fatal("main-only tracker advertises an unavailable preview owner")
	}
	var preview InstalledArtifacts = preparationPreviewArtifactTracker{&preparationArtifactTracker{}}
	if _, ok := preview.(InstalledPreviewArtifacts); !ok {
		t.Fatal("preview tracker lost its dedicated preview capability")
	}
}
