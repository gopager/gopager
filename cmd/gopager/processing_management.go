package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/assignmentstore"
	"github.com/gopager/gopager/internal/persistence/ent/commandstore"
	"github.com/gopager/gopager/internal/persistence/ent/eventstore"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/persistence/ent/unitstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
)

type processingAPIRoutes struct {
	workflows *processingmanagement.Service
	reads     *processingmanagement.ProcessingReads
	retries   *processingmanagement.RetryService
	preview   *processingmanagement.PreviewService
	instances *processingmanagement.InstanceService
}

type processingWorkflowAPI struct {
	transport *identity.HTTPTransport
	service   *processingmanagement.Service
	reads     *processingmanagement.ProcessingReads
	retries   *processingmanagement.RetryService
	preview   *processingmanagement.PreviewService
	instances *processingmanagement.InstanceService
}

type workflowPreviewMutation struct {
	SchemaVersion        int                    `json:"schemaVersion"`
	VersionID            uuid.UUID              `json:"versionId"`
	Definition           *processing.Definition `json:"definition"`
	SyntheticJSON        json.RawMessage        `json:"syntheticJson"`
	StoredTransmissionID uuid.UUID              `json:"storedTransmissionId"`
}

type workflowRetryMutation struct {
	SchemaVersion    int   `json:"schemaVersion"`
	ExpectedRevision int64 `json:"expectedRevision,string"`
}

type workflowRetryResponse struct {
	CandidateID uuid.UUID `json:"candidateId"`
	Revision    int64     `json:"revision,string"`
}

