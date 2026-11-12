// Package processingstore implements Processing-owned state through borrowed primary transactions.
package processingstore

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/attributestore"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/persistence/ent/workflow"
	"github.com/gopager/gopager/internal/persistence/ent/workflowagency"
	"github.com/gopager/gopager/internal/persistence/ent/workflowscopereference"
	"github.com/gopager/gopager/internal/persistence/ent/workflowversion"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

// Store persists Processing state: Workflows and their versions and scope references,
// plugin instances, candidates, results and publication participation. It holds no
// transaction of its own: every method runs in the caller's active primary transaction.
type Store struct {
	primary *postgres.Store
	// catalog reads the Attribute schemas that Workflow outputs reference.
	catalog *attributestore.Store
	// registries yields the field registry of the catalog visible in a transaction.
	registries *attributes.RegistrySource
}

// New binds this owner to the existing primary store, never an independent connection.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing primary"))
	}
	catalog, err := attributestore.New(primary)
	if err != nil {
		return nil, err
	}
	registries, err := attributes.NewRegistrySource(catalog)
	if err != nil {
		return nil, err
	}
	return &Store{primary: primary, catalog: catalog, registries: registries}, nil
}

func (s *Store) client(ctx context.Context, tx platform.TransactionContext, predicates map[string]entql.P, create map[string]bool) (*ent.Client, context.Context, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return nil, nil, err
	}
	return ent.NewClient(ent.Driver(driver)), guard.Bind(ctx, guard.Access{
		Current: driver.Current, Predicates: predicates, Create: create,
	}), nil
}

// LockRevision takes the shared Processing-control lock (candidate work, reads, intake
// admission) and returns the current configuration revision; configuration changes take the
// exclusive lock through LockRevisionForChange.
func (s *Store) LockRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	return s.lockRevision(ctx, tx, false)
}

// LockRevisionForChange takes the exclusive Processing-control lock for a command that
// changes Workflow or instance configuration and will bump the configuration revision.
func (s *Store) LockRevisionForChange(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	return s.lockRevision(ctx, tx, true)
}

func (s *Store) lockRevision(ctx context.Context, tx platform.TransactionContext, exclusive bool) (int64, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"ProcessingControl": entql.FieldEQ("id", 1)}, nil)
	if err != nil {
		return 0, err
	}
	query := client.ProcessingControl.Query()
	if exclusive {
		query = query.ForUpdate()
	} else {
		query = query.ForShare()
	}
	row, err := query.Only(scope)
	if err != nil {
		return 0, err
	}
	return row.Revision, nil
}

// grantScope translates one grant scope into a Workflow predicate: an Agency grant matches a
// Workflow assigned to one of its Agencies, a Unit grant a Workflow narrowed to one of its Units.
// Matching one assigned Agency suffices to see a Workflow; managing it needs full coverage, which
// the service checks.
func grantScope(kind string, references []uuid.UUID) (entql.P, error) {
	values := make([]any, len(references))
	for i, id := range references {
		values[i] = id
	}
	switch kind {
	case "agency":
		return entql.HasEdgeWith("workflow_agencies", entql.FieldIn("agency_id", values...)), nil
	case "unit":
		return entql.And(entql.FieldEQ("narrowing_kind", "unit"),
			entql.HasEdgeWith("scope_references", entql.And(entql.FieldIn("unit_id", values...),
				entql.HasEdgeWith("unit", entql.FieldEQ("deleted", false))))), nil
	default:
		return nil, authz.ErrInvalidInput
	}
}

func workflowScope(authority authz.Authority, action string, now time.Time) (entql.P, error) {
	plan, err := permissions.ReadScope(authority, action, nil, permissions.Attributes{Scope: grantScope}, now)
	if err != nil {
		return nil, err
	}
	return entql.And(entql.FieldEQ("deleted", false), plan.Predicate), nil
}

