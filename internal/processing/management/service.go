// Package management coordinates current-authority Processing configuration commands.
package management

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
)

// DefinitionPreparer compiles against trusted source declarations/artifacts outside primary transactions.
type DefinitionPreparer interface {
	PrepareWorkflow(context.Context, processing.Definition) (*processing.WorkflowProgram, error)
}

// ScopeContext resolves the Agency of a Radio network or a Unit, so Workflow scope references can
// be checked for existence and authorized by Agency.
type ScopeContext interface {
	ReadNetwork(context.Context, platform.TransactionContext, uuid.UUID) (protocols.Protocol, uuid.UUID, error)
	ReadUnitContext(context.Context, platform.TransactionContext, uuid.UUID) (units.Context, error)
}

// Service manages Workflow configuration: reads, saves and versioning. Every operation
// resolves the caller's proof, locks the configuration revision and reads the primary
// clock inside one transaction; mutations additionally require a non-nil check callback
// and record the audit change, the Processing configuration fact and, for commands
// carrying a command ID, an idempotency receipt in that same transaction.
type Service struct {
	transactions identity.Transactions
	resolver     *identity.Resolver
	clock        platform.Clock
	repository   processing.ConfigurationRepository
	scopes       ScopeContext
	preparer     DefinitionPreparer
	receipts     commands.Repository
	facts        events.ProcessingAppender
}

// New composes existing transaction/proof/owner participants; missing dependencies fail explicitly.
func New(transactions identity.Transactions, resolver *identity.Resolver, clock platform.Clock,
	repository processing.ConfigurationRepository, scopes ScopeContext,
	preparer DefinitionPreparer, receipts commands.Repository, facts events.ProcessingAppender) (*Service, error) {
	if transactions == nil || resolver == nil || clock == nil || repository == nil || scopes == nil ||
		preparer == nil || receipts == nil || facts == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing management dependency"))
	}
	return &Service{transactions: transactions, resolver: resolver, clock: clock, repository: repository,
		scopes: scopes, preparer: preparer, receipts: receipts, facts: facts}, nil
}

// within keeps current proofs, primary time and protected configuration operations in one transaction.
func (s *Service) within(ctx context.Context, proof identity.Proof, mutation bool,
	check func(identity.Authentication) error, operation func(platform.TransactionContext, authz.Authority, time.Time) error) error {
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	return s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if mutation {
			if _, err := s.repository.LockRevisionForChange(tx.Context(), tx); err != nil {
				return err
			}
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		if authentication.ExpiresAt != nil && !now.Before(*authentication.ExpiresAt) {
			return identity.ErrLoginRequired
		}
		if check != nil {
			if err := check(authentication); err != nil {
				return err
			}
		}
		return operation(tx, authentication.Authority, now)
	})
}

// authorizeScope checks that every Workflow scope reference exists and that the caller holds
// action on each reference's Agency (and Unit); an unknown reference is invalid input.
func (s *Service) authorizeScope(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	action string, scope authz.Scope, now time.Time) error {
	switch scope.Kind {
	case "all":
		return authz.Authorize(authority, action, nil, authz.Target{}, now)
	case "network", "unit":
	default:
		return authz.ErrInvalidInput
	}
	for _, reference := range scope.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return authz.ErrInvalidInput
		}
		target, err := s.scopeTarget(ctx, tx, scope.Kind, id)
		if err != nil {
			return err
		}
		if err := authz.Authorize(authority, action, nil, target, now); err != nil {
			return err
		}
	}
	return nil
}

// scopeTarget resolves one Workflow scope reference to its authorization target: a Radio network
// to its Agency, a Unit to its Agency, identity and collections. Unknown or deleted references are
// invalid input, not a hidden-item hint.
func (s *Service) scopeTarget(ctx context.Context, tx platform.TransactionContext, kind string, id uuid.UUID) (authz.Target, error) {
	if kind == "network" {
		_, agency, err := s.scopes.ReadNetwork(ctx, tx, id)
		if errors.Is(err, authz.ErrNotFound) {
			return authz.Target{}, authz.ErrInvalidInput
		}
		if err != nil {
			return authz.Target{}, err
		}
		return authz.Target{AgencyID: agency}, nil
	}
	unit, err := s.scopes.ReadUnitContext(ctx, tx, id)
	if errors.Is(err, authz.ErrNotFound) {
		return authz.Target{}, authz.ErrInvalidInput
	}
	if err != nil {
		return authz.Target{}, err
	}
	if unit.Deleted {
		return authz.Target{}, authz.ErrInvalidInput
	}
	return authz.Target{AgencyID: unit.AgencyID, UnitID: id, UnitCollections: unit.CollectionIDs}, nil
}

