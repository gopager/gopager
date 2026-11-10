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

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// Assignment explicitly chooses one literal or a prior/source reference and optional conversion.
type Assignment struct {
	Field      *FieldRef `json:"field,omitempty"`
	Literal    *Value    `json:"literal,omitempty"`
	Conversion ValueKind `json:"conversion,omitempty"`
}

// CaptureMapping maps one extraction output to a named regex capture group. Name must
// occur exactly once in the pattern. Without Conversion the output must be a string;
// with it, Conversion must equal the output kind (string, integer, number, boolean or
// timestamp).
type CaptureMapping struct {
	Name       string    `json:"name"`
	Conversion ValueKind `json:"conversion,omitempty"`
}

// ExtractStep matches a declared string input against a Core regex pattern and maps
// named captures, keyed by output field name, to outputs. Every required output must be
// mapped; outputs whose group did not match are absent.
type ExtractStep struct {
	Input    FieldRef                  `json:"input"`
	Pattern  string                    `json:"pattern"`
	Captures map[string]CaptureMapping `json:"captures"`
}

// BranchStep runs Then when Condition is true and Else when it is false, then evaluates the
// matching outputs mapping. The condition is ordinary two-valued logic.
type BranchStep struct {
	Condition   Predicate             `json:"condition"`
	Then        []Step                `json:"then"`
	Else        []Step                `json:"else"`
	ThenOutputs map[string]Assignment `json:"thenOutputs"`
	ElseOutputs map[string]Assignment `json:"elseOutputs"`
}

// Step declares one bounded local operation; unrelated configuration is rejected.
type Step struct {
	ID           string                 `json:"id"`
	Kind         string                 `json:"kind"`
	OutputSchema map[string]FieldSchema `json:"outputSchema"`
	Map          map[string]Assignment  `json:"map,omitempty"`
	Extract      *ExtractStep           `json:"extract,omitempty"`
	Branch       *BranchStep            `json:"branch,omitempty"`
	Installed    *InstalledStep         `json:"installed,omitempty"`
}

// MarshalJSON retains an explicitly empty map configuration without adding other kinds' fields.
func (s Step) MarshalJSON() ([]byte, error) {
	type alias Step
	wire := struct {
		alias
		Map *map[string]Assignment `json:"map,omitempty"`
	}{alias: alias(s)}
	if s.Map != nil {
		wire.Map = &s.Map
	}
	return json.Marshal(wire)
}

// Definition is an immutable prepared Workflow definition, separate from activation.
type Definition struct {
	SchemaVersion int                    `json:"schemaVersion"`
	Selector      Predicate              `json:"selector"`
	Steps         []Step                 `json:"steps"`
	OutputSchema  map[string]FieldSchema `json:"outputSchema"`
	Outputs       map[string]Assignment  `json:"outputs"`
}

type compiledStep struct {
	definition Step
	condition  *PredicateProgram
	then       []compiledStep
	otherwise  []compiledStep
	installed  *InstalledArtifact
}

// WorkflowProgram holds detached checked definitions and prepared CEL programs.
type WorkflowProgram struct {
	definition           Definition
	selector             *PredicateProgram
	steps                []compiledStep
	targeted             bool
	selectorReferences   *referenceSet
	bodyReferences       *referenceSet
	regex                RegexEvaluator
	preparationArtifacts any
}

// CheckedDefinition returns a detached copy of the immutable, successfully compiled definition.
func (p *WorkflowProgram) CheckedDefinition() (Definition, error) {
	if p == nil {
		return Definition{}, invalid(errors.New("missing prepared Workflow"))
	}
	encoded, err := json.Marshal(p.definition)
	if err != nil {
		return Definition{}, invalid(err)
	}
	var result Definition
	if err := Decode(encoded, &result); err != nil {
		return Definition{}, err
	}
	return result, nil
}

type workflowDeclarations struct {
	source Declarations
	local  map[string]FieldSchema
}

