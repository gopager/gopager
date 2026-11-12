// Package attributestore implements the PostgreSQL side of Attribute schemas in raw SQL, inside
// the caller's primary transaction.
package attributestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/attributes"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent/agencyscope"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store reads and writes Attribute schemas. It holds no transaction of its own.
type Store struct{ primary *postgres.Store }

var (
	_ attributes.Store         = (*Store)(nil)
	_ attributes.CatalogSource = (*Store)(nil)
)

// New binds the store to the primary store whose transactions it joins.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}

// selection selects a schema with its Agencies as a JSON array.
const selection = `SELECT s.key, s.revision, s.label, s.description, s.subject, s.fields, s.deprecated, s.created_at, s.updated_at,
	(SELECT COALESCE(json_agg(a.agency_id ORDER BY a.agency_id), '[]'::json) FROM attribute_schema_agencies a WHERE a.schema_key = s.key)::text
	FROM attribute_schemas s`

func scan(row interface{ Scan(...any) error }) (attributes.Schema, error) {
	var schema attributes.Schema
	var subject string
	var fields []byte
	var agencies string
	if err := row.Scan(&schema.Key, &schema.Revision, &schema.Label, &schema.Description, &subject, &fields,
		&schema.Deprecated, &schema.CreatedAt, &schema.UpdatedAt, &agencies); err != nil {
		return schema, err
	}
	schema.Subject = attributes.Subject(subject)
	if err := json.Unmarshal(fields, &schema.Fields); err != nil {
		return schema, err
	}
	return schema, json.Unmarshal([]byte(agencies), &schema.Agencies)
}

// execer is the statement runner of a resolved transaction.
type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// touchCatalog increments the catalog revision. The row lock also orders concurrent changes.
func touchCatalog(ctx context.Context, executor execer) error {
	_, err := executor.ExecContext(ctx, `UPDATE attribute_catalog SET revision = revision + 1 WHERE id = 1`)
	return err
}

func insertAgencies(ctx context.Context, executor execer, key string, agencies []uuid.UUID) error {
	for _, agency := range agencies {
		if _, err := executor.ExecContext(ctx, `INSERT INTO attribute_schema_agencies (schema_key, agency_id) VALUES ($1, $2)`,
			key, agency); err != nil {
			return agencyscope.InvalidAgency(err)
		}
	}
	return nil
}

// Insert implements attributes.Store.
func (s *Store) Insert(ctx context.Context, tx platform.TransactionContext, schema attributes.Schema, by uuid.UUID) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	fields, err := json.Marshal(schema.Fields)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO attribute_schemas (key, revision, label, description, subject, fields,
		deprecated, created_by, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10)`,
		schema.Key, schema.Revision, schema.Label, schema.Description, string(schema.Subject), fields, schema.Deprecated,
		by, schema.CreatedAt, schema.UpdatedAt); err != nil {
		var databaseError *pgconn.PgError
		if errors.As(err, &databaseError) && databaseError.Code == "23505" {
			return authz.ErrRevisionConflict
		}
		return err
	}
	if err := insertAgencies(ctx, executor, schema.Key, schema.Agencies); err != nil {
		return err
	}
	return touchCatalog(ctx, executor)
}

// Get implements attributes.Store.
func (s *Store) Get(ctx context.Context, tx platform.TransactionContext, key string) (attributes.Schema, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return attributes.Schema{}, err
	}
	schema, err := scan(executor.QueryRowContext(ctx, selection+` WHERE s.key = $1`, key))
	if errors.Is(err, sql.ErrNoRows) {
		return attributes.Schema{}, authz.ErrNotFound
	}
	return schema, err
}

// visibleSchemas is the condition that one of the schema's Agencies is visible; the arguments
// are extended by the visibility's placeholders.
func visibleSchemas(visibility authz.AgencyVisibility, args []any) (string, []any) {
	condition, args := agencyscope.Condition(visibility, "v.agency_id", args)
	return `EXISTS (SELECT 1 FROM attribute_schema_agencies v WHERE v.schema_key = s.key AND ` + condition + `)`, args
}

// Visible implements attributes.Store.
func (s *Store) Visible(ctx context.Context, tx platform.TransactionContext, key string, visibility authz.AgencyVisibility) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	condition, args := visibleSchemas(visibility, []any{key})
	var visible bool
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM attribute_schemas s WHERE s.key = $1 AND `+condition+`)`,
		args...).Scan(&visible)
	return visible, err
}

