// Package assignmentstore implements current Address rules and durable correction scopes.
package assignmentstore

import (
	"context"
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
	case "agency":
		return entql.HasEdgeWith("network", entql.FieldIn("agency_id", values...)), nil
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
// LockRevision takes the shared Assignment-control lock (matching, correction batches,
// reads) and returns the current Assignment revision. Rule, Unit-deletion and Suppression
// changes take the exclusive lock through LockRevisionForChange.
func (s *Store) LockRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	return s.lockRevision(ctx, tx, false)
}

// LockRevisionForChange takes the exclusive Assignment-control lock for a command that
// changes current rules or Suppressions and will bump the Assignment revision.
func (s *Store) LockRevisionForChange(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	return s.lockRevision(ctx, tx, true)
}

func (s *Store) lockRevision(ctx context.Context, tx platform.TransactionContext, exclusive bool) (int64, error) {
	client, scope, err := s.client(ctx, tx, map[string]entql.P{"AssignmentControl": entql.FieldEQ("id", 1)}, nil)
	if err != nil {
		return 0, err
	}
	query := client.AssignmentControl.Query()
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

// accessRevision reads the current access revision without taking the access lock; the
// correction scope records it as context, not as a serialization point.
func (s *Store) accessRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var revision int64
	if err := executor.QueryRowContext(ctx, `SELECT access_revision FROM identity_control WHERE id=1`).Scan(&revision); err != nil {
		return 0, err
	}
	return revision, nil
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

// MarkPending bumps the Assignment revision under the exclusive Assignment lock and records one
// queued correction scope per distinct selector. The cache rows of Messages with affected targets
// are marked correcting until a correction batch rebuilds them. Scopes run independently: a
// target re-resolved by a newer scope carries a newer assignment revision and is skipped by older
// scopes, so nothing is superseded or reset.
func (s *Store) MarkPending(ctx context.Context, tx platform.TransactionContext, scopes []assignments.CorrectionScope, reason string) (int64, error) {
	if (reason != "rule-change" && reason != "unit-deleted" && reason != "suppression-change") || len(scopes) == 0 {
		return 0, authz.ErrInvalidInput
	}
	for _, scope := range scopes {
		if err := scope.Validate(); err != nil {
			return 0, err
		}
	}
	revision, err := s.LockRevisionForChange(ctx, tx)
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
	accessRevision, err := s.accessRevision(ctx, tx)
	if err != nil {
		return 0, err
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
	seen := make(map[string]bool, len(scopes))
	for _, scope := range scopes {
		key, err := json.Marshal(scope)
		if err != nil {
			return 0, err
		}
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		if err := insertCorrectionScope(ctx, executor, uuid.New(), revision+1, scope, reason, now, accessRevision); err != nil {
			return 0, err
		}
	}
	return revision + 1, nil
}

// insertCorrectionScope records one queued scope and marks the cache rows of the Messages whose
// targets it will re-resolve as correcting.
func insertCorrectionScope(ctx context.Context, executor postgres.Executor, id uuid.UUID, revision int64,
	scope assignments.CorrectionScope, reason string, now time.Time, accessRevision int64) error {
	args := scopeArgs(scope, revision)
	if _, err := executor.ExecContext(ctx, `INSERT INTO assignment_pending_scopes
		(network_id,protocol,address_from,address_to,address_text,valid_from,valid_until,unit_id,transmission_id,target_position,
		 assignment_revision,id,reason,recorded_at,state,access_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'queued',$15)`,
		append(args, id, reason, now, accessRevision)...); err != nil {
		return err
	}
	_, err := executor.ExecContext(ctx, `UPDATE message_units SET correction_status='correcting'
		WHERE message_id IN (SELECT t.message_id FROM transmission_targets target
			JOIN transmissions t ON t.id=target.transmission_id WHERE `+inScopeSQL+`)`, args...)
	return err
}

// inScopeSQL selects the targets of a correction scope that still carry an older assignment
// revision; it reads the alias target and the positional parameters of scopeArgs.
const inScopeSQL = `target.assignment_revision < $11 AND (
	($1::uuid IS NOT NULL AND target.network_id=$1 AND target.protocol=$2
		AND (($5::varchar IS NOT NULL AND target.address_text=$5)
			OR ($3::bigint IS NOT NULL AND target.address_number BETWEEN $3 AND $4))
		AND ($6::timestamptz IS NULL OR target.observed_at>=$6)
		AND ($7::timestamptz IS NULL OR target.observed_at<$7))
	OR ($8::uuid IS NOT NULL AND target.unit_id=$8)
	OR ($9::uuid IS NOT NULL AND target.transmission_id=$9 AND target.position=$10))`

// scopeArgs returns the eleven positional parameters of inScopeSQL: the scope's selector columns
// in table order and the assignment revision below which a target counts as stale.
func scopeArgs(scope assignments.CorrectionScope, revision int64) []any {
	args := make([]any, 11)
	args[0], args[1], args[2], args[3], args[4] = scope.NetworkID, nil, nil, nil, scope.AddressText
	if scope.NetworkID != nil {
		args[1] = string(scope.Protocol)
	}
	if scope.AddressFrom != nil && scope.AddressTo != nil {
		args[2], args[3] = int64(min(*scope.AddressFrom, math.MaxInt64)), int64(min(*scope.AddressTo, math.MaxInt64))
	}
	args[5], args[6], args[7], args[8], args[9], args[10] = scope.ValidFrom, scope.ValidUntil, scope.UnitID,
		scope.TransmissionID, scope.TargetPosition, revision
	return args
}

// InvalidateUnit disables its current rules and records pending correction in the same transaction.
func (s *Store) InvalidateUnit(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	if _, err := s.LockRevisionForChange(ctx, tx); err != nil {
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
	_, err = s.MarkPending(ctx, tx, []assignments.CorrectionScope{assignments.UnitScope(id)}, "unit-deleted")
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
