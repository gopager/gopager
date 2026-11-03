package processing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"cel.dev/cel-go/cel"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type predicateFixture struct {
	schemas map[string]FieldSchema
	values  map[string]FieldValue
	mu      sync.Mutex
	calls   map[string]int
}

func (f *predicateFixture) FieldSchema(ref FieldRef) (FieldSchema, error) {
	schema, ok := f.schemas[ref.key()]
	if !ok {
		return FieldSchema{}, invalid(errors.New("undeclared fixture field"))
	}
	return schema, nil
}

func (f *predicateFixture) LookupField(_ context.Context, ref FieldRef) (FieldValue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[ref.key()]++
	value, ok := f.values[ref.key()]
	if !ok {
		return FieldValue{}, invalid(errors.New("unavailable fixture field"))
	}
	return value, nil
}

func predicateDependency(groups ...string) Dependency {
	return Dependency{TransmissionID: uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		NetworkID:      uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		SourceRevision: 1, ContextRevision: 1, Groups: groups}
}

func fixtureFor(ref FieldRef, schema FieldSchema, value *Value, state FieldState) *predicateFixture {
	return &predicateFixture{schemas: map[string]FieldSchema{ref.key(): schema},
		values: map[string]FieldValue{ref.key(): {State: state, Value: value,
			Dependencies: []Dependency{predicateDependency("payload")}}}, calls: make(map[string]int)}
}

func TestPredicateTypedCompatibility(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"text"}}
	integer := FieldSchema{Kind: IntegerValue}
	for _, item := range []struct {
		name       string
		schema     FieldSchema
		op         string
		input      Value
		comparison Value
		truth      Truth
	}{
		{"exact int64", integer, "eq", Value{IntegerValue, json.RawMessage(`"9223372036854775807"`)},
			Value{IntegerValue, json.RawMessage(`"9223372036854775807"`)}, DefiniteTrue},
		{"ordered int64", integer, "lt", Value{IntegerValue, json.RawMessage(`"-9223372036854775808"`)},
			Value{IntegerValue, json.RawMessage(`"9223372036854775807"`)}, DefiniteTrue},
		{"finite number", FieldSchema{Kind: NumberValue}, "gte", Value{NumberValue, json.RawMessage(`1.5`)},
			Value{NumberValue, json.RawMessage(`1.25`)}, DefiniteTrue},
		{"offset timestamp", FieldSchema{Kind: TimestampValue}, "eq",
			Value{TimestampValue, json.RawMessage(`"2026-11-01T12:00:00+01:00"`)},
			Value{TimestampValue, json.RawMessage(`"2026-11-01T11:00:00Z"`)}, DefiniteTrue},
		{"literal not source", FieldSchema{Kind: StringValue}, "contains",
			Value{StringValue, json.RawMessage(`"x\" || true || \"y"`)},
			Value{StringValue, json.RawMessage(`"\" || true || \""`)}, DefiniteTrue},
		{"no normalization", FieldSchema{Kind: StringValue}, "eq",
			Value{StringValue, json.RawMessage(`"\u00e9"`)},
			Value{StringValue, json.RawMessage(`"e\u0301"`)}, DefiniteFalse},
		{"explicit null", FieldSchema{Kind: StringValue, Nullable: true}, "eq",
			Value{NullValue, json.RawMessage(`null`)}, Value{NullValue, json.RawMessage(`null`)}, DefiniteTrue},
		{"ordered runtime null", FieldSchema{Kind: IntegerValue, Nullable: true}, "lt",
			Value{NullValue, json.RawMessage(`null`)}, Value{IntegerValue, json.RawMessage(`"1"`)}, ""},
		{"closed list", FieldSchema{Kind: ListValue, Element: &integer}, "eq",
			Value{ListValue, json.RawMessage(`[{"kind":"integer","value":"7"}]`)},
			Value{ListValue, json.RawMessage(`[{"kind":"integer","value":"7"}]`)}, DefiniteTrue},
		{"closed object timestamp", FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{
			"time": {Kind: TimestampValue}}}, "eq",
			Value{ObjectValue, json.RawMessage(`{"time":{"kind":"timestamp","value":"2026-11-01T12:00:00+01:00"}}`)},
			Value{ObjectValue, json.RawMessage(`{"time":{"kind":"timestamp","value":"2026-11-01T11:00:00Z"}}`)}, DefiniteTrue},
		{"nullable object", FieldSchema{Kind: ObjectValue, Nullable: true, Properties: map[string]FieldSchema{}}, "eq",
			Value{NullValue, json.RawMessage(`null`)}, Value{NullValue, json.RawMessage(`null`)}, DefiniteTrue},
	} {
		t.Run(item.name, func(t *testing.T) {
			fixture := fixtureFor(ref, item.schema, &item.input, FieldPresent)
			program, err := CompilePredicate(context.Background(), Predicate{Op: item.op, Field: &ref, Value: &item.comparison}, fixture)
			if err != nil {
				t.Fatal(err)
			}
			result, err := program.Evaluate(context.Background(), fixture)
			if item.truth == "" {
				if platform.FailureCode(err) != platform.CodeInvalidInput {
					t.Fatalf("invalid ordered null returned %v", err)
				}
			} else if err != nil || result.Truth != item.truth {
				t.Fatalf("got %v, %v; expected %s", result.Truth, err, item.truth)
			}
		})
	}
}

