package readstore

import (
	"fmt"
	"strings"
	"time"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// selectionFilter renders the compiled predicate as two conditions on the Message m: its indexed
// prefilter (empty when none can be derived) and the exact check. Both are empty without a
// predicate.
func (b *feedBuilder) selectionFilter() (narrowing, exact string) {
	if b.query.Selection == nil {
		return "", ""
	}
	root := b.query.Selection.Root()
	return b.prefilter(root), b.predicate(root)
}

// predicate renders one node as a definite boolean: every leaf is an EXISTS over the candidate
// values of its field, so that NOT never meets an unknown. The rules are those of
// fields.Filter.Match: a comparison holds when one candidate satisfies it, present when
// there is a candidate.
func (b *feedBuilder) predicate(n fields.Node) string {
	switch n.Op {
	case "and", "or":
		parts := make([]string, len(n.Args))
		for i, arg := range n.Args {
			parts[i] = b.predicate(arg)
		}
		return "(" + strings.Join(parts, " "+strings.ToUpper(n.Op)+" ") + ")"
	case "not":
		return "(NOT " + b.predicate(n.Args[0]) + ")"
	}
	candidates := b.candidates(n.Field)
	switch {
	case n.Op == "present" && n.Values[0].(bool):
		return "EXISTS (" + candidates + ")"
	case n.Op == "present":
		return "NOT EXISTS (" + candidates + ")"
	}
	return "EXISTS (SELECT 1 FROM (" + candidates + ") c WHERE " + b.comparison(n, scalarSchema(n.Field.Type)) + ")"
}

// scalarSchema is the type of one candidate of a field: the element of a list, else the type.
func scalarSchema(schema processing.FieldSchema) processing.FieldSchema {
	if schema.Kind == processing.ListValue && schema.Element != nil {
		return *schema.Element
	}
	return schema
}

// sqlType names the PostgreSQL type that holds values of a scalar kind.
func sqlType(kind processing.ValueKind) string {
	switch kind {
	case processing.IntegerValue:
		return "bigint"
	case processing.NumberValue:
		return "float8"
	case processing.TimestampValue:
		return "timestamptz"
	case processing.BooleanValue:
		return "boolean"
	}
	return "text"
}

// decoded renders the stored typed value x of a scalar kind as a PostgreSQL value. It must only
// be evaluated for a candidate of that kind, which is why callers wrap it in a CASE: SQL does
// not promise to short-circuit AND.
func decoded(x string, kind processing.ValueKind) string {
	if kind == processing.StringValue {
		return x + "->>'value'"
	}
	return "(" + x + "->>'value')::" + sqlType(kind)
}

// comparison renders the test of a comparison or membership leaf on the candidate column
// c.x. A null operand matches a stored null; any other operand matches a value of the field's
// kind, a candidate of another kind never matches.
func (b *feedBuilder) comparison(n fields.Node, scalar processing.FieldSchema) string {
	var alternatives []string
	var operands []any
	for _, value := range n.Values {
		if value == nil {
			alternatives = append(alternatives, "c.x->>'kind' = 'null'")
			continue
		}
		operands = append(operands, value)
	}
	if len(operands) > 0 {
		left := decoded("c.x", scalar.Kind)
		var test string
		switch n.Op {
		case "eq":
			test = left + " = " + b.params.add(operands[0]) + "::" + sqlType(scalar.Kind)
		case "in":
			test = left + " = ANY(" + b.params.add(typedArray(scalar.Kind, operands)) + "::" + sqlType(scalar.Kind) + "[])"
		case "contains":
			test = "strpos(" + left + ", " + b.params.add(operands[0]) + "::text) > 0"
		case "lt", "lte", "gt", "gte":
			test = left + " " + map[string]string{"lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[n.Op] + " " +
				b.params.add(operands[0]) + "::" + sqlType(scalar.Kind)
		default:
			test = "false"
		}
		alternatives = append(alternatives, fmt.Sprintf("CASE WHEN c.x->>'kind' = '%s' THEN %s ELSE false END", scalar.Kind, test))
	}
	return "(" + strings.Join(alternatives, " OR ") + ")"
}

// typedArray converts native operands of one scalar kind to a slice that the driver sends as an
// array of that kind.
func typedArray(kind processing.ValueKind, operands []any) any {
	switch kind {
	case processing.IntegerValue:
		return convertAll[int64](operands)
	case processing.NumberValue:
		return convertAll[float64](operands)
	case processing.TimestampValue:
		return convertAll[time.Time](operands)
	case processing.BooleanValue:
		return convertAll[bool](operands)
	}
	return convertAll[string](operands)
}

func convertAll[T any](operands []any) []T {
	out := make([]T, len(operands))
	for i, operand := range operands {
		out[i] = operand.(T)
	}
	return out
}
