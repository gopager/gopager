package assignments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// CreateSuppressionInput identifies the target position to suppress.
type CreateSuppressionInput struct {
	TransmissionID uuid.UUID `json:"transmissionId"`
	TargetPosition int       `json:"targetPosition"`
}

func (s *Service) correctionRepository() (CorrectionRepository, error) {
	repository, ok := s.repository.(CorrectionRepository)
	if !ok {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("correction repository unavailable"))
	}
	if s.correctionFacts == nil {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("correction fact appender unavailable"))
	}
	return repository, nil
}

// ListSuppressions returns a page of suppressions (active and lifted) whose
// target the caller may read; suppressions on hidden or missing targets are
// silently omitted and not counted in Total. It fails with a
// CodeAuthorityUnavailable failure when the Service was built without
// correction support.
func (s *Service) ListSuppressions(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[Suppression], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Suppression]{}, err
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return units.Page[Suppression]{}, err
	}
	var result units.Page[Suppression]
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		values, err := repository.ListSuppressions(tx.Context(), tx)
		if err != nil {
			return err
		}
		visible := make([]Suppression, 0, len(values))
		for _, value := range values {
			target, err := repository.ReadTargetContext(tx.Context(), tx, value.TransmissionID, value.TargetPosition)
			if errors.Is(err, authz.ErrNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if err := s.targetAuthority(tx.Context(), tx, authority, "suppression.read", target, now); errors.Is(err, authz.ErrForbidden) {
				continue
			} else if err != nil {
				return err
			}
			visible = append(visible, value)
		}
		result = units.Page[Suppression]{Items: []Suppression{}, Total: len(visible)}
		if page.Offset >= len(visible) {
			return nil
		}
		end := min(page.Offset+page.Limit, len(visible))
		result.Items = visible[page.Offset:end]
		return nil
	})
	if err != nil {
		return units.Page[Suppression]{}, err
	}
	return result, nil
}

func (s *Service) targetAuthority(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	action string, value TargetContext, now time.Time,
) error {
	_, agency, err := s.network(ctx, tx, value.NetworkID)
	if err != nil {
		return err
	}
	target := authz.Target{AgencyID: agency}
	if value.Resolution.UnitID != nil {
		unit, err := s.units.ReadUnitContext(ctx, tx, *value.Resolution.UnitID)
		if err != nil {
			return err
		}
		target.UnitID = unit.ID
		target.UnitCollections = unit.CollectionIDs
	}
	if err := authz.Authorize(authority, action, nil, target, now); err != nil {
		return err
	}
	return nil
}

func (s *Service) recordSuppression(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	value Suppression, target TargetContext, kind, action string, command *uuid.UUID, now time.Time,
) error {
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if err := s.repository.RecordChange(ctx, tx, actor, value.ID, action, now); err != nil {
		return err
	}
	_, err = s.correctionFacts.AppendCorrectionFact(ctx, tx, events.CorrectionFact{
		ID: uuid.New(), Type: "suppression.changed", Aggregate: events.EntityRef{Type: "suppression", ID: value.ID},
		Revision: value.Revision, OccurredAt: now, CausationID: command,
		TargetPosition: &target.Position, TransmissionID: target.TransmissionID,
		NetworkID: target.NetworkID, UnitID: target.Resolution.UnitID, ChangeKind: kind,
	})
	return err
}

func suppressionResult(value Suppression, correction CorrectionStatus) SuppressionResult {
	return SuppressionResult{Suppression: value, Correction: correction}
}

