package management

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// UnitParameterStore keeps the per-Unit input values of Delivery instances.
type UnitParameterStore interface {
	UnitExists(ctx context.Context, tx platform.TransactionContext, unitID uuid.UUID) (bool, error)
	ListUnitParameters(ctx context.Context, tx platform.TransactionContext, instanceID uuid.UUID, unit *uuid.UUID,
		page processing.PageInput) (processing.Page[processing.UnitParameters], error)
	ReplaceUnitParameters(ctx context.Context, tx platform.TransactionContext, instanceID, unitID uuid.UUID,
		values map[string]processing.Value) error
	DeleteUnitParameters(ctx context.Context, tx platform.TransactionContext, instanceID, unitID uuid.UUID) error
}

// UnitParameterService manages the per-Unit values of Delivery instances under global
// plugin.configure.
type UnitParameterService struct {
	instances *InstanceService
	store     UnitParameterStore
}

// NewUnitParameters requires the instance service (for its transactions and inspection) and the store.
func NewUnitParameters(instances *InstanceService, store UnitParameterStore) (*UnitParameterService, error) {
	if instances == nil || store == nil {
		return nil, authz.ErrInvalidInput
	}
	return &UnitParameterService{instances: instances, store: store}, nil
}

// unitResultRevision is the constant revision of a unit-parameter command result; the values
// themselves carry no revision.
const unitResultRevision = 1

// deliveryInstance reads the current instance and requires that its artifact is a Delivery capability.
func (u *UnitParameterService) deliveryInstance(ctx context.Context, instance processing.InstanceConfiguration) (
	processing.InstalledManifest, error) {
	manifest, err := u.instances.inspector.InspectManifest(ctx, instance.Main)
	if err != nil {
		return processing.InstalledManifest{}, err
	}
	return manifest, requireDelivery(manifest)
}

// List returns a page of the instance's per-Unit values, optionally of one Unit.
func (u *UnitParameterService) List(ctx context.Context, proof identity.Proof, id uuid.UUID, unit *uuid.UUID,
	page processing.PageInput) (processing.Page[processing.UnitParameters], error) {
	if id == uuid.Nil || validateInstancePage(page) != nil {
		return processing.Page[processing.UnitParameters]{}, authz.ErrInvalidInput
	}
	var instance processing.InstanceConfiguration
	var result processing.Page[processing.UnitParameters]
	if err := u.instances.withConfigureRead(ctx, proof, nil, func(tx platform.TransactionContext, authority authz.Authority,
		now time.Time) error {
		var err error
		if instance, err = u.instances.repository.ReadCurrentInstance(tx.Context(), tx, authority, id, now); err != nil {
			return err
		}
		result, err = u.store.ListUnitParameters(tx.Context(), tx, id, unit, page)
		return err
	}); err != nil {
		return processing.Page[processing.UnitParameters]{}, err
	}
	_, err := u.deliveryInstance(ctx, instance)
	return result, err
}

// Replace sets all of the Unit's values for the instance. Keys must be inputs the current manifest
// declares and values valid for them; the Unit must exist.
func (u *UnitParameterService) Replace(ctx context.Context, proof identity.Proof, command, id, unit uuid.UUID,
	values map[string]processing.Value, check func(identity.Authentication) error) (commands.Result, error) {
	if values == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	return u.change(ctx, proof, command, id, unit, values, check, "processing.unit-parameters.replace")
}

// Delete removes all of the Unit's values for the instance.
func (u *UnitParameterService) Delete(ctx context.Context, proof identity.Proof, command, id, unit uuid.UUID,
	check func(identity.Authentication) error) (commands.Result, error) {
	return u.change(ctx, proof, command, id, unit, nil, check, "processing.unit-parameters.delete")
}

// change replaces (non-nil values) or removes (nil values) a Unit's values in two phases: the
// manifest is read outside SQL, so the write re-checks that the instance did not change.
func (u *UnitParameterService) change(ctx context.Context, proof identity.Proof, command, id, unit uuid.UUID,
	values map[string]processing.Value, check func(identity.Authentication) error, operation string) (commands.Result, error) {
	if command == uuid.Nil || id == uuid.Nil || unit == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(struct {
		InstanceID, UnitID uuid.UUID
		Values             map[string]processing.Value
	}{id, unit, values})
	if err != nil {
		return commands.Result{}, err
	}
	var instance processing.InstanceConfiguration
	var replay commands.Result
	if err := u.instances.management.within(ctx, proof, true, check, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return authz.ErrInvalidInput
		}
		if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
			return err
		}
		if replay, err = u.replayed(tx, actor, command, operation, fingerprint); err != nil || replay.ID != uuid.Nil {
			return err
		}
		instance, err = u.instances.repository.ReadCurrentInstance(tx.Context(), tx, authority, id, now)
		return err
	}); err != nil {
		return commands.Result{}, err
	}
	if replay.ID != uuid.Nil {
		return replay, nil
	}
	manifest, err := u.deliveryInstance(ctx, instance)
	if err != nil {
		return commands.Result{}, err
	}
	if values != nil {
		if err := manifest.ValidateUnitParameters(values); err != nil {
			return commands.Result{}, err
		}
	}
	var result commands.Result
	err = u.instances.management.within(ctx, proof, true, check, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return authz.ErrInvalidInput
		}
		if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
			return err
		}
		if replay, err := u.replayed(tx, actor, command, operation, fingerprint); err != nil || replay.ID != uuid.Nil {
			result = replay
			return err
		}
		current, err := u.instances.repository.ReadCurrentInstance(tx.Context(), tx, authority, id, now)
		if err != nil {
			return err
		}
		if current.Revision != instance.Revision {
			return authz.ErrRevisionConflict
		}
		exists, err := u.store.UnitExists(tx.Context(), tx, unit)
		if err != nil {
			return err
		}
		if !exists {
			return authz.ErrNotFound
		}
		if values == nil {
			err = u.store.DeleteUnitParameters(tx.Context(), tx, id, unit)
		} else {
			err = u.store.ReplaceUnitParameters(tx.Context(), tx, id, unit, values)
		}
		if err != nil {
			return err
		}
		result = commands.Result{ID: unit, Revision: unitResultRevision}
		return u.instances.management.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: actor, CommandID: command, Operation: operation, Fingerprint: fingerprint,
			Result: result, RecordedAt: now})
	})
	return result, err
}

// replayed returns the stored result of an identical earlier command, an idempotency conflict for
// a different one, and the zero result when the command is new.
func (u *UnitParameterService) replayed(tx platform.TransactionContext, actor, command uuid.UUID, operation string,
	fingerprint [32]byte) (commands.Result, error) {
	receipt, exists, err := u.instances.management.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
	if err != nil || !exists {
		return commands.Result{}, err
	}
	return receipt.Check(operation, fingerprint)
}
