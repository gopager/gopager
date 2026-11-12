package management

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// InstanceInspector checks exact provisioned bytes outside SQL without running initialization.
type InstanceInspector interface {
	InspectInstance(context.Context, processing.InstanceConfiguration) error
	// InspectManifest verifies a provisioned artifact and returns its manifest without starting the plugin.
	InspectManifest(context.Context, processing.ProvisionedArtifact) (processing.InstalledManifest, error)
}

// InstanceLifecycle receives only committed changes, so rolled-back writes do not cancel valid runs.
type InstanceLifecycle interface {
	InstanceChanged(uuid.UUID, int64)
}

// PluginStatusSource reports what this node knows about the plugin processes of an instance.
type PluginStatusSource interface {
	Status(uuid.UUID) processing.PluginStatus
}

// InstanceService coordinates fresh configuration authority, artifact inspection and immutable writes.
type InstanceService struct {
	management *Service
	repository processing.InstanceRepository
	inspector  InstanceInspector
	facts      events.ProcessingInstanceAppender
	lifecycle  InstanceLifecycle
	secrets    secretSettings
	status     PluginStatusSource
}

// InstanceOption configures optional collaborators of an InstanceService.
type InstanceOption func(*InstanceService)

// WithSecrets enables writing Secrets. cipher is nil when the operator configured no master key,
// which makes writing a Secret value unavailable; writer stores the sealed rows.
func WithSecrets(cipher *processing.SecretCipher, writer SecretWriter) InstanceOption {
	return func(s *InstanceService) { s.secrets = secretSettings{cipher: cipher, writer: writer} }
}

// WithStatus adds the plugin status of this node to the instance view.
func WithStatus(source PluginStatusSource) InstanceOption {
	return func(s *InstanceService) { s.status = source }
}

