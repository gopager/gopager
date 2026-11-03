package processing

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

//go:generate env GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -trimpath -buildvcs=false -o testdata/preview.wasm ./testdata/preview

//go:embed testdata/preview.wasm
var installedPreviewCommand []byte

type installedOwnerFixture struct {
	artifact    *InstalledArtifact
	instance    uuid.UUID
	envelope    InvocationEnvelope
	calls       int
	failAt      int
	alter       bool
	contexts    []InvocationContext
	inputs      [][]byte
	preview     *InstalledArtifact
	previewMode bool
}

func (o *installedOwnerFixture) PrepareInstalled(_ context.Context, id uuid.UUID, revision int64) (*InstalledArtifact, error) {
	if id != o.instance || revision != o.envelope.ConfigurationRevision {
		return nil, capabilityUnavailable(errors.New("fixture instance revision unavailable"))
	}
	return o.artifact, nil
}

func (o *installedOwnerFixture) PrepareInstalledPreview(ctx context.Context, id uuid.UUID, revision int64) (*InstalledArtifact, bool, error) {
	if _, err := o.PrepareInstalled(ctx, id, revision); err != nil {
		return nil, false, err
	}
	return o.preview, o.preview != nil, nil
}

func (o *installedOwnerFixture) AuthorizeInstalled(ctx context.Context, id uuid.UUID, revision int64, call InvocationContext, input map[string]FieldValue) (InvocationEnvelope, []Dependency, bool, error) {
	if err := ctx.Err(); err != nil {
		return InvocationEnvelope{}, nil, true, err
	}
	o.calls++
	if call.Source.TransmissionID == uuid.Nil || call.Source.SourceRevision <= 0 || call.Source.ContextRevision <= 0 ||
		(call.TargetPosition != nil && *call.TargetPosition >= call.Source.OriginalTargetCount) {
		return InvocationEnvelope{}, nil, true, capabilityUnavailable(errors.New("fixture source context unavailable"))
	}
	o.contexts = append(o.contexts, call)
	encoded, err := json.Marshal(input)
	if err != nil {
		return InvocationEnvelope{}, nil, true, err
	}
	o.inputs = append(o.inputs, encoded)
	if id != o.instance || revision != o.envelope.ConfigurationRevision || o.calls == o.failAt {
		return InvocationEnvelope{}, nil, true, capabilityUnavailable(errors.New("fixture current authorization revoked"))
	}
	envelope := o.envelope
	if o.previewMode && o.preview != nil {
		envelope.ArtifactDigest = o.preview.manifest.Artifact.Digest
	}
	envelope.InvocationID = uuid.New()
	envelope.Input = make(map[string]Value)
	for name, field := range input {
		if field.State == FieldPresent {
			envelope.Input[name] = *field.Value
		}
	}
	if o.alter {
		envelope.Input["text"] = Value{Kind: StringValue, Value: []byte(`"different source"`)}
	}
	return envelope, nil, o.previewMode, nil
}

