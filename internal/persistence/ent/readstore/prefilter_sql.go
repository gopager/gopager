package readstore

import (
	"slices"
	"strings"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// prefilter derives from the predicate a condition on the Message m that every Message matching
// the predicate satisfies, or "" when none can be derived. It narrows the page through the
// message_attribute_values index before the exact effective-value check, which stays
// authoritative: the projection holds every published value, so the condition may keep
// Messages whose value is not the effective one but never drops a match.
func (b *feedBuilder) prefilter(n fields.Node) string {
	switch n.Op {
	case "and":
		var parts []string
		for _, arg := range n.Args {
			if part := b.prefilter(arg); part != "" {
				parts = append(parts, part)
			}
		}
		return joinCondition(parts, " AND ")
	case "or":
		if !derivable(n) {
			return ""
		}
		parts := make([]string, len(n.Args))
		for i, arg := range n.Args {
			parts[i] = b.prefilter(arg)
		}
		return joinCondition(parts, " OR ")
	case "eq", "in", "lt", "lte", "gt", "gte":
		return b.leafPrefilter(n)
	}
	return ""
}

// derivable reports whether prefilter yields a condition for the node. It binds no parameter, so
// that an unusable alternative leaves none behind.
func derivable(n fields.Node) bool {
	switch n.Op {
	case "and":
		return slices.ContainsFunc(n.Args, derivable)
	case "or":
		return !slices.ContainsFunc(n.Args, func(arg fields.Node) bool { return !derivable(arg) })
	case "eq", "in", "lt", "lte", "gt", "gte":
		return n.Field.Ref.Namespace == "attributes" && valueColumn(scalarSchema(n.Field.Type).Kind) != "" &&
			!slices.Contains(n.Values, nil)
	}
	return false
}

// joinCondition renders conditions joined by the operator, "" when there are none.
func joinCondition(parts []string, operator string) string {
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, operator) + ")"
}

// leafPrefilter renders the indexed lookup of the Messages that have a published value of an
// attribute field satisfying the comparison. A null operand has no projected row, so such a
// leaf has no prefilter.
func (b *feedBuilder) leafPrefilter(n fields.Node) string {
	if !derivable(n) {
		return ""
	}
	ref := n.Field.Ref
	scalar := scalarSchema(n.Field.Type)
	column := valueColumn(scalar.Kind)
	var test string
	switch n.Op {
	case "eq":
		test = "= " + b.params.add(n.Values[0]) + "::" + sqlType(scalar.Kind)
	case "in":
		test = "= ANY(" + b.params.add(typedArray(scalar.Kind, n.Values)) + "::" + sqlType(scalar.Kind) + "[])"
	default:
		test = map[string]string{"lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[n.Op] + " " +
			b.params.add(n.Values[0]) + "::" + sqlType(scalar.Kind)
	}
	return "m.id IN (SELECT t.message_id FROM message_attribute_values v JOIN transmissions t ON t.id = v.transmission_id" +
		" WHERE v.schema_key = " + b.params.add(ref.Path[0]) + "::text AND v.path = " +
		b.params.add(strings.Join(ref.Path[1:], ".")) + "::text AND v." + column + " " + test + ")"
}

// valueColumn names the projection column of a scalar kind, "" for a kind that is not projected.
func valueColumn(kind processing.ValueKind) string {
	switch kind {
	case processing.StringValue:
		return "text_value"
	case processing.IntegerValue:
		return "integer_value"
	case processing.NumberValue:
		return "number_value"
	case processing.TimestampValue:
		return "time_value"
	case processing.BooleanValue:
		return "boolean_value"
	}
	return ""
}
