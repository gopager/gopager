package processingstore

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// requireOutputSchemas fails with INVALID_INPUT when one of the Attribute schema keys no longer
// exists. It first locks the catalog row FOR SHARE: deleting a schema updates that row, so a
// concurrent deletion either committed before this check, which then sees it, or waits until
// this transaction ends. Without the lock, a version compiled while its schema existed could
// be saved or activated after the schema was deleted, because deletion only looks at active
// versions.
//
// keys are the schema keys of a definition's outputs; none is a no-op.
func (s *Store) requireOutputSchemas(ctx context.Context, tx platform.TransactionContext, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `SELECT 1 FROM attribute_catalog WHERE id = 1 FOR SHARE`); err != nil {
		return err
	}
	rows, err := executor.QueryContext(ctx, `SELECT k FROM unnest($1::text[]) AS k
		WHERE NOT EXISTS (SELECT 1 FROM attribute_schemas WHERE key = k) ORDER BY k`, keys)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	var missing []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return err
		}
		missing = append(missing, key)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(missing) > 0 {
		return platform.NewFailure(platform.CodeInvalidInput, fmt.Errorf("attribute schema %q does not exist", missing[0]))
	}
	return nil
}

// requireVersionOutputSchemas applies requireOutputSchemas to the outputs of a stored version.
func (s *Store) requireVersionOutputSchemas(ctx context.Context, tx platform.TransactionContext, version uuid.UUID) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	rows, err := executor.QueryContext(ctx, `SELECT jsonb_object_keys(CASE WHEN jsonb_typeof(definition->'outputs') = 'object'
		THEN definition->'outputs' ELSE '{}'::jsonb END) FROM processing_workflow_versions WHERE id = $1`, version)
	if err != nil {
		return err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return err
		}
		keys = append(keys, key)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return s.requireOutputSchemas(ctx, tx, keys)
}
