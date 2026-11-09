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

// workflowPreparer compiles a stored definition so its source references can be read.
type workflowPreparer interface {
	PrepareWorkflow(context.Context, processing.Definition) (*processing.WorkflowProgram, error)
	ClosePreparedWorkflow(context.Context, *processing.WorkflowProgram) error
}

// workflowClassification answers readmodel.Classification from the stored Workflow definition
// versions: `derived` plus the field groups of every source field the version reads.
type workflowClassification struct {
	store    *postgres.Store
	reads    *readstore.Store
	preparer workflowPreparer
}

// ClassificationGroups loads the immutable version in its own short transaction and compiles it
// outside any transaction, as Workflow preparation expects.
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
	program, err := c.preparer.PrepareWorkflow(ctx, definition)
	if err != nil {
		return nil, err
	}
	groups := []string{"derived"}
	for _, reference := range program.SourceReferences() {
		groups = append(groups, reference.Groups...)
	}
	if err := c.preparer.ClosePreparedWorkflow(ctx, program); err != nil {
		return nil, err
	}
	slices.Sort(groups)
	return slices.Compact(groups), nil
}

// defaultClassification compiles definitions without installed plugin artifacts. Definitions
// that need one stay unclassified, which hides their results, until the Processing runtime
// supplies its own preparer.
func defaultClassification(c *core) (readmodel.Classification, error) {
	preparer, err := processing.NewWorkflowDefinitionPreparer(processing.NewSyntheticDeclarationOwner(), processing.CoreRegex{}, nil)
	if err != nil {
		return nil, err
	}
	return workflowClassification{store: c.store, reads: c.reads, preparer: preparer}, nil
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
	classification := c.classification
	if classification == nil {
		var err error
		if classification, err = defaultClassification(c); err != nil {
			return nil, err
		}
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