// List implements attributes.Store.
func (s *Store) List(ctx context.Context, tx platform.TransactionContext, visibility authz.AgencyVisibility,
	agency *uuid.UUID, offset, limit int) ([]attributes.Schema, int, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, 0, err
	}
	condition, args := visibleSchemas(visibility, nil)
	if agency != nil {
		args = append(args, *agency)
		condition += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM attribute_schema_agencies f WHERE f.schema_key = s.key AND f.agency_id = $%d)`, len(args))
	}
	var total int
	if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM attribute_schemas s WHERE `+condition, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, offset, limit)
	rows, err := executor.QueryContext(ctx, fmt.Sprintf(selection+` WHERE %s ORDER BY s.key OFFSET $%d LIMIT $%d`,
		condition, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var schemas []attributes.Schema
	for rows.Next() {
		schema, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		schemas = append(schemas, schema)
	}
	return schemas, total, rows.Err()
}

// Update implements attributes.Store.
func (s *Store) Update(ctx context.Context, tx platform.TransactionContext, schema attributes.Schema, expected int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	fields, err := json.Marshal(schema.Fields)
	if err != nil {
		return err
	}
	// The catalog row lock first serializes this update with Unit writes holding the shared lock.
	if err := touchCatalog(ctx, executor); err != nil {
		return err
	}
	removed, err := removedAgencies(ctx, executor, schema.Key, schema.Agencies)
	if err != nil {
		return err
	}
	if err := requireNoEntriesOf(ctx, executor, schema.Key, removed); err != nil {
		return err
	}
	result, err := executor.ExecContext(ctx, `UPDATE attribute_schemas SET label = $3, description = $4, fields = $5::jsonb,
		deprecated = $6, revision = revision + 1, updated_at = $7 WHERE key = $1 AND revision = $2`,
		schema.Key, expected, schema.Label, schema.Description, fields, schema.Deprecated, schema.UpdatedAt)
	if err := requireOneRow(result, err); err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM attribute_schema_agencies WHERE schema_key = $1`, schema.Key); err != nil {
		return err
	}
	return insertAgencies(ctx, executor, schema.Key, schema.Agencies)
}

// fieldReference is the JSON path condition of a field reference to the schema $key: a Message
// attribute (`attributes.<key>…`), a Unit attribute (`unit.attributes.<key>…`) or a Workflow's
// target Unit attribute (`target.unit.attributes.<key>…`).
const fieldReference = `'$.** ? ((@.namespace == "attributes" && @.path[0] == $key)
	|| (@.namespace == "unit" && @.path[0] == "attributes" && @.path[1] == $key)
	|| (@.namespace == "target" && @.path[0] == "unit" && @.path[1] == "attributes" && @.path[2] == $key))',
	jsonb_build_object('key', $1::text)`

// referenced reports whether anything refers to the key: an active Workflow version writing or
// reading it, a current Layout version, a Subscription filter or target-input binding, or a saved
// view. Workflow outputs are keyed by the schema key.
const referenced = `SELECT
	EXISTS (SELECT 1 FROM processing_workflows w JOIN processing_workflow_versions v ON v.id = w.active_version_id
		WHERE NOT w.deleted AND (jsonb_exists(v.definition->'outputs', $1) OR jsonb_path_exists(v.definition, ` + fieldReference + `)))
	OR EXISTS (SELECT 1 FROM layout_definitions l JOIN layout_definition_versions v ON v.layout_id = l.id AND v.version = l.revision
		WHERE jsonb_path_exists(v.definition, ` + fieldReference + `))
	OR EXISTS (SELECT 1 FROM subscriptions WHERE jsonb_path_exists(filter, ` + fieldReference + `))
	OR EXISTS (SELECT 1 FROM subscription_targets WHERE jsonb_path_exists(input_bindings, ` + fieldReference + `))
	OR EXISTS (SELECT 1 FROM saved_views WHERE jsonb_path_exists(definition, ` + fieldReference + `))`

// Delete implements attributes.Store.
func (s *Store) Delete(ctx context.Context, tx platform.TransactionContext, key string, expected int64) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	// Locking the catalog row first serializes this delete with every other schema change.
	if err := touchCatalog(ctx, executor); err != nil {
		return err
	}
	var inUse bool
	if err := executor.QueryRowContext(ctx, referenced, key).Scan(&inUse); err != nil {
		return err
	}
	if inUse {
		return authz.ErrRevisionConflict
	}
	if inUse, err = entriesInUse(ctx, executor, key); err != nil {
		return err
	}
	if inUse {
		return authz.ErrRevisionConflict
	}
	result, err := executor.ExecContext(ctx, `DELETE FROM attribute_schemas WHERE key = $1 AND revision = $2`, key, expected)
	if err := requireOneRow(result, err); err != nil {
		return err
	}
	return stripDeletedEntries(ctx, executor, key)
}

// Catalog implements attributes.CatalogSource. The revision is read first, so the schemas are
// at least as new as the revision returned with them.
func (s *Store) Catalog(ctx context.Context, tx platform.TransactionContext) ([]attributes.Schema, int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, 0, err
	}
	var revision int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM attribute_catalog WHERE id = 1`).Scan(&revision); err != nil {
		return nil, 0, err
	}
	rows, err := executor.QueryContext(ctx, selection+` ORDER BY s.key`)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var schemas []attributes.Schema
	for rows.Next() {
		schema, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		schemas = append(schemas, schema)
	}
	return schemas, revision, rows.Err()
}