// CreateSuppression suppresses one target position and, in the same
// transaction, bumps the assignment revision, queues a correction scope for
// exactly that target, records an audit change, appends a suppression.changed
// fact and stores a command receipt. No assignment revision token is required
// because the exclusive Assignment lock serializes the active-suppression check.
// It returns authz.ErrInvalidInput for invalid input or a nil check,
// authz.ErrRevisionConflict when the target is already actively suppressed,
// commands.ErrIdempotencyConflict when the command ID was already used with
// different input, and authorization errors when the caller may not create
// suppressions for the target. Replaying the same command ID with the same
// input returns the stored result even after the assignment revision has advanced.
func (s *Service) CreateSuppression(ctx context.Context, proof identity.Proof, command uuid.UUID,
	input CreateSuppressionInput, check func(identity.Authentication) error,
) (SuppressionResult, error) {
	if command == uuid.Nil || input.TransmissionID == uuid.Nil || input.TargetPosition < 0 {
		return SuppressionResult{}, authz.ErrInvalidInput
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return SuppressionResult{}, err
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return SuppressionResult{}, err
	}
	var result SuppressionResult
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		// The exclusive Assignment lock serializes the active-suppression check with other changes;
		// no client-supplied assignment revision is required, the target is the only context.
		if _, err := s.repository.LockRevisionForChange(tx.Context(), tx); err != nil {
			return err
		}
		target, err := repository.ReadTargetContext(tx.Context(), tx, input.TransmissionID, input.TargetPosition)
		if err != nil {
			return err
		}
		if err := s.targetAuthority(tx.Context(), tx, authority, "suppression.create", target, now); err != nil {
			return err
		}
		// A same-command replay (for example after COMMIT_UNCERTAIN) must return the
		// stored result even though the revision moved and the suppression is active.
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			saved, err := receipt.Check("suppression.create", fingerprint)
			if err != nil {
				return err
			}
			if len(saved.Items) != 1 || saved.Items[0].Status != "correction-job" {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid suppression receipt"))
			}
			suppression, err := repository.ReadSuppression(tx.Context(), tx, saved.ID)
			if err != nil {
				return err
			}
			correction, err := repository.ReadCorrection(tx.Context(), tx, saved.Items[0].ID)
			if err != nil {
				return err
			}
			result = suppressionResult(suppression, correction)
			return nil
		}
		if _, active, err := repository.FindActiveSuppression(tx.Context(), tx, input.TransmissionID, input.TargetPosition); err != nil {
			return err
		} else if active {
			return authz.ErrRevisionConflict
		}
		value := Suppression{ID: uuid.New(), TransmissionID: target.TransmissionID,
			TargetPosition: target.Position, Revision: 1, CreatedAt: now}
		if err := repository.CreateSuppression(tx.Context(), tx, value, actor); err != nil {
			return err
		}
		revision, err := s.repository.MarkPending(tx.Context(), tx,
			[]CorrectionScope{TargetScope(target.TransmissionID, target.Position)}, "suppression-change")
		if err != nil {
			return err
		}
		correction, err := repository.CorrectionForRevision(tx.Context(), tx, revision)
		if err != nil {
			return err
		}
		commandID := command
		if err := s.recordSuppression(tx.Context(), tx, authority, value, target,
			"created", "suppression.create", &commandID, now); err != nil {
			return err
		}
		result = suppressionResult(value, correction)
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: actor, CommandID: command, Operation: "suppression.create", Fingerprint: fingerprint,
			Result: commands.Result{ID: value.ID, Revision: value.Revision, Items: []commands.ResultItem{
				{ID: correction.ID, Revision: correction.AssignmentRev, Status: "correction-job"},
			}}, RecordedAt: now,
		})
	})
	if err != nil {
		return SuppressionResult{}, err
	}
	return result, nil
}

// ReadSuppression returns the suppression without correction status. It
// returns authz.ErrNotFound when the suppression, its target, or the caller's
// read permission on the target is missing.
func (s *Service) ReadSuppression(ctx context.Context, proof identity.Proof, id uuid.UUID) (SuppressionResult, error) {
	if id == uuid.Nil {
		return SuppressionResult{}, authz.ErrInvalidInput
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return SuppressionResult{}, err
	}
	var result SuppressionResult
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		value, err := repository.ReadSuppression(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		target, err := repository.ReadTargetContext(tx.Context(), tx, value.TransmissionID, value.TargetPosition)
		if err != nil {
			return authz.ErrNotFound
		}
		if err := s.targetAuthority(tx.Context(), tx, authority, "suppression.read", target, now); errors.Is(err, authz.ErrForbidden) {
			return authz.ErrNotFound
		} else if err != nil {
			return err
		}
		result = SuppressionResult{Suppression: value}
		return nil
	})
	if err != nil {
		return SuppressionResult{}, err
	}
	return result, nil
}

