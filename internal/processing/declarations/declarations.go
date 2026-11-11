// Package declarations wires the field registry into Workflow compilation: it supplies the
// source declarations a definition reads and the Attribute declarations it writes.
package declarations

import (
	"context"
	"errors"
	"slices"

	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// For returns the declarations a Workflow is compiled against: source and target fields from the
// `workflow` scope, outputs from the `message` scope. With strict, deprecated fields are
// rejected, which is what saving, previewing and activating a version want. Evaluating an active
// version is not strict, so a deprecation never stops processing.
func For(registry *fields.Registry, strict bool) processing.DeclarationSet {
	return processing.DeclarationSet{
		Source:  registry.Declarations(fields.ScopeWorkflow, strict),
		Outputs: registry.Declarations(fields.ScopeMessage, strict),
	}
}

// FromSchemas returns the declarations of the built-in fields and the given Attribute schemas,
// for callers that have no catalog to read, such as tests and previews of a fixed catalog.
func FromSchemas(schemas []attributes.Schema, strict bool) processing.DeclarationSet {
	return For(fields.New(fields.Builtin(), attributes.NewProvider(schemas, 1)), strict)
}

// Builtin returns the declarations of the server-owned source and target fields only.
func Builtin() processing.Declarations {
	return fields.New(fields.Builtin()).Declarations(fields.ScopeWorkflow, false)
}

// BuiltinSourceRefs returns the references of the server-owned source fields, sorted.
func BuiltinSourceRefs() []processing.FieldRef {
	var refs []processing.FieldRef
	for _, field := range fields.New(fields.Builtin()).List(fields.ScopeWorkflow) {
		if field.Ref.Namespace == "source" {
			refs = append(refs, field.Ref)
		}
	}
	return slices.Clone(refs)
}

// Transactions runs a function in a read transaction.
type Transactions interface {
	WithinTransaction(ctx context.Context, fn platform.TxFunc) error
}

// Registries yields the field registry visible in a transaction, for example
// attributes.RegistrySource.
type Registries interface {
	Registry(ctx context.Context, tx platform.TransactionContext) (*fields.Registry, error)
}

// Owner implements processing.SourceDeclarationOwner. The definition preparer runs outside any
// source transaction, so the owner reads the catalog in a short transaction of its own; the
// registry is cached by catalog revision, so the cost is one revision lookup.
type Owner struct {
	transactions Transactions
	registries   Registries
	strict       bool
}

// NewOwner returns an Owner. With strict, deprecated fields are rejected.
func NewOwner(transactions Transactions, registries Registries, strict bool) (*Owner, error) {
	if transactions == nil || registries == nil {
		return nil, errors.New("declaration owner requires transactions and registries")
	}
	return &Owner{transactions: transactions, registries: registries, strict: strict}, nil
}

// ProcessingDeclarations implements processing.SourceDeclarationOwner.
func (o *Owner) ProcessingDeclarations(ctx context.Context) (processing.DeclarationSet, error) {
	var set processing.DeclarationSet
	err := o.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		registry, err := o.registries.Registry(tx.Context(), tx)
		if err != nil {
			return err
		}
		set = For(registry, o.strict)
		return nil
	})
	return set, err
}

var _ processing.SourceDeclarationOwner = (*Owner)(nil)
