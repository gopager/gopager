package readstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

var _ readmodel.LiveStore = (*Store)(nil)

// CurrentTxID returns the highest transaction ID a position can carry so far. A page position
// holds the oldest in-flight transaction minus one, which never exceeds the next unassigned ID
// minus one. Reading the snapshot bound does not assign a transaction ID to the reader.
func (s *Store) CurrentTxID(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var head int64
	err = executor.QueryRowContext(ctx, `SELECT pg_snapshot_xmax(pg_current_snapshot())::text::bigint - 1`).Scan(&head)
	return head, err
}

// UnitContexts returns the current Agency and Unit collections of the listed Units, deleted
// ones included, because a fact may name a Unit that was deleted afterwards.
func (s *Store) UnitContexts(ctx context.Context, tx platform.TransactionContext,
	ids []uuid.UUID) (map[uuid.UUID]readmodel.UnitContext, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT u.id, u.agency_id, m.collection_id
		FROM units u LEFT JOIN unit_collection_memberships m ON m.unit_id = u.id
		WHERE u.id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	contexts := make(map[uuid.UUID]readmodel.UnitContext, len(ids))
	for rows.Next() {
		var id, agency uuid.UUID
		var collection uuid.NullUUID
		if err := rows.Scan(&id, &agency, &collection); err != nil {
			return nil, err
		}
		context := contexts[id]
		context.AgencyID = agency
		if collection.Valid {
			context.Collections = append(context.Collections, collection.UUID)
		}
		contexts[id] = context
	}
	return contexts, rows.Err()
}

// NetworkAgencies returns the Agency of each listed Network, deleted ones included.
func (s *Store) NetworkAgencies(ctx context.Context, tx platform.TransactionContext,
	ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id, agency_id FROM networks WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	agencies := make(map[uuid.UUID]uuid.UUID, len(ids))
	for rows.Next() {
		var id, agency uuid.UUID
		if err := rows.Scan(&id, &agency); err != nil {
			return nil, err
		}
		agencies[id] = agency
	}
	return agencies, rows.Err()
}

// TransmissionMessages returns the current Message of each listed Transmission. A merge moves
// Transmissions to the surviving Message, so this resolves chains of merges to the final one.
func (s *Store) TransmissionMessages(ctx context.Context, tx platform.TransactionContext,
	ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id, message_id FROM transmissions WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	messages := make(map[uuid.UUID]uuid.UUID, len(ids))
	for rows.Next() {
		var id, message uuid.UUID
		if err := rows.Scan(&id, &message); err != nil {
			return nil, err
		}
		messages[id] = message
	}
	return messages, rows.Err()
}