// CheckSyntheticPreviewScope rechecks the editor's current preview authority for one captured context.
func (s *Service) CheckSyntheticPreviewScope(ctx context.Context, tx platform.TransactionContext,
	authority authz.Authority, workflowID, networkID, unitID uuid.UUID, now time.Time) error {
	if s == nil || ctx == nil || tx == nil || workflowID == uuid.Nil || networkID == uuid.Nil || now.IsZero() {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid synthetic preview scope check"))
	}
	workflow, err := s.repository.ReadWorkflow(ctx, tx, authority, "rule.preview", workflowID, now)
	if err != nil {
		return err
	}
	if err := s.authorizeScope(ctx, tx, authority, "rule.preview", workflow.Scope, now); err != nil {
		return err
	}
	target, err := s.scopeTarget(ctx, tx, "network", networkID)
	if err != nil {
		return err
	}
	switch workflow.Scope.Kind {
	case "all":
	case "network":
		if !slices.Contains(workflow.Scope.References, networkID.String()) {
			return authz.ErrForbidden
		}
	case "unit":
		if unitID == uuid.Nil || !slices.Contains(workflow.Scope.References, unitID.String()) {
			return authz.ErrForbidden
		}
		unit, err := s.scopes.ReadUnitContext(ctx, tx, unitID)
		if err != nil {
			return err
		}
		target.UnitID, target.UnitCollections = unitID, unit.CollectionIDs
	default:
		return authz.ErrInvalidInput
	}
	return authz.Authorize(authority, "rule.preview", nil, target, now)
}

// MetadataInput excludes active-version changes, which require the independent activation action.
type MetadataInput struct {
	Name    string      `json:"name"`
	Scope   authz.Scope `json:"scope"`
	Enabled *bool       `json:"enabled"`
}

// canonical bounds and detaches input and normalizes only set-valued scope references.
func (input MetadataInput) canonical() (MetadataInput, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return MetadataInput{}, err
	}
	var result MetadataInput
	if err := processing.Decode(encoded, &result); err != nil {
		return MetadataInput{}, err
	}
	if result.Enabled == nil {
		return MetadataInput{}, authz.ErrInvalidInput
	}
	for i, reference := range result.Scope.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return MetadataInput{}, authz.ErrInvalidInput
		}
		result.Scope.References[i] = id.String()
	}
	slices.Sort(result.Scope.References)
	for i := 1; i < len(result.Scope.References); i++ {
		if result.Scope.References[i-1] == result.Scope.References[i] {
			return MetadataInput{}, authz.ErrInvalidInput
		}
	}
	value := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1,
		Name: result.Name, Scope: result.Scope, Enabled: *result.Enabled}
	if err := value.Validate(); err != nil {
		return MetadataInput{}, err
	}
	return result, nil
}

// record atomically joins own/shared revision, safe audit and the separate Processing configuration fact.
func (s *Service) record(tx platform.TransactionContext, authority authz.Authority,
	value processing.WorkflowConfiguration, action, change string, command *uuid.UUID, now time.Time) error {
	actor, err := uuid.Parse(authority.PrincipalID)
	if err != nil {
		return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	if err := s.repository.RecordChange(tx.Context(), tx, actor, value.ID, action, now); err != nil {
		return err
	}
	_, err = s.facts.AppendProcessingConfiguration(tx.Context(), tx, events.ProcessingConfigurationFact{
		ID: uuid.New(), WorkflowID: value.ID, Revision: value.Revision, OccurredAt: now,
		CausationID: command, ChangeKind: change})
	return err
}

// ReadWorkflow returns current natively authorized metadata, never a write-authority fallback.
func (s *Service) ReadWorkflow(ctx context.Context, proof identity.Proof, id uuid.UUID) (processing.WorkflowConfiguration, error) {
	if id == uuid.Nil {
		return processing.WorkflowConfiguration{}, authz.ErrInvalidInput
	}
	var result processing.WorkflowConfiguration
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadWorkflow(tx.Context(), tx, authority, "rule.read", id, now)
		return err
	})
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	return result, nil
}

// ListWorkflows returns only the current permission-filtered page and count.
func (s *Service) ListWorkflows(ctx context.Context, proof identity.Proof, input processing.PageInput) (processing.Page[processing.WorkflowConfiguration], error) {
	var result processing.Page[processing.WorkflowConfiguration]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListWorkflows(tx.Context(), tx, authority, input, now)
		return err
	})
	if err != nil {
		return processing.Page[processing.WorkflowConfiguration]{}, err
	}
	return result, nil
}

