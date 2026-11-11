package fields

import (
	"strings"
	"time"

	"github.com/gopager/gopager/internal/processing"
)

// ValueSource supplies the candidate values of a field for one evaluation.
type ValueSource interface {
	// Values returns the values of a field; none when the field is absent.
	Values(ref processing.FieldRef) []processing.Value
}

// Match evaluates the filter with two-valued logic. A field resolves to a set of candidate
// scalars: each value decoded against the field type, a list contributing its elements, a
// value that fails to decode ignored. A comparison holds when one candidate satisfies it;
// present holds when the set is not empty, and its negation when it is empty.
func (f *Filter) Match(source ValueSource) bool { return matchNode(f.root, source) }

func matchNode(n Node, source ValueSource) bool {
	switch n.Op {
	case "and":
		for _, arg := range n.Args {
			if !matchNode(arg, source) {
				return false
			}
		}
		return true
	case "or":
		for _, arg := range n.Args {
			if matchNode(arg, source) {
				return true
			}
		}
		return false
	case "not":
		return !matchNode(n.Args[0], source)
	}
	candidates := candidateValues(n.Field, source)
	if n.Op == "present" {
		return (len(candidates) > 0) == n.Values[0].(bool)
	}
	for _, candidate := range candidates {
		if satisfies(n, candidate) {
			return true
		}
	}
	return false
}

// candidateValues decodes the field's values into native scalars.
func candidateValues(field *Field, source ValueSource) []any {
	var result []any
	for _, value := range source.Values(field.Ref) {
		native, err := value.Native(field.Type)
		if err != nil {
			continue
		}
		if elements, isList := native.([]any); isList {
			result = append(result, elements...)
			continue
		}
		if native == nil && field.Type.Kind == processing.ListValue {
			continue
		}
		result = append(result, native)
	}
	return result
}

func satisfies(n Node, candidate any) bool {
	switch n.Op {
	case "eq":
		return equal(candidate, n.Values[0])
	case "in":
		for _, value := range n.Values {
			if equal(candidate, value) {
				return true
			}
		}
		return false
	case "contains":
		text, ok := candidate.(string)
		return ok && strings.Contains(text, n.Values[0].(string))
	}
	order, ok := compare(candidate, n.Values[0])
	if !ok {
		return false
	}
	switch n.Op {
	case "lt":
		return order < 0
	case "lte":
		return order <= 0
	case "gt":
		return order > 0
	case "gte":
		return order >= 0
	}
	return false
}

func equal(a, b any) bool {
	if at, ok := a.(time.Time); ok {
		bt, isTime := b.(time.Time)
		return isTime && at.Equal(bt)
	}
	return a == b
}

// compare orders two candidates of the same ordered kind; ok is false for any other pair.
func compare(a, b any) (order int, ok bool) {
	switch av := a.(type) {
	case int64:
		bv, isInt := b.(int64)
		return cmpOrdered(av, bv), isInt
	case float64:
		bv, isFloat := b.(float64)
		return cmpOrdered(av, bv), isFloat
	case time.Time:
		bv, isTime := b.(time.Time)
		return av.Compare(bv), isTime
	}
	return 0, false
}

func cmpOrdered[T int64 | float64](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