func TestPredicateBytesOperationsAndClassifiedStates(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"binary"}}
	schema := FieldSchema{Kind: BytesValue}
	observed, err := Literal(BytesValue, []byte{0, 1, 2, 255})
	if err != nil {
		t.Fatal(err)
	}
	other, err := Literal(BytesValue, []byte{0, 1, 2, 255})
	if err != nil {
		t.Fatal(err)
	}
	different, err := Literal(BytesValue, []byte{0, 1, 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name  string
		op    string
		value *Value
		vals  []Value
		want  Truth
	}{
		{name: "equal", op: "eq", value: &other, want: DefiniteTrue},
		{name: "not equal", op: "eq", value: &different, want: DefiniteFalse},
		{name: "membership", op: "in", vals: []Value{different, other}, want: DefiniteTrue},
	} {
		t.Run(item.name, func(t *testing.T) {
			fixture := fixtureFor(ref, schema, &observed, FieldPresent)
			program, err := CompilePredicate(context.Background(),
				Predicate{Op: item.op, Field: &ref, Value: item.value, Values: item.vals}, fixture)
			if err != nil {
				t.Fatal(err)
			}
			result, err := program.Evaluate(context.Background(), fixture)
			if err != nil || result.Truth != item.want {
				t.Fatalf("got %s, %v; wanted %s", result.Truth, err, item.want)
			}
		})
	}

	trueValue, err := Literal(BooleanValue, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name   string
		schema FieldSchema
		value  *Value
		state  FieldState
		want   Truth
	}{
		{name: "present bytes", schema: schema, value: &observed, state: FieldPresent, want: DefiniteTrue},
		{name: "explicit nullable null", schema: FieldSchema{Kind: BytesValue, Nullable: true},
			value: &Value{Kind: NullValue, Value: json.RawMessage("null")}, state: FieldPresent, want: DefiniteTrue},
		{name: "absent optional bytes", schema: FieldSchema{Kind: BytesValue, Optional: true},
			state: FieldAbsent, want: DefiniteFalse},
		{name: "hidden bytes", schema: schema, state: FieldHidden, want: Indeterminate},
	} {
		t.Run("present/"+item.name, func(t *testing.T) {
			fixture := fixtureFor(ref, item.schema, item.value, item.state)
			program, err := CompilePredicate(context.Background(),
				Predicate{Op: "present", Field: &ref, Value: &trueValue}, fixture)
			if err != nil {
				t.Fatal(err)
			}
			result, err := program.Evaluate(context.Background(), fixture)
			if err != nil || result.Truth != item.want {
				t.Fatalf("got %s, %v; wanted %s", result.Truth, err, item.want)
			}
		})
	}

	nullable := FieldSchema{Kind: BytesValue, Nullable: true}
	null := Value{Kind: NullValue, Value: json.RawMessage("null")}
	fixture := fixtureFor(ref, nullable, &null, FieldPresent)
	program, err := CompilePredicate(context.Background(),
		Predicate{Op: "eq", Field: &ref, Value: &null}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(context.Background(), fixture)
	if err != nil || result.Truth != DefiniteTrue {
		t.Fatalf("null equality got %s, %v", result.Truth, err)
	}
}

func TestPredicateBytesRejectsStringAndOrderedOperations(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"binary"}}
	fixture := &predicateFixture{schemas: map[string]FieldSchema{ref.key(): {Kind: BytesValue}},
		values: map[string]FieldValue{}, calls: make(map[string]int)}
	value, err := Literal(BytesValue, []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range []string{"contains", "lt", "lte", "gt", "gte"} {
		t.Run(op, func(t *testing.T) {
			_, err := CompilePredicate(context.Background(),
				Predicate{Op: op, Field: &ref, Value: &value}, fixture)
			if platform.FailureCode(err) != platform.CodeInvalidInput {
				t.Fatalf("unsupported bytes operation accepted: %v", err)
			}
		})
	}
}

