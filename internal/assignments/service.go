package assignments

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

type Service struct {
	transactions identity.Transactions
	resolver     *identity.Resolver
	clock        platform.Clock
	repository   Repository
	units        units.ContextReader
	networks     NetworkOwner
	commands     commands.Repository
	facts        events.ConfigurationAppender
}

// NewService composes current rules; operations needing an unwired Network owner fail explicitly.
func NewService(transactions identity.Transactions, resolver *identity.Resolver, clock platform.Clock, repository Repository, unitOwner units.ContextReader, networks NetworkOwner, receipts commands.Repository, facts events.ConfigurationAppender) (*Service, error) {
	if transactions == nil || resolver == nil || clock == nil || repository == nil || unitOwner == nil || receipts == nil || facts == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Service{transactions: transactions, resolver: resolver, clock: clock, repository: repository,
		units: unitOwner, networks: networks, commands: receipts, facts: facts}, nil
}

// within resolves current authority before the Assignment lock, using one primary transaction.
func (s *Service) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error, operation func(platform.TransactionContext, authz.Authority, time.Time) error) error {
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	return s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(authentication); err != nil {
				return err
			}
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return operation(tx, authentication.Authority, now)
	})
}

// canonicalInput validates before hashing and preserves set semantics for Function filters.
func canonicalInput(input RuleInput) (RuleInput, error) {
	if err := input.Rule(uuid.New(), 1).Validate(); err != nil {
		return RuleInput{}, err
	}
	input.Selector.Functions = slices.Clone(input.Selector.Functions)
	slices.Sort(input.Selector.Functions)
	if input.ValidFrom != nil {
		value := input.ValidFrom.UTC()
		input.ValidFrom = &value
	}
	if input.ValidUntil != nil {
		value := input.ValidUntil.UTC()
		input.ValidUntil = &value
	}
	return input, nil
}

// networkProtocol never substitutes a default Protocol when the owning domain is unavailable.
func (s *Service) networkProtocol(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, error) {
	if s.networks == nil {
		return "", platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Network owner is not wired"))
	}
	value, err := s.networks.ReadNetworkProtocol(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if !value.Valid() {
		return "", platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid current Network Protocol"))
	}
	return value, nil
}

// authorizeCandidate derives current owner references without exposing configuration from hidden uses.
func (s *Service) authorizeCandidate(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, candidate Rule, now time.Time) error {
	unit, err := s.units.ReadUnitContext(ctx, tx, candidate.UnitID)
	if errors.Is(err, authz.ErrNotFound) {
		return authz.ErrForbidden
	}
	if err != nil {
		return err
	}
	if unit.Deleted {
		return authz.ErrForbidden
	}
	if err := authz.Authorize(authority, action, nil, authz.Target{
		NetworkID: candidate.NetworkID, UnitID: unit.ID, UnitCollections: unit.CollectionIDs,
	}, now); err != nil {
		return err
	}
	protocol, err := s.networkProtocol(ctx, tx, candidate.NetworkID)
	if errors.Is(err, authz.ErrNotFound) {
		return authz.ErrForbidden
	}
	if err != nil {
		return err
	}
	if protocol != candidate.Protocol {
		return ErrInvalidRule
	}
	return nil
}

// validateCandidate checks all current rules, including conflicts hidden from management reads.
func (s *Service) validateCandidate(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, candidate Rule, now time.Time) error {
	if err := s.authorizeCandidate(ctx, tx, authority, action, candidate, now); err != nil {
		return err
	}
	current, err := s.repository.CurrentRules(ctx, tx, []uuid.UUID{candidate.NetworkID})
	if err != nil {
		return err
	}
	return ValidateChange(ctx, candidate, current)
}

// record atomically retains safe audit, current access revision and the typed owner fact.
func (s *Service) record(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value Rule, kind, action string, command *uuid.UUID, now time.Time) error {
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if err := s.repository.RecordChange(ctx, tx, actor, value.ID, action, now); err != nil {
		return err
	}
	_, err = s.facts.AppendConfigurationFact(ctx, tx, events.ConfigurationFact{ID: uuid.New(),
		Aggregate: events.EntityRef{Type: "addressRule", ID: value.ID}, Revision: value.Revision,
		OccurredAt: now, CausationID: command, ChangeKind: kind})
	return err
}

// ReadRule returns only a currently permitted rule DTO, never a conflicting hidden rule.
func (s *Service) ReadRule(ctx context.Context, proof identity.Proof, id uuid.UUID) (Rule, error) {
	if id == uuid.Nil {
		return Rule{}, authz.ErrInvalidInput
	}
	var result Rule
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadRule(tx.Context(), tx, authority, "address-rule.read", id, now)
		return err
	})
	if err != nil {
		return Rule{}, err
	}
	return result, nil
}