func (d workflowDeclarations) FieldSchema(ref FieldRef) (FieldSchema, error) {
	if ref.Namespace == "step" {
		if schema, ok := d.local[ref.key()]; ok {
			return schema, nil
		}
		return FieldSchema{}, invalid(errors.New("step field is not in lexical scope"))
	}
	return d.source.FieldSchema(ref)
}

func validLocalName(name string) bool {
	return name != "" && utf8.ValidString(name) && !strings.ContainsRune(name, '\x00')
}

func schemaCompatible(from, to FieldSchema) bool {
	from.Optional, to.Optional = false, false
	left, leftErr := json.Marshal(from)
	right, rightErr := json.Marshal(to)
	return leftErr == nil && rightErr == nil && string(left) == string(right)
}

func validateAssignment(assignment Assignment, schema FieldSchema, declarations Declarations) error {
	if (assignment.Field == nil) == (assignment.Literal == nil) {
		return invalid(errors.New("assignment requires exactly one input"))
	}
	var source FieldSchema
	if assignment.Field != nil {
		if err := assignment.Field.validate(); err != nil {
			return err
		}
		var err error
		source, err = declarations.FieldSchema(*assignment.Field)
		if err != nil {
			return err
		}
	} else {
		source = schema
		if assignment.Conversion != "" && assignment.Literal.Kind == StringValue {
			source = FieldSchema{Kind: StringValue}
		}
		if _, err := assignment.Literal.Native(source); err != nil {
			return err
		}
	}
	if assignment.Conversion == "" {
		if !schemaCompatible(source, schema) {
			return invalid(errors.New("assignment types are incompatible"))
		}
		return nil
	}
	if assignment.Conversion != schema.Kind ||
		(!schemaCompatible(source, schema) && (source.Kind != StringValue ||
			!slices.Contains([]ValueKind{IntegerValue, NumberValue, BooleanValue, TimestampValue}, schema.Kind))) {
		return invalid(errors.New("undeclared or incompatible conversion"))
	}
	return nil
}

func validateMappings(schema map[string]FieldSchema, mappings map[string]Assignment, declarations Declarations) error {
	for name, assignment := range mappings {
		field, ok := schema[name]
		if !ok {
			return invalid(errors.New("mapping writes an undeclared output"))
		}
		if err := validateAssignment(assignment, field, declarations); err != nil {
			return err
		}
	}
	for name, field := range schema {
		if _, exists := mappings[name]; !exists && !field.Optional {
			return invalid(errors.New("required output has no mapping"))
		}
	}
	return nil
}

func sortedNames[V any](values map[string]V) []string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func cloneDeclarations(local map[string]FieldSchema) map[string]FieldSchema {
	result := make(map[string]FieldSchema, len(local))
	for key, schema := range local {
		result[key] = schema
	}
	return result
}

// CompileWorkflow prepares a closed definition outside source transactions.
// The supplied owner declarations are authoritative; this does not accept a user schema as authority.
func CompileWorkflow(ctx context.Context, definition Definition, source Declarations, regex RegexEvaluator) (*WorkflowProgram, error) {
	return compileWorkflow(ctx, definition, source, regex, nil)
}

// CompileWorkflowInstalled additionally prepares declared artifacts through their trusted instance owner.
func CompileWorkflowInstalled(ctx context.Context, definition Definition, source Declarations, regex RegexEvaluator, artifacts InstalledArtifacts) (*WorkflowProgram, error) {
	if artifacts == nil {
		return nil, capabilityUnavailable(errors.New("installed artifact owner unavailable"))
	}
	return compileWorkflow(ctx, definition, source, regex, artifacts)
}