// ReadVersion exposes definitions only with the ordinary read action.
func (s *Service) ReadVersion(ctx context.Context, proof identity.Proof, parent, id uuid.UUID) (processing.Version, error) {
	if parent == uuid.Nil || id == uuid.Nil {
		return processing.Version{}, authz.ErrInvalidInput
	}
	var result processing.Version
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ReadVersion(tx.Context(), tx, authority, parent, id, now)
		return err
	})
	if err != nil {
		return processing.Version{}, err
	}
	return result, nil
}

// ListVersions keeps immutable history behind the owning Workflow's current read permission.
func (s *Service) ListVersions(ctx context.Context, proof identity.Proof, parent uuid.UUID, input processing.PageInput) (processing.Page[processing.Version], error) {
	if parent == uuid.Nil {
		return processing.Page[processing.Version]{}, authz.ErrInvalidInput
	}
	var result processing.Page[processing.Version]
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		var err error
		result, err = s.repository.ListVersions(tx.Context(), tx, authority, parent, input, now)
		return err
	})
	if err != nil {
		return processing.Page[processing.Version]{}, err
	}
	return result, nil
}

// CreateWorkflow returns an actor-bound compact creation receipt without exposing unreadable metadata.
func (s *Service) CreateWorkflow(ctx context.Context, proof identity.Proof, command uuid.UUID,
	input MetadataInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.canonical()
	if err != nil {
		return commands.Result{}, err
	}
	if command == uuid.Nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	hash, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if err := s.authorizeScope(tx.Context(), tx, authority, "rule.create", input.Scope, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("rule.create", hash)
			return err
		}
		value := processing.WorkflowConfiguration{ID: uuid.New(), Revision: 1,
			Name: input.Name, Scope: input.Scope, Enabled: *input.Enabled}
		if err := s.repository.SaveWorkflow(tx.Context(), tx, authority, "rule.create", value, 0, now); err != nil {
			return err
		}
		if err := s.record(tx, authority, value, "rule.create", "created", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: value.ID, Revision: value.Revision}
		return s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "rule.create", Fingerprint: hash, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// UpdateWorkflow validates both old/new scope uses; it never changes the active definition.
func (s *Service) UpdateWorkflow(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	input MetadataInput, check func(identity.Authentication) error) (commands.Result, error) {
	input, err := input.canonical()
	if err != nil {
		return commands.Result{}, err
	}
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.current(tx, authority, "rule.update", id, expected, now)
		if err != nil {
			return err
		}
		if err := s.authorizeScope(tx.Context(), tx, authority, "rule.update", input.Scope, now); err != nil {
			return err
		}
		current.Name, current.Scope, current.Enabled = input.Name, input.Scope, *input.Enabled
		if err := s.repository.SaveWorkflow(tx.Context(), tx, authority, "rule.update", current, expected, now); err != nil {
			return err
		}
		if err := s.record(tx, authority, current, "rule.update", "updated", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: current.ID, Revision: current.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// current authorizes all current scope uses and checks expected revision before any mutation.
func (s *Service) current(tx platform.TransactionContext, authority authz.Authority,
	action string, id uuid.UUID, expected int64, now time.Time) (processing.WorkflowConfiguration, error) {
	value, err := s.repository.ReadWorkflow(tx.Context(), tx, authority, action, id, now)
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	if err := s.authorizeScope(tx.Context(), tx, authority, action, value.Scope, now); err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	value.Revision, err = commands.NextRevision(value.Revision, expected)
	return value, err
}

// DeleteWorkflow stops future starts by logical deletion while retaining referenced immutable versions.
func (s *Service) DeleteWorkflow(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.current(tx, authority, "rule.delete", id, expected, now)
		if err != nil {
			return err
		}
		current.Deleted, current.Enabled, current.ActiveVersionID = true, false, nil
		if err := s.repository.SaveWorkflow(tx.Context(), tx, authority, "rule.delete", current, expected, now); err != nil {
			return err
		}
		if err := s.record(tx, authority, current, "rule.delete", "deleted", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: current.ID, Revision: current.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// prepare compiles a detached definition and rejects substitution by a trusted preparation adapter.
func (s *Service) prepare(ctx context.Context, definition processing.Definition) (result processing.Definition, err error) {
	encoded, err := json.Marshal(definition)
	if err != nil {
		return processing.Definition{}, err
	}
	var detached processing.Definition
	if err := processing.Decode(encoded, &detached); err != nil {
		return processing.Definition{}, err
	}
	program, err := s.preparer.PrepareWorkflow(ctx, detached)
	if err != nil {
		return processing.Definition{}, err
	}
	if program == nil {
		return processing.Definition{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("Workflow preparation owner returned no program"))
	}
	if closer, ok := s.preparer.(interface {
		ClosePreparedWorkflow(context.Context, *processing.WorkflowProgram) error
	}); ok {
		defer func() {
			err = errors.Join(err, closer.ClosePreparedWorkflow(context.WithoutCancel(ctx), program))
		}()
	}
	checked, err := program.CheckedDefinition()
	if err != nil {
		return processing.Definition{}, err
	}
	actual, err := json.Marshal(checked)
	if err != nil {
		return processing.Definition{}, err
	}
	if !bytes.Equal(encoded, actual) {
		return processing.Definition{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("Workflow preparation substituted the definition"))
	}
	return checked, ctx.Err()
}

// SaveVersion preauthorizes, compiles without a transaction, then reauthorizes/CAS-writes all participants.
func (s *Service) SaveVersion(ctx context.Context, proof identity.Proof, command, parent uuid.UUID,
	expected int64, definition processing.Definition, check func(identity.Authentication) error) (commands.Result, error) {
	if command == uuid.Nil || parent == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		return commands.Result{}, err
	}
	var detached processing.Definition
	if err := processing.Decode(encoded, &detached); err != nil {
		return commands.Result{}, err
	}
	hash, err := commands.Fingerprint(struct {
		Parent     uuid.UUID
		Expected   int64
		Definition processing.Definition
	}{parent, expected, detached})
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	var replay bool
	var preparationRevision int64
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadWorkflow(tx.Context(), tx, authority, "rule.update", parent, now)
		if err != nil {
			return err
		}
		if err := s.authorizeScope(tx.Context(), tx, authority, "rule.update", current.Scope, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("rule.version.save", hash)
			replay = err == nil
			return err
		}
		preparationRevision = authority.Revision
		_, err = commands.NextRevision(current.Revision, expected)
		return err
	})
	if err != nil {
		return commands.Result{}, err
	}
	if replay {
		return result, nil
	}
	detached, err = s.prepare(ctx, detached)
	if err != nil {
		return commands.Result{}, err
	}
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.repository.ReadWorkflow(tx.Context(), tx, authority, "rule.update", parent, now)
		if err != nil {
			return err
		}
		if err := s.authorizeScope(tx.Context(), tx, authority, "rule.update", current.Scope, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authority.PrincipalID)
		if err != nil {
			return err
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			result, err = receipt.Check("rule.version.save", hash)
			return err
		}
		if authority.Revision != preparationRevision {
			return authz.ErrRevisionConflict
		}
		current.Revision, err = commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		version := processing.Version{ID: uuid.New(), WorkflowID: parent,
			Definition: detached, CreatedAt: now, CreatedBy: actor}
		if err := s.repository.SaveVersion(tx.Context(), tx, version); err != nil {
			return err
		}
		if err := s.repository.SaveWorkflow(tx.Context(), tx, authority, "rule.update", current, expected, now); err != nil {
			return err
		}
		if err := s.record(tx, authority, current, "rule.update", "version-saved", &command, now); err != nil {
			return err
		}
		result = commands.Result{ID: version.ID, Revision: current.Revision}
		return s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
			Operation: "rule.version.save", Fingerprint: hash, Result: result, RecordedAt: now})
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}