// NewInstances requires existing management/receipt infrastructure and the actual inspection participant.
func NewInstances(management *Service, repository processing.InstanceRepository,
	inspector InstanceInspector, facts events.ProcessingInstanceAppender, lifecycle InstanceLifecycle,
	options ...InstanceOption) (*InstanceService, error) {
	if management == nil || repository == nil || inspector == nil || facts == nil || lifecycle == nil {
		return nil, authz.ErrInvalidInput
	}
	service := &InstanceService{management: management, repository: repository, inspector: inspector, facts: facts,
		lifecycle: lifecycle}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

// InstanceInput excludes all server-owned identity/revision/audit fields. Secrets maps the dotted
// path of an administrative Secret property to its write; a path left out keeps the previous
// revision's Secret, null removes it. Secret properties never appear inside Configuration.
type InstanceInput struct {
	Enabled              *bool                          `json:"enabled"`
	ExecutionPrincipalID uuid.UUID                      `json:"executionPrincipalId"`
	Main                 processing.ProvisionedArtifact `json:"main"`
	Configuration        processing.Value               `json:"configuration"`
	TargetUse            *processing.TargetUse          `json:"targetUse,omitempty"`
	Secrets              map[string]*SecretInput        `json:"secrets,omitempty"`
}

// instanceFingerprint is what a save command is compared by. Secrets enter only as keyed digests
// or reference strings, never as plaintext.
type instanceFingerprint struct {
	ID                   uuid.UUID
	Expected             int64
	Enabled              bool
	ExecutionPrincipalID uuid.UUID
	Main                 processing.ProvisionedArtifact
	Configuration        processing.Value
	TargetUse            *processing.TargetUse
	Secrets              map[string]string
}

// instanceDraft is what the first transaction of a save establishes for the later steps.
type instanceDraft struct {
	configuration processing.InstanceConfiguration
	previous      *processing.InstanceConfiguration
	control       int64
}

// SaveInstance performs actor-bound creation or expected-revision update with two-phase reauthorization.
func (s *InstanceService) SaveInstance(ctx context.Context, proof identity.Proof, command, id uuid.UUID,
	expected int64, input InstanceInput, check func(identity.Authentication) error) (commands.Result, error) {
	if command == uuid.Nil || expected < 0 || (expected == 0) != (id == uuid.Nil) || input.Enabled == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return commands.Result{}, err
	}
	var detached InstanceInput
	if err := processing.Decode(encoded, &detached); err != nil {
		return commands.Result{}, err
	}
	secretDigests, err := secretFingerprints(s.secrets.cipher, detached.Secrets)
	if err != nil {
		return commands.Result{}, err
	}
	fingerprint, err := commands.Fingerprint(instanceFingerprint{ID: id, Expected: expected,
		Enabled: *detached.Enabled, ExecutionPrincipalID: detached.ExecutionPrincipalID, Main: detached.Main,
		Configuration: detached.Configuration, TargetUse: detached.TargetUse, Secrets: secretDigests})
	if err != nil {
		return commands.Result{}, err
	}
	const operation = "processing.instance.save"
	var draft instanceDraft
	var replay commands.Result
	preauthorize := func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return authz.ErrInvalidInput
		}
		receipt, exists, err := s.management.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if exists {
			replay, err = receipt.Check(operation, fingerprint)
			return err
		}
		if expected != 0 {
			previous, err := s.repository.ReadInstance(tx.Context(), tx, id, expected)
			if err != nil {
				return err
			}
			draft.previous = &previous
		}
		if _, err := s.management.resolver.ResolvePrincipalAuthority(tx.Context(), tx, detached.ExecutionPrincipalID); err != nil {
			return err
		}
		draft.control, err = s.management.repository.LockRevisionForChange(tx.Context(), tx)
		if err != nil {
			return err
		}
		revision := int64(1)
		if expected > 0 {
			revision, err = commands.NextRevision(expected, expected)
			if err != nil {
				return err
			}
		}
		instanceID := id
		if instanceID == uuid.Nil {
			instanceID = uuid.New()
		}
		draft.configuration = processing.InstanceConfiguration{ID: instanceID, Revision: revision,
			Enabled: *detached.Enabled, ExecutionPrincipalID: detached.ExecutionPrincipalID,
			Main: detached.Main, TargetUse: detached.TargetUse, CreatedAt: now, CreatedBy: actor}
		return nil
	}
	if err := s.management.within(ctx, proof, true, check, preauthorize); err != nil {
		return commands.Result{}, err
	}
	if replay.ID != uuid.Nil {
		return replay, nil
	}
	configuration := draft.configuration
	plan, err := s.completeConfiguration(ctx, &configuration, draft.previous, detached)
	if err != nil {
		return commands.Result{}, err
	}
	inspectionBytes, err := json.Marshal(configuration)
	if err != nil {
		return commands.Result{}, err
	}
	var inspection processing.InstanceConfiguration
	if err := processing.Decode(inspectionBytes, &inspection); err != nil {
		return commands.Result{}, err
	}
	if err := s.inspector.InspectInstance(ctx, inspection); err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	changed := false
	err = s.management.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
			return err
		}
		current, err := s.management.repository.LockRevisionForChange(tx.Context(), tx)
		if err != nil {
			return err
		}
		receipt, exists, err := s.management.receipts.ReadCommandReceipt(tx.Context(), tx, configuration.CreatedBy, command)
		if err != nil {
			return err
		}
		if exists {
			result, err = receipt.Check(operation, fingerprint)
			return err
		}
		if current != draft.control {
			return authz.ErrRevisionConflict
		}
		if _, err := s.management.resolver.ResolvePrincipalAuthority(tx.Context(), tx, configuration.ExecutionPrincipalID); err != nil {
			return err
		}
		configuration.CreatedAt = now
		if err := s.repository.SaveInstance(tx.Context(), tx, authority, configuration, expected); err != nil {
			return err
		}
		if err := s.storeSecrets(tx, plan.rows); err != nil {
			return err
		}
		changeKind := "updated"
		if expected == 0 {
			changeKind = "created"
		}
		if _, err := s.facts.AppendProcessingInstance(tx.Context(), tx, events.ProcessingInstanceFact{
			ID: uuid.New(), InstanceID: configuration.ID, Revision: configuration.Revision, OccurredAt: now,
			ChangeKind: changeKind,
		}); err != nil {
			return err
		}
		result = commands.Result{ID: configuration.ID, Revision: configuration.Revision}
		if err := s.management.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: configuration.CreatedBy, CommandID: command, Operation: operation,
			Fingerprint: fingerprint, Result: result, RecordedAt: now,
		}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	if changed {
		s.lifecycle.InstanceChanged(result.ID, result.Revision)
	}
	return result, nil
}

// storeSecrets writes the sealed rows of a change in the change's transaction.
func (s *InstanceService) storeSecrets(tx platform.TransactionContext, rows []processing.StoredSecret) error {
	if len(rows) == 0 {
		return nil
	}
	if s.secrets.writer == nil {
		return secretUnavailable("secret store is not configured")
	}
	return s.secrets.writer.InsertSecrets(tx.Context(), tx, rows)
}
