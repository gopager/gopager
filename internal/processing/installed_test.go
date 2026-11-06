package processing

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

func installedEnvelope(t *testing.T, pin ArtifactPin, input map[string]Value) InvocationEnvelope {
	t.Helper()
	configuration, err := Literal(ObjectValue, map[string]Value{})
	if err != nil {
		t.Fatal(err)
	}
	return InvocationEnvelope{SchemaVersion: 2, InvocationID: uuid.New(), InstanceID: uuid.New(),
		WorkflowID: uuid.New(), DefinitionVersionID: uuid.New(), RunID: uuid.New(),
		ExecutionPrincipalID: uuid.New(), ConfigurationRevision: 1, ArtifactDigest: pin.Digest,
		CapabilityID: pin.CapabilityID, Configuration: configuration, Input: input}
}

func loadCommandPlugin(t *testing.T, host *PluginHost, schema map[string]FieldSchema) (*InstalledArtifact, ArtifactPin) {
	t.Helper()
	_, root, manifest, pin := artifactFixture(t, pluginfixture.Binary(t, "command"))
	manifest.InputSchema, manifest.ResultSchema = schema, schema
	writeManifest(t, root, manifest)
	artifact, err := host.LoadInstalled(context.Background(), root, "manifest.json", pin)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := artifact.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return artifact, pin
}

func TestInstalledPluginWireInvocationRestartAndIsolation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	artifact, pin := loadCommandPlugin(t, host, map[string]FieldSchema{"mode": {Kind: StringValue}, "text": {Kind: StringValue}})
	text, err := Literal(StringValue, "original text")
	if err != nil {
		t.Fatal(err)
	}
	envelope := installedEnvelope(t, pin, nil)
	for _, mode := range []string{"ok", "mixed", "private-failure", "fail", "loop", "ok", "crash", "ok", "env", "instance"} {
		value, err := Literal(StringValue, mode)
		if err != nil {
			t.Fatal(err)
		}
		envelope.Input = map[string]Value{"mode": value, "text": text}
		timeout := 10 * time.Second
		if mode == "loop" {
			timeout = 200 * time.Millisecond
		}
		callContext, cancel := context.WithTimeout(ctx, timeout)
		fields, err := artifact.invokeProjected(callContext, envelope)
		cancel()
		switch mode {
		case "ok":
			if err != nil || string(fields["text"].Value) != `"original text"` {
				t.Fatalf("installed success failed (also after restart): %+v, %v", fields, err)
			}
		case "loop":
			if !errors.Is(err, context.DeadlineExceeded) || platform.FailureCode(err) == platform.CodeResourceLimit || fields != nil {
				t.Fatalf("plugin loop escaped caller deadline or became a resource bound: %+v, %v", fields, err)
			}
		case "env":
			if err != nil || string(fields["text"].Value) != `""` {
				t.Fatalf("Core environment reached the plugin: %+v, %v", fields, err)
			}
		case "instance":
			if err != nil || string(fields["text"].Value) != `"`+envelope.InstanceID.String()+`"` {
				t.Fatalf("instance ID did not reach the plugin: %+v, %v", fields, err)
			}
		default:
			if platform.FailureCode(err) != platform.CodeInvalidInput || fields != nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("%s: malformed/failed plugin became success or leaked text: %+v, %v", mode, fields, err)
			}
		}
	}
	envelope.Mode = InvocationModePreview
	artifact.manifest.SupportsPreview = false
	if _, err := artifact.invokeProjected(ctx, envelope); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("undeclared preview capability was invoked: %v", err)
	}
	artifact.manifest.SupportsPreview = true
	envelope.Mode = InvocationModeLive
	secret := true
	artifact.manifest.ConfigurationPolicy.Secret = &secret
	if _, err := artifact.invokeProjected(ctx, envelope); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("unresolved Secret configuration was invoked: %v", err)
	}
	envelope.InstanceID = uuid.Nil
	artifact.manifest.ConfigurationPolicy.Secret = new(bool)
	if _, err := artifact.invokeProjected(ctx, envelope); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("envelope without instance ID was accepted: %v", err)
	}
}

func TestInstalledPluginOwnDeadlineIsResourceLimit(t *testing.T) { //nolint:paralleltest // swaps the package variable installedInvocationTimeout
	ctx := context.Background()
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	artifact, pin := loadCommandPlugin(t, host, map[string]FieldSchema{"mode": {Kind: StringValue}, "text": {Kind: StringValue}})
	original := installedInvocationTimeout
	installedInvocationTimeout = 300 * time.Millisecond
	t.Cleanup(func() { installedInvocationTimeout = original })
	for _, mode := range []string{"loop", "ok"} {
		if mode == "ok" {
			// The restart after the deadline starts a new process, which can exceed the short test
			// deadline under load; the restarted call gets the production limit.
			installedInvocationTimeout = original
		}
		value, err := Literal(StringValue, mode)
		if err != nil {
			t.Fatal(err)
		}
		envelope := installedEnvelope(t, pin, map[string]Value{"mode": value, "text": value})
		fields, err := artifact.invokeProjected(ctx, envelope)
		switch mode {
		case "loop":
			if platform.FailureCode(err) != platform.CodeResourceLimit || fields != nil {
				t.Fatalf("plugin exceeding its own deadline was not RESOURCE_LIMIT: %+v, %v", fields, err)
			}
		case "ok":
			if err != nil || string(fields["text"].Value) != `"ok"` {
				t.Fatalf("plugin was not restarted after its deadline: %+v, %v", fields, err)
			}
		}
	}
}

func TestInstalledPluginInstancesRunInSeparateProcesses(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	schema := map[string]FieldSchema{"mode": {Kind: StringValue}, "text": {Kind: StringValue}}
	first, pin := loadCommandPlugin(t, host, schema)
	second, _ := loadCommandPlugin(t, host, schema)
	mode, err := Literal(StringValue, "instance")
	if err != nil {
		t.Fatal(err)
	}
	placeholder, err := Literal(StringValue, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range []*InstalledArtifact{first, second} {
		envelope := installedEnvelope(t, pin, map[string]Value{"mode": mode, "text": placeholder})
		fields, err := artifact.invokeProjected(ctx, envelope)
		if err != nil || string(fields["text"].Value) != `"`+envelope.InstanceID.String()+`"` {
			t.Fatalf("instance invocation failed: %+v, %v", fields, err)
		}
	}
	if first.process == second.process || !first.process.running() || !second.process.running() {
		t.Fatal("configured instances did not get their own plugin processes")
	}
	if err := first.Close(ctx); err != nil {
		t.Fatal(err)
	}
	envelope := installedEnvelope(t, pin, map[string]Value{"mode": mode, "text": placeholder})
	if _, err := second.invokeProjected(ctx, envelope); err != nil {
		t.Fatalf("closing one instance stopped another: %v", err)
	}
	if _, err := first.invokeProjected(ctx, envelope); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("closed instance still invoked its plugin: %v", err)
	}
}

func TestInstalledSDK2RoundTripsTypedBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	artifact, pin := loadCommandPlugin(t, host, map[string]FieldSchema{"binary": {Kind: BytesValue}})
	binary, err := Literal(BytesValue, []byte{0, 1, 2, 255})
	if err != nil {
		t.Fatal(err)
	}
	fields, err := artifact.invokeProjected(ctx, installedEnvelope(t, pin, map[string]Value{"binary": binary}))
	if err != nil || string(fields["binary"].Value) != `"AAEC/w=="` {
		t.Fatalf("SDK 2.0 bytes changed across the plugin boundary: %+v, %v", fields, err)
	}
}
