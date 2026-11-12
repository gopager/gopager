package pluginconfigstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ReadSecrets implements processing.SecretStore: it returns the rows that exist for ids.
func (s *Store) ReadSecrets(ctx context.Context, tx platform.TransactionContext,
	ids []uuid.UUID) (map[uuid.UUID]processing.StoredSecret, error) {
	rows := map[uuid.UUID]processing.StoredSecret{}
	if len(ids) == 0 {
		return rows, nil
	}
	executor, err := s.executor(tx)
	if err != nil {
		return nil, err
	}
	result, err := executor.QueryContext(ctx, `SELECT id, instance_id, owner_id, key_id, nonce, ciphertext, created_at
		FROM configuration_secrets WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer result.Close()
	for result.Next() {
		var row processing.StoredSecret
		if err := result.Scan(&row.ID, &row.InstanceID, &row.OwnerID, &row.KeyID, &row.Nonce, &row.Ciphertext,
			&row.CreatedAt); err != nil {
			return nil, err
		}
		rows[row.ID] = row
	}
	return rows, result.Err()
}

// InsertSecrets stores new sealed rows in the caller's transaction; the store assigns CreatedAt.
// Rows are immutable, so an existing ID is an error.
func (s *Store) InsertSecrets(ctx context.Context, tx platform.TransactionContext, rows []processing.StoredSecret) error {
	if len(rows) == 0 {
		return nil
	}
	executor, err := s.executor(tx)
	if err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := executor.ExecContext(ctx, `INSERT INTO configuration_secrets
			(id, instance_id, owner_id, key_id, nonce, ciphertext, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			row.ID, row.InstanceID, row.OwnerID, row.KeyID, row.Nonce, row.Ciphertext, now); err != nil {
			return referenceGone(err)
		}
	}
	return nil
}