func compileWorkflow(ctx context.Context, definition Definition, source Declarations, regex RegexEvaluator, artifacts InstalledArtifacts) (*WorkflowProgram, error) {
	if ctx == nil || source == nil {
		return nil, invalid(errors.New("missing Workflow preparation participant"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		return nil, invalid(err)
	}
	var detached Definition
	if err := Decode(encoded, &detached); err != nil {
		return nil, err
	}
	if detached.SchemaVersion != 1 {
		return nil, invalid(errors.New("unsupported Workflow schema"))
	}
	program := &WorkflowProgram{definition: detached, regex: regex,
		selectorReferences: newReferenceSet(source), bodyReferences: newReferenceSet(source)}
	declarations := workflowDeclarations{source: source, local: make(map[string]FieldSchema)}
	selectorTracking := declarationTracker{Declarations: declarations, targeted: &program.targeted,
		references: program.selectorReferences}
	tracking := declarationTracker{Declarations: declarations, targeted: &program.targeted,
		references: program.bodyReferences}
	program.selector, err = CompilePredicate(ctx, detached.Selector, selectorTracking)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]bool)
	count := 0
	var prepare func([]Step, map[string]FieldSchema, int) ([]compiledStep, error)
	prepare = func(steps []Step, local map[string]FieldSchema, depth int) ([]compiledStep, error) {
		if depth > MaxDepth {
			return nil, resource(errors.New("workflow branch depth boundary"))
		}
		var compiled []compiledStep
		for _, step := range steps {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			count++
			if count > MaxSteps {
				return nil, resource(errors.New("workflow total step boundary"))
			}
			if !validLocalName(step.ID) || ids[step.ID] {
				return nil, invalid(errors.New("duplicate/invalid local step ID"))
			}
			ids[step.ID] = true
			for name, schema := range step.OutputSchema {
				if !validLocalName(name) {
					return nil, invalid(errors.New("invalid output name"))
				}
				if err := schema.Validate(); err != nil {
					return nil, err
				}
			}
			if len(step.OutputSchema) > MaxCollection {
				return nil, resource(errors.New("step output schema boundary"))
			}
			current := declarationTracker{Declarations: workflowDeclarations{source: source, local: local},
				targeted: &program.targeted, references: program.bodyReferences}
			item := compiledStep{definition: step}
			switch step.Kind {
			case "map":
				if step.Extract != nil || step.Branch != nil || step.Installed != nil || step.Map == nil {
					return nil, invalid(errors.New("invalid map step configuration"))
				}
				if err := validateMappings(step.OutputSchema, step.Map, current); err != nil {
					return nil, err
				}
			case "extract":
				if step.Extract == nil || step.Branch != nil || step.Installed != nil || step.Map != nil || regex == nil {
					return nil, invalid(errors.New("missing/invalid extraction participant"))
				}
				if err := step.Extract.Input.validate(); err != nil {
					return nil, err
				}
				schema, err := current.FieldSchema(step.Extract.Input)
				if err != nil {
					return nil, err
				}
				if schema.Kind != StringValue {
					return nil, invalid(errors.New("extraction requires declared string input"))
				}
				inspection, err := regex.Regex(ctx, step.Extract.Pattern, nil)
				if err != nil {
					return nil, err
				}
				for name, mapping := range step.Extract.Captures {
					output, exists := step.OutputSchema[name]
					if !exists || mapping.Name == "" || !slices.Contains(inspection.Names, mapping.Name) {
						return nil, invalid(errors.New("undeclared extraction output/capture"))
					}
					count := 0
					for _, name := range inspection.Names {
						if name == mapping.Name {
							count++
						}
					}
					if count != 1 {
						return nil, invalid(errors.New("ambiguous named capture"))
					}
					if mapping.Conversion == "" && output.Kind != StringValue ||
						mapping.Conversion != "" && (mapping.Conversion != output.Kind ||
							!slices.Contains([]ValueKind{StringValue, IntegerValue, NumberValue, BooleanValue, TimestampValue}, output.Kind)) {
						return nil, invalid(errors.New("invalid capture conversion"))
					}
				}
				for name, schema := range step.OutputSchema {
					if _, exists := step.Extract.Captures[name]; !exists && !schema.Optional {
						return nil, invalid(errors.New("required capture output unmapped"))
					}
				}
			case "branch":
				if step.Branch == nil || step.Extract != nil || step.Map != nil || step.Installed != nil {
					return nil, invalid(errors.New("invalid branch configuration"))
				}
				item.condition, err = CompilePredicate(ctx, step.Branch.Condition, current)
				if err != nil {
					return nil, err
				}
				thenLocal, elseLocal := cloneDeclarations(local), cloneDeclarations(local)
				item.then, err = prepare(step.Branch.Then, thenLocal, depth+1)
				if err != nil {
					return nil, err
				}
				item.otherwise, err = prepare(step.Branch.Else, elseLocal, depth+1)
				if err != nil {
					return nil, err
				}
				if err := validateMappings(step.OutputSchema, step.Branch.ThenOutputs,
					declarationTracker{Declarations: workflowDeclarations{source, thenLocal}, targeted: &program.targeted,
						references: program.bodyReferences}); err != nil {
					return nil, err
				}
				if err := validateMappings(step.OutputSchema, step.Branch.ElseOutputs,
					declarationTracker{Declarations: workflowDeclarations{source, elseLocal}, targeted: &program.targeted,
						references: program.bodyReferences}); err != nil {
					return nil, err
				}
			case "installed":
				if step.Installed == nil || step.Map != nil || step.Extract != nil || step.Branch != nil ||
					step.Installed.InstanceID == uuid.Nil || step.Installed.ConfigurationRevision <= 0 {
					return nil, invalid(errors.New("invalid installed step configuration"))
				}
				if artifacts == nil {
					return nil, capabilityUnavailable(errors.New("installed artifact owner unavailable"))
				}
				item.installed, err = artifacts.PrepareInstalled(ctx, step.Installed.InstanceID, step.Installed.ConfigurationRevision)
				if err != nil {
					return nil, err
				}
				if item.installed == nil {
					return nil, capabilityUnavailable(errors.New("installed artifact owner returned no artifact"))
				}
				manifest, manifestErr := item.installed.Manifest()
				if manifestErr != nil {
					return nil, manifestErr
				}
				if err := validateMappings(manifest.InputSchema, step.Installed.Input, current); err != nil {
					return nil, err
				}
				if !sameOutputSchema(step.OutputSchema, manifest.ResultSchema) {
					return nil, invalid(errors.New("installed step result declaration differs from capability"))
				}
			default:
				return nil, invalid(errors.New("unknown Workflow step kind"))
			}
			for name, schema := range step.OutputSchema {
				local[(FieldRef{Namespace: "step", Path: []string{step.ID, name}}).key()] = schema
			}
			compiled = append(compiled, item)
		}
		return compiled, nil
	}
	program.steps, err = prepare(detached.Steps, declarations.local, 1)
	if err != nil {
		return nil, err
	}
	for name, schema := range detached.OutputSchema {
		if !validLocalName(name) {
			return nil, invalid(errors.New("invalid final output name"))
		}
		if err := schema.Validate(); err != nil {
			return nil, err
		}
	}
	if len(detached.OutputSchema) > MaxCollection {
		return nil, resource(errors.New("final schema boundary"))
	}
	if err := validateMappings(detached.OutputSchema, detached.Outputs, tracking); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return program, nil
}

