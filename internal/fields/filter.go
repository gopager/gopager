package fields

import (
	"cmp"
	"slices"

	"github.com/gopager/gopager/internal/processing"
)

// Node is one checked predicate node, for walkers such as the read store's SQL compiler.
type Node struct {
	// Op is the predicate operator.
	Op string
	// Field is the compared field of a leaf; nil for and, or, not.
	Field *Field
	// Values are the native operands: string, int64, float64, bool, time.Time, or nil for null.
	// For present, Values[0] is the boolean.
	Values []any
	// Args are the operands of and, or, not.
	Args []Node
}

// Filter is a predicate checked against a Declarations. It is immutable and safe for
// concurrent use.
type Filter struct {
	root   Node
	fields []Field
	groups []string
}

// Fields returns the distinct fields the filter reads, sorted by Path.
func (f *Filter) Fields() []Field { return slices.Clone(f.fields) }

// Groups returns the distinct non-empty field groups of the filter's fields, sorted.
func (f *Filter) Groups() []string { return slices.Clone(f.groups) }

// Root returns the checked predicate tree.
func (f *Filter) Root() Node { return f.root }

// CompileFilter checks a predicate against the declarations: shape, bounds, field existence
// and filterability, and operand kinds. A malformed predicate is CodeInvalidInput and an
// exceeded bound CodeResourceLimit.
func CompileFilter(predicate processing.Predicate, declarations Declarations) (*Filter, error) {
	if declarations.registry == nil {
		return nil, invalidInput("missing field declarations")
	}
	distinct := make(map[string]Field)
	root, err := checkNode(predicate, 1, declarations, distinct)
	if err != nil {
		return nil, err
	}
	filter := &Filter{root: root}
	groups := make(map[string]bool)
	for _, field := range distinct {
		filter.fields = append(filter.fields, field)
		if field.Group != "" {
			groups[field.Group] = true
		}
	}
	slices.SortFunc(filter.fields, func(a, b Field) int { return cmp.Compare(a.Path(), b.Path()) })
	for group := range groups {
		filter.groups = append(filter.groups, group)
	}
	slices.Sort(filter.groups)
	return filter, nil
}

func checkNode(p processing.Predicate, depth int, declarations Declarations, distinct map[string]Field) (Node, error) {
	if depth > processing.MaxDepth {
		return Node{}, resourceLimit("predicate nesting boundary")
	}
	switch p.Op {
	case "and", "or", "not":
		return checkLogical(p, depth, declarations, distinct)
	}
	if p.Field == nil || len(p.Args) != 0 {
		return Node{}, invalidInput("invalid field operands")
	}
	field, err := declarations.Field(*p.Field)
	if err != nil {
		return Node{}, err
	}
	if !field.Filterable {
		return Node{}, invalidInput("field cannot be filtered")
	}
	values, err := checkOperands(p, scalarType(field.Type))
	if err != nil {
		return Node{}, err
	}
	distinct[refKey(field.Ref)] = field
	return Node{Op: p.Op, Field: &field, Values: values}, nil
}

func checkLogical(p processing.Predicate, depth int, declarations Declarations, distinct map[string]Field) (Node, error) {
	if p.Field != nil || p.Value != nil || p.Values != nil || len(p.Args) == 0 || (p.Op == "not" && len(p.Args) != 1) {
		return Node{}, invalidInput("invalid logical operands")
	}
	if len(p.Args) > processing.MaxArguments {
		return Node{}, resourceLimit("predicate argument boundary")
	}
	args := make([]Node, len(p.Args))
	for i, arg := range p.Args {
		var err error
		if args[i], err = checkNode(arg, depth+1, declarations, distinct); err != nil {
			return Node{}, err
		}
	}
	return Node{Op: p.Op, Args: args}, nil
}

// checkOperands decodes the operands of a leaf against the field's scalar type.
func checkOperands(p processing.Predicate, scalar processing.FieldSchema) ([]any, error) {
	switch p.Op {
	case "present":
		if p.Value == nil || p.Value.Kind != processing.BooleanValue || p.Values != nil {
			return nil, invalidInput("invalid presence operand")
		}
		value, err := p.Value.Native(processing.FieldSchema{Kind: processing.BooleanValue})
		return []any{value}, err
	case "in":
		if p.Value != nil || len(p.Values) == 0 {
			return nil, invalidInput("invalid membership operand")
		}
		if len(p.Values) > processing.MaxArguments {
			return nil, resourceLimit("membership collection boundary")
		}
		values := make([]any, len(p.Values))
		for i, value := range p.Values {
			var err error
			if values[i], err = value.Native(scalar); err != nil {
				return nil, err
			}
			values[i] = atPrecision(values[i])
		}
		return values, nil
	case "eq", "lt", "lte", "gt", "gte", "contains":
		return checkComparison(p, scalar)
	}
	return nil, invalidInput("unknown operator")
}

func checkComparison(p processing.Predicate, scalar processing.FieldSchema) ([]any, error) {
	if p.Value == nil || p.Values != nil {
		return nil, invalidInput("malformed comparison")
	}
	if p.Op == "contains" && scalar.Kind != processing.StringValue {
		return nil, invalidInput("contains requires a string field")
	}
	if p.Op != "eq" && p.Op != "contains" && (!isOrdered(scalar.Kind) || p.Value.Kind == processing.NullValue) {
		return nil, invalidInput("ordered comparison requires nonnull numeric or timestamp operands")
	}
	value, err := p.Value.Native(scalar)
	if err != nil {
		return nil, err
	}
	if p.Op == "contains" && value == nil {
		return nil, invalidInput("contains requires a nonnull string")
	}
	return []any{atPrecision(value)}, nil
}

func isOrdered(kind processing.ValueKind) bool {
	return kind == processing.IntegerValue || kind == processing.NumberValue || kind == processing.TimestampValue
}