func TestPredicateBytesComparisonsAreChargedByLength(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"binary"}}
	const length = 400_000
	observed, err := Literal(BytesValue, make([]byte, length))
	if err != nil {
		t.Fatal(err)
	}
	fixture := fixtureFor(ref, FieldSchema{Kind: BytesValue}, &observed, FieldPresent)
	equal, err := Literal(BytesValue, []byte{0})
	if err != nil {
		t.Fatal(err)
	}
	args := make([]Predicate, 6)
	for i := range args {
		candidate := equal
		args[i] = Predicate{Op: "eq", Field: &ref, Value: &candidate}
	}
	program, err := CompilePredicate(context.Background(), Predicate{Op: "and", Args: args}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := program.Evaluate(context.Background(), fixture); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("repeated byte comparisons escaped CEL cost boundary: %v", err)
	}
	for _, count := range []int{4, 5} {
		values := make([]Value, count)
		for i := range values {
			values[i] = equal
		}
		program, err := CompilePredicate(context.Background(), Predicate{Op: "in", Field: &ref, Values: values}, fixture)
		if err != nil {
			t.Fatal(err)
		}
		result, err := program.Evaluate(context.Background(), fixture)
		if count == 4 && (err != nil || result.Truth != DefiniteFalse) {
			t.Fatalf("membership below ceiling rejected: %+v %v", result, err)
		}
		if count == 5 && platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("membership sum at CEL ceiling accepted: %v", err)
		}
	}
}

func TestPredicateHiddenAbsenceAndConservativeDependencies(t *testing.T) {
	left := FieldRef{Namespace: "source", Path: []string{"left"}}
	right := FieldRef{Namespace: "source", Path: []string{"right"}}
	yes := Value{BooleanValue, json.RawMessage(`true`)}
	no := Value{BooleanValue, json.RawMessage(`false`)}
	for _, item := range []struct {
		op    string
		left  Value
		truth Truth
	}{
		{"and", no, DefiniteFalse}, {"and", yes, Indeterminate},
		{"or", yes, DefiniteTrue}, {"or", no, Indeterminate},
	} {
		fixture := fixtureFor(left, FieldSchema{Kind: BooleanValue}, &item.left, FieldPresent)
		fixture.schemas[right.key()] = FieldSchema{Kind: BooleanValue}
		fixture.values[right.key()] = FieldValue{State: FieldHidden, Dependencies: []Dependency{predicateDependency("address")}}
		predicate := Predicate{Op: item.op, Args: []Predicate{
			{Op: "eq", Field: &left, Value: &yes}, {Op: "eq", Field: &right, Value: &yes}}}
		program, err := CompilePredicate(context.Background(), predicate, fixture)
		if err != nil {
			t.Fatal(err)
		}
		result, err := program.Evaluate(context.Background(), fixture)
		if err != nil || result.Truth != item.truth || fixture.calls[right.key()] != 1 ||
			len(result.Dependencies) != 1 || strings.Join(result.Dependencies[0].Groups, ",") != "address,payload" {
			t.Fatalf("short-circuit lost truth/dependencies: %+v, %v", result, err)
		}
	}
	for _, state := range []FieldState{FieldAbsent, FieldHidden} {
		fixture := fixtureFor(left, FieldSchema{Kind: BooleanValue, Optional: true}, nil, state)
		for _, predicate := range []Predicate{
			{Op: "eq", Field: &left, Value: &yes},
			{Op: "present", Field: &left, Value: &no},
			{Op: "not", Args: []Predicate{{Op: "eq", Field: &left, Value: &yes}}},
		} {
			program, err := CompilePredicate(context.Background(), predicate, fixture)
			if err != nil {
				t.Fatal(err)
			}
			result, err := program.Evaluate(context.Background(), fixture)
			expected := Indeterminate
			if state == FieldAbsent {
				expected = DefiniteTrue
				if predicate.Op == "eq" {
					expected = DefiniteFalse
				}
			}
			if err != nil || result.Truth != expected {
				t.Fatalf("state %s/op %s got %s, %v", state, predicate.Op, result.Truth, err)
			}
		}
	}
}

