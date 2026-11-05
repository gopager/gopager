// Package assignmentstore implements current Address rules and durable correction scopes.
package assignmentstore

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/gopager/gopager/internal/persistence/ent/permissions"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// Store persists Address rules, target suppressions and the durable correction scopes that
// re-resolve stored Message targets after Assignment changes. It holds no transaction of
// its own: every method runs in the caller's active primary transaction.
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
	if (reason != "rule-change" && reason != "unit-deleted" && reason != "suppression-change") ||
		(unitID != nil && *unitID == uuid.Nil) {
		return 0, authz.ErrInvalidInput
	}
	revision, err := s.LockRevision(ctx, tx)
	if err != nil {
		return 0, err
	}
	if revision == math.MaxInt64 {
		return 0, platform.NewFailure(platform.CodeResourceLimit, errors.New("Assignment revision exhausted"))
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return 0, err
	}
	access, err := identitystore.New(s.primary)
	if err != nil {
		return 0, err
	}
	accessRevision, err := access.LockAccess(ctx, tx)
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
	updated, err := executor.ExecContext(ctx, `UPDATE assignment_control SET revision=$2
		WHERE id=1 AND revision=$1`, revision, revision+1)
	if err != nil {
		return 0, err
	}
	count, err := updated.RowsAffected()
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, authz.ErrRevisionConflict
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	highWaterAt, highWaterID, err := correctionHighWater(ctx, executor, ids, unitID)
	if err != nil {
		return 0, err
	}
	encodedNetworks, err := json.Marshal(ids)
	if err != nil {
		return 0, err
	}
	textIDs := make([]string, len(ids))
	for i, id := range ids {
		textIDs[i] = id.String()
	}
	if _, err := executor.ExecContext(ctx, `UPDATE message_contributions SET ready=false
		WHERE message_id IN (
			SELECT DISTINCT t.message_id FROM transmissions t
			WHERE (cardinality($1::uuid[])=0 OR t.network_id=ANY($1::uuid[]))
			AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
				WHERE target.transmission_id=t.id AND target.unit_id=$2)))`, ids, unitID); err != nil {
		return 0, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE messages SET revision=revision+1,publication_status='pending'
		WHERE id IN (
			SELECT DISTINCT t.message_id FROM transmissions t
			WHERE (cardinality($1::uuid[])=0 OR t.network_id=ANY($1::uuid[]))
			AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
				WHERE target.transmission_id=t.id AND target.unit_id=$2)))`, ids, unitID); err != nil {
		return 0, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE transmissions SET processing_status='pending',publication_status='pending'
		WHERE message_id IN (
			SELECT DISTINCT t.message_id FROM transmissions t
			WHERE (cardinality($1::uuid[])=0 OR t.network_id=ANY($1::uuid[]))
			AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
				WHERE target.transmission_id=t.id AND target.unit_id=$2)))`, ids, unitID); err != nil {
		return 0, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes SET state='superseded',lease_until=NULL
		WHERE state IN ('queued','running','failed') AND assignment_revision < $1
		AND (jsonb_array_length(network_ids)=0 OR cardinality($2::text[])=0 OR EXISTS (
			SELECT 1 FROM jsonb_array_elements_text(network_ids) AS old_network(value)
			WHERE old_network.value=ANY($2::text[])))
		AND ($3::uuid IS NULL OR unit_id IS NULL OR unit_id=$3)`,
		revision+1, textIDs, unitID); err != nil {
		return 0, err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE assignment_pending_scopes scope SET
		assignment_revision=$1,state='queued',attempts=0,lease_until=NULL,
		fencing_token=fencing_token+1,cursor_accepted_at=NULL,cursor_transmission_id=NULL,
		failure_code=NULL,
		high_water_accepted_at=(SELECT t.accepted_at FROM transmissions t
			WHERE (jsonb_array_length(scope.network_ids)=0 OR scope.network_ids @> jsonb_build_array(t.network_id::text))
			AND (scope.unit_id IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
				WHERE target.transmission_id=t.id AND target.unit_id=scope.unit_id))
			ORDER BY t.accepted_at DESC,t.id DESC LIMIT 1),
		high_water_transmission_id=(SELECT t.id FROM transmissions t
			WHERE (jsonb_array_length(scope.network_ids)=0 OR scope.network_ids @> jsonb_build_array(t.network_id::text))
			AND (scope.unit_id IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
				WHERE target.transmission_id=t.id AND target.unit_id=scope.unit_id))
			ORDER BY t.accepted_at DESC,t.id DESC LIMIT 1)
		WHERE scope.state IN ('queued','running') AND scope.assignment_revision<$1
		AND NOT ((jsonb_array_length(scope.network_ids)=0 OR cardinality($2::text[])=0 OR EXISTS (
			SELECT 1 FROM jsonb_array_elements_text(scope.network_ids) AS old_network(value)
			WHERE old_network.value=ANY($2::text[])))
		AND ($3::uuid IS NULL OR scope.unit_id IS NULL OR scope.unit_id=$3))`,
		revision+1, textIDs, unitID); err != nil {
		return 0, err
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO assignment_pending_scopes
		(id,assignment_revision,network_ids,unit_id,reason,recorded_at,state,
		 high_water_accepted_at,high_water_transmission_id,access_revision)
		VALUES($1,$2,$3::jsonb,$4,$5,$6,'queued',$7,$8,$9)`,
		uuid.New(), revision+1, string(encodedNetworks), unitID, reason, now,
		highWaterAt, highWaterID, accessRevision); err != nil {
		return 0, err
	}
	return revision + 1, nil
}

func correctionHighWater(ctx context.Context, executor postgres.Executor, networks []uuid.UUID,
	unitID *uuid.UUID,
) (*time.Time, *uuid.UUID, error) {
	var accepted time.Time
	var transmission uuid.UUID
	err := executor.QueryRowContext(ctx, `SELECT t.accepted_at,t.id FROM transmissions t
		WHERE (cardinality($1::uuid[]) = 0 OR t.network_id=ANY($1::uuid[]))
		AND ($2::uuid IS NULL OR EXISTS (SELECT 1 FROM transmission_targets target
			WHERE target.transmission_id=t.id AND target.unit_id=$2))
		ORDER BY t.accepted_at DESC,t.id DESC LIMIT 1`, networks, unitID).Scan(&accepted, &transmission)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &accepted, &transmission, nil
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