// declarationTracker records, at compile time, whether a definition is target-scoped and which
// source and target fields it reads, so that its classification never depends on evaluation.
type declarationTracker struct {
	Declarations
	targeted   *bool
	references *referenceSet
}

func (d declarationTracker) FieldSchema(ref FieldRef) (FieldSchema, error) {
	if ref.Namespace == "target" {
		*d.targeted = true
	}
	schema, err := d.Declarations.FieldSchema(ref)
	if err != nil {
		return FieldSchema{}, err
	}
	return schema, d.references.record(ref)
}

// TargetResult is one derived output set tied to an original target position.
type TargetResult struct {
	TargetPosition uint32                `json:"targetPosition"`
	Fields         map[string]FieldValue `json:"fields"`
}

// Evaluation exposes no partial successful output on any selected-context failure.
// Final fields are derived values: they carry no per-value classification. They implicitly
// require global derived visibility plus the groups of Dependencies, which classify the
// whole definition's outputs once.
type Evaluation struct {
	Selected      bool                  `json:"selected"`
	Fields        map[string]FieldValue `json:"fields,omitempty"`
	TargetResults []TargetResult        `json:"targetResults,omitempty"`
	dependencies  []Dependency
}

// Dependencies returns the detached classification of the evaluation: the owner dependency of
// every source field the definition references, including fields on a branch arm that did not
// run and fields read by a false selector. The set is a property of the definition, not of the
// values or of any reader. It is host-only and is never added to the public result envelope.
func (e Evaluation) Dependencies() ([]Dependency, error) {
	dependencies := append([]Dependency(nil), e.dependencies...)
	for _, field := range e.Fields {
		dependencies = append(dependencies, field.Dependencies...)
	}
	for _, target := range e.TargetResults {
		for _, field := range target.Fields {
			dependencies = append(dependencies, field.Dependencies...)
		}
	}
	return UnionDependencies(dependencies)
}