type instanceMutation struct {
	SchemaVersion    int       `json:"schemaVersion"`
	CommandID        uuid.UUID `json:"commandId"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	processingmanagement.InstanceInput
}

type workflowMutation struct {
	SchemaVersion    int         `json:"schemaVersion"`
	CommandID        uuid.UUID   `json:"commandId"`
	ExpectedRevision int64       `json:"expectedRevision,string"`
	Name             string      `json:"name"`
	Scope            authz.Scope `json:"scope"`
	Enabled          *bool       `json:"enabled"`
}

type workflowVersionMutation struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	CommandID        uuid.UUID             `json:"commandId"`
	ExpectedRevision int64                 `json:"expectedRevision,string"`
	Definition       processing.Definition `json:"definition"`
}

// decodeProcessingJSON uses the kernel's closed codec, including duplicate-key and size checks.
func decodeProcessingJSON(r *http.Request, input any) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return authz.ErrInvalidInput
	}
	raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, processing.MaxInputBytes))
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			return platform.NewFailure(platform.CodeResourceLimit, err)
		}
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	return processing.Decode(raw, input)
}

func (input workflowMutation) metadata() processingmanagement.MetadataInput {
	return processingmanagement.MetadataInput{Name: input.Name, Scope: input.Scope, Enabled: input.Enabled}
}

func registerProcessingRoutes(mux *http.ServeMux, transport *identity.HTTPTransport, routes processingAPIRoutes) {
	api := processingWorkflowAPI{transport: transport, service: routes.workflows,
		reads: routes.reads, retries: routes.retries, preview: routes.preview, instances: routes.instances}
	if api.service != nil {
		api.registerWorkflows(mux)
		api.registerVersions(mux)
	}
	if api.reads != nil {
		api.registerReads(mux)
	}
	if api.retries != nil {
		api.registerRetry(mux)
	}
	if api.preview != nil {
		api.registerPreview(mux)
	}
	if api.instances != nil {
		api.registerInstances(mux)
	}
}

func (api processingWorkflowAPI) registerInstances(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/instances", api.route(func(r *http.Request, proof identity.Proof,
		_ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.instances.ListInstances(r.Context(), proof,
			processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/instances/{id}", api.route(func(r *http.Request, proof identity.Proof,
		_ func(identity.Authentication) error) (any, error) {
		id, err := managementPathID(r, "id")
		if err != nil {
			return nil, err
		}
		return api.instances.ReadInstance(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/instances", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		var input instanceMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.instances.SaveInstance(r.Context(), proof, input.CommandID, uuid.Nil, 0,
			input.InstanceInput, check)
	}, http.StatusCreated))
	mux.HandleFunc("PUT /api/v1/instances/{id}", api.route(func(r *http.Request, proof identity.Proof,
		check func(identity.Authentication) error) (any, error) {
		var input instanceMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementPathID(r, "id")
		if err != nil || input.SchemaVersion != 1 || input.ExpectedRevision <= 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.instances.SaveInstance(r.Context(), proof, input.CommandID, id, input.ExpectedRevision,
			input.InstanceInput, check)
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerReads(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workflows/{id}/candidates", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.reads.ListCandidateStatuses(r.Context(), proof, workflow,
			processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/candidates/{candidate}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		candidate, err := managementPathID(r, "candidate")
		if err != nil {
			return nil, err
		}
		return api.reads.ReadCandidateStatus(r.Context(), proof, workflow, candidate)
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/candidates/{candidate}/result", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		workflow, err := managementID(r)
		if err != nil {
			return nil, err
		}
		candidate, err := managementPathID(r, "candidate")
		if err != nil {
			return nil, err
		}
		return api.reads.ReadPrivateResultMetadata(r.Context(), proof, workflow, candidate)
	}, http.StatusOK))
}

func (api processingWorkflowAPI) route(operation managementOperation, status int) http.HandlerFunc {
	return (managementAPI{transport: api.transport}).route(operation, status)
}

func managementPathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, authz.ErrInvalidInput
	}
	return id, nil
}

func (api processingWorkflowAPI) registerWorkflows(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workflows", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListWorkflows(r.Context(), proof, processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.ReadWorkflow(r.Context(), proof, id)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/workflows", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 || input.ExpectedRevision != 0 {
			return nil, authz.ErrInvalidInput
		}
		return api.service.CreateWorkflow(r.Context(), proof, input.CommandID, input.metadata(), check)
	}, http.StatusCreated))
	mux.HandleFunc("PUT /api/v1/workflows/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		id, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 || input.CommandID != uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.service.UpdateWorkflow(r.Context(), proof, id, input.ExpectedRevision, input.metadata(), check)
	}, http.StatusOK))
	mux.HandleFunc("DELETE /api/v1/workflows/{id}", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		if input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		id, err := managementID(r)
		if err != nil {
			return nil, err
		}
		return api.service.DeleteWorkflow(r.Context(), proof, id, input.ExpectedRevision, check)
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerVersions(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		parent, err := managementID(r)
		if err != nil {
			return nil, err
		}
		page, err := managementPage(r)
		if err != nil {
			return nil, err
		}
		return api.service.ListVersions(r.Context(), proof, parent, processing.PageInput{Offset: page.Offset, Limit: page.Limit})
	}, http.StatusOK))
	mux.HandleFunc("GET /api/v1/workflows/{id}/versions/{version}", api.route(func(r *http.Request, proof identity.Proof, _ func(identity.Authentication) error) (any, error) {
		parent, err := managementID(r)
		if err != nil {
			return nil, err
		}
		version, err := uuid.Parse(r.PathValue("version"))
		if err != nil || version == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.service.ReadVersion(r.Context(), proof, parent, version)
	}, http.StatusOK))
	mux.HandleFunc("POST /api/v1/workflows/{id}/versions", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowVersionMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		parent, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.service.SaveVersion(r.Context(), proof, input.CommandID, parent,
			input.ExpectedRevision, input.Definition, check)
	}, http.StatusCreated))
	mux.HandleFunc("POST /api/v1/workflows/{id}/versions/{version}/activate", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input deleteMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		parent, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		version, err := uuid.Parse(r.PathValue("version"))
		if err != nil || version == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		return api.service.ActivateVersion(r.Context(), proof, parent, version, input.ExpectedRevision, check)
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerRetry(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflows/{id}/candidates/{candidate}/retry", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowRetryMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		parent, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		candidate, err := uuid.Parse(r.PathValue("candidate"))
		if err != nil || candidate == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		retried, err := api.retries.RetryFailed(r.Context(), proof, parent, candidate, input.ExpectedRevision, check)
		if err != nil {
			return nil, err
		}
		return workflowRetryResponse{CandidateID: retried.ID, Revision: retried.Revision}, nil
	}, http.StatusOK))
}

func (api processingWorkflowAPI) registerPreview(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/workflows/{id}/preview", api.route(func(r *http.Request, proof identity.Proof, check func(identity.Authentication) error) (any, error) {
		var input workflowPreviewMutation
		if err := decodeProcessingJSON(r, &input); err != nil {
			return nil, err
		}
		workflow, err := managementID(r)
		if err != nil || input.SchemaVersion != 1 {
			return nil, authz.ErrInvalidInput
		}
		return api.preview.Preview(r.Context(), proof, processingmanagement.PreviewInput{
			WorkflowID: workflow, VersionID: input.VersionID, Definition: input.Definition,
			SyntheticJSON: input.SyntheticJSON, StoredTransmissionID: input.StoredTransmissionID,
		}, check)
	}, http.StatusOK))
}

type processingAPIRuntime struct {
	host            *processing.PluginHost
	root            *os.Root
	executions      *processing.InstanceExecutions
	routes          processingAPIRoutes
	workerSource    worker.WorkSourceOwner
	workerPublisher worker.Publisher
	// correctionReadmission is the same source owner; it re-admits Processing for
	// Transmissions whose target resolution a correction changed.
	correctionReadmission assignments.ProcessingReadmission
}

func composeProcessingAPIRuntime(ctx context.Context, store *postgres.Store, artifactRoot string,
	previewNetworks ...synthetic.NetworkOwner) (_ *processingAPIRuntime, result error) {
	if len(previewNetworks) > 1 {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("multiple Processing preview Network owners"))
	}
	var networks synthetic.NetworkOwner
	if len(previewNetworks) == 1 && previewNetworks[0] != nil {
		networks = previewNetworks[0]
	}
	ctx, cancel := context.WithTimeout(ctx, platform.DefaultCallTimeout)
	defer cancel()
	host := processing.NewPluginHost()
	var root *os.Root
	if artifactRoot != "" {
		var err error
		root, err = os.OpenRoot(artifactRoot)
		if err != nil {
			return nil, errors.Join(
				platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Processing artifact root")),
				host.Close(context.WithoutCancel(ctx)),
			)
		}
	}
	var executions *processing.InstanceExecutions
	defer func() {
		if result != nil {
			if executions != nil {
				result = errors.Join(result, executions.Close(context.WithoutCancel(ctx)))
			}
			result = errors.Join(result, host.Close(context.WithoutCancel(ctx)))
			if root != nil {
				result = errors.Join(result, root.Close())
			}
		}
	}()
	identityRepository, err := identitystore.New(store)
	if err != nil {
		return nil, err
	}
	unitRepository, err := unitstore.New(store)
	if err != nil {
		return nil, err
	}
	assignmentRepository, err := assignmentstore.New(store)
	if err != nil {
		return nil, err
	}
	receipts, err := commandstore.New(store)
	if err != nil {
		return nil, err
	}
	facts, err := eventstore.New(store)
	if err != nil {
		return nil, err
	}
	intakeRepository, err := ingeststore.New(store, facts, facts)
	if err != nil {
		return nil, err
	}
	if networks == nil {
		networks = intakeRepository
	}
	repository, err := processingstore.New(store)
	if err != nil {
		return nil, err
	}
	owners := applicationScopeOwners{identity: identityRepository, units: unitRepository}
	resolver, err := identity.NewResolver(identityRepository, store, owners)
	if err != nil {
		return nil, err
	}
	matcher, err := assignments.NewService(store, resolver, store, assignmentRepository, unitRepository,
		syntheticAssignmentNetworkOwner{owner: networks}, receipts, facts, facts)
	if err != nil {
		return nil, err
	}
	artifacts := &processingDefinitionArtifacts{host: host, root: root, store: store,
		repository: repository, resolver: resolver}
	preparer, err := processing.NewWorkflowDefinitionPreparer(processing.NewSyntheticDeclarationOwner(), processing.CoreRegex{}, artifacts)
	if err != nil {
		return nil, err
	}
	workflows, err := processingmanagement.New(store, resolver, store, repository,
		processingScopeContext{networks: syntheticAssignmentNetworkOwner{owner: networks}, units: unitRepository},
		preparer, receipts, facts)
	if err != nil {
		return nil, err
	}
	sourceOwner, err := synthetic.NewOwner(matcher, networks, store, workflows)
	if err != nil {
		return nil, err
	}
	reads, err := processingmanagement.NewProcessingReads(workflows, repository)
	if err != nil {
		return nil, err
	}
	inspector := processingmanagement.InstanceInspector(unavailableInstanceInspector{})
	executions, err = processing.NewInstanceExecutions(host, root, store, repository, resolver)
	if err != nil {
		return nil, err
	}
	sourceMatcher, err := assignments.NewService(store, resolver, store, assignmentRepository,
		unitRepository, intakeRepository, receipts, facts, facts)
	if err != nil {
		return nil, err
	}
	workerSource, err := ingeststore.NewProcessingSourceOwner(intakeRepository, sourceMatcher,
		repository, facts, store, artifacts, executions, resolver)
	if err != nil {
		return nil, err
	}
	retries, err := processingmanagement.NewRetry(workflows, repository, workerSource)
	if err != nil {
		return nil, err
	}
	if root != nil {
		inspector, err = processing.NewLocalInstanceInspector(host, root)
		if err != nil {
			return nil, err
		}
	}
	instances, err := processingmanagement.NewInstances(workflows, repository, inspector, facts, executions)
	if err != nil {
		return nil, err
	}
	preview, err := processingmanagement.NewPreview(workflows, preparer, executions, sourceOwner)
	if err != nil {
		return nil, err
	}
	return &processingAPIRuntime{host: host, root: root, executions: executions,
		workerSource: workerSource, workerPublisher: workerSource, correctionReadmission: workerSource,
		routes: processingAPIRoutes{workflows: workflows, reads: reads, retries: retries,
			preview: preview, instances: instances},
	}, nil
}

// processingScopeContext resolves Workflow scope references to their Agencies through the Network
// owner in use (production intake or a synthetic preview owner) and the master-data store.
type processingScopeContext struct {
	networks assignments.NetworkOwner
	units    *unitstore.Store
}

// ReadNetwork reads the Radio network's Protocol and Agency.
func (c processingScopeContext) ReadNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	return c.networks.ReadNetwork(ctx, tx, id)
}

// ReadUnitContext reads the Unit's current Agency, collections and deletion state.
func (c processingScopeContext) ReadUnitContext(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (units.Context, error) {
	return c.units.ReadUnitContext(ctx, tx, id)
}

type syntheticAssignmentNetworkOwner struct {
	owner synthetic.NetworkOwner
}

// ReadNetwork reads the synthetic Network's Protocol and Agency for matching and authorization.
func (owner syntheticAssignmentNetworkOwner) ReadNetwork(ctx context.Context,
	tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	if owner.owner == nil {
		return "", uuid.Nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Network owner is not wired"))
	}
	network, err := owner.owner.ReadSyntheticNetwork(ctx, tx, id)
	if err != nil {
		return "", uuid.Nil, err
	}
	if network.ID != id || network.Deleted || !network.Protocol.Valid() {
		return "", uuid.Nil, authz.ErrNotFound
	}
	return network.Protocol, network.AgencyID, nil
}

func (runtime *processingAPIRuntime) Close(ctx context.Context) error {
	if ctx == nil {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing runtime close context"))
	}
	var failure error
	if runtime.executions != nil {
		executionErr := runtime.executions.Close(ctx)
		failure = errors.Join(failure, executionErr)
		if errors.Is(executionErr, context.Canceled) || errors.Is(executionErr, context.DeadlineExceeded) {
			return failure
		}
	}
	failure = errors.Join(failure, runtime.host.Close(context.WithoutCancel(ctx)))
	if runtime.root != nil {
		failure = errors.Join(failure, runtime.root.Close())
	}
	return failure
}

type unavailableInstanceInspector struct{}

func (unavailableInstanceInspector) InspectInstance(context.Context, processing.InstanceConfiguration) error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Processing artifact root unavailable"))
}

type unavailableInstanceLifecycle struct{}

func (unavailableInstanceLifecycle) InstanceChanged(uuid.UUID, int64) {}
