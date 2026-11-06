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
}

// InstanceLifecycle receives only committed changes, so rolled-back writes do not cancel valid runs.
type InstanceLifecycle interface {
	InstanceChanged(uuid.UUID, int64)
}

// InstanceService coordinates fresh configuration authority, artifact inspection and immutable writes.
type InstanceService struct {
	management *Service
	repository processing.InstanceRepository
	inspector  InstanceInspector
	facts      events.ProcessingInstanceAppender
	lifecycle  InstanceLifecycle
}

// NewInstances requires existing management/receipt infrastructure and the actual inspection participant.
func NewInstances(management *Service, repository processing.InstanceRepository,
	inspector InstanceInspector, facts events.ProcessingInstanceAppender, lifecycle InstanceLifecycle) (*InstanceService, error) {
	if management == nil || repository == nil || inspector == nil || facts == nil || lifecycle == nil {
		return nil, authz.ErrInvalidInput
	}
	return &InstanceService{management: management, repository: repository, inspector: inspector, facts: facts, lifecycle: lifecycle}, nil
}

// InstanceInput excludes all server-owned identity/revision/audit fields.
type InstanceInput struct {
	Enabled              *bool                          `json:"enabled"`
	ExecutionPrincipalID uuid.UUID                      `json:"executionPrincipalId"`
	Main                 processing.ProvisionedArtifact `json:"main"`
	Configuration        processing.Value               `json:"configuration"`
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
	fingerprint, err := commands.Fingerprint(struct {
		ID       uuid.UUID
		Expected int64
		Input    InstanceInput
	}{id, expected, detached})
	if err != nil {
		return commands.Result{}, err
	}
	const operation = "processing.instance.save"
	var configuration processing.InstanceConfiguration
	var replay commands.Result
	var control int64
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
			if _, err := s.repository.ReadInstance(tx.Context(), tx, id, expected); err != nil {
				return err
			}
		}
		if _, err := s.management.resolver.ResolvePrincipalAuthority(tx.Context(), tx, detached.ExecutionPrincipalID); err != nil {
			return err
		}
		control, err = s.management.repository.LockRevisionForChange(tx.Context(), tx)
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
		configuration = processing.InstanceConfiguration{ID: instanceID, Revision: revision,
			Enabled: *detached.Enabled, ExecutionPrincipalID: detached.ExecutionPrincipalID,
			Main: detached.Main, Configuration: detached.Configuration,
			CreatedAt: now, CreatedBy: actor}
		return configuration.Validate()
	}
	if err := s.management.within(ctx, proof, true, check, preauthorize); err != nil {
		return commands.Result{}, err
	}
	if replay.ID != uuid.Nil {
		return replay, nil
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
		if current != control {
			return authz.ErrRevisionConflict
		}
		if _, err := s.management.resolver.ResolvePrincipalAuthority(tx.Context(), tx, configuration.ExecutionPrincipalID); err != nil {
			return err
		}
		configuration.CreatedAt = now
		if err := s.repository.SaveInstance(tx.Context(), tx, authority, configuration, expected); err != nil {
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
