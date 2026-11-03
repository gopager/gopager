// Package assignmentstore implements current Address rules and durable correction scopes.
package assignmentstore

import (
	"context"
	"errors"
	"math"
	"time"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/addressrule"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/identitystore"
	"github.com/gopager/gopager/internal/persistence/ent/pendingassignmentscope"
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

type Store struct{ primary *postgres.Store }

// New uses the existing Primary and borrowed transaction boundary.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}

// client installs exact adapter-owned native predicates and create capabilities.
func (s *Store) client(ctx context.Context, tx platform.TransactionContext, predicates map[string]entql.P, create map[string]bool) (*ent.Client, context.Context, error) {
	driver, err := borrowed.New(s.primary, tx)
	if err != nil {
		return nil, nil, err
	}
	return ent.NewClient(ent.Driver(driver)), guard.Bind(ctx, guard.Access{
		Current: driver.Current, Predicates: predicates, Create: create,
	}), nil
}

// scopePredicate correlates each rule with its direct Unit/Network and current collections.
func scopePredicate(kind string, references []uuid.UUID) (entql.P, error) {
	values := make([]any, len(references))
	for i, id := range references {
		values[i] = id
	}
	switch kind {
	case "network":
		return entql.FieldIn("network_id", values...), nil
	case "unit":
		return entql.FieldIn("unit_id", values...), nil
	case "unitCollection":
		return entql.HasEdgeWith("unit", entql.HasEdgeWith("collections", entql.FieldIn("id", values...))), nil
	default:
		return nil, authz.ErrInvalidInput
	}
}

// plan binds the central grant/key composition to one actual rule context.
func plan(authority authz.Authority, action string, now time.Time) (entql.P, error) {
	var fields []string
	if action == "address-rule.read" {
		fields = []string{"address", "unit"}
	}
	result, err := permissions.ReadScope(authority, action, fields, permissions.Attributes{Scope: scopePredicate}, now)
	if err != nil {
		return nil, err
	}
	return result.Predicate, nil
}

// ruleDTO constructs an owner DTO, never returning an Ent record across the boundary.
func ruleDTO(row *ent.AddressRule) assignments.Rule {
	return assignments.Rule{ID: row.ID, Revision: row.Revision, NetworkID: row.NetworkID, UnitID: row.UnitID,
		Protocol: protocols.Protocol(row.Protocol), Selector: row.Selector, ValidFrom: row.ValidFrom, ValidUntil: row.ValidUntil, Enabled: row.Enabled}
}

// LockRevision locks shared configuration before Assignment state, including current Unit context.
func (s *Store) LockRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	owner, err := identitystore.New(s.primary)
	if err != nil {
		return 0, err
	}
	if _, err := owner.LockAccess(ctx, tx); err != nil {
		return 0, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AssignmentControl": entql.FieldEQ("id", 1)}, nil)
	if err != nil {
		return 0, err
	}
	row, err := client.AssignmentControl.Query().ForUpdate().Only(scope)
	if err != nil {
		return 0, err
	}
	return row.Revision, nil
}

// CurrentRules reads only the server-derived candidate Network set, including disabled configuration.
func (s *Store) CurrentRules(ctx context.Context, tx platform.TransactionContext, networks []uuid.UUID) ([]assignments.Rule, error) {
	if len(networks) == 0 {
		return []assignments.Rule{}, ctx.Err()
	}
	if len(networks) > 1024 {
		return nil, authz.ErrInvalidInput
	}
	values := make([]any, len(networks))
	for i, id := range networks {
		if id == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
		values[i] = id
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": entql.FieldIn("network_id", values...)}, nil)
	if err != nil {
		return nil, err
	}
	rows, err := client.AddressRule.Query().Order(ent.Asc(addressrule.FieldID)).All(scope)
	if err != nil {
		return nil, err
	}
	result := make([]assignments.Rule, len(rows))
	for i, row := range rows {
		result[i] = ruleDTO(row)
	}
	return result, nil
}

// ReadRule filters authority and current Unit existence before selecting the named rule.
func (s *Store) ReadRule(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, action string, id uuid.UUID, now time.Time) (assignments.Rule, error) {
	predicate, err := plan(authority, action, now)
	if errors.Is(err, authz.ErrForbidden) {
		return assignments.Rule{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.Rule{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": entql.And(predicate, entql.FieldEQ("id", id))}, nil)
	if err != nil {
		return assignments.Rule{}, err
	}
	row, err := client.AddressRule.Query().Only(scope)
	if ent.IsNotFound(err) {
		return assignments.Rule{}, authz.ErrNotFound
	}
	if err != nil {
		return assignments.Rule{}, err
	}
	return ruleDTO(row), nil
}

// ListRules applies native row/field predicates before its count and stable bounded page.
func (s *Store) ListRules(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, page units.PageInput, now time.Time) (units.Page[assignments.Rule], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[assignments.Rule]{}, err
	}
	predicate, err := plan(authority, "address-rule.read", now)
	if err != nil {
		return units.Page[assignments.Rule]{}, err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": predicate}, nil)
	if err != nil {
		return units.Page[assignments.Rule]{}, err
	}
	total, err := client.AddressRule.Query().Count(scope)
	if err != nil {
		return units.Page[assignments.Rule]{}, err
	}
	rows, err := client.AddressRule.Query().Order(ent.Asc(addressrule.FieldID)).Offset(page.Offset).Limit(page.Limit).All(scope)
	if err != nil {
		return units.Page[assignments.Rule]{}, err
	}
	result := units.Page[assignments.Rule]{Items: make([]assignments.Rule, len(rows)), Total: total}
	for i, row := range rows {
		result.Items[i] = ruleDTO(row)
	}
	return result, nil
}

// SaveRule persists only a validated candidate under native current authority and expected revision.
func (s *Store) SaveRule(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value assignments.Rule, expected int64, now time.Time) error {
	if expected > 0 {
		if _, err := commands.NextRevision(expected, expected); err != nil {
			return err
		}
	}
	if value.Validate() != nil || expected < 0 || value.Revision != expected+1 {
		return authz.ErrInvalidInput
	}
	if expected == 0 {
		client, scope, err := s.client(ctx, tx, nil, map[string]bool{"AddressRule": true})
		if err != nil {
			return err
		}
		return client.AddressRule.Create().SetID(value.ID).SetRevision(1).SetNetworkID(value.NetworkID).SetUnitID(value.UnitID).
			SetProtocol(addressrule.Protocol(value.Protocol)).SetSelector(value.Selector).
			SetNillableValidFrom(value.ValidFrom).SetNillableValidUntil(value.ValidUntil).SetEnabled(value.Enabled).Exec(scope)
	}
	predicate, err := plan(authority, "address-rule.update", now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": entql.And(predicate,
		entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", expected))}, nil)
	if err != nil {
		return err
	}
	update := client.AddressRule.Update().SetRevision(value.Revision).SetNetworkID(value.NetworkID).SetUnitID(value.UnitID).
		SetProtocol(addressrule.Protocol(value.Protocol)).SetSelector(value.Selector).SetEnabled(value.Enabled)
	if value.ValidFrom == nil {
		update.ClearValidFrom()
	} else {
		update.SetValidFrom(*value.ValidFrom)
	}
	if value.ValidUntil == nil {
		update.ClearValidUntil()
	} else {
		update.SetValidUntil(*value.ValidUntil)
	}
	updated, err := update.Save(scope)
	if err != nil {
		return err
	}
	if updated != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// DeleteRule removes current configuration only after the caller captures its correction scope.
func (s *Store) DeleteRule(ctx context.Context, tx platform.TransactionContext, authority authz.Authority, value assignments.Rule, now time.Time) error {
	predicate, err := plan(authority, "address-rule.delete", now)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": entql.And(predicate,
		entql.FieldEQ("id", value.ID), entql.FieldEQ("revision", value.Revision))}, nil)
	if err != nil {
		return err
	}
	deleted, err := client.AddressRule.Delete().Exec(scope)
	if err != nil {
		return err
	}
	if deleted != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// MarkPending conservatively invalidates entire affected Networks, preserving durable unfinished work.
func (s *Store) MarkPending(ctx context.Context, tx platform.TransactionContext, networks []uuid.UUID, unitID *uuid.UUID, reason string) (int64, error) {
	if (reason != "rule-change" && reason != "unit-deleted") || (unitID != nil && *unitID == uuid.Nil) {
		return 0, authz.ErrInvalidInput
	}
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return 0, err
	}
	if revision == math.MaxInt64 {
		return 0, platform.NewFailure(platform.CodeResourceLimit, errors.New("Assignment revision exhausted"))
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AssignmentControl": entql.And(
		entql.FieldEQ("id", 1), entql.FieldEQ("revision", revision))}, map[string]bool{"PendingAssignmentScope": true})
	if err != nil {
		return 0, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return 0, err
	}
	seen := make(map[uuid.UUID]bool, len(networks))
	ids := make([]uuid.UUID, 0, len(networks))
	for _, id := range networks {
		if id == uuid.Nil {
			return 0, authz.ErrInvalidInput
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if _, err := client.AssignmentControl.UpdateOneID(1).SetRevision(revision + 1).Save(scope); err != nil {
		return 0, err
	}
	if err := client.PendingAssignmentScope.Create().SetAssignmentRevision(revision + 1).SetNetworkIds(ids).
		SetNillableUnitID(unitID).SetReason(pendingassignmentscope.Reason(reason)).SetRecordedAt(now).Exec(scope); err != nil {
		return 0, err
	}
	return revision + 1, nil
}

// NetworksForUnit reports actual current configuration uses, including disabled rules.
func (s *Store) NetworksForUnit(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) ([]uuid.UUID, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": entql.FieldEQ("unit_id", id)}, nil)
	if err != nil {
		return nil, err
	}
	rows, err := client.AddressRule.Query().Select(addressrule.FieldNetworkID).Unique(true).All(scope)
	if err != nil {
		return nil, err
	}
	result := make([]uuid.UUID, len(rows))
	for i, row := range rows {
		result[i] = row.NetworkID
	}
	return result, nil
}

// InvalidateUnit disables its current rules and records pending correction in the same transaction.
func (s *Store) InvalidateUnit(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return err
	}
	networks, err := s.NetworksForUnit(ctx, tx, id)
	if err != nil {
		return err
	}
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AddressRule": entql.And(
		entql.FieldEQ("unit_id", id), entql.FieldEQ("enabled", true))}, nil)
	if err != nil {
		return err
	}
	overflow, err := client.AddressRule.Query().Where(addressrule.RevisionEQ(math.MaxInt64)).Exist(scope)
	if err != nil {
		return err
	}
	if overflow {
		return platform.NewFailure(platform.CodeResourceLimit, errors.New("Address-rule revision exhausted"))
	}
	if _, err := client.AddressRule.Update().SetEnabled(false).AddRevision(1).Save(scope); err != nil {
		return err
	}
	_, err = s.MarkPending(ctx, tx, networks, &id, "unit-deleted")
	return err
}

// RecordChange delegates shared authority/audit ownership rather than writing foreign tables.
func (s *Store) RecordChange(ctx context.Context, tx platform.TransactionContext, actor, target uuid.UUID, action string, now time.Time) error {
	owner, err := identitystore.New(s.primary)
	if err != nil {
		return err
	}
	return owner.RecordConfigurationChange(ctx, tx, actor, target, action, now)
}

var _ assignments.Repository = (*Store)(nil)
var _ units.RuleOwner = (*Store)(nil)
