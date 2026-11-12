package processing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Invocation modes carried in InvocationEnvelope.Mode.
const (
	// InvocationModeLive is an ordinary processing run; it is the default for an empty Mode.
	InvocationModeLive = "live"
	// InvocationModePreview is a dry run for an editor: the plugin must compute its result
	// without any effect outside its process. Only plugins whose manifest sets supportsPreview
	// are ever invoked in this mode.
	InvocationModePreview = "preview"
)

// InvocationEnvelope contains only current owner-projected input/configuration and pinned identities.
// It is not accepted from an HTTP caller as execution authority. Mode selects live or preview
// behaviour; the wire schemaVersion stays 2 because the field is additive and a plugin that does
// not know it is never sent a preview request (see InstalledManifest.SupportsPreview).
type InvocationEnvelope struct {
	SchemaVersion         int              `json:"schemaVersion"`
	InvocationID          uuid.UUID        `json:"invocationId"`
	InstanceID            uuid.UUID        `json:"instanceId"`
	ArtifactDigest        string           `json:"artifactDigest"`
	CapabilityID          string           `json:"capabilityId"`
	WorkflowID            uuid.UUID        `json:"workflowId"`
	DefinitionVersionID   uuid.UUID        `json:"definitionVersionId"`
	RunID                 uuid.UUID        `json:"runId"`
	ExecutionPrincipalID  uuid.UUID        `json:"executionPrincipalId"`
	ConfigurationRevision int64            `json:"configurationRevision,string"`
	Deadline              time.Time        `json:"deadline"`
	Input                 map[string]Value `json:"input"`
	Configuration         Value            `json:"configuration"`
	Mode                  string           `json:"mode"`
}

type invocationResponse struct {
	SchemaVersion int               `json:"schemaVersion"`
	Fields        *map[string]Value `json:"fields,omitempty"`
	Failure       string            `json:"failure,omitempty"`
}

func validateValues(schema map[string]FieldSchema, values map[string]Value) error {
	if len(values) > MaxCollection {
		return resource(errors.New("installed field collection boundary"))
	}
	for name, value := range values {
		declaration, exists := schema[name]
		if !exists {
			return invalid(errors.New("installed command returned an undeclared field"))
		}
		if _, err := value.Native(declaration); err != nil {
			return err
		}
	}
	for name, declaration := range schema {
		if _, exists := values[name]; !exists && !declaration.Optional {
			return invalid(errors.New("installed command required field is absent"))
		}
	}
	return nil
}

func hasSecretPolicy(policy ConfigurationPolicy) bool {
	if policy.Secret != nil && *policy.Secret {
		return true
	}
	for _, child := range policy.Properties {
		if hasSecretPolicy(child) {
			return true
		}
	}
	return policy.Element != nil && hasSecretPolicy(*policy.Element)
}

// invokeProjected is deliberately package-private: the current configuration/Principal/source
// participant must authorize and detach this envelope before the runtime is called.
func (a *InstalledArtifact) invokeProjected(ctx context.Context, envelope InvocationEnvelope) (map[string]Value, error) {
	return a.invokeProjectedBudget(ctx, envelope, nil)
}

// invokeProjectedBudget validates the authorized envelope against the pinned manifest and invokes the
// plugin. A preview envelope is refused for a plugin that does not declare supportsPreview, so a
// plugin that ignores envelope.mode can never run its live behaviour for an editor's dry run.
func (a *InstalledArtifact) invokeProjectedBudget(ctx context.Context, envelope InvocationEnvelope, budget *executionBudget) (map[string]Value, error) {
	if a == nil || a.host == nil || ctx == nil {
		return nil, invalid(errors.New("missing installed invocation participant"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if envelope.SchemaVersion != 2 || envelope.InvocationID == uuid.Nil || envelope.InstanceID == uuid.Nil || envelope.WorkflowID == uuid.Nil ||
		envelope.DefinitionVersionID == uuid.Nil || envelope.RunID == uuid.Nil || envelope.ExecutionPrincipalID == uuid.Nil ||
		envelope.ConfigurationRevision <= 0 || envelope.ArtifactDigest != a.manifest.Artifact.Digest ||
		envelope.CapabilityID != a.manifest.CapabilityID {
		return nil, invalid(errors.New("invalid or unpinned invocation identity"))
	}
	switch envelope.Mode {
	case "":
		envelope.Mode = InvocationModeLive
	case InvocationModeLive:
	case InvocationModePreview:
		if !a.manifest.SupportsPreview {
			return nil, capabilityUnavailable(errors.New("plugin does not support preview"))
		}
	default:
		return nil, invalid(errors.New("unknown invocation mode"))
	}
	if err := a.manifest.requireProcessing(); err != nil {
		return nil, err
	}
	if hasSecretPolicy(a.manifest.ConfigurationPolicy) {
		return nil, capabilityUnavailable(errors.New("secret resolution is not supplied by this adapter"))
	}
	if err := validateValues(a.manifest.InputSchema, envelope.Input); err != nil {
		return nil, err
	}
	if _, err := envelope.Configuration.Native(a.manifest.ConfigurationSchema); err != nil {
		return nil, err
	}
	parent := ctx
	if budget != nil {
		if err := budget.computeContext().Err(); err != nil {
			return nil, budget.bound(err)
		}
		parent = budget.parent
	}
	ctx, cancel := context.WithTimeoutCause(parent, installedInvocationTimeout, errInvocationDeadline)
	defer cancel()
	envelope.Deadline, _ = ctx.Deadline()
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return nil, invalid(err)
	}
	var response invocationResponse
	if err := a.call(ctx, encoded, &response, installedInvocationTimeout, budget); err != nil {
		return nil, err
	}
	if response.SchemaVersion != 2 || (response.Fields == nil) == (response.Failure == "") {
		return nil, invalid(errors.New("installed command response is not exactly success or failure"))
	}
	if response.Failure != "" {
		switch response.Failure {
		case "resource-limit":
			return nil, resource(errors.New("installed command reported resource limit"))
		case "capability-unavailable":
			return nil, capabilityUnavailable(errors.New("installed command reported unavailable capability"))
		case "conversion-failed", "evaluation-failed":
			return nil, invalid(errors.New("installed command reported failure"))
		default:
			return nil, invalid(errors.New("installed command returned an unknown failure reason"))
		}
	}
	if err := validateValues(a.manifest.ResultSchema, *response.Fields); err != nil {
		return nil, err
	}
	if budget != nil {
		if err := budget.computeContext().Err(); err != nil {
			return nil, budget.bound(err)
		}
	}
	return *response.Fields, nil
}
