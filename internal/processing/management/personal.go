package management

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ManageOwnAction is the right that lets a User manage the personal values of their own account.
const ManageOwnAction = "subscription.manage-own"

// CurrentInstances reads the current revision of an instance without administrator authority;
// callers authorize through the instance's own target use.
type CurrentInstances interface {
	ReadCurrentConfiguration(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (processing.InstanceConfiguration, error)
}

// PersonalStore keeps the personal configurations of Delivery instances.
type PersonalStore interface {
	processing.PersonalConfigurations
	SavePersonalConfiguration(ctx context.Context, tx platform.TransactionContext, instanceID, ownerID uuid.UUID,
		expected int64, configuration processing.Value) (int64, error)
	DeletePersonalConfiguration(ctx context.Context, tx platform.TransactionContext, instanceID, ownerID uuid.UUID,
		expected int64) error
}

// PersonalInput is a write of the caller's personal values: Configuration replaces the user-scoped
// non-Secret values; Secrets maps the dotted path of a user-scoped Secret property to a new stored
// value (null removes it, a path left out keeps it). References to files or variables are not
// accepted for personal Secrets.
type PersonalInput struct {
	Configuration processing.Value        `json:"configuration"`
	Secrets       map[string]*SecretInput `json:"secrets,omitempty"`
}

// PersonalView is the caller's personal configuration as shown to them: Secrets only as
// configured, redacted, with their source.
type PersonalView struct {
	InstanceID    uuid.UUID                            `json:"instanceId"`
	Revision      int64                                `json:"revision,string"`
	Configuration processing.InstanceConfigurationView `json:"configuration"`
}

// PersonalService manages the personal values a User holds for Delivery instances admitting them.
type PersonalService struct {
	instances *InstanceService
	store     PersonalStore
	current   CurrentInstances
}

// NewPersonal requires the instance service (for its transactions, inspection and Secret
// settings) and the stores.
func NewPersonal(instances *InstanceService, store PersonalStore, current CurrentInstances) (*PersonalService, error) {
	if instances == nil || store == nil || current == nil {
		return nil, authz.ErrInvalidInput
	}
	return &PersonalService{instances: instances, store: store, current: current}, nil
}

// personalFingerprint is what a personal write is compared by; Secrets enter only as keyed digests.
type personalFingerprint struct {
	InstanceID    uuid.UUID
	Expected      int64
	Configuration processing.Value
	Secrets       map[string]string
}

// caller authorizes the caller's own account and returns its ID.
func caller(authority authz.Authority, now time.Time) (uuid.UUID, error) {
	principal, err := uuid.Parse(authority.PrincipalID)
	if err != nil || principal == uuid.Nil {
		return uuid.Nil, authz.ErrForbidden
	}
	return principal, authz.Authorize(authority, ManageOwnAction, nil, authz.Target{AccountID: principal}, now)
}

// admitted reads the current instance and requires that it is enabled and admits the caller;
// otherwise it does not exist for them.
func (p *PersonalService) admitted(ctx context.Context, tx platform.TransactionContext, id, principal uuid.UUID) (
	processing.InstanceConfiguration, error) {
	instance, err := p.current.ReadCurrentConfiguration(ctx, tx, id)
	if err != nil {
		return processing.InstanceConfiguration{}, err
	}
	if !instance.Enabled || !instance.AllowsTargetUse(principal) {
		return processing.InstanceConfiguration{}, authz.ErrNotFound
	}
	return instance, nil
}

// Read returns the caller's personal configuration of the instance, or authz.ErrNotFound when the
// instance does not admit them or they hold none.
func (p *PersonalService) Read(ctx context.Context, proof identity.Proof, id uuid.UUID) (PersonalView, error) {
	if id == uuid.Nil {
		return PersonalView{}, authz.ErrInvalidInput
	}
	var instance processing.InstanceConfiguration
	var personal processing.PersonalConfiguration
	if err := p.instances.management.within(ctx, proof, false, nil, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		principal, err := caller(authority, now)
		if err != nil {
			return err
		}
		if instance, err = p.admitted(tx.Context(), tx, id, principal); err != nil {
			return err
		}
		personal, err = p.store.ReadPersonalConfiguration(tx.Context(), tx, id, principal)
		return err
	}); err != nil {
		return PersonalView{}, err
	}
	manifest, err := p.instances.inspector.InspectManifest(ctx, instance.Main)
	if err != nil {
		return PersonalView{}, err
	}
	if err := requireDelivery(manifest); err != nil {
		return PersonalView{}, err
	}
	view, err := manifest.ProjectPersonalConfiguration(personal.Configuration)
	if err != nil {
		return PersonalView{}, err
	}
	return PersonalView{InstanceID: id, Revision: personal.Revision, Configuration: view}, nil
}

