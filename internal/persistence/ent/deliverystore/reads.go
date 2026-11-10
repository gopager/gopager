package deliverystore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// parameters numbers SQL arguments.
type parameters struct{ values []any }

func (p *parameters) add(value any) string {
	p.values = append(p.values, value)
	return fmt.Sprintf("$%d", len(p.values))
}

// clauseSQL compiles one access clause against the Delivery row d.
func clauseSQL(clause authz.AccessClause, params *parameters) string {
	var condition string
	references := clause.References
	if references == nil {
		references = []uuid.UUID{}
	}
	switch clause.Kind {
	case "all":
		condition = "TRUE"
	case "account":
		condition = "d.subscriber_id = ANY(" + params.add(references) + "::uuid[])"
	case "agency":
		condition = "d.agency_ids && " + params.add(references) + "::uuid[]"
	case "unit":
		condition = "d.unit_ids && " + params.add(references) + "::uuid[]"
	case "unitCollection":
		condition = "EXISTS (SELECT 1 FROM unit_collection_memberships m WHERE m.unit_id = ANY(d.unit_ids) AND m.collection_id = ANY(" +
			params.add(references) + "::uuid[]))"
	default:
		condition = "FALSE"
	}
	if clause.Cutoff != nil {
		condition = "(" + condition + " AND d.age_anchor >= " + params.add(clause.Cutoff.UTC()) + ")"
	}
	return condition
}

// anyClause ORs the clauses; no clause is FALSE.
func anyClause(clauses []authz.AccessClause, params *parameters) string {
	if len(clauses) == 0 {
		return "FALSE"
	}
	parts := make([]string, len(clauses))
	for i, clause := range clauses {
		parts[i] = clauseSQL(clause, params)
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// accessSQL compiles an action's owner and key clauses into one row predicate, the same
// decision authz.ActionAccess.AllowsDelivery makes for one Delivery.
func accessSQL(access authz.ActionAccess, params *parameters) string {
	condition := anyClause(access.Owner, params)
	if !access.Restricted {
		return condition
	}
	condition += " AND " + anyClause(access.Key, params)
	if access.KeyCutoff != nil {
		condition += " AND d.age_anchor >= " + params.add(access.KeyCutoff.UTC())
	}
	return condition
}

// ListDeliveries implements delivery.Store; authorization is part of the statement, before
// ordering and paging.
func (s *Store) ListDeliveries(ctx context.Context, tx platform.TransactionContext,
	query delivery.DeliveryListQuery) ([]delivery.DeliveryRecord, int, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return nil, 0, err
	}
	params := &parameters{}
	where := accessSQL(query.Access, params)
	if query.SubscriptionID != nil {
		where += " AND d.subscription_id = " + params.add(*query.SubscriptionID)
	}
	if query.State != "" {
		where += " AND d.state = " + params.add(query.State)
	}
	var total int
	if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM deliveries d WHERE `+where, params.values...).Scan(&total); err != nil {
		return nil, 0, err
	}
	offset, limit := params.add(query.Offset), params.add(query.Limit)
	rows, err := executor.QueryContext(ctx, `SELECT `+deliveryColumns+` FROM deliveries d WHERE `+where+
		` ORDER BY d.created_at DESC, d.id DESC OFFSET `+offset+` LIMIT `+limit, params.values...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	records := []delivery.DeliveryRecord{}
	for rows.Next() {
		record, err := scanDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		records = append(records, record)
	}
	return records, total, rows.Err()
}

// UnitCollections implements delivery.Store.
func (s *Store) UnitCollections(ctx context.Context, tx platform.TransactionContext, units []uuid.UUID) ([]uuid.UUID, error) {
	if len(units) == 0 {
		return nil, nil
	}
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT DISTINCT collection_id FROM unit_collection_memberships
		WHERE unit_id = ANY($1::uuid[]) ORDER BY collection_id`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var collections []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		collections = append(collections, id)
	}
	return collections, rows.Err()
}