// LiftSuppression lifts the suppression at revision expected, then bumps the
// assignment revision, queues a correction scope for the target, records an
// audit change and appends a suppression.changed fact in the same transaction.
// It returns authz.ErrRevisionConflict for a stale suppression revision and
// authz.ErrNotFound when the caller may not delete suppressions for the target.
func (s *Service) LiftSuppression(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error,
) (SuppressionResult, error) {
	if id == uuid.Nil || expected <= 0 {
		return SuppressionResult{}, authz.ErrInvalidInput
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return SuppressionResult{}, err
	}
	var result SuppressionResult
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if _, err := s.repository.LockRevisionForChange(tx.Context(), tx); err != nil {
			return err
		}
		value, err := repository.ReadSuppression(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		target, err := repository.ReadTargetContext(tx.Context(), tx, value.TransmissionID, value.TargetPosition)
		if err != nil {
			return authz.ErrNotFound
		}
		if err := s.targetAuthority(tx.Context(), tx, authority, "suppression.delete", target, now); errors.Is(err, authz.ErrForbidden) {
			return authz.ErrNotFound
		} else if err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		value, err = repository.LiftSuppression(tx.Context(), tx, id, expected, actor, now)
		if err != nil {
			return err
		}
		revision, err := s.repository.MarkPending(tx.Context(), tx,
			[]CorrectionScope{TargetScope(target.TransmissionID, target.Position)}, "suppression-change")
		if err != nil {
			return err
		}
		correction, err := repository.CorrectionForRevision(tx.Context(), tx, revision)
		if err != nil {
			return err
		}
		if err := s.recordSuppression(tx.Context(), tx, authority, value, target, "lifted", "suppression.delete", nil, now); err != nil {
			return err
		}
		result = suppressionResult(value, correction)
		return nil
	})
	if err != nil {
		return SuppressionResult{}, err
	}
	return result, nil
}

func (s *Service) correctionAuthority(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	action string, status CorrectionStatus, now time.Time,
) error {
	targets, err := s.correctionTargets(ctx, tx, status.Scope)
	if err != nil {
		return authz.ErrNotFound
	}
	for _, target := range targets {
		if err := authz.Authorize(authority, action, nil, target, now); err != nil {
			return authz.ErrNotFound
		}
	}
	return nil
}

// correctionTargets resolves a correction scope to the authorization targets that must all be
// covered: the Gateway with each Agency its scope affects (with none, the Gateway alone, which
// only an installation-wide grant covers) or the single target of the other kinds.
func (s *Service) correctionTargets(ctx context.Context, tx platform.TransactionContext,
	scope CorrectionScope) ([]authz.Target, error) {
	if scope.Kind() != "gateway" {
		target, err := s.correctionTarget(ctx, tx, scope)
		if err != nil {
			return nil, err
		}
		return []authz.Target{target}, nil
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return nil, err
	}
	agencies, err := repository.GatewayCorrectionAgencies(ctx, tx, *scope.GatewayID, scope.Protocol)
	if err != nil {
		return nil, err
	}
	targets := make([]authz.Target, 0, max(len(agencies), 1))
	for _, agency := range agencies {
		targets = append(targets, authz.Target{AgencyID: agency, GatewayID: *scope.GatewayID})
	}
	if len(targets) == 0 {
		targets = append(targets, authz.Target{GatewayID: *scope.GatewayID})
	}
	return targets, nil
}

