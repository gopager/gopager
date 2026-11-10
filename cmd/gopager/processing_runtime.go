package main

import (
	"context"
	"errors"
	"os"

	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/httpapi"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/processing/synthetic"
	"github.com/gopager/gopager/internal/processing/worker"
)

type processingAPIRuntime struct {
	host            *processing.PluginHost
	root            *os.Root
	executions      *processing.InstanceExecutions
	routes          httpapi.ProcessingRoutes
	workerSource    worker.WorkSourceOwner
	workerPublisher worker.Publisher
	// correctionReadmission is the same source owner; it re-admits Processing for
	// Transmissions whose target resolution a correction changed.
	correctionReadmission assignments.ProcessingReadmission
}

// composeProcessingAPIRuntime builds the Processing management services, the plugin host and
// the work source on top of the shared core. A non-nil previewNetworks replaces the production
// Network owner for synthetic previews (tests only).
func composeProcessingAPIRuntime(ctx context.Context, c *core, artifactRoot string,
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
	if networks == nil {
		networks = c.intake
	}
	matcher, err := c.newRuleService(syntheticAssignmentNetworkOwner{owner: networks})
	if err != nil {
		return nil, err
	}
	store := c.store
	artifacts := &processingDefinitionArtifacts{host: host, root: root, store: store,
		repository: c.processing, resolver: c.resolver}
	preparer, err := processing.NewWorkflowDefinitionPreparer(processing.NewSyntheticDeclarationOwner(), processing.CoreRegex{}, artifacts)
	if err != nil {
		return nil, err
	}
	workflows, err := processingmanagement.New(store, c.resolver, store, c.processing,
		processingScopeContext{networks: syntheticAssignmentNetworkOwner{owner: networks}, units: c.units},
		preparer, c.receipts, c.facts)
	if err != nil {
		return nil, err
	}
	sourceOwner, err := synthetic.NewOwner(matcher, networks, store, workflows)
	if err != nil {
		return nil, err
	}
	reads, err := processingmanagement.NewProcessingReads(workflows, c.processing)
	if err != nil {
		return nil, err
	}
	inspector := processingmanagement.InstanceInspector(unavailableInstanceInspector{})
	executions, err = processing.NewInstanceExecutions(host, root, store, c.processing, c.resolver)
	if err != nil {
		return nil, err
	}
	workerSource, err := ingeststore.NewProcessingSourceOwner(c.intake, c.rules,
		c.processing, c.facts, store, artifacts, executions, c.resolver)
	if err != nil {
		return nil, err
	}
	retries, err := processingmanagement.NewRetry(workflows, c.processing, workerSource)
	if err != nil {
		return nil, err
	}
	if root != nil {
		inspector, err = processing.NewLocalInstanceInspector(host, root)
		if err != nil {
			return nil, err
		}
	}
	instances, err := processingmanagement.NewInstances(workflows, c.processing, inspector, c.facts, executions)
	if err != nil {
		return nil, err
	}
	preview, err := processingmanagement.NewPreview(workflows, preparer, executions, sourceOwner)
	if err != nil {
		return nil, err
	}
	return &processingAPIRuntime{host: host, root: root, executions: executions,
		workerSource: workerSource, workerPublisher: workerSource, correctionReadmission: workerSource,
		routes: httpapi.ProcessingRoutes{Workflows: workflows, Reads: reads, Retries: retries,
			Preview: preview, Instances: instances},
	}, nil
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

// unavailableInstanceInspector rejects instance inspection when no artifact root is provisioned.
type unavailableInstanceInspector struct{}

func (unavailableInstanceInspector) InspectInstance(context.Context, processing.InstanceConfiguration) error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("processing artifact root unavailable"))
}