func TestPredicateRejectsMixedTargetContexts(t *testing.T) {
	left := FieldRef{Namespace: "target", Path: []string{"network"}}
	right := FieldRef{Namespace: "target", Path: []string{"unit"}}
	yes := Value{BooleanValue, json.RawMessage(`true`)}
	fixture := fixtureFor(left, FieldSchema{Kind: BooleanValue}, &yes, FieldPresent)
	fixture.schemas[right.key()] = FieldSchema{Kind: BooleanValue}
	first, second := uint32(0), uint32(1)
	d1, d2 := predicateDependency("unit"), predicateDependency("unit")
	d1.TargetPosition, d2.TargetPosition = &first, &second
	fixture.values[left.key()] = FieldValue{State: FieldPresent, Value: &yes, Dependencies: []Dependency{d1}}
	fixture.values[right.key()] = FieldValue{State: FieldPresent, Value: &yes, Dependencies: []Dependency{d2}}
	program, err := CompilePredicate(context.Background(), Predicate{Op: "and", Args: []Predicate{
		{Op: "eq", Field: &left, Value: &yes}, {Op: "eq", Field: &right, Value: &yes}}}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := program.Evaluate(context.Background(), fixture); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("mixed targets accepted: %v", err)
	}
}

func TestPredicateBoundsAndParallelReuse(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"text"}}
	text := Value{StringValue, json.RawMessage(`"x"`)}
	fixture := fixtureFor(ref, FieldSchema{Kind: StringValue}, &text, FieldPresent)
	leaf := Predicate{Op: "eq", Field: &ref, Value: &text}
	for count := MaxArguments; count <= MaxArguments+1; count++ {
		predicate := Predicate{Op: "and", Args: make([]Predicate, count)}
		for i := range predicate.Args {
			predicate.Args[i] = leaf
		}
		program, err := CompilePredicate(context.Background(), predicate, fixture)
		if count > MaxArguments {
			if platform.FailureCode(err) != platform.CodeResourceLimit {
				t.Fatalf("argument +1 accepted: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		var workers sync.WaitGroup
		for range 8 {
			workers.Go(func() {
				result, err := program.Evaluate(context.Background(), fixture)
				if err != nil || result.Truth != DefiniteTrue {
					t.Errorf("parallel evaluation: %s, %v", result.Truth, err)
				}
			})
		}
		workers.Wait()
	}
	for _, op := range []string{"matches", "free-cel", "unknown"} {
		if _, err := CompilePredicate(context.Background(), Predicate{Op: op, Field: &ref, Value: &text}, fixture); err == nil {
			t.Fatalf("unapproved operator %s accepted", op)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CompilePredicate(ctx, leaf, fixture); !errors.Is(err, context.Canceled) {
		t.Fatalf("compile ignored cancellation: %v", err)
	}
	program, err := CompilePredicate(context.Background(), leaf, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := program.Evaluate(ctx, fixture); !errors.Is(err, context.Canceled) {
		t.Fatalf("evaluation ignored cancellation: %v", err)
	}
}

func TestDependencyUnionPreservesContextsAndOwnership(t *testing.T) {
	position := uint32(0)
	left, right := predicateDependency("payload", "payload"), predicateDependency("address")
	left.TargetPosition, right.TargetPosition = &position, &position
	other := right
	other.ContextRevision = 2
	result, err := UnionDependencies([]Dependency{left, right, other})
	if err != nil || len(result) != 2 || strings.Join(result[0].Groups, ",") != "address,payload" {
		t.Fatalf("bad union: %+v, %v", result, err)
	}

	result[0].Groups[0] = "unit"
	*result[0].TargetPosition = 8
	if left.Groups[0] != "payload" || right.Groups[0] != "address" || position != 0 {
		t.Fatal("result aliases source classification")
	}
}

func TestPredicateExactEncodedInputBoundary(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"text"}}
	comparison := Value{StringValue, json.RawMessage(`"x"`)}
	value := Value{StringValue, json.RawMessage(`""`)}
	fixture := fixtureFor(ref, FieldSchema{Kind: StringValue}, &value, FieldPresent)
	base, err := json.Marshal(fixture.values[ref.key()])
	if err != nil {
		t.Fatal(err)
	}
	program, err := CompilePredicate(context.Background(), Predicate{Op: "eq", Field: &ref, Value: &comparison}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	for extra := 0; extra <= 1; extra++ {
		value.Value = json.RawMessage(`"` + strings.Repeat("a", MaxInputBytes-len(base)+extra) + `"`)
		encoded, err := json.Marshal(fixture.values[ref.key()])
		if err != nil || len(encoded) != MaxInputBytes+extra {
			t.Fatalf("fixture does not measure exact encoded boundary: %d, %v", len(encoded), err)
		}
		result, err := program.Evaluate(context.Background(), fixture)
		if extra == 0 {
			if err != nil || result.Truth != DefiniteFalse {
				t.Fatalf("exact encoded boundary failed: %s, %v", result.Truth, err)
			}
		} else if platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("encoded input +1 accepted: %v", err)
		}
	}
}

func TestPredicateCELResourceFailuresAreSafe(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"text"}}
	text := Value{StringValue, json.RawMessage(`"private input value"`)}
	fixture := fixtureFor(ref, FieldSchema{Kind: StringValue}, &text, FieldPresent)
	env, err := cel.NewEnv(cel.Variable("f0", cel.StringType), cel.Variable("pf0", cel.BoolType),
		cel.Variable("k0", cel.StringType))
	if err != nil {
		t.Fatal(err)
	}
	ast, issues := env.Compile(`pf0 && f0.contains(k0)`)
	if issues.Err() != nil {
		t.Fatal(issues.Err())
	}
	for _, limit := range []cel.ProgramOption{cel.CostLimit(1), cel.MemoryLimit(1)} {
		compiled, err := env.Program(ast, cel.Globals(map[string]any{"k0": "value"}), limit)
		if err != nil {
			t.Fatal(err)
		}
		program := &PredicateProgram{program: compiled, fields: []predicateField{
			{reference: ref, schema: FieldSchema{Kind: StringValue}, name: "f0"}}}
		result, err := program.Evaluate(context.Background(), fixture)
		if platform.FailureCode(err) != platform.CodeResourceLimit || result.Truth != "" ||
			strings.Contains(err.Error(), "private") {
			t.Fatalf("CEL cancellation was not a safe failed result: %+v, %v", result, err)
		}
	}

}

func TestPredicateMembershipUsesFilterNotTypedListBoundary(t *testing.T) {
	ref := FieldRef{Namespace: "source", Path: []string{"text"}}
	value := Value{StringValue, json.RawMessage(`"x"`)}
	fixture := fixtureFor(ref, FieldSchema{Kind: StringValue}, &value, FieldPresent)
	for count := MaxArguments; count <= MaxArguments+1; count++ {
		predicate := Predicate{Op: "in", Field: &ref, Values: make([]Value, count)}
		for i := range predicate.Values {
			predicate.Values[i] = value
		}
		program, err := CompilePredicate(context.Background(), predicate, fixture)
		if count > MaxArguments {
			if platform.FailureCode(err) != platform.CodeResourceLimit {
				t.Fatalf("membership +1 accepted: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		result, err := program.Evaluate(context.Background(), fixture)
		if err != nil || result.Truth != DefiniteTrue {
			t.Fatalf("exact membership boundary failed: %s, %v", result.Truth, err)
		}
	}
}
