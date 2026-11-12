package attributes

import (
	"context"
	"errors"
	"sync"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
)

// RegistrySource builds the field registry of the built-in fields and the current Attribute
// schemas. Building the attribute provider resolves every addressable path, so the registry is
// cached and rebuilt only when the catalog revision changes. It is safe for concurrent use.
type RegistrySource struct {
	catalog CatalogSource

	mu       sync.Mutex
	revision int64
	registry *fields.Registry
}

// NewRegistrySource returns a RegistrySource reading schemas from catalog.
func NewRegistrySource(catalog CatalogSource) (*RegistrySource, error) {
	if catalog == nil {
		return nil, errors.New("attribute catalog is required")
	}
	return &RegistrySource{catalog: catalog}, nil
}

// LockCatalog locks the catalog for the rest of the write transaction tx. A write that saves
// references to schemas calls it before reading the registry it validates them against, so that
// a concurrent schema deletion cannot pass its reference check before the write commits. Reads
// must not call it: the lock fails in read-only transactions and would only add contention.
func (s *RegistrySource) LockCatalog(ctx context.Context, tx platform.TransactionContext) error {
	return s.catalog.LockCatalog(ctx, tx)
}

// Registry returns the registry for the catalog state visible in tx. A registry built from a
// newer revision than the cached one replaces the cache; an older one (a concurrent reader on an
// earlier snapshot) is returned without replacing it.
func (s *RegistrySource) Registry(ctx context.Context, tx platform.TransactionContext) (*fields.Registry, error) {
	revision, err := s.catalog.CatalogRevision(ctx, tx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.registry != nil && s.revision == revision {
		registry := s.registry
		s.mu.Unlock()
		return registry, nil
	}
	s.mu.Unlock()
	schemas, revision, err := s.catalog.Catalog(ctx, tx)
	if err != nil {
		return nil, err
	}
	registry := fields.New(fields.Builtin(), NewProvider(schemas, revision))
	s.mu.Lock()
	if revision >= s.revision {
		s.revision, s.registry = revision, registry
	}
	s.mu.Unlock()
	return registry, nil
}
