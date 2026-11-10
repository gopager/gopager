package deliverystore

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/delivery"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
)

const subscriptionColumns = `s.id, s.revision, s.kind, s.subscriber_id, s.created_by, s.state,
	array_to_json(s.event_types)::text, s.filter::text, s.layout_id, s.split_by_unit, s.max_age_seconds,
	s.created_at, s.updated_at`

// scanSubscription reads one row of subscriptionColumns.
func scanSubscription(row interface{ Scan(...any) error }) (delivery.Subscription, error) {
	var subscription delivery.Subscription
	var types string
	var filter sql.NullString
	var layout uuid.NullUUID
	var maxAge sql.NullInt64
	if err := row.Scan(&subscription.ID, &subscription.Revision, &subscription.Kind, &subscription.SubscriberID,
		&subscription.CreatedBy, &subscription.State, &types, &filter, &layout, &subscription.SplitByUnit, &maxAge,
		&subscription.CreatedAt, &subscription.UpdatedAt); err != nil {
		return delivery.Subscription{}, err
	}
	if err := json.Unmarshal([]byte(types), &subscription.EventTypes); err != nil {
		return delivery.Subscription{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	if filter.Valid {
		var predicate processing.Predicate
		if err := json.Unmarshal([]byte(filter.String), &predicate); err != nil {
			return delivery.Subscription{}, platform.NewFailure(platform.CodeInternalError, err)
		}
		subscription.Filter = &predicate
	}
	if layout.Valid {
		id := layout.UUID
		subscription.LayoutID = &id
	}
	if maxAge.Valid {
		seconds := int(maxAge.Int64)
		subscription.MaxAgeSeconds = &seconds
	}
	subscription.CreatedAt, subscription.UpdatedAt = subscription.CreatedAt.UTC(), subscription.UpdatedAt.UTC()
	return subscription, nil
}

// withTargets loads the targets of the Subscriptions in position order.
func withTargets(ctx context.Context, executor postgres.Executor, subscriptions []delivery.Subscription) error {
	if len(subscriptions) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(subscriptions))
	index := make(map[uuid.UUID]int, len(subscriptions))
	for i, subscription := range subscriptions {
		ids[i], index[subscription.ID] = subscription.ID, i
		subscriptions[i].Targets = []delivery.TargetReference{}
	}
	rows, err := executor.QueryContext(ctx, `SELECT subscription_id, instance_id, capability_kind, instance_revision, inputs::text
		FROM subscription_targets WHERE subscription_id = ANY($1::uuid[]) ORDER BY subscription_id, position`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var owner uuid.UUID
		var target delivery.TargetReference
		var inputs string
		if err := rows.Scan(&owner, &target.InstanceID, &target.CapabilityKind, &target.Revision, &inputs); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(inputs), &target.Inputs); err != nil {
			return platform.NewFailure(platform.CodeInternalError, err)
		}
		i := index[owner]
		subscriptions[i].Targets = append(subscriptions[i].Targets, target)
	}
	return rows.Err()
}

// subscriptionValues returns the encoded optional columns of a Subscription.
func subscriptionValues(subscription delivery.Subscription) (filter any, layout any, maxAge any, err error) {
	if subscription.Filter != nil {
		encoded, err := json.Marshal(subscription.Filter)
		if err != nil {
			return nil, nil, nil, authz.ErrInvalidInput
		}
		filter = string(encoded)
	}
	if subscription.LayoutID != nil {
		layout = *subscription.LayoutID
	}
	if subscription.MaxAgeSeconds != nil {
		maxAge = *subscription.MaxAgeSeconds
	}
	return filter, layout, maxAge, nil
}