// FieldGroups returns the definition-level classification of the result's outputs: the
// `derived` group plus the groups of every source field in Dependencies, sorted and without
// duplicates. It is stored with the result so that readers never compile the definition.
func (e Evaluation) FieldGroups() ([]string, error) {
	dependencies, err := e.Dependencies()
	if err != nil {
		return nil, err
	}
	groups := []string{"derived"}
	for _, dependency := range dependencies {
		groups = append(groups, dependency.Groups...)
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	for _, group := range groups {
		if !authz.ValidField(group) {
			return nil, invalid(errors.New("invalid field group"))
		}
	}
	return groups, nil
}

// MarshalJSON preserves an explicitly empty successful field map as a result container.
func (e Evaluation) MarshalJSON() ([]byte, error) {
	type wire struct {
		Selected      bool                   `json:"selected"`
		Fields        *map[string]FieldValue `json:"fields,omitempty"`
		TargetResults []TargetResult         `json:"targetResults,omitempty"`
	}
	result := wire{Selected: e.Selected, TargetResults: e.TargetResults}
	if e.Fields != nil {
		result.Fields = &e.Fields
	}
	return json.Marshal(result)
}

type executionBudget struct {
	used      int
	bytesUsed int
	parent    context.Context
	active    context.Context
	cancel    context.CancelFunc
	remaining time.Duration
	started   time.Time
}

func (b *executionBudget) consume(value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return invalid(err)
	}
	b.used += len(encoded)
	if b.used > MaxInputBytes {
		return resource(errors.New("workflow aggregate input/intermediate boundary"))
	}
	decodedBytes, err := decodedBytesInJSON(encoded)
	if err != nil {
		return err
	}
	b.bytesUsed += decodedBytes
	if b.bytesUsed > MaxInputBytes {
		return resource(errors.New("workflow aggregate decoded bytes boundary"))
	}
	return nil
}

// executionFrame is the lexical state of one evaluation context. dependencies is the
// context's definition-level classification (the owner dependencies of every source field the
// definition references plus installed-configuration dependencies); it is never attached to a
// derived value.
type executionFrame struct {
	source       FieldReader
	local        map[string]FieldValue
	dependencies []Dependency
	budget       *executionBudget
	invocations  InstalledInvocationOwner
	invocation   InvocationContext
}

func (f *executionFrame) LookupField(ctx context.Context, ref FieldRef) (FieldValue, error) {
	if ref.Namespace != "step" {
		return f.source.LookupField(ctx, ref)
	}
	value, exists := f.local[ref.key()]
	if !exists {
		return FieldValue{}, invalid(errors.New("step value is outside lexical scope"))
	}
	return cloneField(value)
}

