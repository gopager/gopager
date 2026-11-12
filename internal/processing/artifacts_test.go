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

// markerScript records that it was started; loading must inspect artifacts without executing them.
var markerScript = []byte("#!/bin/sh\necho started > started.marker\nexit 1\n")

func artifactFixture(t *testing.T, data []byte) (string, *os.Root, InstalledManifest, ArtifactPin) {
	t.Helper()
	directory := t.TempDir()
	digest := sha256.Sum256(data)
	noSecret := false
	manifest := InstalledManifest{SchemaVersion: 2, SDKMajor: 2, SDKMinor: 0,
		PluginID: "test-command", Version: "1", CapabilityID: "test", Pure: true, SupportsPreview: true,
		Artifact:    ArtifactReference{Path: "plugin", Digest: hex.EncodeToString(digest[:])},
		InputSchema: map[string]FieldSchema{"text": {Kind: StringValue}}, ResultSchema: map[string]FieldSchema{},
		ConfigurationSchema: FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{}},
		ConfigurationPolicy: ConfigurationPolicy{Scope: "admin", Secret: &noSecret},
	}
	if err := os.WriteFile(filepath.Join(directory, "plugin"), data, 0o755); err != nil {
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

func TestInstalledLoadDoesNotStartPlugin(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	directory, root, manifest, pin := artifactFixture(t, markerScript)
	writeManifest(t, root, manifest)
	artifact, err := host.LoadInstalled(ctx, root, "manifest.json", pin)
	if err != nil {
		t.Fatalf("valid plugin metadata rejected: %v", err)
	}
	defer func() {
		if err := artifact.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	marker := filepath.Join(directory, "started.marker")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("loading started the plugin: %v", err)
	}
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
	// On Linux, executing a script that was just written can fail with "text file busy" while a
	// parallel test forks with the file still open for writing; the script then never runs. A
	// few attempts tell that transient failure from an invocation that never starts the plugin.
	started := false
	for attempt := 0; attempt < 5 && !started; attempt++ {
		var result any
		if err := artifact.call(ctx, []byte(`{}`), &result, 5*time.Second, nil); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
			t.Fatalf("plugin without handshake was not an explicit capability failure: %v", err)
		}
		_, err := os.Stat(marker)
		started = err == nil
		if !started {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if !started {
		t.Fatal("invocation did not start the plugin from its artifact directory")
	}
	if err := root.WriteFile("plugin", []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := host.LoadInstalled(ctx, root, "manifest.json", pin); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("changed executable was not rejected by exact pin: %v", err)
	}
}

func TestInstalledArtifactPathsExecutableAndHashBoundary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, root, _, _ := artifactFixture(t, markerScript)
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside")
	if err := os.WriteFile(outsideFile, markerScript, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := root.Symlink(outsideFile, "escape"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"escape", outsideFile, "../outside", "."} {
		if digest, err := hashExecutable(ctx, root, path); err == nil || digest != "" {
			t.Fatalf("path %q escaped regular rooted executable requirements", path)
		}
	}
	if err := root.WriteFile("data", markerScript, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := hashExecutable(ctx, root, "data"); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("non-executable artifact accepted: %v", err)
	}
	file, err := root.OpenFile("large", os.O_CREATE|os.O_WRONLY, 0o755)
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
	if digest, err := hashExecutable(ctx, root, "large"); err != nil || len(digest) != 64 {
		t.Fatalf("exact artifact boundary rejected: %q, %v", digest, err)
	}
	if err := file.Truncate(MaxArtifactBytes + 1); err != nil {
		t.Fatal(err)
	}
	if digest, err := hashExecutable(ctx, root, "large"); platform.FailureCode(err) != platform.CodeResourceLimit || digest != "" {
		t.Fatalf("artifact +1 accepted: %q, %v", digest, err)
	}
	if data, err := readArtifactFile(ctx, root, "escape", MaxInputBytes); err == nil || data != nil {
		t.Fatal("manifest reader followed an escaping symlink")
	}
}

func TestInstalledManifestRejectsUnsupportedAndUnclassifiedConfiguration(t *testing.T) {
	t.Parallel()
	_, _, manifest, pin := artifactFixture(t, markerScript)
	for _, mutate := range []func(*InstalledManifest){
		func(m *InstalledManifest) { m.Pure = false },
		func(m *InstalledManifest) { m.HostCapabilities = []string{"network"} },
		func(m *InstalledManifest) { m.RequiredOperations = []string{"unit.read"} },
		func(m *InstalledManifest) { m.SDKMajor = 1 },
		func(m *InstalledManifest) { m.SchemaVersion = 1 },
		func(m *InstalledManifest) { m.Artifact.Path = "../outside" },
		func(m *InstalledManifest) { m.Artifact.Path = "/bin/sh" },
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

func TestInstalledManifestBytesAreSDK2AndCannotBeConfiguration(t *testing.T) {
	t.Parallel()
	_, _, manifest, pin := artifactFixture(t, markerScript)
	manifest.InputSchema = map[string]FieldSchema{"binary": {Kind: BytesValue}}
	manifest.ResultSchema = map[string]FieldSchema{"items": {Kind: ListValue, Element: &FieldSchema{Kind: BytesValue}}}
	if err := validateManifest(manifest, pin); err != nil {
		t.Fatalf("SDK 2.0 rejected bytes schemas: %v", err)
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
