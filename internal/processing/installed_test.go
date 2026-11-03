package processing

import (
	"context"
	_ "embed"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

//go:generate env GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -trimpath -buildvcs=false -o testdata/command.wasm ./testdata/command

//go:embed testdata/command.wasm
var installedCommand []byte

func TestInstalledPureCommandWireInvocationAndIsolation(t *testing.T) {
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
	manifest.InputSchema = map[string]FieldSchema{"mode": {Kind: StringValue}, "text": {Kind: StringValue}}
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
	text, err := Literal(StringValue, "original text")
	if err != nil {
		t.Fatal(err)
	}
	envelope := InvocationEnvelope{SchemaVersion: 1, InvocationID: uuid.New(), WorkflowID: uuid.New(),
		DefinitionVersionID: uuid.New(), RunID: uuid.New(), ExecutionPrincipalID: uuid.New(),
		ConfigurationRevision: 1, ArtifactDigest: pin.Digest, CapabilityID: pin.CapabilityID, Configuration: configuration}
	for _, mode := range []string{"ok", "mixed", "private-failure", "fail", "loop", "ok"} {
		value, err := Literal(StringValue, mode)
		if err != nil {
			t.Fatal(err)
		}
		envelope.Input = map[string]Value{"mode": value, "text": text}
		callContext, cancel := context.WithTimeout(ctx, time.Second)
		if mode == "loop" {
			cancel()
			callContext, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
		}
		fields, err := artifact.invokeProjected(callContext, envelope, true)
		cancel()
		switch mode {
		case "ok":
			if err != nil || string(fields["text"].Value) != `"original text"` {
				t.Fatalf("installed success failed: %+v, %v", fields, err)
			}
		case "loop":
			if !errors.Is(err, context.DeadlineExceeded) || fields != nil {
				t.Fatalf("guest loop escaped deadline: %+v, %v", fields, err)
			}
		default:
			if platform.FailureCode(err) != platform.CodeInvalidInput || fields != nil || strings.Contains(err.Error(), "private") {
				t.Fatalf("malformed/failed guest became success or leaked text: %+v, %v", fields, err)
			}
		}

	}
	artifact.manifest.PurePreview = false
	if _, err := artifact.invokeProjected(ctx, envelope, true); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("undeclared preview capability was invoked: %v", err)
	}

	artifact.manifest.PurePreview = true
	secret := true
	artifact.manifest.ConfigurationPolicy.Secret = &secret
	if _, err := artifact.invokeProjected(ctx, envelope, false); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("unresolved Secret configuration was invoked: %v", err)
	}
}

func TestInstalledSDK11RoundTripsTypedBytesThroughWASI(t *testing.T) {
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
	manifest.InputSchema = map[string]FieldSchema{"binary": {Kind: BytesValue}}
	manifest.ResultSchema = map[string]FieldSchema{"binary": {Kind: BytesValue}}
	writeManifest(t, root, manifest)
	if _, err := host.LoadInstalled(ctx, root, "manifest.json", pin); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("SDK 1.0 bytes artifact was loadable: %v", err)
	}
	manifest.SDKMinor = 1
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
	binary, err := Literal(BytesValue, []byte{0, 1, 2, 255})
	if err != nil {
		t.Fatal(err)
	}
	envelope := InvocationEnvelope{SchemaVersion: 1, InvocationID: uuid.New(), WorkflowID: uuid.New(),
		DefinitionVersionID: uuid.New(), RunID: uuid.New(), ExecutionPrincipalID: uuid.New(),
		ConfigurationRevision: 1, ArtifactDigest: pin.Digest, CapabilityID: pin.CapabilityID,
		Configuration: configuration, Input: map[string]Value{"binary": binary}}
	fields, err := artifact.invokeProjected(ctx, envelope, true)
	if err != nil || string(fields["binary"].Value) != `"AAEC/w=="` {
		t.Fatalf("SDK 1.1 bytes changed across WASI boundary: %+v, %v", fields, err)
	}
}
