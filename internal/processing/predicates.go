package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"cel.dev/cel-go/cel"
	"cel.dev/cel-go/common/types"
	"cel.dev/cel-go/interpreter"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// FieldRef names an explicitly declared field, never executable expression text.
type FieldRef struct {
	Namespace string   `json:"namespace"`
	Path      []string `json:"path"`
}

func (f FieldRef) validate() error {
	if !slices.Contains([]string{"source", "target", "step"}, f.Namespace) || len(f.Path) == 0 || len(f.Path) > MaxDepth {
		return invalid(errors.New("invalid field reference"))
	}
	for _, segment := range f.Path {
		if segment == "" || !utf8.ValidString(segment) || strings.ContainsRune(segment, '\x00') {
			return invalid(errors.New("invalid field path"))
		}
	}
	return nil
}

func (f FieldRef) key() string { return f.Namespace + "\x00" + strings.Join(f.Path, "\x00") }

// Dependency retains source classification/context, not an authorization snapshot.
type Dependency struct {
	TransmissionID  uuid.UUID `json:"transmissionId"`
	SourceRevision  int64     `json:"sourceRevision,string"`
	ContextRevision int64     `json:"contextRevision,string"`
	NetworkID       uuid.UUID `json:"networkId"`
	TargetPosition  *uint32   `json:"targetPosition,omitempty"`
	Groups          []string  `json:"groups"`
}

func (d Dependency) validate() error {
	if d.TransmissionID == uuid.Nil || d.NetworkID == uuid.Nil || d.SourceRevision <= 0 || d.ContextRevision <= 0 ||
		len(d.Groups) == 0 || len(d.Groups) > MaxArguments {
		return invalid(errors.New("invalid source dependency"))
	}
	for _, group := range d.Groups {
		if !authz.ValidField(group) {
			return invalid(errors.New("undeclared dependency field group"))
		}
	}
	return nil
}

// FieldState classifies a field value as present, readably absent, or hidden from the
// current authority. Hidden fields carry no Value and make dependent predicates
// Indeterminate.
type FieldState string

const (
	// FieldPresent marks a field with a Value (possibly an explicit null).
	FieldPresent FieldState = "present"
	// FieldAbsent marks a field the reader may see but that has no value; only optional
	// fields may be absent.
	FieldAbsent FieldState = "absent"
	// FieldHidden marks a field whose value the current authority may not read.
	FieldHidden FieldState = "hidden"
)

// FieldValue distinguishes readable absence from hidden data and retains dependencies.
type FieldValue struct {
	State        FieldState   `json:"state"`
	Value        *Value       `json:"value,omitempty"`
	Dependencies []Dependency `json:"dependencies"`
}

// Declarations is supplied by source/step owners, not inferred from untrusted JSON.
type Declarations interface {
	FieldSchema(FieldRef) (FieldSchema, error)
}

// FieldReader resolves only captured, correlated data; evaluation performs no Core/SQL reads.
type FieldReader interface {
	LookupField(context.Context, FieldRef) (FieldValue, error)
}

// Predicate is the closed typed-predicate 1.0 representation.
type Predicate struct {
	Op     string      `json:"op"`
	Field  *FieldRef   `json:"field,omitempty"`
	Value  *Value      `json:"value,omitempty"`
	Values []Value     `json:"values,omitempty"`
	Args   []Predicate `json:"args,omitempty"`
}

// Truth is the three-valued outcome of a predicate.
type Truth string

// The predicate outcomes.
const (
	DefiniteFalse Truth = "false"
	DefiniteTrue  Truth = "true"
	// Indeterminate means the result depends on a hidden field. Branch steps fail with an
	// authority-unavailable failure rather than choosing a side.
	Indeterminate Truth = "indeterminate"
)

// PredicateResult is a predicate outcome together with the union of the source
// dependencies of every field it read.
type PredicateResult struct {
	Truth        Truth
	Dependencies []Dependency
}

type predicateField struct {
	reference FieldRef
	schema    FieldSchema
	name      string
}

// PredicateProgram contains a checked immutable CEL program with closed owner declarations.
type PredicateProgram struct {
	program         cel.Program
	env             *cel.Env
	ast             *cel.Ast
	constants       map[string]any
	fields          []predicateField
	byteComparisons []byteComparison
}

// MaxPredicateCELCost is the CEL cost budget for one predicate evaluation, shared between
// the CEL program and the byte-wise comparisons of bytes operands. Exceeding it is a
// resource-limit failure.
const MaxPredicateCELCost uint64 = 2_000_000

type byteComparison struct {
	field      int
	candidates []any
}