func newInstalledOwnerFixture(t *testing.T) *installedOwnerFixture {
	t.Helper()
	ctx := context.Background()
	host, err := NewWASMHost(ctx)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	_, root, manifest, pin := artifactFixture(t, installedCommand)
	manifest.InputSchema = map[string]FieldSchema{"mode": {Kind: StringValue}, "text": {Kind: StringValue, Optional: true}}
	manifest.ResultSchema = manifest.InputSchema
	writeManifest(t, root, manifest)
	artifact, err := host.LoadInstalled(ctx, root, "manifest.json", pin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := artifact.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	configuration, err := Literal(ObjectValue, map[string]Value{})
	if err != nil {
		t.Fatal(err)
	}
	return &installedOwnerFixture{artifact: artifact, instance: uuid.New(), envelope: InvocationEnvelope{
		SchemaVersion: 1, WorkflowID: uuid.New(), DefinitionVersionID: uuid.New(), RunID: uuid.New(),
		ExecutionPrincipalID: uuid.New(), ConfigurationRevision: 1, ArtifactDigest: pin.Digest,
		CapabilityID: pin.CapabilityID, Configuration: configuration}}
}

func supplyInstalledPreviewFixture(t *testing.T, owner *installedOwnerFixture) {
	t.Helper()
	_, root, manifest, pin := artifactFixture(t, installedPreviewCommand)
	primary, err := owner.artifact.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	manifest.InputSchema, manifest.ResultSchema = primary.InputSchema, primary.ResultSchema
	manifest.ConfigurationSchema, manifest.ConfigurationPolicy = primary.ConfigurationSchema, primary.ConfigurationPolicy
	writeManifest(t, root, manifest)
	preview, err := owner.artifact.host.LoadInstalled(t.Context(), root, "manifest.json", pin)
	if err != nil {
		t.Fatal(err)
	}
	owner.preview = preview
	t.Cleanup(func() {
		if err := preview.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
}

func TestInstalledWorkflowUsesDedicatedPreviewWithoutMainArtifactFallback(t *testing.T) {
	owner := newInstalledOwnerFixture(t)
	_, source := sourceFixture(t)
	definition := installedDefinition(owner, "ok", FieldRef{Namespace: "source", Path: []string{"text"}})
	program, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	owner.previewMode = true
	result, err := program.EvaluateInstalled(t.Context(), source, owner)
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable || result.Selected {
		t.Fatalf("main artifact substituted for absent preview: %+v %v", result, err)
	}
	supplyInstalledPreviewFixture(t, owner)
	program, err = CompileWorkflowInstalled(t.Context(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err = program.EvaluateInstalled(t.Context(), source, owner)
	if err != nil || result.Fields["text"].Value == nil || string(result.Fields["text"].Value.Value) != `"PREVIEW:LEVEL=7"` {
		t.Fatalf("dedicated preview did not actually run: %+v %v", result, err)
	}
	owner.previewMode = false
	result, err = program.EvaluateInstalled(t.Context(), source, owner)
	if err != nil || result.Fields["text"].Value == nil || string(result.Fields["text"].Value.Value) != `"LEVEL=7"` {
		t.Fatalf("preview replaced ordinary processing: %+v %v", result, err)
	}
	owner.preview.manifest.PurePreview = false
	if _, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("undeclared dedicated preview accepted: %v", err)
	}
	owner.preview.manifest.PurePreview = true
	owner.preview.manifest.InputSchema["mode"] = FieldSchema{Kind: BooleanValue}
	if _, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("incompatible dedicated preview input accepted: %v", err)
	}
	owner.preview.manifest.InputSchema["mode"] = FieldSchema{Kind: StringValue}
	secret := true
	owner.preview.manifest.ConfigurationPolicy.Secret = &secret
	if _, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("preview altered configuration classification: %v", err)
	}
	owner.preview = owner.artifact
	if _, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("same prepared artifact masqueraded as dedicated preview: %v", err)
	}
}

func installedDefinition(owner *installedOwnerFixture, mode string, input FieldRef) Definition {
	definition := selectedDefinition()
	modeValue := Value{Kind: StringValue, Value: []byte(`"` + mode + `"`)}
	definition.Steps = []Step{{ID: "module", Kind: "installed", OutputSchema: owner.artifact.manifest.ResultSchema,
		Installed: &InstalledStep{InstanceID: owner.instance, ConfigurationRevision: 1,
			Input: map[string]Assignment{"mode": {Literal: &modeValue}, "text": {Field: &input}}}}}
	output := FieldRef{Namespace: "step", Path: []string{"module", "text"}}
	definition.OutputSchema["text"] = FieldSchema{Kind: StringValue, Optional: true}
	definition.Outputs["text"] = Assignment{Field: &output}
	return definition
}

func TestInstalledWorkflowIdenticalInputsStillCarryDistinctOriginalContexts(t *testing.T) {
	owner := newInstalledOwnerFixture(t)
	projection, source := sourceFixture(t, "111", "222")
	definition := installedDefinition(owner, "ok", FieldRef{Namespace: "source", Path: []string{"text"}})
	address := FieldRef{Namespace: "target", Path: []string{"address"}}
	first, err := Literal(StringValue, "111")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Literal(StringValue, "222")
	if err != nil {
		t.Fatal(err)
	}
	definition.Selector = Predicate{Op: "or", Args: []Predicate{
		{Op: "eq", Field: &address, Value: &first},
		{Op: "eq", Field: &address, Value: &second},
	}}
	program, err := CompileWorkflowInstalled(t.Context(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := program.EvaluateInstalled(t.Context(), source, owner)
	if err != nil || len(result.TargetResults) != 2 || len(owner.contexts) != 2 || len(owner.inputs) != 2 {
		t.Fatalf("per-context invocation failed: %+v %v", result, err)
	}
	if string(owner.inputs[0]) != string(owner.inputs[1]) {
		t.Fatal("fixture did not actually have indistinguishable mapped inputs")
	}
	for i, call := range owner.contexts {
		if call.Source.TransmissionID != projection.TransmissionID || call.Source.NetworkID != projection.NetworkID ||
			call.Source.SourceRevision != projection.SourceRevision || call.Source.ContextRevision != projection.ContextRevision ||
			call.TargetPosition == nil || *call.TargetPosition != uint32(i) {
			t.Fatalf("mapped inputs lost actual host target context: %+v", call)
		}
	}
	*owner.contexts[0].TargetPosition = 999
	if result.TargetResults[0].TargetPosition != 0 || projection.Targets[0].Position != 0 {
		t.Fatal("owner mutated an original target/result position")
	}
}

func TestInstalledWorkflowActuallyWaitsBeyondBuiltinBudgetWithoutDetachingCaller(t *testing.T) {
	owner := newInstalledOwnerFixture(t)
	_, source := sourceFixture(t)
	definition := installedDefinition(owner, "loop", FieldRef{Namespace: "source", Path: []string{"text"}})
	program, err := CompileWorkflowInstalled(context.Background(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	result, err := program.EvaluateInstalled(ctx, source, owner)
	callerFailure := ctx.Err()
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(callerFailure, context.DeadlineExceeded) ||
		result.Selected || time.Since(started) < time.Second {
		t.Fatalf("real installed loop used builtin deadline rather than caller deadline: %+v, %v", result, err)
	}
	definition = installedDefinition(owner, "ok", FieldRef{Namespace: "source", Path: []string{"text"}})
	program, err = CompileWorkflowInstalled(context.Background(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	result, err = program.EvaluateInstalled(context.Background(), source, owner)
	if err != nil || !result.Selected || result.Fields["text"].Value == nil ||
		string(result.Fields["text"].Value.Value) != `"LEVEL=7"` {
		t.Fatalf("host mapping after installed call or prior deadline failed: %+v, %v", result, err)
	}
	if len(result.Fields["text"].Dependencies) != 1 || result.Fields["text"].Dependencies[0].Groups[0] != "payload" {
		t.Fatalf("installed output lost input/selector classification: %+v", result)
	}
}

func TestInstalledWorkflowRechecksEveryTargetAndRejectsInputSubstitution(t *testing.T) {
	owner := newInstalledOwnerFixture(t)
	_, source := sourceFixture(t, "111", "222")
	definition := installedDefinition(owner, "ok", FieldRef{Namespace: "target", Path: []string{"address"}})
	program, err := CompileWorkflowInstalled(context.Background(), definition, source, nil, owner)
	if err != nil {
		t.Fatal(err)
	}
	owner.failAt = 2
	result, err := program.EvaluateInstalled(context.Background(), source, owner)
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable || result.Selected || owner.calls != 2 {
		t.Fatalf("second-context revocation published first-context result: %+v, %v, calls=%d", result, err, owner.calls)
	}
	owner.calls, owner.failAt, owner.alter = 0, 0, true
	result, err = program.EvaluateInstalled(context.Background(), source, owner)
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable || result.Selected {
		t.Fatalf("owner input substitution changed immutable source: %+v, %v", result, err)
	}
	owner.alter = false
	result, err = program.EvaluateInstalled(context.Background(), source, owner)
	if err != nil || len(result.TargetResults) != 2 {
		t.Fatalf("authorized installed target results failed after rejection: %+v, %v", result, err)
	}
	for i, target := range result.TargetResults {
		if target.TargetPosition != uint32(i) || string(target.Fields["text"].Value.Value) != []string{`"111"`, `"222"`}[i] {
			t.Fatalf("installed guest mixed original targets: %+v", result)
		}
	}
	if _, err := program.Evaluate(context.Background(), source); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("installed Workflow invoked without current owner: %v", err)
	}
}
