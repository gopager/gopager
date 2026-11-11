package main

import (
	"context"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
)

// builtinRegistry serves the built-in fields to the Message read side. It keeps one registry,
// because the built-in catalog never changes while the process runs.
type builtinRegistry struct{ registry *fields.Registry }

func newBuiltinRegistry() builtinRegistry {
	return builtinRegistry{registry: fields.New(fields.Builtin())}
}

// Registry implements readmodel.FieldRegistry.
func (r builtinRegistry) Registry(context.Context, platform.TransactionContext) (*fields.Registry, error) {
	return r.registry, nil
}
