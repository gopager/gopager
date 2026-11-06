package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

// InstalledStep references separately revisioned administrative configuration, not caller authority.
type InstalledStep struct {
	InstanceID            uuid.UUID             `json:"instanceId"`
	ConfigurationRevision int64                 `json:"configurationRevision,string"`
	Input                 map[string]Assignment `json:"input"`
}

// InstalledArtifacts prepares the exact current instance artifact outside source transactions.
// The owner retains artifact lifetime until dependent programs and invocations have drained.
type InstalledArtifacts interface {
	PrepareInstalled(context.Context, uuid.UUID, int64) (*InstalledArtifact, error)
}

// InstalledArtifactLoader loads the artifact of an instance configuration the caller has already
// read and validated in its own primary transaction. It touches no storage, so a source owner
// can call it inside that transaction without nesting another one.
type InstalledArtifactLoader interface {
	LoadInstalled(context.Context, InstanceConfiguration) (*InstalledArtifact, error)
}

// InstalledArtifactOwner serves both the worker's out-of-transaction preparation and the
// source owner's in-transaction validation of installed steps.
type InstalledArtifactOwner interface {
	InstalledArtifacts
	InstalledArtifactLoader
}

// InvocationContext binds a call to the actual captured source and optional original body target.
// It is host metadata, never a caller-selected public input or guest capability.
type InvocationContext struct {
	Source         SourceIdentity
	TargetPosition *uint32
}

// InstalledInvocationOwner rechecks current configuration, Principal and captured source under
// the shared primary lock, closes that transaction, then returns detached authorized input.
// It also enforces preview's separate editor permission and supplies current configuration dependencies.
type InstalledInvocationOwner interface {
	AuthorizeInstalled(context.Context, uuid.UUID, int64, InvocationContext, map[string]FieldValue) (InvocationEnvelope, []Dependency, bool, error)
}

func sameOutputSchema(left, right map[string]FieldSchema) bool {
	l, err := json.Marshal(left)
	if err != nil {
		return false
	}
	r, err := json.Marshal(right)
	return err == nil && bytes.Equal(l, r)
}

func executeInstalled(ctx context.Context, step compiledStep, frame *executionFrame) (map[string]FieldValue, error) {
	if frame.invocations == nil || step.installed == nil {
		return nil, capabilityUnavailable(errors.New("current installed invocation owner unavailable"))
	}
	manifest, err := step.installed.Manifest()
	if err != nil {
		return nil, err
	}
	input, err := evaluateMappings(ctx, manifest.InputSchema, step.definition.Installed.Input, frame)
	if err != nil {
		return nil, err
	}
	// The plugin is authorized for the whole definition's classification, not per input value:
	// derived values carry no classification of their own.
	for name, field := range input {
		field.Dependencies = frame.dependencies
		input[name] = field
	}
	if err := frame.budget.consume(input); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, invalid(err)
	}
	var detached map[string]FieldValue
	if err := Decode(encoded, &detached); err != nil {
		return nil, err
	}
	call := frame.invocation
	if call.TargetPosition != nil {
		position := *call.TargetPosition
		call.TargetPosition = &position
	}
	// Authorization is a database transaction, not built-in compute: its latency under load must
	// not be charged to the compute allowance, whose expiry would surface as STORAGE_UNAVAILABLE.
	var envelope InvocationEnvelope
	var configurationDependencies []Dependency
	var preview bool
	if err := frame.budget.guestWait(func(parent context.Context) (err error) {
		envelope, configurationDependencies, preview, err = frame.invocations.AuthorizeInstalled(parent,
			step.definition.Installed.InstanceID, step.definition.Installed.ConfigurationRevision, call, detached)
		return err
	}); err != nil {
		return nil, err
	}
	ctx = frame.budget.computeContext() // the pause replaced the allowance's context
	if envelope.ConfigurationRevision != step.definition.Installed.ConfigurationRevision {
		return nil, capabilityUnavailable(errors.New("installed configuration revision changed"))
	}
	expected := make(map[string]Value)
	for name, field := range input {
		if field.State == FieldPresent {
			expected[name] = *field.Value
		}
	}
	want, err := json.Marshal(expected)
	if err != nil {
		return nil, invalid(err)
	}
	got, err := json.Marshal(envelope.Input)
	if err != nil {
		return nil, invalid(err)
	}
	if !bytes.Equal(want, got) {
		return nil, capabilityUnavailable(errors.New("authorized installed projection differs from captured input"))
	}
	dependencies, err := UnionDependencies(frame.dependencies, configurationDependencies)
	if err != nil {
		return nil, err
	}
	if err := frame.budget.consume(envelope); err != nil {
		return nil, err
	}
	mode := InvocationModeLive
	if preview {
		mode = InvocationModePreview
	}
	if envelope.Mode != "" && envelope.Mode != mode {
		return nil, capabilityUnavailable(errors.New("authorized installed mode differs from the run"))
	}
	envelope.Mode = mode
	output, err := step.installed.invokeProjectedBudget(ctx, envelope, frame.budget)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]FieldValue, len(manifest.ResultSchema))
	for _, name := range sortedNames(manifest.ResultSchema) {
		field := FieldValue{State: FieldAbsent}
		if value, present := output[name]; present {
			field.State, field.Value = FieldPresent, &value
		}
		if err := validateFieldValue(manifest.ResultSchema[name], field, false); err != nil {
			return nil, err
		}
		fields[name] = field
	}
	frame.dependencies = dependencies
	return fields, nil
}
