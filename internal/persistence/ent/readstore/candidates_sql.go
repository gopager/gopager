package readstore

import (
	"fmt"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// typedString renders a typed string value of the text expression, for use as a candidate.
func typedString(expression string) string {
	return "jsonb_build_object('kind', 'string', 'value', " + expression + ")"
}

// candidates renders the candidate values of a field for the Message m as rows with the typed
// value in column x: absent values yield no row, a list yields one row per element and a value
// that does not match the declared type yields none, as in fields.Filter.Match.
func (b *feedBuilder) candidates(field *fields.Field) string {
	if field.Ref.Namespace == "attributes" {
		return b.attributeCandidates(field)
	}
	return b.messageCandidates(field.Path())
}

// attributeCandidates renders the candidates of attributes.<schemaKey>.<field>[.<property>…]:
// the effective values of the field at every position the caller may see, descended into the
// object properties of the path. The values come from the per-Message aggregate of fieldValues,
// so that every leaf on the field reuses one computation.
func (b *feedBuilder) attributeCandidates(field *fields.Field) string {
	path := field.Ref.Path
	value := "el.x"
	for _, property := range path[2:] {
		value = "(" + value + "->'value'->" + b.params.add(property) + "::text)"
	}
	values := "SELECT " + value + " AS x FROM jsonb_array_elements(" + b.fieldValues(path[0], path[1]) + ") el(x)"
	scalar := scalarSchema(field.Type)
	if field.Type.Kind != processing.ListValue {
		return "SELECT r.x FROM (" + values + ") r WHERE " + admissible("r.x", scalar)
	}
	return `SELECT el.x FROM (` + values + `) r CROSS JOIN LATERAL jsonb_array_elements(
			CASE WHEN r.x->>'kind' = 'list' AND jsonb_typeof(r.x->'value') = 'array' THEN r.x->'value' ELSE '[]'::jsonb END
		) AS el(x) WHERE ` + admissible("el.x", scalar)
}

// fieldValues returns the jsonb array of the effective typed values of an attribute field of the
// page Message m, at every position the caller may see. The first use of a field registers a
// lateral aggregate for the page's FROM clause (see lateralFields); the expression is the same
// for every later leaf on the field, so the effective-value subquery runs once per Message and
// field, not once per condition.
func (b *feedBuilder) fieldValues(schemaKey, field string) string {
	key := schemaKey + "\x00" + field
	if alias, ok := b.fieldAliases[key]; ok {
		return alias + ".vals"
	}
	if b.fieldAliases == nil {
		b.fieldAliases = map[string]string{}
	}
	alias := fmt.Sprintf("ev%d", len(b.fieldAliases)+1)
	b.fieldAliases[key] = alias
	restriction := " AND k.key = " + b.params.add(schemaKey) + "::text AND f.key = " + b.params.add(field) + "::text"
	b.lateralFields = append(b.lateralFields, " CROSS JOIN LATERAL (SELECT COALESCE(jsonb_agg(e.value), '[]'::jsonb) AS vals FROM ("+
		b.effectiveAttributes("m.id", restriction)+") e) "+alias)
	return alias + ".vals"
}

// admissible renders when the typed value x is a candidate of the scalar type: of its kind, or an
// explicit null where the type admits one.
func admissible(x string, scalar processing.FieldSchema) string {
	condition := x + "->>'kind' = '" + string(scalar.Kind) + "'"
	if scalar.Nullable {
		condition += " OR " + x + "->>'kind' = 'null'"
	}
	return "(" + condition + ")"
}

// representative renders the Representative Payload of the Message m for the caller: the first
// supporting Transmission, complete ones first, then by acceptance.
func (b *feedBuilder) representative() string {
	return `SELECT t.protocol, t.payload FROM transmissions t JOIN networks n ON n.id = t.network_id
		WHERE t.message_id = m.id AND ` + b.supported("t", "n") + `
		ORDER BY t.incomplete, t.accepted_at, t.id LIMIT 1`
}

// messageCandidates renders the candidates of a built-in Message field, derived like the
// projection derives them.
func (b *feedBuilder) messageCandidates(path string) string {
	switch path {
	case "message.id":
		return "SELECT " + typedString("m.id::text") + " AS x"
	case "message.time":
		return `SELECT jsonb_build_object('kind', 'timestamp', 'value',
			to_char(m.first_observed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')) AS x`
	case "message.processingStatus":
		return `SELECT ` + typedString(`CASE WHEN bool_or(t.processing_status = 'pending') THEN 'pending'
				WHEN bool_or(t.processing_status = 'failed') THEN 'failed'
				WHEN bool_or(t.processing_status = 'completed') THEN 'completed' ELSE 'none' END`) + ` AS x
			FROM transmissions t JOIN networks n ON n.id = t.network_id
			WHERE t.message_id = m.id AND ` + b.supported("t", "n")
	case "message.correctionStatus":
		return `SELECT ` + typedString(`CASE WHEN COALESCE(bool_or(mu.correction_status = 'correcting'), false)
				THEN 'correcting' ELSE 'current' END`) + ` AS x
			FROM message_units mu WHERE mu.message_id = m.id AND ` + b.eligible("mu")
	case "message.protocol":
		return b.representativeValue("r.protocol")
	case "message.form":
		return b.representativeValue("r.payload->>'form'")
	case "message.text":
		return b.representativeValue("CASE WHEN r.payload->>'form' = 'text' THEN r.payload->>'value' END")
	case "message.numeric":
		return b.representativeValue("CASE WHEN r.payload->>'form' = 'numeric' THEN r.payload->>'value' END")
	case "message.unitIds":
		return b.appearanceValues("mu.unit_id::text")
	case "message.unitNames":
		return b.appearanceValues("u.name")
	}
	return "SELECT NULL::jsonb AS x WHERE false"
}

// representativeValue renders the candidate of a text expression over the Representative Payload
// r, none when the expression is NULL.
func (b *feedBuilder) representativeValue(expression string) string {
	return "SELECT " + typedString("v.value") + " AS x FROM (SELECT " + expression + " AS value FROM (" + b.representative() +
		") r) v WHERE v.value IS NOT NULL"
}

// appearanceValues renders one candidate per eligible Unit appearance of the Message m.
func (b *feedBuilder) appearanceValues(expression string) string {
	return "SELECT " + typedString(expression) + " AS x FROM message_units mu JOIN units u ON u.id = mu.unit_id " +
		"WHERE mu.message_id = m.id AND " + b.eligible("mu")
}
