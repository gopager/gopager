package processingstore

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// ProjectAttributeValuesSQL is the statement that derives the rows of message_attribute_values
// from the stored evaluation of published results; its only parameter ($1) is the array of
// result IDs. The table holds every published value, not the effective one, because which value
// is effective depends on what the reader may see; readers use it only as an indexed prefilter
// before the exact check. A field yields one row per scalar path (`field`, then `field.property`
// at any depth) and target position, a list of scalars one row per element; null, bytes,
// objects themselves and lists of objects yield none. Deriving the rows in SQL keeps one
// definition for publication and for fixtures that insert results directly.
const ProjectAttributeValuesSQL = `
INSERT INTO message_attribute_values (result_id, transmission_id, schema_key, path, target_position,
	text_digest, integer_value, number_value, time_value, boolean_value)
WITH RECURSIVE roots AS (
	SELECT r.id AS result_id, c.transmission_id, k.key AS schema_key, f.key AS path, NULL::int AS target_position,
		f.value->'value' AS v
	FROM processing_results r JOIN processing_candidates c ON c.id = r.candidate_id
		CROSS JOIN LATERAL jsonb_each(CASE WHEN jsonb_typeof(r.evaluation->'fields') = 'object'
			THEN r.evaluation->'fields' ELSE '{}'::jsonb END) AS k(key, value)
		CROSS JOIN LATERAL jsonb_each(CASE WHEN jsonb_typeof(k.value) = 'object' THEN k.value ELSE '{}'::jsonb END) AS f(key, value)
	WHERE r.id = ANY($1::uuid[]) AND r.published_at IS NOT NULL
	UNION ALL
	SELECT r.id, c.transmission_id, k.key, f.key, (tr->>'targetPosition')::int, f.value->'value'
	FROM processing_results r JOIN processing_candidates c ON c.id = r.candidate_id
		CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(r.evaluation->'targetResults') = 'array'
			THEN r.evaluation->'targetResults' ELSE '[]'::jsonb END) AS tr
		CROSS JOIN LATERAL jsonb_each(CASE WHEN jsonb_typeof(tr->'fields') = 'object' THEN tr->'fields' ELSE '{}'::jsonb END) AS k(key, value)
		CROSS JOIN LATERAL jsonb_each(CASE WHEN jsonb_typeof(k.value) = 'object' THEN k.value ELSE '{}'::jsonb END) AS f(key, value)
	WHERE r.id = ANY($1::uuid[]) AND r.published_at IS NOT NULL
), walk AS (
	SELECT * FROM roots
	UNION ALL
	SELECT w.result_id, w.transmission_id, w.schema_key, w.path || '.' || p.key, w.target_position, p.value
	FROM walk w CROSS JOIN LATERAL jsonb_each(w.v->'value') AS p(key, value)
	WHERE w.v->>'kind' = 'object' AND jsonb_typeof(w.v->'value') = 'object'
), leaves AS (
	SELECT result_id, transmission_id, schema_key, path, target_position, v AS x FROM walk WHERE v->>'kind' <> 'list'
	UNION ALL
	SELECT w.result_id, w.transmission_id, w.schema_key, w.path, w.target_position, e.x
	FROM walk w CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN w.v->>'kind' = 'list' AND jsonb_typeof(w.v->'value') = 'array'
		THEN w.v->'value' ELSE '[]'::jsonb END) AS e(x)
)
SELECT result_id, transmission_id, schema_key, path, target_position,
	CASE WHEN x->>'kind' = 'string' THEN sha256(convert_to(x->>'value', 'UTF8')) END,
	CASE WHEN x->>'kind' = 'integer' THEN (x->>'value')::bigint END,
	CASE WHEN x->>'kind' = 'number' THEN (x->>'value')::float8 END,
	CASE WHEN x->>'kind' = 'timestamp' THEN regexp_replace(x->>'value', '(\.[0-9]{6})[0-9]+', '\1')::timestamptz END,
	CASE WHEN x->>'kind' = 'boolean' THEN (x->>'value')::boolean END
FROM leaves WHERE x->>'kind' IN ('string', 'integer', 'number', 'timestamp', 'boolean')`

// projectAttributeValues writes the value rows of one just-published result. It must run in the
// publication transaction and only once per result, which the guarded publication update ensures.
func (s *Store) projectAttributeValues(ctx context.Context, tx platform.TransactionContext, result uuid.UUID) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	// JIT compilation of this recursive statement costs far more than running it.
	if _, err := executor.ExecContext(ctx, `SET LOCAL jit = off`); err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, ProjectAttributeValuesSQL, []uuid.UUID{result})
	return err
}
