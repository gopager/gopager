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
	if err := s.lockCatalog(ctx, tx); err != nil {
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
	if err := s.requireOutputSchemas(ctx, tx, keys); err != nil {
		return err
	}
	// The version was compiled against the schemas of its save time. A schema created after it
	// replaced a deleted one of the same key, possibly with other field types, so the version
	// must be saved again against the current schema before it can run.
	var recreated string
	if err := executor.QueryRowContext(ctx, `SELECT coalesce(min(a.key), '') FROM attribute_schemas a
		JOIN processing_workflow_versions v ON v.id = $1
		WHERE a.key = ANY($2::text[]) AND a.created_at > v.created_at`, version, keys).Scan(&recreated); err != nil {
		return err
	}
	if recreated != "" {
		return platform.NewFailure(platform.CodeInvalidInput,
			fmt.Errorf("attribute schema %q was re-created after the version was saved", recreated))
	}
	return nil
}

// LockedCatalogRevision implements processing.ConfigurationRepository.
func (s *Store) LockedCatalogRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var revision int64
	err = executor.QueryRowContext(ctx, `SELECT revision FROM attribute_catalog WHERE id = 1 FOR SHARE`).Scan(&revision)
	return revision, err
}

// lockCatalog locks the schema catalog row FOR SHARE, which a schema deletion's update conflicts
// with.
func (s *Store) lockCatalog(ctx context.Context, tx platform.TransactionContext) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `SELECT 1 FROM attribute_catalog WHERE id = 1 FOR SHARE`)
	return err
}

// NewOutputSchemaAgencies implements processing.ConfigurationRepository. A reference that an
// earlier version of the Workflow already holds stays valid when its schema later loses an
// Agency, so only references that no version holds yet need the author's right. A version
// holds a reference only if the current schema existed when it was saved: a schema that was
// deleted and re-created under the same key is a different schema.
func (s *Store) NewOutputSchemaAgencies(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID,
	keys []string) (map[string][]uuid.UUID, error) {
	return s.newSchemaAgencies(ctx, tx, workflow, keys, `jsonb_exists(v.definition->'outputs', k)`)
}

// NewReadSchemaAgencies implements processing.ConfigurationRepository with the rules of
// NewOutputSchemaAgencies for versions that read `target.unit.attributes.<key>…`.
func (s *Store) NewReadSchemaAgencies(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID,
	keys []string) (map[string][]uuid.UUID, error) {
	return s.newSchemaAgencies(ctx, tx, workflow, keys, `jsonb_path_exists(v.definition,
		'$.** ? (@.namespace == "target" && @.path[0] == "unit" && @.path[1] == "attributes" && @.path[2] == $key)',
		jsonb_build_object('key', k))`)
}

// newSchemaAgencies returns the Agencies of the existing schemas among keys that no version of
// the Workflow holds yet; held is the SQL condition on version v that it holds the key k.
func (s *Store) newSchemaAgencies(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID,
	keys []string, held string) (map[string][]uuid.UUID, error) {
	if len(keys) == 0 {
		return map[string][]uuid.UUID{}, nil
	}
	if err := s.lockCatalog(ctx, tx); err != nil {
		return nil, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT k FROM unnest($1::text[]) AS k WHERE NOT EXISTS (
		SELECT 1 FROM processing_workflow_versions v WHERE v.workflow_id = $2 AND `+held+`
			AND v.created_at >= (SELECT s.created_at FROM attribute_schemas s WHERE s.key = k))`,
		keys, workflow)
	if err != nil {
		return nil, err
	}
	var added []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			_ = rows.Close()
			return nil, err
		}
		added = append(added, key)
	}
	err = rows.Err()
	if closeErr := rows.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	assignments, err := s.catalog.SchemaAssignments(ctx, tx, added)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]uuid.UUID, len(assignments))
	for key, assignment := range assignments {
		result[key] = assignment.Agencies
	}
	return result, nil
}
