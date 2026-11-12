package main

import (
	"context"

	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/processing"
)

// Configuration authorities a described target reports.
const (
	authorityAdmin            = "admin"
	authorityAdminAndPersonal = "admin-and-personal"
)

// deliveryPluginHost is the part of the plugin host's Delivery adapter that serves targets.
type deliveryPluginHost interface {
	Describe(ctx context.Context, instance processing.InstanceConfiguration) (processing.DeliveryDescription, error)
	Attempt(ctx context.Context, request processing.DeliveryAttemptRequest) (processing.DeliveryAttemptOutcome, error)
}

// pluginDeliveryTargets serves the Delivery Core's target port from the plugin host, so every
// configured Delivery plugin instance is a selectable target.
type pluginDeliveryTargets struct {
	host deliveryPluginHost
}

var _ delivery.Targets = pluginDeliveryTargets{}

// Describe reports what the instance offers as a target at its current revision. The host's
// failure is returned unchanged; the Core treats any failure as an unavailable target.
func (t pluginDeliveryTargets) Describe(ctx context.Context, instance processing.InstanceConfiguration) (delivery.TargetDescriptor, error) {
	description, err := t.host.Describe(ctx, instance)
	if err != nil {
		return delivery.TargetDescriptor{}, err
	}
	authority := authorityAdmin
	if description.Personal {
		authority = authorityAdminAndPersonal
	}
	inputs := make([]delivery.InputField, 0, len(description.Inputs))
	for _, input := range description.Inputs {
		inputs = append(inputs, delivery.InputField{Name: input.Name, Kind: input.Kind, Required: input.Required, MaxBytes: input.MaxBytes})
	}
	dependencies := make([]delivery.TargetDependency, 0, len(description.Dependencies))
	for _, dependency := range description.Dependencies {
		dependencies = append(dependencies, delivery.TargetDependency{Kind: dependency.Kind, Description: dependency.Description})
	}
	return delivery.TargetDescriptor{Version: delivery.DescriptorVersion, InstanceID: instance.ID,
		InstanceRevision: instance.Revision, CapabilityKind: delivery.CapabilityDelivery,
		ConfigurationAuthority: authority, OutputKinds: description.OutputKinds, Inputs: inputs,
		Dependencies: dependencies, SendsPerMinute: description.SendsPerMinute, Idempotent: description.Idempotent}, nil
}

// Attempt performs one provider call. An error means the effect may have happened; it is returned
// unchanged so the Core records the attempt as uncertain.
func (t pluginDeliveryTargets) Attempt(ctx context.Context, invocation delivery.AttemptInvocation) (delivery.AttemptResult, error) {
	outcome, err := t.host.Attempt(ctx, processing.DeliveryAttemptRequest{InstanceID: invocation.InstanceID,
		InstanceRevision: invocation.InstanceRevision, DeliveryID: invocation.DeliveryID, AttemptID: invocation.AttemptID,
		AttemptNumber: invocation.AttemptNumber, SubscriberID: invocation.SubscriberID,
		CorrelationID: invocation.CorrelationID, CausationID: invocation.CausationID,
		OutputKind: invocation.Input.OutputKind, Body: invocation.Input.Body, Inputs: invocation.Input.Inputs,
		Deadline: invocation.Deadline})
	if err != nil {
		return delivery.AttemptResult{}, err
	}
	return delivery.AttemptResult{Status: delivery.AttemptStatus(outcome.Status), Code: outcome.Code,
		RetryAfter: outcome.RetryAfter, Receipt: outcome.Receipt}, nil
}
