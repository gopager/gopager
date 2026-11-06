package units

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// ReadAgency returns one Agency the caller may read (agency.read on all or on that Agency).
func (s *Service) ReadAgency(ctx context.Context, proof identity.Proof, id uuid.UUID) (Agency, error) {
	if id == uuid.Nil {
		return Agency{}, authz.ErrInvalidInput
	}
	var result Agency
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadAgency(tx.Context(), tx, authority, "agency.read", id, now)
		return err
	})
	if err != nil {
		return Agency{}, err
	}
	return result, nil
}

// ListAgencies pages the Agencies the caller may read.
func (s *Service) ListAgencies(ctx context.Context, proof identity.Proof, page PageInput) (Page[Agency], error) {
	var result Page[Agency]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListAgencies(tx.Context(), tx, authority, page, now)
		return err
	})
	if err != nil {
		return Page[Agency]{}, err
	}
	return result, nil
}

// CreateAgency creates an Agency (agency.create, scope all) with actor-bound command idempotency.
func (s *Service) CreateAgency(ctx context.Context, proof identity.Proof, command uuid.UUID, input AgencyInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	hash, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "agency.create", nil, authz.Target{}, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("agency.create", hash)
			return err
		}
		value := Agency{ID: uuid.New(), Revision: 1, Name: input.Name}
		if err := s.repository.SaveAgency(tx.Context(), tx, authority, value, 0, now); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, value.ID, 1, "agency", "created", "agency.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: value.ID, Revision: 1}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "agency.create", Fingerprint: hash, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// ChangeAgency renames an Agency at the expected revision (agency.update on all or on that Agency).
func (s *Service) ChangeAgency(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input AgencyInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.Canonical()
	if err != nil || id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadAgency(tx.Context(), tx, authority, "agency.update", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if current.Name == input.Name {
			result = commands.Result{ID: id, Revision: current.Revision}
			return nil
		}
		current.Name, current.Revision = input.Name, next
		if err := s.repository.SaveAgency(tx.Context(), tx, authority, current, expected, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return s.record(tx.Context(), tx, authority, id, next, "agency", "updated", "agency.update", nil, now)
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// DeleteAgency deletes an empty Agency (agency.delete, scope all); one that still owns Units
// or Radio networks fails with ErrAgencyInUse.
func (s *Service) DeleteAgency(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadAgency(tx.Context(), tx, authority, "agency.delete", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.repository.DeleteAgency(tx.Context(), tx, authority, current, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return s.record(tx.Context(), tx, authority, id, next, "agency", "deleted", "agency.delete", nil, now)
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}
