package main

import (
	"context"
	"net/http"
	"slices"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/readstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// workflowClassification answers readmodel.Classification from the stored Workflow definition
// versions: `derived` plus the field groups of every source field the version reads. It reads
// the immutable definition and the source declarations only, so the classification does not
// change when a plugin instance used by the version is disabled, updated or removed.
type workflowClassification struct {
	store        *postgres.Store
	reads        *readstore.Store
	declarations processing.SourceDeclarationOwner
}

// ClassificationGroups loads the immutable version in its own short transaction and collects its
// source references outside any transaction.
func (c workflowClassification) ClassificationGroups(ctx context.Context, version uuid.UUID) ([]string, error) {
	var stored []byte
	err := c.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		stored, err = c.reads.VersionDefinition(tx.Context(), tx, version)
		return err
	})
	if err != nil {
		return nil, err
	}
	var definition processing.Definition
	if err := processing.Decode(stored, &definition); err != nil {
		return nil, err
	}
	declarations, err := c.declarations.ProcessingDeclarations(ctx)
	if err != nil {
		return nil, err
	}
	references, err := processing.DefinitionSourceReferences(ctx, definition, declarations, processing.CoreRegex{})
	if err != nil {
		return nil, err
	}
	groups := []string{"derived"}
	for _, reference := range references {
		groups = append(groups, reference.Groups...)
	}
	slices.Sort(groups)
	return slices.Compact(groups), nil
}

// defaultClassification classifies with the trusted source declarations of the Processing owner.
func defaultClassification(c *core) (readmodel.Classification, error) {
	return workflowClassification{store: c.store, reads: c.reads,
		declarations: processing.NewSyntheticDeclarationOwner()}, nil
}

// readSide is the composed Message read service with its search admission.
type readSide struct {
	service   *readmodel.Service
	admission *readmodel.Admission
}

// composeReadSide returns the process's Message read side, composing it on first use.
func composeReadSide(c *core) (*readSide, error) {
	if c.readSide != nil {
		return c.readSide, nil
	}
	classification, err := defaultClassification(c)
	if err != nil {
		return nil, err
	}
	service, err := readmodel.NewService(c.store, c.resolver, c.store, c.reads, c.reads, classification)
	if err != nil {
		return nil, err
	}
	limit := c.searchAttempts
	if limit == 0 {
		limit = readmodel.DefaultSearchAttempts
	}
	admission, err := readmodel.NewAdmission(c.store, c.resolver, c.store, c.reads, limit)
	if err != nil {
		return nil, err
	}
	c.readSide = &readSide{service: service, admission: admission}
	return c.readSide, nil
}

// registerQueryRoutes composes the Message read side and binds its routes.
func registerQueryRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, authenticator httpapi.Authenticator, c *core) error {
	side, err := composeReadSide(c)
	if err != nil {
		return err
	}
	views, err := readmodel.NewSavedViews(c.store, c.resolver, c.store, c.reads, c.receipts)
	if err != nil {
		return err
	}
	httpapi.RegisterMessageRoutes(mux, transport, authenticator, side.service, side.admission)
	httpapi.RegisterSavedViewRoutes(mux, transport, authenticator, views)
	return nil
}