// UnitParameters implements delivery.Store.
func (s *Store) UnitParameters(ctx context.Context, tx platform.TransactionContext, instance uuid.UUID,
	units []uuid.UUID) (map[uuid.UUID]map[string]processing.Value, error) {
	result := map[uuid.UUID]map[string]processing.Value{}
	if len(units) == 0 {
		return result, nil
	}
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT unit_id, key, value::text FROM integration_unit_parameters
		WHERE instance_id = $1 AND unit_id = ANY($2::uuid[])`, instance, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var unit uuid.UUID
		var key, encoded string
		if err := rows.Scan(&unit, &key, &encoded); err != nil {
			return nil, err
		}
		var value processing.Value
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil, platform.NewFailure(platform.CodeInternalError, err)
		}
		if result[unit] == nil {
			result[unit] = map[string]processing.Value{}
		}
		result[unit][key] = value
	}
	return result, rows.Err()
}

// Gateway implements delivery.Store. A deleted Gateway is not found.
func (s *Store) Gateway(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (delivery.GatewayView, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return delivery.GatewayView{}, err
	}
	var view delivery.GatewayView
	var agencies string
	err = executor.QueryRowContext(ctx, `SELECT g.id, g.name,
		coalesce(array_to_json(array_agg(DISTINCT n.agency_id) FILTER (WHERE n.agency_id IS NOT NULL)), '[]')::text
		FROM gateways g LEFT JOIN gateway_networks gn ON gn.gateway_id = g.id LEFT JOIN networks n ON n.id = gn.network_id
		WHERE g.id = $1 AND NOT g.deleted GROUP BY g.id`, id).Scan(&view.ID, &view.Name, &agencies)
	if err != nil {
		return delivery.GatewayView{}, notFound(err)
	}
	view.AgencyIDs, err = uuidList(agencies)
	return view, err
}

// DomainEvent implements delivery.Store.
func (s *Store) DomainEvent(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (json.RawMessage, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	var envelope []byte
	if err := executor.QueryRowContext(ctx, `SELECT envelope FROM domain_events WHERE id = $1`, id).Scan(&envelope); err != nil {
		return nil, notFound(err)
	}
	return envelope, nil
}

// CurrentMessage implements delivery.Store.
func (s *Store) CurrentMessage(ctx context.Context, tx platform.TransactionContext, message, transmission uuid.UUID) (uuid.UUID, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return uuid.Nil, err
	}
	var current uuid.NullUUID
	if err := executor.QueryRowContext(ctx, `SELECT coalesce((SELECT id FROM messages WHERE id = $1),
		(SELECT message_id FROM transmissions WHERE id = $2))`, message, transmission).Scan(&current); err != nil {
		return uuid.Nil, err
	}
	if !current.Valid {
		return uuid.Nil, authz.ErrNotFound
	}
	return current.UUID, nil
}

// MessageExists implements delivery.Store.
func (s *Store) MessageExists(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (bool, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return false, err
	}
	var exists bool
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM messages WHERE id = $1)`, id).Scan(&exists)
	return exists, err
}

// expiredSweepLimit bounds how many expired windows of other buckets one admitted send deletes.
// Each admitted send adds at most one row per bucket, so the sweep keeps the table proportional
// to the current minute's subscribers and instances.
const expiredSweepLimit = 100

// TakeSendAttempt implements delivery.Store. The upsert increments only below the limit. In the
// same transaction an admitted send deletes the older windows of its own bucket and up to
// expiredSweepLimit expired windows of any bucket, so buckets that never return (deleted
// subscribers, removed instances) do not stay behind; rows locked by a concurrent sweep are
// skipped.
func (s *Store) TakeSendAttempt(ctx context.Context, tx platform.TransactionContext, bucket [32]byte, window time.Time,
	limit int) (bool, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return false, err
	}
	rows, err := executor.QueryContext(ctx, `INSERT INTO delivery_admissions (bucket, window_start, count) VALUES ($1, $2, 1)
		ON CONFLICT (bucket, window_start) DO UPDATE SET count = delivery_admissions.count + 1
		WHERE delivery_admissions.count < $3 RETURNING count`, bucket[:], window.UTC(), limit)
	if err != nil {
		return false, err
	}
	taken := rows.Next()
	rows.Close()
	if err := rows.Err(); err != nil {
		return false, err
	}
	if !taken {
		return false, nil
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM delivery_admissions WHERE bucket = $1 AND window_start < $2`,
		bucket[:], window.UTC()); err != nil {
		return false, err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM delivery_admissions WHERE (bucket, window_start) IN (
		SELECT bucket, window_start FROM delivery_admissions WHERE window_start < $1
		ORDER BY window_start LIMIT $2 FOR UPDATE SKIP LOCKED)`, window.UTC(), expiredSweepLimit); err != nil {
		return false, err
	}
	return true, nil
}

// ReleaseSendAttempt implements delivery.Store.
func (s *Store) ReleaseSendAttempt(ctx context.Context, tx platform.TransactionContext, bucket [32]byte, window time.Time) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE delivery_admissions SET count = count - 1
		WHERE bucket = $1 AND window_start = $2 AND count > 1`, bucket[:], window.UTC()); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM delivery_admissions WHERE bucket = $1 AND window_start = $2 AND count = 1`,
		bucket[:], window.UTC())
	return err
}

// DeleteAccountData implements delivery.Store: the account's Subscriptions and the Deliveries
// and attempts it is the subscriber of. Admission windows are keyed by a hash and are swept by
// later admitted sends once their minute has passed.
func (s *Store) DeleteAccountData(ctx context.Context, tx platform.TransactionContext, account uuid.UUID) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM deliveries WHERE subscriber_id = $1`, account); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `DELETE FROM subscriptions WHERE subscriber_id = $1`, account)
	return err
}
