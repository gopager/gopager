package readstore

import (
	"cmp"
	"slices"
	"strings"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/readmodel"
)

// isUnitAttribute reports whether a field is a Unit attribute, which a Message does not carry
// but each of its appearances does through the appearance's Unit.
func isUnitAttribute(field *fields.Field) bool {
	return field.Ref.Namespace == "unit" && len(field.Ref.Path) >= 3 && field.Ref.Path[0] == "attributes"
}

// usesUnitAttributes reports whether a leaf of the predicate reads a Unit attribute. Such a
// predicate has to hold on one appearance, so that its Unit leaves all describe the same Unit.
func usesUnitAttributes(n fields.Node) bool {
	for _, arg := range n.Args {
		if usesUnitAttributes(arg) {
			return true
		}
	}
	return n.Field != nil && isUnitAttribute(n.Field)
}

// unitAssigned renders when the schema key (a SQL expression) is assigned to the Agency of the
// Unit row u.
func unitAssigned(schemaKey, u string) string {
	return "EXISTS (SELECT 1 FROM attribute_schema_agencies sa WHERE sa.schema_key = " + schemaKey +
		" AND sa.agency_id = " + u + ".agency_id)"
}

// unitFieldValue renders the effective typed value of a top-level attribute field of the Unit
// row u (NULL when there is none). The Unit's own entry decides alone: a value is effective and
// an explicit unset hides the Organization's default. Only a missing entry inherits the default,
// and only while the schema is assigned to the Unit's Agency. A deleted Unit keeps its values.
func (b *feedBuilder) unitFieldValue(u, schemaKey, field string) string {
	path := "ARRAY['attributes', " + b.params.add(schemaKey) + "::text, " + b.params.add(field) + "::text]"
	own := "(" + u + ".settings #> " + path + ")"
	return `(CASE WHEN ` + own + ` IS NOT NULL THEN CASE WHEN ` + own + `->>'mode' = 'value' THEN ` + own + `->'value' END
		ELSE (SELECT o.defaults #> ` + path + ` ->'value' FROM organizations o
			WHERE o.id = ` + u + `.organization_id AND o.defaults #> ` + path + ` ->>'mode' = 'value'
			AND ` + unitAssigned(b.params.add(schemaKey)+"::text", u) + `) END)`
}

// unitAttributeCandidates renders the candidates of unit.attributes.<schemaKey>.<field>[.<property>…]:
// the effective value of the Unit of the current appearance (the aliases mu and u of the
// appearance subquery), descended into the object properties of the path. A Unit-less
// appearance has none.
func (b *feedBuilder) unitAttributeCandidates(field *fields.Field) string {
	path := field.Ref.Path
	value := b.descend(b.unitFieldValue("u", path[1], path[2]), path[3:])
	return b.scalarCandidates("SELECT "+value+" AS x", field)
}

// scalarCandidates renders the admissible candidates of a field from rows of typed values in
// column x: the value itself, or for a list its elements.
func (b *feedBuilder) scalarCandidates(values string, field *fields.Field) string {
	scalar := scalarSchema(field.Type)
	if field.Type.Kind != processing.ListValue {
		return "SELECT r.x FROM (" + values + ") r WHERE " + admissible("r.x", scalar)
	}
	return `SELECT el.x FROM (` + values + `) r CROSS JOIN LATERAL jsonb_array_elements(
			CASE WHEN r.x->>'kind' = 'list' AND jsonb_typeof(r.x->'value') = 'array' THEN r.x->'value' ELSE '[]'::jsonb END
		) AS el(x) WHERE ` + admissible("el.x", scalar)
}

// descend renders the typed property value at the object properties below the typed value.
func (b *feedBuilder) descend(value string, properties []string) string {
	for _, property := range properties {
		value = "(" + value + "->'value'->" + b.params.add(property) + "::text)"
	}
	return value
}

// unitAttributesColumn renders the effective attribute values of the Unit row u as a jsonb
// array of {schemaKey, field, value, source}, ordered by schema key and field in byte order.
func unitAttributesColumn(u string) string {
	return `COALESCE((SELECT jsonb_agg(jsonb_build_object('schemaKey', e.schema_key, 'field', e.field,
			'value', e.value, 'source', e.source) ORDER BY e.schema_key COLLATE "C", e.field COLLATE "C")
		FROM (
			SELECT s.key AS schema_key, f.key AS field, f.value->'value' AS value, 'unit' AS source
			FROM jsonb_each(` + objectOrEmpty(u+".settings->'attributes'") + `) s
				CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("s.value") + `) f
			WHERE f.value->>'mode' = 'value' AND f.value->'value' IS NOT NULL
			UNION ALL
			SELECT s.key, f.key, f.value->'value', 'organization'
			FROM organizations o
				CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("o.defaults->'attributes'") + `) s
				CROSS JOIN LATERAL jsonb_each(` + objectOrEmpty("s.value") + `) f
			WHERE o.id = ` + u + `.organization_id AND f.value->>'mode' = 'value' AND f.value->'value' IS NOT NULL
				AND (` + u + `.settings #> ARRAY['attributes', s.key, f.key]) IS NULL AND ` + unitAssigned("s.key", u) + `
		) e), '[]'::jsonb)`
}

// sortedUnitAttributes orders the attributes of a Unit by schema key and field in byte order,
// the order of the API, which does not depend on the database collation.
func sortedUnitAttributes(attributes []readmodel.UnitAttribute) []readmodel.UnitAttribute {
	sorted := append([]readmodel.UnitAttribute{}, attributes...)
	slices.SortFunc(sorted, func(a, b readmodel.UnitAttribute) int {
		return cmp.Or(strings.Compare(a.SchemaKey, b.SchemaKey), strings.Compare(a.Field, b.Field))
	})
	return sorted
}
