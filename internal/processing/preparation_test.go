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

func (o fixedDeclarationOwner) ProcessingDeclarations(context.Context) (DeclarationSet, error) {
	return DeclarationSet{Source: o.declarations, Outputs: testOutputs}, o.err
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
	t.Parallel()
	ctx := t.Context()
	flag := FieldRef{Namespace: "source", Path: []string{"flag"}}
	preparer, err := NewWorkflowDefinitionPreparer(fixedDeclarationOwner{
		declarations: fixedDeclarations{flag.key(): {Kind: BooleanValue}},
	}, CoreRegex{}, nil)
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
	t.Parallel()
	ctx := t.Context()
	preparer, err := NewWorkflowDefinitionPreparer(MissingSourceDeclarationOwner{}, CoreRegex{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := preparer.PrepareWorkflow(ctx, selectedDefinition()); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("missing trusted owner did not fail closed: %v", err)
	}
}

func TestMissingDeclarationOwnerDoesNotMaskCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	preparer, err := NewWorkflowDefinitionPreparer(MissingSourceDeclarationOwner{}, CoreRegex{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := preparer.CompileWorkflow(ctx, Definition{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled preparation consulted missing owner: %v", err)
	}
}
