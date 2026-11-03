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

// InstalledPreviewArtifacts resolves an independently pinned optional preview implementation.
// False explicitly means unconfigured; an invalid configured artifact returns an error.
type InstalledPreviewArtifacts interface {
	PrepareInstalledPreview(context.Context, uuid.UUID, int64) (*InstalledArtifact, bool, error)
}

func validatePreviewArtifact(main, preview *InstalledArtifact) error {
	if preview == nil || preview == main {
		return capabilityUnavailable(errors.New("dedicated preview artifact unavailable"))
	}
	primary, err := main.Manifest()
	if err != nil {
		return err
	}
	declaration, err := preview.Manifest()
	if err != nil {
		return err
	}
	if !declaration.Pure || !declaration.PurePreview || declaration.PluginID != primary.PluginID ||
		declaration.Version != primary.Version || declaration.CapabilityID != primary.CapabilityID ||
		!sameOutputSchema(primary.InputSchema, declaration.InputSchema) ||
		!sameOutputSchema(primary.ResultSchema, declaration.ResultSchema) ||
		!sameOutputSchema(map[string]FieldSchema{"config": primary.ConfigurationSchema},
			map[string]FieldSchema{"config": declaration.ConfigurationSchema}) {
		return capabilityUnavailable(errors.New("dedicated preview declaration differs from processing capability"))
	}
	left, err := json.Marshal(primary.ConfigurationPolicy)
	if err != nil {
		return invalid(err)
	}
	right, err := json.Marshal(declaration.ConfigurationPolicy)
	if err != nil {
		return invalid(err)
	}
	if !bytes.Equal(left, right) {
		return capabilityUnavailable(errors.New("dedicated preview configuration classification differs"))
	}
	return nil
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
	envelope, configurationDependencies, preview, err := frame.invocations.AuthorizeInstalled(ctx,
		step.definition.Installed.InstanceID, step.definition.Installed.ConfigurationRevision, call, detached)
	if err != nil {
		return nil, err
	}
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
	artifact := step.installed
	if preview {
		if step.preview == nil {
			return nil, capabilityUnavailable(errors.New("dedicated preview implementation unavailable"))
		}
		artifact = step.preview
	}
	output, err := artifact.invokeProjectedBudget(ctx, envelope, preview, frame.budget)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]FieldValue, len(manifest.ResultSchema))
	for _, name := range sortedNames(manifest.ResultSchema) {
		field := FieldValue{State: FieldAbsent, Dependencies: dependencies}
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
