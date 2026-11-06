package main

import (
	"context"
	"errors"
	"os"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// processingDefinitionArtifacts loads current instance declarations without invoking code.
// The definition preparer owns each returned compilation until validation has finished.
type processingDefinitionArtifacts struct {
	host       *processing.PluginHost
	root       *os.Root
	store      *postgres.Store
	repository *processingstore.Store
	resolver   *identity.Resolver
}

// PrepareInstalled validates the enabled instance, configuration and current execution Principal
// in its own transaction and then loads the artifact; callers that already hold a transaction
// validate the instance themselves and use LoadInstalled.
func (p *processingDefinitionArtifacts) PrepareInstalled(ctx context.Context, id uuid.UUID,
	revision int64) (*processing.InstalledArtifact, error) {
	if p.root == nil {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing artifact root unavailable"))
	}
	var configuration processing.InstanceConfiguration
	err := p.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		configuration, err = p.repository.ReadInstance(tx.Context(), tx, id, revision)
		if err != nil {
			return err
		}
		if !configuration.Enabled {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing instance disabled"))
		}
		_, err = p.resolver.ResolvePrincipalAuthority(tx.Context(), tx, configuration.ExecutionPrincipalID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p.LoadInstalled(ctx, configuration)
}

// LoadInstalled inspects and loads the main artifact of an already validated instance
// configuration without touching storage.
func (p *processingDefinitionArtifacts) LoadInstalled(ctx context.Context,
	configuration processing.InstanceConfiguration) (*processing.InstalledArtifact, error) {
	if p.root == nil {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing artifact root unavailable"))
	}
	inspector, err := processing.NewLocalInstanceInspector(p.host, p.root)
	if err != nil {
		return nil, err
	}
	if err := inspector.InspectInstance(ctx, configuration); err != nil {
		return nil, err
	}
	return p.host.LoadInstalled(ctx, p.root, configuration.Main.ManifestPath, configuration.Main.Pin)
}