// correctionTarget resolves a non-gateway correction scope to its authorization target: the Agency
// of the selector's Network, the Unit with its Agency and collections, or the Agency of the
// target's Network together with its current Unit.
func (s *Service) correctionTarget(ctx context.Context, tx platform.TransactionContext, scope CorrectionScope) (authz.Target, error) {
	switch scope.Kind() {
	case "address-rules":
		_, agency, err := s.network(ctx, tx, *scope.NetworkID)
		if err != nil {
			return authz.Target{}, err
		}
		return authz.Target{AgencyID: agency}, nil
	case "unit":
		unit, err := s.units.ReadUnitContext(ctx, tx, *scope.UnitID)
		if err != nil {
			return authz.Target{}, err
		}
		return authz.Target{AgencyID: unit.AgencyID, UnitID: unit.ID, UnitCollections: unit.CollectionIDs}, nil
	case "target":
		repository, err := s.correctionRepository()
		if err != nil {
			return authz.Target{}, err
		}
		context, err := repository.ReadTargetContext(ctx, tx, *scope.TransmissionID, *scope.TargetPosition)
		if err != nil {
			return authz.Target{}, err
		}
		_, agency, err := s.network(ctx, tx, context.NetworkID)
		if err != nil {
			return authz.Target{}, err
		}
		target := authz.Target{AgencyID: agency}
		if context.Resolution.UnitID != nil {
			unit, err := s.units.ReadUnitContext(ctx, tx, *context.Resolution.UnitID)
			if err != nil {
				return authz.Target{}, err
			}
			target.UnitID, target.UnitCollections = unit.ID, unit.CollectionIDs
		}
		return target, nil
	}
	return authz.Target{}, authz.ErrInvalidInput
}

// ReadCorrection returns a correction scope's status if the caller may read
// assignment corrections for every Network (and the Unit) in its scope, for a
// gateway scope every Agency it affects; otherwise it returns authz.ErrNotFound.
func (s *Service) ReadCorrection(ctx context.Context, proof identity.Proof, id uuid.UUID) (CorrectionStatus, error) {
	if id == uuid.Nil {
		return CorrectionStatus{}, authz.ErrInvalidInput
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return CorrectionStatus{}, err
	}
	var result CorrectionStatus
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		result, err = repository.ReadCorrection(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		return s.correctionAuthority(tx.Context(), tx, authority, "assignment-correction.read", result, now)
	})
	if errors.Is(err, authz.ErrNotFound) {
		return CorrectionStatus{}, authz.ErrNotFound
	}
	if err != nil {
		return CorrectionStatus{}, err
	}
	return result, nil
}

// RetryCorrection replaces a failed correction scope with a new queued scope at
// the current assignment revision and returns the new scope. It is idempotent
// per actor and command ID. It returns authz.ErrRevisionConflict when the
// scope is not failed, commands.ErrIdempotencyConflict when the command ID was
// already used with different input, and authz.ErrNotFound when it is unknown
// or not visible.
func (s *Service) RetryCorrection(ctx context.Context, proof identity.Proof, id uuid.UUID,
	command uuid.UUID, check func(identity.Authentication) error,
) (CorrectionStatus, error) {
	if id == uuid.Nil || command == uuid.Nil {
		return CorrectionStatus{}, authz.ErrInvalidInput
	}
	repository, err := s.correctionRepository()
	if err != nil {
		return CorrectionStatus{}, err
	}
	fingerprint, err := commands.Fingerprint(struct {
		CorrectionID uuid.UUID `json:"correctionId"`
	}{CorrectionID: id})
	if err != nil {
		return CorrectionStatus{}, err
	}
	var result CorrectionStatus
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := repository.ReadCorrection(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := s.correctionAuthority(tx.Context(), tx, authority, "assignment-correction.retry", current, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		receipt, found, err := s.commands.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			saved, err := receipt.Check("assignment-correction.retry", fingerprint)
			if err != nil {
				return err
			}
			result, err = repository.ReadCorrection(tx.Context(), tx, saved.ID)
			return err
		}
		result, err = repository.RetryCorrection(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: actor, CommandID: command, Operation: "assignment-correction.retry", Fingerprint: fingerprint,
			Result: commands.Result{ID: result.ID, Revision: result.AssignmentRev}, RecordedAt: now,
		})
	})
	if err != nil {
		return CorrectionStatus{}, err
	}
	return result, nil
}