// ListRules applies the repository's native scope before count and bounded pagination.
func (s *Service) ListRules(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[Rule], error) {
	var result units.Page[Rule]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListRules(tx.Context(), tx, authority, page, now)
		return err
	})
	if err != nil {
		return units.Page[Rule]{}, err
	}
	return result, nil
}

// Preview validates creation or an expected-revision update without facts or pending work.
func (s *Service) Preview(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input RuleInput) error {
	input, err := canonicalInput(input)
	if err != nil || (id == uuid.Nil) != (expected == 0) || expected < 0 {
		return authz.ErrInvalidInput
	}
	return s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if _, err := s.repository.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		action := "address-rule.create"
		if id == uuid.Nil {
			id = uuid.New()
		} else {
			action = "address-rule.update"
			current, err := s.repository.ReadRule(tx.Context(), tx, authority, action, id, now)
			if err != nil {
				return err
			}
			if _, err := commands.NextRevision(current.Revision, expected); err != nil {
				return err
			}
		}
		return s.validateCandidate(tx.Context(), tx, authority, action, input.Rule(id, expected+1), now)
	})
}

// CreateRule commits state, correction scope, fact and actor-bound receipt together.
func (s *Service) CreateRule(ctx context.Context, proof identity.Proof, command uuid.UUID, input RuleInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := canonicalInput(input)
	if err != nil || command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	hash, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if _, err := s.repository.LockRevision(tx.Context(), tx); err != nil {
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
		id := uuid.New()
		if found {
			id = receipt.Result.ID
			if _, err := receipt.Check("address-rule.create", hash); err != nil {
				return err
			}
		}
		value := input.Rule(id, 1)
		if found {
			if err := s.authorizeCandidate(tx.Context(), tx, authority, "address-rule.create", value, now); err != nil {
				return err
			}
			result = receipt.Result
			return nil
		}
		if err := s.validateCandidate(tx.Context(), tx, authority, "address-rule.create", value, now); err != nil {
			return err
		}
		if err := s.repository.SaveRule(tx.Context(), tx, authority, value, 0, now); err != nil {
			return err
		}
		if _, err := s.repository.MarkPending(tx.Context(), tx, []uuid.UUID{value.NetworkID}, nil, "rule-change"); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, value, "created", "address-rule.create", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: 1}
		return s.commands.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "address-rule.create", Fingerprint: hash, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// UpdateRule checks old and new contexts before replacing one expected revision.
func (s *Service) UpdateRule(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input RuleInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := canonicalInput(input)
	if err != nil || id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if _, err := s.repository.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		before, err := s.repository.ReadRule(tx.Context(), tx, authority, "address-rule.update", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(before.Revision, expected)
		if err != nil {
			return err
		}
		value := input.Rule(id, next)
		if err := s.validateCandidate(tx.Context(), tx, authority, "address-rule.update", value, now); err != nil {
			return err
		}
		if err := s.repository.SaveRule(tx.Context(), tx, authority, value, expected, now); err != nil {
			return err
		}
		if _, err := s.repository.MarkPending(tx.Context(), tx, []uuid.UUID{before.NetworkID, value.NetworkID}, nil, "rule-change"); err != nil {
			return err
		}
		if err := s.record(tx.Context(), tx, authority, value, "updated", "address-rule.update", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: value.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// DeleteRule retains an explicit pending correction scope rather than claiming historical completion.
func (s *Service) DeleteRule(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if _, err := s.repository.LockRevision(tx.Context(), tx); err != nil {
			return err
		}
		value, err := s.repository.ReadRule(tx.Context(), tx, authority, "address-rule.delete", id, now)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(value.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.repository.DeleteRule(tx.Context(), tx, authority, value, now); err != nil {
			return err
		}
		if _, err := s.repository.MarkPending(tx.Context(), tx, []uuid.UUID{value.NetworkID}, nil, "rule-change"); err != nil {
			return err
		}
		value.Revision = next
		if err := s.record(tx.Context(), tx, authority, value, "deleted", "address-rule.delete", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: value.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// resolve loads only winning current Unit contexts; missing/deleted owners are never unassigned.
func (s *Service) resolve(ctx context.Context, tx platform.TransactionContext, network uuid.UUID, protocol protocols.Protocol, observed time.Time, targets []protocols.Target, rules []Rule, revision int64) (MatchResult, error) {
	actual, err := s.networkProtocol(ctx, tx, network)
	if err != nil {
		return MatchResult{}, err
	}
	if actual != protocol {
		return MatchResult{}, ErrInvalidRule
	}
	resolutions, err := Resolve(ctx, network, protocol, observed, targets, rules)
	if err != nil {
		return MatchResult{}, err
	}
	result := MatchResult{Revision: revision, Resolutions: resolutions, UnitContext: make(map[uuid.UUID]units.Context)}
	for _, resolution := range resolutions {
		if resolution.UnitID == nil {
			continue
		}
		if _, found := result.UnitContext[*resolution.UnitID]; found {
			continue
		}
		unit, err := s.units.ReadUnitContext(ctx, tx, *resolution.UnitID)
		if err != nil {
			return MatchResult{}, err
		}
		if unit.Deleted {
			return MatchResult{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("matched Unit is deleted"))
		}
		result.UnitContext[unit.ID] = unit
	}
	return result, nil
}

// MatchTargets participates in authenticated ingest's transaction and preserves every original position.
func (s *Service) MatchTargets(ctx context.Context, tx platform.TransactionContext, network uuid.UUID, protocol protocols.Protocol, observed time.Time, targets []protocols.Target) (MatchResult, error) {
	revision, err := s.repository.LockRevision(ctx, tx)
	if err != nil {
		return MatchResult{}, err
	}
	rules, err := s.repository.CurrentRules(ctx, tx, []uuid.UUID{network})
	if err != nil {
		return MatchResult{}, err
	}
	return s.resolve(ctx, tx, network, protocol, observed, targets, rules, revision)
}

// NetworksWithMatches evaluates only the server-derived Gateway Network set at one captured revision.
func (s *Service) NetworksWithMatches(ctx context.Context, tx platform.TransactionContext, allowed []uuid.UUID, protocol protocols.Protocol, observed time.Time, targets []protocols.Target) (NetworkCandidates, error) {
	if len(allowed) > 1024 {
		return NetworkCandidates{}, authz.ErrInvalidInput
	}
	if err := validateTargets(ctx, protocol, observed, targets); err != nil {
		return NetworkCandidates{}, err
	}
	revision, err := s.repository.LockRevision(ctx, tx)
	if err != nil {
		return NetworkCandidates{}, err
	}
	rules, err := s.repository.CurrentRules(ctx, tx, allowed)
	if err != nil {
		return NetworkCandidates{}, err
	}
	result := NetworkCandidates{Revision: revision, Networks: []uuid.UUID{}}
	seen := make(map[uuid.UUID]bool, len(allowed))
	for _, network := range allowed {
		if seen[network] {
			continue
		}
		seen[network] = true
		actual, err := s.networkProtocol(ctx, tx, network)
		if err != nil {
			return NetworkCandidates{}, err
		}
		if actual != protocol {
			continue
		}
		resolved, err := s.resolve(ctx, tx, network, protocol, observed, targets, rules, revision)
		if err != nil {
			return NetworkCandidates{}, err
		}
		if len(resolved.UnitContext) > 0 {
			result.Networks = append(result.Networks, network)
		}
	}
	return result, nil
}