func celType(schema FieldSchema) *cel.Type {
	var result *cel.Type
	switch schema.Kind {
	case StringValue:
		result = cel.StringType
	case BooleanValue:
		result = cel.BoolType
	case IntegerValue:
		result = cel.IntType
	case NumberValue:
		result = cel.DoubleType
	case TimestampValue:
		result = cel.TimestampType
	case BytesValue:
		result = cel.BytesType
	case ObjectValue:
		result = cel.MapType(cel.StringType, cel.DynType)
	case ListValue:
		result = cel.ListType(celType(*schema.Element))
	}
	if schema.Nullable {
		result = cel.NullableType(result)
	}
	return result
}

func cloneSchema(schema FieldSchema) FieldSchema {
	if schema.Properties != nil {
		properties := make(map[string]FieldSchema, len(schema.Properties))
		for name, child := range schema.Properties {
			properties[name] = cloneSchema(child)
		}
		schema.Properties = properties
	}
	if schema.Element != nil {
		element := cloneSchema(*schema.Element)
		schema.Element = &element
	}
	return schema
}

// CompilePredicate validates structured input and prepares a restricted checked program.
// The caller prepares immutable definitions outside source transactions and reuses the result.
func CompilePredicate(ctx context.Context, predicate Predicate, declarations Declarations) (*PredicateProgram, error) {
	if declarations == nil || ctx == nil {
		return nil, invalid(errors.New("missing predicate declarations/context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(predicate)
	if err != nil {
		return nil, invalid(err)
	}
	if len(encoded) > MaxInputBytes {
		return nil, resource(errors.New("predicate encoded boundary"))
	}
	options := []cel.EnvOption{cel.ClearMacros(), cel.ParserExpressionSizeLimit(MaxInputBytes),
		cel.ParserRecursionLimit(128), cel.ExpressionNodeLimit(20000), cel.ExpressionNestingDepthLimit(128)}
	constants := make(map[string]any)
	fields := make([]predicateField, 0)
	byteComparisons := make([]byteComparison, 0)
	fieldIndexes := make(map[string]int)
	var lower func(Predicate, int) (string, error)
	lower = func(p Predicate, depth int) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if depth > MaxDepth {
			return "", resource(errors.New("predicate nesting boundary"))
		}
		if p.Op == "and" || p.Op == "or" || p.Op == "not" {
			if p.Field != nil || p.Value != nil || p.Values != nil || len(p.Args) == 0 ||
				(p.Op == "not" && len(p.Args) != 1) {
				return "", invalid(errors.New("invalid logical operands"))
			}
			if len(p.Args) > MaxArguments {
				return "", resource(errors.New("predicate argument boundary"))
			}
			args := make([]string, len(p.Args))
			for i, arg := range p.Args {
				var err error
				args[i], err = lower(arg, depth+1)
				if err != nil {
					return "", err
				}
			}
			if p.Op == "not" {
				return "(!" + args[0] + ")", nil
			}
			separator := " && "
			if p.Op == "or" {
				separator = " || "
			}
			return "(" + strings.Join(args, separator) + ")", nil
		}
		if p.Field == nil || len(p.Args) != 0 {
			return "", invalid(errors.New("invalid field operands"))
		}
		if err := p.Field.validate(); err != nil {
			return "", err
		}
		index, exists := fieldIndexes[p.Field.key()]
		if !exists {
			schema, err := declarations.FieldSchema(*p.Field)
			if err != nil {
				return "", platform.NewFailure(platform.FailureCode(err), err)
			}
			if err := schema.Validate(); err != nil {
				return "", err
			}
			index = len(fields)
			fieldIndexes[p.Field.key()] = index
			reference := *p.Field
			reference.Path = slices.Clone(reference.Path)
			name := fmt.Sprintf("f%d", index)
			fields = append(fields, predicateField{reference: reference, schema: cloneSchema(schema), name: name})
			options = append(options, cel.Variable(name, celType(schema)), cel.Variable("p"+name, cel.BoolType))
		}
		field := fields[index]
		if p.Op == "present" {
			if p.Value == nil || p.Value.Kind != BooleanValue || p.Values != nil {
				return "", invalid(errors.New("invalid presence operand"))
			}
			value, err := p.Value.Native(FieldSchema{Kind: BooleanValue})
			if err != nil {
				return "", err
			}
			if value.(bool) {
				return "(p" + field.name + ")", nil
			}
			return "(!p" + field.name + ")", nil
		}
		name := fmt.Sprintf("k%d", len(constants))
		if p.Op == "in" {
			if p.Value != nil || len(p.Values) == 0 {
				return "", invalid(errors.New("invalid membership operand"))
			}
			if len(p.Values) > MaxArguments {
				return "", resource(errors.New("typed membership collection boundary"))
			}
			native := make([]any, len(p.Values))
			for i, value := range p.Values {
				var err error
				native[i], err = value.Native(field.schema)
				if err != nil {
					return "", err
				}
			}
			if schemaContainsBytes(field.schema) {
				byteComparisons = append(byteComparisons, byteComparison{field: index, candidates: native})
			}
			constants[name] = native
			options = append(options, cel.Variable(name, cel.ListType(celType(field.schema))))
		} else {
			if !slices.Contains([]string{"eq", "lt", "lte", "gt", "gte", "contains"}, p.Op) ||
				p.Value == nil || p.Values != nil {
				return "", invalid(errors.New("unknown or malformed comparison"))
			}
			if p.Op == "contains" && field.schema.Kind != StringValue {
				return "", invalid(errors.New("contains requires a string field"))
			}
			if p.Op != "eq" && p.Op != "contains" &&
				(!slices.Contains([]ValueKind{IntegerValue, NumberValue, TimestampValue}, field.schema.Kind) || p.Value.Kind == NullValue) {
				return "", invalid(errors.New("ordered comparison requires nonnull numeric/timestamp operands"))
			}
			value, err := p.Value.Native(field.schema)
			if err != nil {
				return "", err
			}
			valueType := celType(field.schema)
			if value == nil {
				valueType = cel.NullType
			}
			constants[name] = value
			if schemaContainsBytes(field.schema) {
				byteComparisons = append(byteComparisons, byteComparison{field: index, candidates: []any{value}})
			}
			options = append(options, cel.Variable(name, valueType))
		}
		expression := field.name + " " + map[string]string{"eq": "==", "in": "in", "lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[p.Op] + " " + name
		if p.Op == "contains" {
			expression = field.name + ".contains(" + name + ")"
		}
		return "(p" + field.name + " && (" + expression + "))", nil
	}
	expression, err := lower(predicate, 1)
	if err != nil {
		return nil, err
	}
	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, invalid(err)
	}
	ast, issues := env.Compile(expression)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if issues.Err() != nil {
		return nil, invalid(issues.Err())
	}
	if ast.OutputType() != cel.BoolType {
		return nil, invalid(errors.New("predicate must produce boolean"))
	}
	program, err := env.Program(ast, cel.Globals(constants), cel.EvalOptions(cel.OptPartialEval, cel.OptTrackCost),
		cel.CostLimit(MaxPredicateCELCost), cel.MemoryLimit(2<<20), cel.InterruptCheckFrequency(1))
	if err != nil {
		return nil, invalid(err)
	}
	return &PredicateProgram{program: program, env: env, ast: ast, constants: constants,
		fields: fields, byteComparisons: byteComparisons}, nil
}

// Evaluate resolves every declared operand before CEL evaluation, retaining all dependencies.
// Hidden operands remain unknown even when boolean short-circuiting decides the result.
func (p *PredicateProgram) Evaluate(ctx context.Context, reader FieldReader) (PredicateResult, error) {
	if p == nil || p.program == nil || reader == nil || ctx == nil {
		return PredicateResult{}, invalid(errors.New("missing predicate program/reader/context"))
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	input := make(map[string]any, len(p.fields)*2)
	var unknowns []*cel.AttributePatternType
	var dependencies []Dependency
	var targetContext *dependencyKey
	encodedSize := 0
	decodedBytes := 0
	fieldValues := make([]FieldValue, len(p.fields))
	for fieldIndex, field := range p.fields {
		if err := ctx.Err(); err != nil {
			return PredicateResult{}, err
		}
		value, err := reader.LookupField(ctx, field.reference)
		if err != nil {
			return PredicateResult{}, platform.NewFailure(platform.FailureCode(err), err)
		}
		fieldValues[fieldIndex] = value
		if !slices.Contains([]FieldState{FieldPresent, FieldAbsent, FieldHidden}, value.State) ||
			(value.State == FieldPresent && value.Value == nil) || (value.State != FieldPresent && value.Value != nil) ||
			(value.State == FieldAbsent && !field.schema.Optional) ||
			(field.reference.Namespace != "step" && len(value.Dependencies) == 0) {
			return PredicateResult{}, invalid(errors.New("invalid classified field state"))
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return PredicateResult{}, invalid(err)
		}
		encodedSize += len(encoded)
		if encodedSize > MaxInputBytes {
			return PredicateResult{}, resource(errors.New("predicate input/dependency boundary"))
		}
		fieldBytes, err := decodedBytesInJSON(encoded)
		if err != nil {
			return PredicateResult{}, err
		}
		decodedBytes += fieldBytes
		if decodedBytes > MaxInputBytes {
			return PredicateResult{}, resource(errors.New("predicate decoded bytes boundary"))
		}
		for _, dependency := range value.Dependencies {
			if err := dependency.validate(); err != nil {
				return PredicateResult{}, err
			}
			if field.reference.Namespace == "target" && dependency.TargetPosition != nil {
				key := dependency.key()
				if targetContext != nil && *targetContext != key {
					return PredicateResult{}, invalid(errors.New("predicate mixes target contexts"))
				}
				targetContext = &key
			}
			dependency.Groups = slices.Clone(dependency.Groups)
			if dependency.TargetPosition != nil {
				position := *dependency.TargetPosition
				dependency.TargetPosition = &position
			}
			dependencies = append(dependencies, dependency)
		}
		input["p"+field.name] = value.State == FieldPresent
		if value.State == FieldHidden {
			unknowns = append(unknowns, cel.AttributePattern(field.name), cel.AttributePattern("p"+field.name))
		} else if value.State == FieldPresent {
			native, err := value.Value.Native(field.schema)
			if err != nil {
				return PredicateResult{}, err
			}
			input[field.name] = native
		}
	}
	activation, err := cel.PartialVars(input, unknowns...)
	if err != nil {
		return PredicateResult{}, invalid(err)
	}
	byteCost := uint64(0)
	for _, comparison := range p.byteComparisons {
		field := fieldValues[comparison.field]
		if field.State != FieldPresent || field.Value == nil || field.Value.Kind == NullValue {
			continue
		}
		observed := input[p.fields[comparison.field].name]
		for _, candidate := range comparison.candidates {
			byteCost += byteComparisonCost(observed, candidate)
			if byteCost >= MaxPredicateCELCost {
				return PredicateResult{}, resource(errors.New("CEL byte comparison cost boundary"))
			}
		}
	}
	program := p.program
	if byteCost != 0 {
		program, err = p.env.Program(p.ast, cel.Globals(p.constants),
			cel.EvalOptions(cel.OptPartialEval, cel.OptTrackCost),
			cel.CostLimit(MaxPredicateCELCost-byteCost), cel.MemoryLimit(2<<20),
			cel.InterruptCheckFrequency(1))
		if err != nil {
			return PredicateResult{}, invalid(err)
		}
	}
	result, details, err := program.ContextEval(ctx, activation)
	if err != nil {
		if ctx.Err() != nil {
			return PredicateResult{}, ctx.Err()
		}
		var cancellation interpreter.EvalCancelledError
		if errors.As(err, &cancellation) &&
			(cancellation.Cause == interpreter.CostLimitExceeded || cancellation.Cause == interpreter.MemoryLimitExceeded) {
			return PredicateResult{}, resource(err)
		}
		return PredicateResult{}, invalid(err)
	}
	if details != nil {
		if actual := details.ActualCost(); actual != nil && *actual+byteCost > MaxPredicateCELCost {
			return PredicateResult{}, resource(errors.New("CEL combined comparison cost boundary"))
		}
	}
	if err := ctx.Err(); err != nil {
		return PredicateResult{}, err
	}
	truth := Indeterminate
	if !types.IsUnknown(result) {
		boolean, ok := result.Value().(bool)
		if !ok {
			return PredicateResult{}, invalid(errors.New("invalid predicate runtime result"))
		}
		truth = DefiniteFalse
		if boolean {
			truth = DefiniteTrue
		}
	}
	dependencies, err = UnionDependencies(dependencies)
	if err != nil {
		return PredicateResult{}, err
	}
	return PredicateResult{Truth: truth, Dependencies: dependencies}, nil
}

// byteComparisonCost charges maximum operand lengths at each corresponding bytes leaf.
// Composite equality and membership retain the sum rather than treating bytes as unit-cost scalars.
func byteComparisonCost(left, right any) uint64 {
	switch observed := left.(type) {
	case []byte:
		candidate, ok := right.([]byte)
		if !ok {
			return 0
		}
		return uint64(max(len(observed), len(candidate)))
	case []any:
		candidates, ok := right.([]any)
		if !ok {
			return 0
		}
		var total uint64
		for i := 0; i < min(len(observed), len(candidates)); i++ {
			total += byteComparisonCost(observed[i], candidates[i])
		}
		return total
	case map[string]any:
		candidates, ok := right.(map[string]any)
		if !ok {
			return 0
		}
		var total uint64
		for key, value := range observed {
			total += byteComparisonCost(value, candidates[key])
		}
		return total
	default:
		return 0
	}
}
