package declarations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/declarations"
)

func catalog() []attributes.Schema {
	return []attributes.Schema{
		{Key: "alerts", Subject: attributes.SubjectMessage, Fields: map[string]attributes.Field{
			"level": {Kind: processing.StringValue},
			"old":   {Kind: processing.StringValue, Deprecated: true},
		}},
		{Key: "retired", Subject: attributes.SubjectMessage, Deprecated: true, Fields: map[string]attributes.Field{
			"level": {Kind: processing.StringValue},
		}},
		{Key: "unit-info", Subject: attributes.SubjectUnit, Fields: map[string]attributes.Field{
			"level": {Kind: processing.StringValue},
		}},
	}
}

func definition(schema, field string) processing.Definition {
	protocol := processing.FieldRef{Namespace: "source", Path: []string{"protocol"}}
	literal, _ := processing.Literal(processing.StringValue, "pocsag")
	return processing.Definition{SchemaVersion: 1,
		Selector: processing.Predicate{Op: "eq", Field: &protocol, Value: &literal},
		Outputs:  map[string]map[string]processing.Assignment{schema: {field: {Literal: &literal}}}}
}

func compile(set processing.DeclarationSet, definition processing.Definition) error {
	_, err := processing.CompileWorkflow(context.Background(), definition, set.Source, set.Outputs, nil)
	return err
}

func TestCompileRejectsUnknownAndUnitSchemasAndDeprecatedFieldsOnlyWhenStrict(t *testing.T) {
	t.Parallel()
	strict, lenient := declarations.FromSchemas(catalog(), true), declarations.FromSchemas(catalog(), false)
	if err := compile(strict, definition("alerts", "level")); err != nil {
		t.Fatalf("declared output rejected: %v", err)
	}
	for name, candidate := range map[string]processing.Definition{
		"unknown schema": definition("missing", "level"),
		"unknown field":  definition("alerts", "missing"),
		"unit schema":    definition("unit-info", "level"),
	} {
		for mode, set := range map[string]processing.DeclarationSet{"strict": strict, "lenient": lenient} {
			if err := compile(set, candidate); platform.FailureCode(err) != platform.CodeInvalidInput {
				t.Errorf("%s (%s): %v, want INVALID_INPUT", name, mode, err)
			}
		}
	}
	for name, candidate := range map[string]processing.Definition{
		"deprecated field":  definition("alerts", "old"),
		"deprecated schema": definition("retired", "level"),
	} {
		if err := compile(strict, candidate); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s accepted when saving: %v", name, err)
		}
		if err := compile(lenient, candidate); err != nil {
			t.Errorf("%s stopped the evaluation of an active version: %v", name, err)
		}
	}
}

type registries struct {
	registry *fields.Registry
	err      error
	calls    int
}

func (r *registries) Registry(context.Context, platform.TransactionContext) (*fields.Registry, error) {
	r.calls++
	return r.registry, r.err
}

type transactions struct{}

type transaction struct{ platform.TransactionContext }

func (transaction) Context() context.Context { return context.Background() }

func (transactions) WithinTransaction(_ context.Context, fn platform.TxFunc) error {
	return fn(transaction{})
}

func TestOwnerAppliesItsStrictness(t *testing.T) {
	t.Parallel()
	registry := fields.New(fields.Builtin(), attributes.NewProvider(catalog(), 1))
	deprecated := definition("alerts", "old")
	for strict, wantRejected := range map[bool]bool{true: true, false: false} {
		owner, err := declarations.NewOwner(transactions{}, &registries{registry: registry}, strict)
		if err != nil {
			t.Fatal(err)
		}
		set, err := owner.ProcessingDeclarations(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if rejected := compile(set, deprecated) != nil; rejected != wantRejected {
			t.Errorf("strict=%v: deprecated output rejected = %v, want %v", strict, rejected, wantRejected)
		}
	}
	failure := errors.New("catalog unavailable")
	owner, err := declarations.NewOwner(transactions{}, &registries{err: failure}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ProcessingDeclarations(context.Background()); !errors.Is(err, failure) {
		t.Errorf("catalog failure = %v", err)
	}
	if _, err := declarations.NewOwner(nil, nil, true); err == nil {
		t.Error("owner without dependencies created")
	}
}