func metadata(row *ent.Workflow) (processing.WorkflowConfiguration, error) {
	references, err := row.Edges.ScopeReferencesOrErr()
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	agencies, err := row.Edges.WorkflowAgenciesOrErr()
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	out := processing.WorkflowConfiguration{ID: row.ID, Revision: row.Revision, Name: row.Name,
		Enabled: row.Enabled, Deleted: row.Deleted,
		ActiveVersionID: row.ActiveVersionID, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	for _, agency := range agencies {
		out.Scope.Agencies = append(out.Scope.Agencies, agency.AgencyID)
	}
	if row.NarrowingKind != nil {
		out.Scope.Narrowing = &processing.WorkflowNarrowing{Kind: string(*row.NarrowingKind)}
	}
	for _, reference := range references {
		switch {
		case out.Scope.Narrowing == nil:
			return processing.WorkflowConfiguration{}, authz.ErrInvalidInput
		case out.Scope.Narrowing.Kind == "network" && reference.NetworkID != nil && reference.UnitID == nil:
			out.Scope.Narrowing.References = append(out.Scope.Narrowing.References, *reference.NetworkID)
		case out.Scope.Narrowing.Kind == "unit" && reference.UnitID != nil && reference.NetworkID == nil:
			out.Scope.Narrowing.References = append(out.Scope.Narrowing.References, *reference.UnitID)
		default:
			return processing.WorkflowConfiguration{}, authz.ErrInvalidInput
		}
	}
	scope, err := out.Scope.Canonical()
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	out.Scope = scope
	if err := out.Validate(); err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	return out, nil
}

func (s *Store) readClient(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, now time.Time) (*ent.Client, context.Context, error) {
	filter, err := workflowScope(authority, action, now)
	if err != nil {
		return nil, nil, err
	}
	return s.client(ctx, tx, map[string]entql.P{"Workflow": filter,
		"WorkflowVersion":        entql.HasEdgeWith("workflow", filter),
		"WorkflowScopeReference": entql.HasEdgeWith("workflow", filter),
		"WorkflowAgency":         entql.HasEdgeWith("workflow", filter)}, nil)
}

// ReadWorkflow returns identical not-found for an absent or natively filtered Workflow.
func (s *Store) ReadWorkflow(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (processing.WorkflowConfiguration, error) {
	client, scope, err := s.readClient(ctx, tx, authority, action, now)
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	row, err := client.Workflow.Query().Where(workflow.ID(id)).WithScopeReferences().WithWorkflowAgencies().Only(scope)
	if ent.IsNotFound(err) {
		return processing.WorkflowConfiguration{}, authz.ErrNotFound
	}
	if err != nil {
		return processing.WorkflowConfiguration{}, err
	}
	return metadata(row)
}

func page(input processing.PageInput) (processing.PageInput, error) {
	if input.Limit == 0 {
		input.Limit = 50
	}
	if input.Offset < 0 || input.Limit < 1 || input.Limit > 200 {
		return processing.PageInput{}, authz.ErrInvalidInput
	}
	return input, nil
}

// ListWorkflows applies mandatory native scope filtering before count and deterministic paging.
func (s *Store) ListWorkflows(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, input processing.PageInput, now time.Time) (processing.Page[processing.WorkflowConfiguration], error) {
	input, err := page(input)
	if err != nil {
		return processing.Page[processing.WorkflowConfiguration]{}, err
	}
	client, scope, err := s.readClient(ctx, tx, authority, "workflow.read", now)
	if err != nil {
		return processing.Page[processing.WorkflowConfiguration]{}, err
	}
	total, err := client.Workflow.Query().Count(scope)
	if err != nil {
		return processing.Page[processing.WorkflowConfiguration]{}, err
	}
	rows, err := client.Workflow.Query().Order(workflow.ByCreatedAt(), workflow.ByID()).
		Offset(input.Offset).Limit(input.Limit).WithScopeReferences().WithWorkflowAgencies().All(scope)
	if err != nil {
		return processing.Page[processing.WorkflowConfiguration]{}, err
	}
	result := processing.Page[processing.WorkflowConfiguration]{Total: total, Items: make([]processing.WorkflowConfiguration, 0, len(rows))}
	for _, row := range rows {
		value, err := metadata(row)
		if err != nil {
			return processing.Page[processing.WorkflowConfiguration]{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, nil
}

func version(row *ent.WorkflowVersion) processing.Version {
	return processing.Version{ID: row.ID, WorkflowID: row.WorkflowID, Definition: row.Definition,
		CreatedAt: row.CreatedAt, CreatedBy: row.CreatedBy}
}

// ReadVersion keeps immutable definitions inside their currently readable owning Workflow.
func (s *Store) ReadVersion(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, parent, id uuid.UUID, now time.Time) (processing.Version, error) {
	return s.ReadVersionForAction(ctx, tx, authority, "workflow.read", parent, id, now)
}

// ReadVersionForAction loads an owning definition for a trusted command, without adding a public read grant.
func (s *Store) ReadVersionForAction(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, parent, id uuid.UUID, now time.Time) (processing.Version, error) {
	if !slices.Contains([]string{"workflow.read", "workflow.update", "workflow.activate", "workflow.preview"}, action) {
		return processing.Version{}, authz.ErrInvalidInput
	}
	client, scope, err := s.readClient(ctx, tx, authority, action, now)
	if err != nil {
		return processing.Version{}, err
	}
	row, err := client.WorkflowVersion.Query().Where(workflowversion.ID(id), workflowversion.WorkflowID(parent)).Only(scope)
	if ent.IsNotFound(err) {
		return processing.Version{}, authz.ErrNotFound
	}
	if err != nil {
		return processing.Version{}, err
	}
	return version(row), nil
}

// ListVersions filters/counts the owning Workflow before exposing saved definitions.
func (s *Store) ListVersions(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, parent uuid.UUID, input processing.PageInput, now time.Time) (processing.Page[processing.Version], error) {
	input, err := page(input)
	if err != nil {
		return processing.Page[processing.Version]{}, err
	}
	if _, err := s.ReadWorkflow(ctx, tx, authority, "workflow.read", parent, now); err != nil {
		return processing.Page[processing.Version]{}, err
	}
	client, scope, err := s.readClient(ctx, tx, authority, "workflow.read", now)
	if err != nil {
		return processing.Page[processing.Version]{}, err
	}
	query := client.WorkflowVersion.Query().Where(workflowversion.WorkflowID(parent))
	total, err := query.Clone().Count(scope)
	if err != nil {
		return processing.Page[processing.Version]{}, err
	}
	rows, err := query.Order(workflowversion.ByCreatedAt(), workflowversion.ByID()).Offset(input.Offset).Limit(input.Limit).All(scope)
	if err != nil {
		return processing.Page[processing.Version]{}, err
	}
	result := processing.Page[processing.Version]{Total: total, Items: make([]processing.Version, 0, len(rows))}
	for _, row := range rows {
		result.Items = append(result.Items, version(row))
	}
	return result, nil
}

// SaveWorkflow persists a service-authorized complete metadata change with expected-revision CAS.
func (s *Store) SaveWorkflow(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, value processing.WorkflowConfiguration, expected int64, now time.Time) error {
	if !slices.Contains([]string{"workflow.create", "workflow.update", "workflow.activate", "workflow.delete"}, action) ||
		expected < 0 || now.IsZero() {
		return authz.ErrInvalidInput
	}
	next := int64(1)
	if expected > 0 {
		var err error
		next, err = commands.NextRevision(expected, expected)
		if err != nil {
			return err
		}
	}
	if err := value.Validate(); err != nil {
		return err
	}
	if value.Revision != next {
		return authz.ErrInvalidInput
	}
	if action == "workflow.activate" && value.ActiveVersionID != nil {
		if err := s.requireVersionOutputSchemas(ctx, tx, *value.ActiveVersionID); err != nil {
			return err
		}
	}
	filter := entql.FieldEQ("id", value.ID)
	if expected != 0 {
		allowed, err := workflowScope(authority, action, now)
		if err != nil {
			return err
		}
		filter = entql.And(filter, allowed)
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"Workflow": filter,
		"WorkflowScopeReference": entql.FieldEQ("workflow_id", value.ID),
		"WorkflowAgency":         entql.FieldEQ("workflow_id", value.ID)},
		map[string]bool{"Workflow": expected == 0, "WorkflowScopeReference": true, "WorkflowAgency": true})
	if err != nil {
		return err
	}
	if expected == 0 {
		if err := client.Workflow.Create().SetID(value.ID).SetRevision(1).SetName(value.Name).
			SetNillableNarrowingKind(narrowingKind(value.Scope)).SetEnabled(value.Enabled).
			SetDeleted(value.Deleted).SetNillableActiveVersionID(value.ActiveVersionID).
			SetCreatedAt(now).SetUpdatedAt(now).Exec(scope); err != nil {
			return err
		}
	} else {
		update := client.Workflow.Update().Where(workflow.ID(value.ID), workflow.Revision(expected)).
			SetRevision(next).SetName(value.Name).SetNillableNarrowingKind(narrowingKind(value.Scope)).
			SetEnabled(value.Enabled).SetDeleted(value.Deleted).SetUpdatedAt(now)
		if value.Scope.Narrowing == nil {
			update.ClearNarrowingKind()
		}
		if value.ActiveVersionID == nil {
			update.ClearActiveVersionID()
		} else {
			update.SetActiveVersionID(*value.ActiveVersionID)
		}
		count, err := update.Save(scope)
		if err != nil {
			return err
		}
		if count != 1 {
			return authz.ErrRevisionConflict
		}
	}
	return saveScope(scope, client, value)
}

// narrowingKind returns the narrowing kind to persist, nil for a Workflow without narrowing.
func narrowingKind(scope processing.WorkflowScope) *workflow.NarrowingKind {
	if scope.Narrowing == nil {
		return nil
	}
	kind := workflow.NarrowingKind(scope.Narrowing.Kind)
	return &kind
}

// saveScope replaces the Workflow's Agency assignments and narrowing references with the scope.
func saveScope(ctx context.Context, client *ent.Client, value processing.WorkflowConfiguration) error {
	if _, err := client.WorkflowAgency.Delete().Where(workflowagency.WorkflowID(value.ID)).Exec(ctx); err != nil {
		return err
	}
	for _, agency := range value.Scope.Agencies {
		if err := client.WorkflowAgency.Create().SetWorkflowID(value.ID).SetAgencyID(agency).Exec(ctx); err != nil {
			return err
		}
	}
	if _, err := client.WorkflowScopeReference.Delete().Where(workflowscopereference.WorkflowID(value.ID)).Exec(ctx); err != nil {
		return err
	}
	if value.Scope.Narrowing == nil {
		return nil
	}
	for _, id := range value.Scope.Narrowing.References {
		create := client.WorkflowScopeReference.Create().SetWorkflowID(value.ID)
		if value.Scope.Narrowing.Kind == "network" {
			create.SetNetworkID(id)
		} else {
			create.SetUnitID(id)
		}
		if err := create.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// SaveVersion only inserts an already prepared immutable definition; it never changes activation.
func (s *Store) SaveVersion(ctx context.Context, tx platform.TransactionContext, value processing.Version) error {
	if value.ID == uuid.Nil || value.WorkflowID == uuid.Nil || value.CreatedBy == uuid.Nil || value.CreatedAt.IsZero() {
		return authz.ErrInvalidInput
	}
	encoded, err := json.Marshal(value.Definition)
	if err != nil {
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	var detached processing.Definition
	if err := processing.Decode(encoded, &detached); err != nil {
		return err
	}
	if detached.SchemaVersion != 1 {
		return authz.ErrInvalidInput
	}
	keys := make([]string, 0, len(detached.Outputs))
	for key := range detached.Outputs {
		keys = append(keys, key)
	}
	if err := s.requireOutputSchemas(ctx, tx, keys); err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, nil, map[string]bool{"WorkflowVersion": true})
	if err != nil {
		return err
	}
	return client.WorkflowVersion.Create().SetID(value.ID).SetWorkflowID(value.WorkflowID).
		SetDefinition(detached).SetCreatedAt(value.CreatedAt).SetCreatedBy(value.CreatedBy).Exec(scope)
}

// RecordChange increments Processing and shared configuration revisions with the safe existing audit.
func (s *Store) RecordChange(ctx context.Context, tx platform.TransactionContext, actor, target uuid.UUID, action string, now time.Time) error {
	current, err := s.LockRevisionForChange(ctx, tx)
	if err != nil {
		return err
	}
	next, err := commands.NextRevision(current, current)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"ProcessingControl": entql.FieldEQ("id", 1)}, nil)
	if err != nil {
		return err
	}
	if err := client.ProcessingControl.UpdateOneID(1).SetRevision(next).Exec(scope); err != nil {
		return err
	}
	owner, err := identitystore.New(s.primary)
	if err != nil {
		return err
	}
	return owner.RecordConfigurationChange(ctx, tx, actor, target, action, now)
}

var _ processing.ConfigurationRepository = (*Store)(nil)
