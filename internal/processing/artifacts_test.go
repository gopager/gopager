package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

// trapCommand exports _start and also names it as the module start function.
// Inspection may compile it, but instantiating it would execute an unreachable instruction.
var trapCommand = []byte{
	0, 97, 115, 109, 1, 0, 0, 0,
	1, 4, 1, 96, 0, 0,
	3, 2, 1, 0,
	7, 10, 1, 6, 95, 115, 116, 97, 114, 116, 0, 0,
	8, 1, 0,
	10, 5, 1, 3, 0, 0, 11,
}

func artifactFixture(t *testing.T, data []byte) (string, *os.Root, InstalledManifest, ArtifactPin) {
	t.Helper()
	directory := t.TempDir()
	digest := sha256.Sum256(data)
	noSecret := false
	manifest := InstalledManifest{SchemaVersion: 1, SDKMajor: 1, SDKMinor: 0,
		PluginID: "test-command", Version: "1", CapabilityID: "test", Pure: true, PurePreview: true,
		Artifact:    ArtifactReference{Path: "command.wasm", Digest: hex.EncodeToString(digest[:])},
		InputSchema: map[string]FieldSchema{"text": {Kind: StringValue}}, ResultSchema: map[string]FieldSchema{},
		ConfigurationSchema: FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{}},
		ConfigurationPolicy: ConfigurationPolicy{Scope: "admin", Secret: &noSecret},
	}
	if err := os.WriteFile(filepath.Join(directory, "command.wasm"), data, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	pin := ArtifactPin{PluginID: manifest.PluginID, Version: manifest.Version,
		CapabilityID: manifest.CapabilityID, Digest: manifest.Artifact.Digest}
	return directory, root, manifest, pin
}