// insertTargets stores the targets of a Subscription.
func insertTargets(ctx context.Context, executor postgres.Executor, subscription delivery.Subscription) error {
	for position, target := range subscription.Targets {
		if target.Inputs == nil {
			target.Inputs = map[string]processing.Value{}
		}
		inputs, err := json.Marshal(target.Inputs)
		if err != nil {
			return authz.ErrInvalidInput
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO subscription_targets
			(subscription_id, position, instance_id, capability_kind, instance_revision, inputs) VALUES ($1, $2, $3, $4, $5, $6)`,
			subscription.ID, position, target.InstanceID, target.CapabilityKind, target.Revision, string(inputs)); err != nil {
			return err
		}
	}
	return nil
}

// InsertSubscription implements delivery.Store.
func (s *Store) InsertSubscription(ctx context.Context, tx platform.TransactionContext, subscription delivery.Subscription) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	filter, layout, maxAge, err := subscriptionValues(subscription)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO subscriptions (id, revision, kind, subscriber_id, created_by, state,
		event_types, filter, layout_id, split_by_unit, max_age_seconds, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		subscription.ID, subscription.Revision, subscription.Kind, subscription.SubscriberID, subscription.CreatedBy,
		subscription.State, subscription.EventTypes, filter, layout, subscription.SplitByUnit, maxAge,
		subscription.CreatedAt.UTC(), subscription.UpdatedAt.UTC()); err != nil {
		return err
	}
	return insertTargets(ctx, executor, subscription)
}

// Subscription implements delivery.Store.
func (s *Store) Subscription(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, lock bool) (delivery.Subscription, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return delivery.Subscription{}, err
	}
	query := `SELECT ` + subscriptionColumns + ` FROM subscriptions s WHERE s.id = $1`
	if lock {
		query += ` FOR UPDATE`
	}
	subscription, err := scanSubscription(executor.QueryRowContext(ctx, query, id))
	if err != nil {
		return delivery.Subscription{}, notFound(err)
	}
	list := []delivery.Subscription{subscription}
	if err := withTargets(ctx, executor, list); err != nil {
		return delivery.Subscription{}, err
	}
	return list[0], nil
}

// ListSubscriptions implements delivery.Store.
func (s *Store) ListSubscriptions(ctx context.Context, tx platform.TransactionContext,
	query delivery.SubscriptionListQuery) ([]delivery.Subscription, int, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return nil, 0, err
	}
	owner := uuid.NullUUID{UUID: query.Owner, Valid: query.Owner != uuid.Nil}
	subscribers := query.GlobalSubscribers
	if subscribers == nil {
		subscribers = []uuid.UUID{}
	}
	const where = ` FROM subscriptions s WHERE (s.kind = 'personal' AND s.subscriber_id = $1)
		OR (s.kind = 'global' AND ($2 OR s.subscriber_id = ANY($3::uuid[])))`
	var total int
	if err := executor.QueryRowContext(ctx, `SELECT count(*)`+where, owner, query.Global, subscribers).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+subscriptionColumns+where+` ORDER BY s.created_at, s.id OFFSET $4 LIMIT $5`,
		owner, query.Global, subscribers, query.Offset, query.Limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := []delivery.Subscription{}
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	rows.Close()
	return items, total, withTargets(ctx, executor, items)
}

// UpdateSubscription implements delivery.Store.
func (s *Store) UpdateSubscription(ctx context.Context, tx platform.TransactionContext, subscription delivery.Subscription,
	expected int64) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	filter, layout, maxAge, err := subscriptionValues(subscription)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `UPDATE subscriptions SET revision = $2, state = $3, event_types = $4, filter = $5,
		layout_id = $6, split_by_unit = $7, max_age_seconds = $8, updated_at = $9 WHERE id = $1 AND revision = $10`,
		subscription.ID, subscription.Revision, subscription.State, subscription.EventTypes, filter, layout,
		subscription.SplitByUnit, maxAge, subscription.UpdatedAt.UTC(), expected)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return authz.ErrRevisionConflict
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM subscription_targets WHERE subscription_id = $1`, subscription.ID); err != nil {
		return err
	}
	return insertTargets(ctx, executor, subscription)
}

// DeleteSubscription implements delivery.Store.
func (s *Store) DeleteSubscription(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) error {
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = $1 AND revision = $2`, id, expected)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return authz.ErrRevisionConflict
	}
	return nil
}

// ActiveSubscriptions implements delivery.Store.
func (s *Store) ActiveSubscriptions(ctx context.Context, tx platform.TransactionContext, eventType string,
	recordedAt time.Time) ([]delivery.Subscription, error) {
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+subscriptionColumns+` FROM subscriptions s
		WHERE s.state = 'active' AND s.event_types @> ARRAY[$1]::text[] AND s.created_at <= $2
		ORDER BY s.id FOR SHARE`, eventType, recordedAt.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []delivery.Subscription{}
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, subscription)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return items, withTargets(ctx, executor, items)
}