// Save creates (expected 0) or replaces the caller's personal configuration. An administrative or
// unknown property anywhere in the request rejects the whole request.
func (p *PersonalService) Save(ctx context.Context, proof identity.Proof, command, id uuid.UUID, expected int64,
	input PersonalInput, check func(identity.Authentication) error) (commands.Result, error) {
	if command == uuid.Nil || id == uuid.Nil || expected < 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	digests, err := secretFingerprints(p.instances.secrets.cipher, input.Secrets)
	if err != nil {
		return commands.Result{}, err
	}
	fingerprint, err := commands.Fingerprint(personalFingerprint{InstanceID: id, Expected: expected,
		Configuration: input.Configuration, Secrets: digests})
	if err != nil {
		return commands.Result{}, err
	}
	const operation = "processing.personal.save"
	var replay commands.Result
	var instance processing.InstanceConfiguration
	var existing *processing.PersonalConfiguration
	var principal uuid.UUID
	if err := p.instances.management.within(ctx, proof, true, check, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		var err error
		if principal, err = caller(authority, now); err != nil {
			return err
		}
		if replay, err = p.replayed(tx, principal, command, operation, fingerprint); err != nil || replay.ID != uuid.Nil {
			return err
		}
		if instance, err = p.admitted(tx.Context(), tx, id, principal); err != nil {
			return err
		}
		existing, err = p.existing(tx, id, principal, expected)
		return err
	}); err != nil {
		return commands.Result{}, err
	}
	if replay.ID != uuid.Nil {
		return replay, nil
	}
	configuration, plan, err := p.complete(ctx, instance, principal, existing, input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = p.instances.management.within(ctx, proof, true, check, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		if _, err := caller(authority, now); err != nil {
			return err
		}
		if replay, err := p.replayed(tx, principal, command, operation, fingerprint); err != nil || replay.ID != uuid.Nil {
			result = replay
			return err
		}
		current, err := p.admitted(tx.Context(), tx, id, principal)
		if err != nil {
			return err
		}
		if current.Revision != instance.Revision {
			return authz.ErrRevisionConflict
		}
		revision, err := p.store.SavePersonalConfiguration(tx.Context(), tx, id, principal, expected, configuration)
		if err != nil {
			return err
		}
		if err := p.instances.storeSecrets(tx, plan.rows); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: revision}
		return p.instances.management.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: principal, CommandID: command, Operation: operation, Fingerprint: fingerprint,
			Result: result, RecordedAt: now})
	})
	return result, err
}

// replayed returns the stored result of an identical earlier command, an idempotency conflict for
// a different one, and the zero result when the command is new.
func (p *PersonalService) replayed(tx platform.TransactionContext, actor, command uuid.UUID, operation string,
	fingerprint [32]byte) (commands.Result, error) {
	receipt, exists, err := p.instances.management.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
	if err != nil || !exists {
		return commands.Result{}, err
	}
	return receipt.Check(operation, fingerprint)
}

// existing returns the caller's current personal row, nil when there is none, and requires that it
// matches the expected revision (0 meaning none).
func (p *PersonalService) existing(tx platform.TransactionContext, id, principal uuid.UUID, expected int64) (
	*processing.PersonalConfiguration, error) {
	personal, err := p.store.ReadPersonalConfiguration(tx.Context(), tx, id, principal)
	if err == nil {
		if personal.Revision != expected {
			return nil, authz.ErrRevisionConflict
		}
		return &personal, nil
	}
	if errors.Is(err, authz.ErrNotFound) {
		if expected != 0 {
			return nil, authz.ErrRevisionConflict
		}
		return nil, nil
	}
	return nil, err
}

// complete validates the request against the pinned manifest and returns the configuration to
// store and the Secret rows to seal.
func (p *PersonalService) complete(ctx context.Context, instance processing.InstanceConfiguration, principal uuid.UUID,
	existing *processing.PersonalConfiguration, input PersonalInput) (processing.Value, secretPlan, error) {
	manifest, err := p.instances.inspector.InspectManifest(ctx, instance.Main)
	if err != nil {
		return processing.Value{}, secretPlan{}, err
	}
	if err := requireDelivery(manifest); err != nil {
		return processing.Value{}, secretPlan{}, err
	}
	if err := manifest.RejectSecretValues(input.Configuration); err != nil {
		return processing.Value{}, secretPlan{}, err
	}
	var kept map[string]string
	if existing != nil {
		kept = previousReferences(manifest, existing.Configuration)
	}
	plan, err := planSecrets(p.instances.secrets, manifest, secretScope{scope: "user", instanceID: instance.ID,
		owner: &principal}, manifest.RetainableSecretReferences(kept, input.Configuration), input.Secrets)
	if err != nil {
		return processing.Value{}, secretPlan{}, err
	}
	configuration, err := manifest.WithSecretReferences(input.Configuration, plan.references)
	if err != nil {
		return processing.Value{}, secretPlan{}, err
	}
	return configuration, plan, manifest.ValidatePersonalConfiguration(configuration)
}

// Delete removes the caller's personal configuration at the expected revision together with their
// Secrets of the instance.
func (p *PersonalService) Delete(ctx context.Context, proof identity.Proof, command, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if command == uuid.Nil || id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(struct {
		InstanceID uuid.UUID
		Expected   int64
	}{id, expected})
	if err != nil {
		return commands.Result{}, err
	}
	const operation = "processing.personal.delete"
	var result commands.Result
	err = p.instances.management.within(ctx, proof, true, check, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		principal, err := caller(authority, now)
		if err != nil {
			return err
		}
		if replay, err := p.replayed(tx, principal, command, operation, fingerprint); err != nil || replay.ID != uuid.Nil {
			result = replay
			return err
		}
		if _, err := p.admitted(tx.Context(), tx, id, principal); err != nil {
			return err
		}
		if err := p.store.DeletePersonalConfiguration(tx.Context(), tx, id, principal, expected); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: expected}
		return p.instances.management.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: principal, CommandID: command, Operation: operation, Fingerprint: fingerprint,
			Result: result, RecordedAt: now})
	})
	return result, err
}
