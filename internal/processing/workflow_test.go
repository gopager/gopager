package processing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

func sourceFixture(t *testing.T, addresses ...string) (SourceProjection, *CapturedSource) {
	t.Helper()
	dependency := predicateDependency("payload")
	flag := Value{BooleanValue, json.RawMessage(`true`)}
	text := Value{StringValue, json.RawMessage(`"LEVEL=7"`)}
	source := SourceProjection{
		TransmissionID: dependency.TransmissionID, NetworkID: dependency.NetworkID,
		Protocol: protocols.POCSAG, Form: "text", ObservedAt: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
		SourceRevision: 1, ContextRevision: 1,
		Fields: []CapturedField{
			{Reference: FieldRef{"source", []string{"flag"}}, Schema: FieldSchema{Kind: BooleanValue},
				Field: FieldValue{State: FieldPresent, Value: &flag, Dependencies: []Dependency{dependency}}},
			{Reference: FieldRef{"source", []string{"text"}}, Schema: FieldSchema{Kind: StringValue},
				Field: FieldValue{State: FieldPresent, Value: &text, Dependencies: []Dependency{dependency}}},
		},
		TargetDeclarations: []FieldDeclaration{{
			Reference: FieldRef{"target", []string{"address"}}, Schema: FieldSchema{Kind: StringValue, Optional: true},
			Groups: []string{"address"},
		}},
	}
	for i, address := range addresses {
		position := uint32(i)
		dependency := predicateDependency("address")
		dependency.TargetPosition = &position
		value, err := Literal(StringValue, address)
		if err != nil {
			t.Fatal(err)
		}
		source.Targets = append(source.Targets, TargetContext{Position: position, Fields: []CapturedField{{
			Reference: FieldRef{"target", []string{"address"}}, Schema: FieldSchema{Kind: StringValue, Optional: true},
			Field: FieldValue{State: FieldPresent, Value: &value, Dependencies: []Dependency{dependency}},
		}}})
	}
	captured, err := CaptureSource(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	return source, captured
}

func selectedDefinition() Definition {
	flag := FieldRef{"source", []string{"flag"}}
	yes := Value{BooleanValue, json.RawMessage(`true`)}
	return Definition{SchemaVersion: 1, Selector: Predicate{Op: "eq", Field: &flag, Value: &yes},
		OutputSchema: map[string]FieldSchema{}, Outputs: map[string]Assignment{}}
}

func TestWorkflowScopedEvaluationSkipsExcludedContextsWithoutRenumbering(t *testing.T) {
	t.Parallel()
	projection, source := sourceFixture(t, "111", "222")
	projection.Targets[0].Fields[0].Field.State = FieldAbsent
	projection.Targets[0].Fields[0].Field.Value = nil
	source, err := CaptureSource(t.Context(), projection)
	if err != nil {
		t.Fatal(err)
	}
	definition := selectedDefinition()
	address := FieldRef{"target", []string{"address"}}
	match, err := Literal(StringValue, "222")
	if err != nil {
		t.Fatal(err)
	}
	definition.Selector = Predicate{Op: "eq", Field: &address, Value: &match}
	definition.OutputSchema["address"] = FieldSchema{Kind: StringValue, Optional: true}
	definition.Outputs["address"] = Assignment{Field: &address}
	program, err := CompileWorkflow(t.Context(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	scope := EvaluationScope{SourceEligible: true, TargetPositions: []uint32{1}}
	result, err := program.EvaluateScoped(t.Context(), source, scope, nil)
	if err != nil || !result.Selected || len(result.TargetResults) != 1 ||
		result.TargetResults[0].TargetPosition != 1 || string(result.TargetResults[0].Fields["address"].Value.Value) != `"222"` {
		t.Fatalf("excluded context was evaluated or original position renumbered: %+v %v", result, err)
	}
	dependencies, err := result.Dependencies()
	if err != nil || len(dependencies) == 0 {
		t.Fatalf("scoped evaluation lost its classification: %+v, %v", dependencies, err)
	}
	for _, dependency := range dependencies {
		if dependency.TargetPosition == nil || *dependency.TargetPosition != 1 {
			t.Fatalf("excluded context supplied field/selection data: %+v", dependency)
		}
	}
	for _, invalidScope := range []EvaluationScope{
		{SourceEligible: true, TargetPositions: []uint32{1, 0}},
		{SourceEligible: true, TargetPositions: []uint32{1, 1}},
		{SourceEligible: true, TargetPositions: []uint32{2}},
		{TargetPositions: []uint32{1}},
	} {
		if _, err := program.EvaluateScoped(t.Context(), source, invalidScope, nil); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("invalid scope position accepted: %+v %v", invalidScope, err)
		}
	}
	result, err = program.EvaluateScoped(t.Context(), source, EvaluationScope{}, nil)
	if err != nil || result.Selected || result.Fields != nil || len(result.TargetResults) != 0 {
		t.Fatalf("ineligible source got a namespace: %+v %v", result, err)
	}
}

func TestWorkflowCheckedDefinitionReturnsDetachedPreparedData(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t)
	definition := selectedDefinition()
	program, err := CompileWorkflow(t.Context(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := program.CheckedDefinition()
	if err != nil {
		t.Fatal(err)
	}
	checked.Selector.Field.Path[0] = "not-a-field"
	checked.OutputSchema["injected"] = FieldSchema{Kind: StringValue}
	again, err := program.CheckedDefinition()
	if err != nil || again.Selector.Field.Path[0] != "flag" || len(again.OutputSchema) != 0 {
		t.Fatalf("checked definition exposed mutable prepared state: %+v %v", again, err)
	}
	result, err := program.Evaluate(t.Context(), source)
	if err != nil || !result.Selected {
		t.Fatalf("detached metadata changed the executable: %+v %v", result, err)
	}
}

func TestWorkflowTargetResultsAndSourceOnlyCardinality(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t, "111", "222")
	definition := selectedDefinition()
	ref := FieldRef{"target", []string{"address"}}
	definition.OutputSchema["address"] = FieldSchema{Kind: StringValue, Optional: true}
	definition.Outputs["address"] = Assignment{Field: &ref}
	program, err := CompileWorkflow(context.Background(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(context.Background(), source)
	if err != nil || !result.Selected || result.Fields != nil || len(result.TargetResults) != 2 {
		t.Fatalf("bad target result cardinality: %+v, %v", result, err)
	}
	for i, target := range result.TargetResults {
		if target.TargetPosition != uint32(i) || string(target.Fields["address"].Value.Value) != []string{`"111"`, `"222"`}[i] {
			t.Fatalf("target correlation lost: %+v", target)
		}
	}
	result.TargetResults[0].Fields["address"].Value.Value[1] = '9'
	again, err := program.Evaluate(context.Background(), source)
	if err != nil || string(again.TargetResults[0].Fields["address"].Value.Value) != `"111"` {
		t.Fatalf("result mutated captured source: %+v, %v", again, err)
	}
	program, err = CompileWorkflow(context.Background(), selectedDefinition(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err = program.Evaluate(context.Background(), source)
	encoded, encodeErr := json.Marshal(result)
	if err != nil || encodeErr != nil || !result.Selected || result.Fields == nil || result.TargetResults != nil ||
		string(encoded) != `{"selected":true,"fields":{}}` {
		t.Fatalf("source-only empty output lost its container: %s, %v, %v", encoded, err, encodeErr)
	}
}

func TestTargetSelectionDependencyUnionAndNoPartialSuccess(t *testing.T) {
	t.Parallel()
	projection, source := sourceFixture(t, "111", "222")
	ref := FieldRef{"target", []string{"address"}}
	literal := Value{StringValue, json.RawMessage(`"111"`)}
	definition := selectedDefinition()
	definition.Selector = Predicate{Op: "eq", Field: &ref, Value: &literal}
	definition.OutputSchema["constant"] = FieldSchema{Kind: StringValue}
	definition.Outputs["constant"] = Assignment{Literal: &literal}
	program, err := CompileWorkflow(context.Background(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(context.Background(), source)
	dependencies, dependencyErr := result.Dependencies()
	if err != nil || dependencyErr != nil || len(result.TargetResults) != 1 || result.TargetResults[0].TargetPosition != 0 ||
		len(result.TargetResults[0].Fields["constant"].Dependencies) != 0 || len(dependencies) != 2 {
		t.Fatalf("selection context dependencies were lost: %+v, %v, %v", result, err, dependencyErr)
	}
	projection.Targets[1].Fields[0].Field.State = FieldAbsent
	projection.Targets[1].Fields[0].Field.Value = nil
	absentSource, err := CaptureSource(context.Background(), projection)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := program.Evaluate(context.Background(), absentSource); err != nil ||
		!result.Selected || len(result.TargetResults) != 1 || result.TargetResults[0].TargetPosition != 0 {
		t.Fatalf("absent second context changed the selection: %+v, %v", result, err)
	}
	definition = selectedDefinition()
	definition.OutputSchema["number"] = FieldSchema{Kind: IntegerValue}
	definition.Outputs["number"] = Assignment{Field: &ref, Conversion: IntegerValue}
	projection.Targets[1].Fields[0].Field = FieldValue{State: FieldPresent,
		Value:        &Value{StringValue, json.RawMessage(`"secret-not-numeric"`)},
		Dependencies: projection.Targets[0].Fields[0].Field.Dependencies}
	position := uint32(1)
	projection.Targets[1].Fields[0].Field.Dependencies = []Dependency{predicateDependency("address")}
	projection.Targets[1].Fields[0].Field.Dependencies[0].TargetPosition = &position
	source, err = CaptureSource(context.Background(), projection)
	if err != nil {
		t.Fatal(err)
	}
	program, err = CompileWorkflow(context.Background(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err = program.Evaluate(context.Background(), source)
	if err == nil || result.TargetResults != nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("failed second context leaked partial/private result: %+v, %v", result, err)
	}
}

func TestWorkflowTargetlessAndUnusedBranchStaticDependence(t *testing.T) {
	t.Parallel()
	_, source := sourceFixture(t)
	ref := FieldRef{"target", []string{"address"}}
	definition := selectedDefinition()
	definition.OutputSchema["address"] = FieldSchema{Kind: StringValue, Optional: true}
	definition.Outputs["address"] = Assignment{Field: &ref}
	program, err := CompileWorkflow(context.Background(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.Evaluate(context.Background(), source)
	if err != nil || !result.Selected || result.TargetResults != nil || result.Fields["address"].State != FieldAbsent {
		t.Fatalf("targetless evaluation fabricated a target: %+v, %v", result, err)
	}
	_, source = sourceFixture(t, "111", "222")
	flag := FieldRef{"source", []string{"flag"}}
	yes := Value{BooleanValue, json.RawMessage(`true`)}
	definition = selectedDefinition()
	definition.Steps = []Step{{ID: "choose", Kind: "branch", OutputSchema: map[string]FieldSchema{},
		Branch: &BranchStep{Condition: Predicate{Op: "eq", Field: &flag, Value: &yes},
			ThenOutputs: map[string]Assignment{}, ElseOutputs: map[string]Assignment{},
			Else: []Step{{ID: "unused", Kind: "map", OutputSchema: map[string]FieldSchema{"address": {Kind: StringValue}},
				Map: map[string]Assignment{"address": {Field: &ref}}}}}}}
	program, err = CompileWorkflow(context.Background(), definition, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err = program.Evaluate(context.Background(), source)
	if err != nil || len(result.TargetResults) != 2 {
		t.Fatalf("unused branch was not statically target-dependent: %+v, %v", result, err)
	}
}

func TestWorkflowExtractionBranchMappingAndParallelReuse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, source := sourceFixture(t)
	definition := selectedDefinition()
	text := FieldRef{"source", []string{"text"}}
	extracted := FieldRef{"step", []string{"extract", "level"}}
	chosen := FieldRef{"step", []string{"choose", "level"}}
	seven := Value{IntegerValue, json.RawMessage(`"7"`)}
	definition.Steps = []Step{
		{ID: "extract", Kind: "extract", OutputSchema: map[string]FieldSchema{"level": {Kind: IntegerValue}},
			Extract: &ExtractStep{Input: text, Pattern: `LEVEL=(?P<level>[0-9]+)`,
				Captures: map[string]CaptureMapping{"level": {Name: "level", Conversion: IntegerValue}}}},
		{ID: "choose", Kind: "branch", OutputSchema: map[string]FieldSchema{"level": {Kind: IntegerValue}},
			Branch: &BranchStep{Condition: Predicate{Op: "eq", Field: &extracted, Value: &seven},
				ThenOutputs: map[string]Assignment{"level": {Field: &extracted}},
				ElseOutputs: map[string]Assignment{"level": {Literal: &seven}}}},
	}
	definition.OutputSchema["level"] = FieldSchema{Kind: IntegerValue}
	definition.Outputs["level"] = Assignment{Field: &chosen}
	program, err := CompileWorkflow(ctx, definition, source, CoreRegex{})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			result, err := program.Evaluate(ctx, source)
			if err != nil || result.Fields["level"].Value == nil || string(result.Fields["level"].Value.Value) != `"7"` ||
				len(result.Fields["level"].Dependencies) != 0 {
				t.Errorf("extract/branch result: %+v, %v", result, err)
			}
		})
	}
	workers.Wait()
	for _, invalidDefinition := range []Definition{
		{SchemaVersion: 1, Selector: definition.Selector, Steps: []Step{definition.Steps[0], definition.Steps[0]}},
		{SchemaVersion: 1, Selector: definition.Selector, Outputs: map[string]Assignment{"bad": {Field: &chosen}},
			OutputSchema: map[string]FieldSchema{"bad": {Kind: IntegerValue}}},
	} {
		if _, err := CompileWorkflow(ctx, invalidDefinition, source, CoreRegex{}); err == nil {
			t.Fatal("duplicate/forward step reference accepted")
		}
	}
}

func TestSourceCaptureRejectsForeignAndMutableContext(t *testing.T) {
	t.Parallel()
	projection, captured := sourceFixture(t, "111")
	projection.Fields[0].Field.Value.Value = json.RawMessage(`false`)
	program, err := CompileWorkflow(context.Background(), selectedDefinition(), captured, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := program.Evaluate(context.Background(), captured); err != nil || !result.Selected {
		t.Fatalf("owner mutation affected detached capture: %+v, %v", result, err)
	}
	projection.Targets[0].Position = 1
	if _, err := CaptureSource(context.Background(), projection); err == nil {
		t.Fatal("rewritten original positions accepted")
	}
	projection, _ = sourceFixture(t, "111")
	projection.Targets[0].Fields[0].Field.Dependencies[0].ContextRevision++
	if _, err := CaptureSource(context.Background(), projection); err == nil {
		t.Fatal("stale/foreign context dependency accepted")
	}
	if _, err := (MissingSourceOwner{}).Capture(context.Background(), nil, projection.TransmissionID); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("missing source owner returned success: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := program.Evaluate(ctx, captured); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Workflow proceeded: %v", err)
	}
}