func writeManifest(t *testing.T, root *os.Root, manifest InstalledManifest) {
	t.Helper()
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("manifest.json", data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledInspectionDoesNotExecuteInitialization(t *testing.T) {
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
	_, root, manifest, pin := artifactFixture(t, trapCommand)
	writeManifest(t, root, manifest)
	artifact, err := host.LoadInstalled(ctx, root, "manifest.json", pin)
	if err != nil {
		t.Fatalf("inspection executed initialization or rejected valid command metadata: %v", err)
	}
	defer func() {
		if err := artifact.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	detached, err := artifact.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	detached.InputSchema["text"] = FieldSchema{Kind: BooleanValue}
	*detached.ConfigurationPolicy.Secret = true
	again, err := artifact.Manifest()
	if err != nil || again.InputSchema["text"].Kind != StringValue || *again.ConfigurationPolicy.Secret {
		t.Fatalf("manifest view aliases inspected configuration: %+v, %v", again, err)
	}
	var result any
	if err := host.invoke(ctx, artifact.module, []byte(`{}`), &result, time.Second); err == nil {
		t.Fatal("fixture did not trap when actually instantiated")
	}
	if err := root.WriteFile("command.wasm", []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := host.LoadInstalled(ctx, root, "manifest.json", pin); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("changed binary was not rejected by exact pin: %v", err)
	}
}

func TestInstalledArtifactPathsAndExactLoadBoundary(t *testing.T) {
	ctx := context.Background()
	_, root, _, _ := artifactFixture(t, nil)
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.wasm")
	if err := os.WriteFile(outsideFile, trapCommand, 0600); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink(outsideFile, "escape.wasm"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"escape.wasm", outsideFile, "../outside.wasm", "."} {
		if data, err := readArtifactFile(ctx, root, path, MaxArtifactBytes); err == nil || data != nil {
			t.Fatalf("path %q escaped regular rooted-file requirements", path)
		}
	}
	file, err := root.Create("large.wasm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := file.Truncate(MaxArtifactBytes); err != nil {
		t.Fatal(err)
	}
	if data, err := readArtifactFile(ctx, root, "large.wasm", MaxArtifactBytes); err != nil || len(data) != MaxArtifactBytes {
		t.Fatalf("exact artifact boundary rejected: %d, %v", len(data), err)
	}
	if err := file.Truncate(MaxArtifactBytes + 1); err != nil {
		t.Fatal(err)
	}
	if data, err := readArtifactFile(ctx, root, "large.wasm", MaxArtifactBytes); platform.FailureCode(err) != platform.CodeResourceLimit || data != nil {
		t.Fatalf("artifact +1 accepted: %d, %v", len(data), err)
	}
}

func TestInstalledManifestRejectsUnsupportedAndUnclassifiedConfiguration(t *testing.T) {
	_, _, manifest, pin := artifactFixture(t, trapCommand)
	for _, mutate := range []func(*InstalledManifest){
		func(m *InstalledManifest) { m.Pure = false },
		func(m *InstalledManifest) { m.HostCapabilities = []string{"network"} },
		func(m *InstalledManifest) { m.RequiredOperations = []string{"unit.read"} },
		func(m *InstalledManifest) { m.SDKMajor = 2 },
		func(m *InstalledManifest) { m.ConfigurationPolicy.Secret = nil },
		func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties = map[string]FieldSchema{"token": {Kind: StringValue}}
		},
		func(m *InstalledManifest) { m.ConfigurationPolicy.Scope = "automatic" },
		func(m *InstalledManifest) { m.Artifact.Digest = strings.Repeat("0", 64) },
	} {
		copy := manifest
		mutate(&copy)
		if err := validateManifest(copy, pin); err == nil {
			t.Fatal("unsupported/unclassified manifest was accepted")
		}
	}

	noSecret := false
	manifest.ConfigurationSchema.Properties = map[string]FieldSchema{"items": {
		Kind: ListValue, Element: &FieldSchema{Kind: StringValue},
	}}
	manifest.ConfigurationPolicy.Properties = map[string]ConfigurationPolicy{"items": {
		Scope: "user", Secret: &noSecret,
		Element: &ConfigurationPolicy{Scope: "user", Secret: &noSecret},
	}}
	if err := validateManifest(manifest, pin); err != nil {
		t.Fatalf("explicit complete nested policy rejected: %v", err)
	}

}

func TestInstalledManifestBytesRequireSDK11AndCannotBeConfiguration(t *testing.T) {
	_, _, manifest, pin := artifactFixture(t, trapCommand)
	manifest.InputSchema = map[string]FieldSchema{"binary": {Kind: BytesValue}}
	if err := validateManifest(manifest, pin); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("SDK 1.0 accepted bytes schema: %v", err)
	}
	manifest.SDKMinor = 1
	if err := validateManifest(manifest, pin); err != nil {
		t.Fatalf("SDK 1.1 rejected bytes input schema: %v", err)
	}
	manifest.InputSchema = map[string]FieldSchema{}
	manifest.ResultSchema = map[string]FieldSchema{"items": {Kind: ListValue, Element: &FieldSchema{Kind: BytesValue}}}
	manifest.SDKMinor = 0
	if err := validateManifest(manifest, pin); platform.FailureCode(err) != platform.CodeIncompatibleVersion {
		t.Fatalf("SDK 1.0 accepted nested bytes result schema: %v", err)
	}
	manifest.SDKMinor = 1
	if err := validateManifest(manifest, pin); err != nil {
		t.Fatalf("SDK 1.1 rejected nested bytes result schema: %v", err)
	}
	for _, field := range []FieldSchema{
		{Kind: BytesValue},
		{Kind: ListValue, Element: &FieldSchema{Kind: ObjectValue,
			Properties: map[string]FieldSchema{"binary": {Kind: BytesValue}}}},
	} {
		manifest.ConfigurationSchema.Properties = map[string]FieldSchema{"value": field}
		if err := validateManifest(manifest, pin); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("recursive bytes configuration schema accepted: %v", err)
		}
	}
}
