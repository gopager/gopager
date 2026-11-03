package management

import (
	"context"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

type preparationFixture struct {
	program  *processing.WorkflowProgram
	closed   int
	closeErr error
}

func (p *preparationFixture) PrepareWorkflow(context.Context, processing.Definition) (*processing.WorkflowProgram, error) {
	return p.program, nil
}

func (p *preparationFixture) ClosePreparedWorkflow(context.Context, *processing.WorkflowProgram) error {
	p.closed++
	return p.closeErr
}

type preparationDeclarations map[string]processing.FieldSchema

func (d preparationDeclarations) FieldSchema(ref processing.FieldRef) (processing.FieldSchema, error) {
	schema, exists := d[ref.Namespace+"\x00"+joinPath(ref.Path)]
	if !exists {
		return processing.FieldSchema{}, errors.New("undeclared test field")
	}
	return schema, nil
}

func joinPath(path []string) string {
	result := ""
	for _, part := range path {
		if result != "" {
			result += "\x00"
		}
		result += part
	}
	return result
}

func preparedDefinition(t *testing.T) processing.Definition {
	t.Helper()
	field := processing.FieldRef{Namespace: "source", Path: []string{"flag"}}
	value, err := processing.Literal(processing.BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	definition := processing.Definition{SchemaVersion: 1,
		Selector:     processing.Predicate{Op: "eq", Field: &field, Value: &value},
		OutputSchema: map[string]processing.FieldSchema{}, Outputs: map[string]processing.Assignment{}}
	program, err := processing.CompileWorkflow(t.Context(), definition,
		preparationDeclarations{"source\x00flag": {Kind: processing.BooleanValue}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := program.CheckedDefinition()
	if err != nil {
		t.Fatal(err)
	}
	return checked
}

func TestPrepareClosesTemporaryPreparedProgram(t *testing.T) {
	programDefinition := preparedDefinition(t)
	program, err := processing.CompileWorkflow(t.Context(), programDefinition,
		preparationDeclarations{"source\x00flag": {Kind: processing.BooleanValue}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := &preparationFixture{program: program}
	service := &Service{preparer: owner}
	if _, err := service.prepare(t.Context(), programDefinition); err != nil {
		t.Fatal(err)
	}
	if owner.closed != 1 {
		t.Fatalf("prepared artifact close calls = %d, want 1", owner.closed)
	}
}

func TestPrepareClosesProgramAfterSubstitutionRejection(t *testing.T) {
	programDefinition := preparedDefinition(t)
	program, err := processing.CompileWorkflow(t.Context(), programDefinition,
		preparationDeclarations{"source\x00flag": {Kind: processing.BooleanValue}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner := &preparationFixture{program: program}
	service := &Service{preparer: owner}
	input := programDefinition
	input.SchemaVersion = 2
	if _, err := service.prepare(t.Context(), input); err == nil {
		t.Fatal("substituted prepared definition accepted")
	}
	if owner.closed != 1 {
		t.Fatalf("prepared artifact close calls = %d, want 1", owner.closed)
	}
}

func TestPrepareReturnsTemporaryArtifactCloseFailure(t *testing.T) {
	programDefinition := preparedDefinition(t)
	program, err := processing.CompileWorkflow(t.Context(), programDefinition,
		preparationDeclarations{"source\x00flag": {Kind: processing.BooleanValue}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	closeFailure := errors.New("artifact close failed")
	owner := &preparationFixture{program: program, closeErr: closeFailure}
	service := &Service{preparer: owner}
	if _, err := service.prepare(t.Context(), programDefinition); !errors.Is(err, closeFailure) {
		t.Fatalf("close error not returned: %v", err)
	}
}