func convertValue(value Value, target FieldSchema, conversion ValueKind) (Value, error) {
	if conversion == "" || value.Kind == target.Kind {
		_, err := value.Native(target)
		value.Value = slices.Clone(value.Value)
		return value, err
	}
	if value.Kind != StringValue {
		return Value{}, invalid(errors.New("conversion-failed"))
	}
	native, err := value.Native(FieldSchema{Kind: StringValue})
	if err != nil {
		return Value{}, err
	}
	text := native.(string)
	converted := Value{Kind: target.Kind}
	switch target.Kind {
	case IntegerValue, TimestampValue:
		converted.Value, err = json.Marshal(text)
	case BooleanValue:
		if text != "true" && text != "false" {
			return Value{}, invalid(errors.New("conversion-failed"))
		}
		converted.Value = json.RawMessage(text)
	case NumberValue:
		if strings.TrimSpace(text) != text {
			return Value{}, invalid(errors.New("conversion-failed"))
		}
		converted.Value = json.RawMessage(text)
	default:
		return Value{}, invalid(errors.New("conversion-failed"))
	}
	if err != nil {
		return Value{}, invalid(err)
	}
	if _, err := converted.Native(target); err != nil {
		return Value{}, invalid(errors.New("conversion-failed"))
	}
	return converted, nil
}

// evaluateMappings produces derived values: whatever the source of an assignment, the result
// carries no per-value classification, because the definition is classified as a whole.
func evaluateMappings(ctx context.Context, schema map[string]FieldSchema, mappings map[string]Assignment, frame *executionFrame) (map[string]FieldValue, error) {
	result := make(map[string]FieldValue, len(schema))
	for _, name := range sortedNames(schema) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		assignment, exists := mappings[name]
		field := FieldValue{State: FieldAbsent}
		if exists {
			if assignment.Field != nil {
				var err error
				field, err = frame.LookupField(ctx, *assignment.Field)
				if err != nil {
					return nil, err
				}
			} else {
				field.State, field.Value = FieldPresent, assignment.Literal
			}
			if field.State == FieldPresent {
				value, err := convertValue(*field.Value, schema[name], assignment.Conversion)
				if err != nil {
					return nil, err
				}
				field.Value = &value
			}
		}
		field.Dependencies = nil
		if err := validateFieldValue(schema[name], field, false); err != nil {
			return nil, err
		}
		result[name] = field
	}
	return result, nil
}

func (p *WorkflowProgram) execute(ctx context.Context, steps []compiledStep, frame *executionFrame) error {
	for _, step := range steps {
		ctx = frame.budget.computeContext()
		if err := ctx.Err(); err != nil {
			return err
		}
		var fields map[string]FieldValue
		var err error
		switch step.definition.Kind {
		case "map":
			fields, err = evaluateMappings(ctx, step.definition.OutputSchema, step.definition.Map, frame)
		case "extract":
			fields, err = p.extract(ctx, step.definition, frame)
		case "branch":
			var condition bool
			condition, err = step.condition.Evaluate(ctx, frame)
			if err != nil {
				break
			}
			local := make(map[string]FieldValue, len(frame.local))
			for key, value := range frame.local {
				local[key] = value
			}
			nested := &executionFrame{source: frame.source, local: local, dependencies: frame.dependencies, budget: frame.budget,
				invocations: frame.invocations, invocation: frame.invocation}
			body, outputs := step.otherwise, step.definition.Branch.ElseOutputs
			if condition {
				body, outputs = step.then, step.definition.Branch.ThenOutputs
			}
			err = p.execute(ctx, body, nested)
			if err == nil {
				fields, err = evaluateMappings(frame.budget.computeContext(), step.definition.OutputSchema, outputs, nested)
			}
			frame.dependencies = nested.dependencies
		case "installed":
			fields, err = executeInstalled(ctx, step, frame)
		}
		if err != nil {
			err = frame.budget.bound(err)
			return fmt.Errorf("step %q: %w", step.definition.ID, platform.NewFailure(platform.FailureCode(err), err))
		}
		if err := frame.budget.computeContext().Err(); err != nil {
			return err
		}
		for name, field := range fields {
			frame.local[(FieldRef{Namespace: "step", Path: []string{step.definition.ID, name}}).key()] = field
		}
		if err := frame.budget.consume(fields); err != nil {
			return err
		}
	}
	return nil
}