// ActivateVersion explicitly selects any retained version using activate alone, never an implied CRUD action.
func (s *Service) ActivateVersion(ctx context.Context, proof identity.Proof, parent, version uuid.UUID,
	expected int64, check func(identity.Authentication) error) (commands.Result, error) {
	if parent == uuid.Nil || version == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var definition processing.Definition
	var preparationRevision int64
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		if _, err := s.current(tx, authority, "rule.activate", parent, expected, now); err != nil {
			return err
		}
		selected, err := s.repository.ReadVersionForAction(tx.Context(), tx, authority, "rule.activate", parent, version, now)
		definition = selected.Definition
		preparationRevision = authority.Revision
		return err
	})
	if err != nil {
		return commands.Result{}, err
	}
	if _, err := s.prepare(ctx, definition); err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
		current, err := s.current(tx, authority, "rule.activate", parent, expected, now)
		if err != nil {
			return err
		}
		if authority.Revision != preparationRevision {
			return authz.ErrRevisionConflict
		}
		if _, err := s.repository.ReadVersionForAction(tx.Context(), tx, authority, "rule.activate", parent, version, now); err != nil {
			return err
		}
		current.ActiveVersionID = &version
		if err := s.repository.SaveWorkflow(tx.Context(), tx, authority, "rule.activate", current, expected, now); err != nil {
			return err
		}
		if err := s.record(tx, authority, current, "rule.activate", "activated", nil, now); err != nil {
			return err
		}
		result = commands.Result{ID: current.ID, Revision: current.Revision}
		return nil
	})
	if err != nil {
		return commands.Result{}, err
	}
	return result, nil
}
