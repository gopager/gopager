package readstore

// supported renders when Transmission t (with Network n) supports an eligible appearance of
// its Message, so that its published results may contribute to what the caller sees.
func (b *feedBuilder) supported(t, n string) string {
	return "EXISTS (SELECT 1 FROM message_units mu WHERE mu.message_id = " + t + ".message_id AND " +
		b.eligible("mu") + " AND " + supports(t, n, "mu") + ")"
}

// attributeRows renders the attribute values of the published results of the supporting
// Transmissions of message that the caller may read, one row per schema key, field and result
// position. Target-level values count only for targets the caller may see. restriction is an
// extra condition over the schema key (k.key) and field (f.key) aliases of both branches.
func (b *feedBuilder) attributeRows(message, restriction string) string {
	results := `FROM transmissions dt JOIN networks dn ON dn.id = dt.network_id
			JOIN processing_candidates c ON c.transmission_id = dt.id
			JOIN processing_results r ON r.candidate_id = c.id AND r.published_at IS NOT NULL AND r.message_id = dt.message_id`
	conditions := `WHERE dt.message_id = ` + message + ` AND r.field_groups <@ ` + b.groupsParam() +
		` AND ` + b.supported("dt", "dn") + restriction
	appearance := func(condition string) string {
		return "EXISTS (SELECT 1 FROM message_units v WHERE v.message_id = dt.message_id AND " + condition +
			" AND " + b.eligible("v") + ")"
	}
	visible := b.targetVisibility(appearance, "dn.agency_id", "dt.observed_at", "dt.accepted_at")
	return `SELECT k.key AS schema_key, f.key AS field, NULL::int AS position, NULL::uuid AS unit_id,
			f.value->'value' AS value, r.id AS result_id, c.workflow_id, c.definition_version_id, r.published_at
		` + results + `
			CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("r.evaluation->'fields'") + `) AS k(key, value)
			CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("k.value") + `) AS f(key, value)
		` + conditions + `
		UNION ALL
		SELECT k.key, f.key, (tr->>'targetPosition')::int, CASE WHEN tt.resolution_status = 'assigned' THEN tt.unit_id END,
			f.value->'value', r.id, c.workflow_id, c.definition_version_id, r.published_at
		` + results + `
			CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(r.evaluation->'targetResults') = 'array'
				THEN r.evaluation->'targetResults' ELSE '[]'::jsonb END) AS tr
			CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("tr->'fields'") + `) AS k(key, value)
			CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("k.value") + `) AS f(key, value)
			JOIN transmission_targets tt ON tt.transmission_id = dt.id AND tt.position = (tr->>'targetPosition')::int
				AND ` + visible + `
		` + conditions
}

// objectOrEmpty renders the jsonb expression as is when it is an object and as an empty
// object otherwise, because jsonb_each fails on any other value.
func objectOrEmpty(expression string) string {
	return "CASE WHEN jsonb_typeof(" + expression + ") = 'object' THEN " + expression + " ELSE '{}'::jsonb END"
}

// effectiveAttributes renders the effective attribute values of message: per schema key, field
// and position (NULL for a Transmission-level value) the present value of the latest published
// result, by publication time and then result ID. The columns are schema_key, field, position,
// unit_id, value (the typed value), result_id, workflow_id, definition_version_id and
// published_at. A restriction selects schema keys and fields before the latest is chosen.
func (b *feedBuilder) effectiveAttributes(message, restriction string) string {
	return `SELECT DISTINCT ON (e.schema_key, e.field, e.position) e.schema_key, e.field, e.position, e.unit_id, e.value,
			e.result_id, e.workflow_id, e.definition_version_id, e.published_at
		FROM (` + b.attributeRows(message, restriction) + `) e
		ORDER BY e.schema_key, e.field, e.position, e.published_at DESC, e.result_id DESC`
}

// publishedResultsColumn renders the IDs of the readable published results of the page Message p
// as a jsonb array, empty unless the query asks for them and the caller can read derived results.
// A result is readable like in attributeRows, but it counts without contributing an effective
// value.
func (b *feedBuilder) publishedResultsColumn() string {
	if !b.query.WithPublishedResults || len(b.query.DerivedGroups) == 0 {
		return `'[]'::jsonb`
	}
	return `COALESCE((SELECT jsonb_agg(DISTINCT r.id)
		FROM transmissions dt JOIN networks dn ON dn.id = dt.network_id
			JOIN processing_candidates c ON c.transmission_id = dt.id
			JOIN processing_results r ON r.candidate_id = c.id AND r.published_at IS NOT NULL AND r.message_id = dt.message_id
		WHERE dt.message_id = p.id AND r.field_groups <@ ` + b.groupsParam() + ` AND ` + b.supported("dt", "dn") + `), '[]'::jsonb)`
}

// attributesColumn renders the effective attribute values of the page Message p as a jsonb
// array, empty when the caller cannot read derived results.
func (b *feedBuilder) attributesColumn() string {
	if len(b.query.DerivedGroups) == 0 {
		return `'[]'::jsonb`
	}
	return `COALESCE((SELECT jsonb_agg(jsonb_build_object('schemaKey', e.schema_key,
			'field', e.field, 'targetPosition', e.position, 'unitId', e.unit_id, 'value', e.value,
			'workflowId', e.workflow_id, 'definitionVersionId', e.definition_version_id,
			'resultId', e.result_id, 'publishedAt', e.published_at))
		FROM (` + b.effectiveAttributes("p.id", "") + `) e), '[]'::jsonb)`
}