func (p *WorkflowProgram) extract(ctx context.Context, step Step, frame *executionFrame) (map[string]FieldValue, error) {
	input, err := frame.LookupField(ctx, step.Extract.Input)
	if err != nil {
		return nil, err
	}
	var matched RegexResult
	if input.State == FieldPresent {
		native, err := input.Value.Native(FieldSchema{Kind: StringValue})
		if err != nil {
			return nil, err
		}
		text := native.(string)
		matched, err = p.regex.Regex(ctx, step.Extract.Pattern, &text)
		if err != nil {
			return nil, err
		}
	}
	fields := make(map[string]FieldValue, len(step.OutputSchema))
	for _, name := range sortedNames(step.OutputSchema) {
		field := FieldValue{State: FieldAbsent}
		mapping, exists := step.Extract.Captures[name]
		if exists && matched.Matched {
			for _, capture := range matched.Captures {
				if capture.Name == mapping.Name && capture.Text != nil {
					value, err := Literal(StringValue, *capture.Text)
					if err != nil {
						return nil, err
					}
					value, err = convertValue(value, step.OutputSchema[name], mapping.Conversion)
					if err != nil {
						return nil, err
					}
					field.State, field.Value = FieldPresent, &value
				}
			}
		}
		if err := validateFieldValue(step.OutputSchema[name], field, false); err != nil {
			return nil, err
		}
		fields[name] = field
	}
	return fields, nil
}

// Evaluate computes all selected contexts under one total built-in deadline.
// It writes no configuration, live result, fact, publication state or external effect.
func (p *WorkflowProgram) Evaluate(ctx context.Context, source *CapturedSource) (Evaluation, error) {
	return p.evaluate(ctx, source, nil)
}

// EvaluateInstalled requires fresh owner authorization for each actual installed invocation.
// The owner binds the current run, instance revision, Principal and source, not HTTP-supplied authority.
func (p *WorkflowProgram) EvaluateInstalled(ctx context.Context, source *CapturedSource, invocations InstalledInvocationOwner) (Evaluation, error) {
	if invocations == nil {
		return Evaluation{}, capabilityUnavailable(errors.New("installed invocation owner unavailable"))
	}
	return p.evaluate(ctx, source, invocations)
}

func (p *WorkflowProgram) evaluate(ctx context.Context, source *CapturedSource, invocations InstalledInvocationOwner) (Evaluation, error) {
	return p.evaluateScoped(ctx, source, invocations, nil)
}

// EvaluationScope is trusted owner eligibility, separate from field visibility and CEL selection.
// TargetPositions retain original order/indexes; excluded contexts are not evaluated and never read as a false selector.
type EvaluationScope struct {
	SourceEligible  bool
	TargetPositions []uint32
}

// EvaluateScoped evaluates only owner-authorized Workflow scope contexts under the existing shared budget.
func (p *WorkflowProgram) EvaluateScoped(ctx context.Context, source *CapturedSource, scope EvaluationScope, invocations InstalledInvocationOwner) (Evaluation, error) {
	scope.TargetPositions = slices.Clone(scope.TargetPositions)
	return p.evaluateScoped(ctx, source, invocations, &scope)
}