// CatalogRevision returns the catalog revision alone, so that a cached registry can be reused
// without loading every schema.
func (s *Store) CatalogRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, err
	}
	var revision int64
	err = executor.QueryRowContext(ctx, `SELECT revision FROM attribute_catalog WHERE id = 1`).Scan(&revision)
	return revision, err
}

// LockCatalog implements attributes.CatalogSource. Deleting a schema updates the catalog row, so
// it waits for every transaction holding this shared lock.
func (s *Store) LockCatalog(ctx context.Context, tx platform.TransactionContext) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `SELECT 1 FROM attribute_catalog WHERE id = 1 FOR SHARE`)
	return err
}

// requireOneRow reports a statement that matched no row as a revision conflict.
func requireOneRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}

// SchemaAssignment is the assignment of an existing Attribute schema.
type SchemaAssignment struct {
	// Agencies the schema is assigned to, ordered by ID; empty when it has none.
	Agencies []uuid.UUID
	// CreatedAt identifies this incarnation of the key: a deleted and re-created schema is newer.
	CreatedAt time.Time
}

// SchemaAssignments returns the assignment of each of the keys that names an existing schema. A
// key without a schema is absent from the result.
func (s *Store) SchemaAssignments(ctx context.Context, tx platform.TransactionContext, keys []string) (map[string]SchemaAssignment, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT s.key, s.created_at, a.agency_id FROM attribute_schemas s
		LEFT JOIN attribute_schema_agencies a ON a.schema_key = s.key WHERE s.key = ANY($1::text[]) ORDER BY s.key, a.agency_id`, keys)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]SchemaAssignment)
	for rows.Next() {
		var key string
		var createdAt time.Time
		var agency *uuid.UUID
		if err := rows.Scan(&key, &createdAt, &agency); err != nil {
			return nil, err
		}
		assignment := result[key]
		assignment.CreatedAt = createdAt
		assignment.Agencies = append([]uuid.UUID{}, assignment.Agencies...)
		if agency != nil {
			assignment.Agencies = append(assignment.Agencies, *agency)
		}
		result[key] = assignment
	}
	return result, rows.Err()
}