func (p *WorkflowProgram) evaluateScoped(ctx context.Context, source *CapturedSource, invocations InstalledInvocationOwner,
	scope *EvaluationScope) (evaluation Evaluation, resultErr error) {
	if p == nil || source == nil || ctx == nil {
		return Evaluation{}, invalid(errors.New("missing Workflow/source/context"))
	}
	identity, err := source.Identity()
	if err != nil {
		return Evaluation{}, err
	}
	if scope != nil {
		for i, position := range scope.TargetPositions {
			if position >= identity.OriginalTargetCount || (i > 0 && position <= scope.TargetPositions[i-1]) {
				return Evaluation{}, invalid(errors.New("scope target positions are not ordered original contexts"))
			}
		}
		if !scope.SourceEligible && len(scope.TargetPositions) != 0 {
			return Evaluation{}, invalid(errors.New("ineligible source has eligible target contexts"))
		}
	}
	budget := &executionBudget{used: source.encodedSize}
	budget.startCompute(ctx, computeAllowance)
	defer budget.closeCompute()
	defer func() {
		// Expiry of the run's own compute allowance is an explicit bound, not an internal error.
		if resultErr != nil {
			evaluation, resultErr = Evaluation{}, budget.bound(resultErr)
		}
	}()
	ctx = budget.computeContext()
	if err := ctx.Err(); err != nil {
		return Evaluation{}, err
	}
	if scope != nil {
		if err := budget.consume(scope); err != nil {
			return Evaluation{}, err
		}
		if !scope.SourceEligible {
			return Evaluation{}, nil
		}
	}
	contexts := []*uint32{nil}
	if p.targeted && len(source.targets) > 0 {
		var positions []uint32
		if scope == nil {
			positions = make([]uint32, len(source.targets))
			for i := range positions {
				positions[i] = uint32(i)
			}
		} else {
			positions = scope.TargetPositions
		}
		contexts = make([]*uint32, len(positions))
		for i, position := range positions {
			contexts[i] = &position
		}
	}
	var selected []*uint32
	var selectionDependencies []Dependency
	for _, target := range contexts {
		isSelected, err := p.selector.Evaluate(ctx, capturedView{source, target})
		if err != nil {
			return Evaluation{}, err
		}
		dependencies, err := source.referenceDependencies(ctx, p.selectorReferences.fields(), target)
		if err != nil {
			return Evaluation{}, err
		}
		selectionDependencies, err = UnionDependencies(selectionDependencies, dependencies)
		if err != nil {
			return Evaluation{}, err
		}
		if isSelected {
			selected = append(selected, target)
		}
	}
	result := Evaluation{Selected: len(selected) != 0, dependencies: selectionDependencies}
	if err := budget.consume(selectionDependencies); err != nil {
		return Evaluation{}, err
	}
	for _, target := range selected {
		contextDependencies, err := source.referenceDependencies(budget.computeContext(),
			append(p.selectorReferences.fields(), p.bodyReferences.fields()...), target)
		if err != nil {
			return Evaluation{}, err
		}
		frame := &executionFrame{source: capturedView{source, target}, local: make(map[string]FieldValue),
			dependencies: contextDependencies, budget: budget, invocations: invocations,
			invocation: InvocationContext{Source: identity, TargetPosition: target}}
		if err := p.execute(ctx, p.steps, frame); err != nil {
			return Evaluation{}, err
		}
		fields, err := evaluateMappings(budget.computeContext(), p.definition.OutputSchema, p.definition.Outputs, frame)
		if err != nil {
			return Evaluation{}, err
		}
		if err := budget.consume(fields); err != nil {
			return Evaluation{}, err
		}
		result.dependencies, err = UnionDependencies(result.dependencies, frame.dependencies)
		if err != nil {
			return Evaluation{}, err
		}
		if target == nil {
			result.Fields = fields
		} else {
			result.TargetResults = append(result.TargetResults, TargetResult{TargetPosition: *target, Fields: fields})
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return Evaluation{}, invalid(err)
		}
		if len(encoded) > MaxOutputBytes {
			return Evaluation{}, resource(errors.New("workflow complete result boundary"))
		}
	}
	if err := budget.computeContext().Err(); err != nil {
		return Evaluation{}, err
	}
	return result, nil
}
