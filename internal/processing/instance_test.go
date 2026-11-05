package processing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

func TestInstanceInspectionPinsConfigurationAndDedicatedPreviewWithoutExecution(t *testing.T) {
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	directory, root, manifest, pin := artifactFixture(t, markerScript)
	writeManifest(t, root, manifest)
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("preview.json", encoded, 0600); err != nil {
		t.Fatal(err)
	}
	configuration, err := Literal(ObjectValue, map[string]Value{})
	if err != nil {
		t.Fatal(err)
	}
	instance := InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true,
		ExecutionPrincipalID: uuid.New(), CreatedBy: uuid.New(), CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
		Main: ProvisionedArtifact{ManifestPath: "manifest.json", Pin: pin}, Configuration: configuration}
	inspector, err := NewLocalInstanceInspector(host, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := inspector.InspectInstance(t.Context(), instance); err != nil {
		t.Fatalf("inspection ran trapping initialization or rejected valid main: %v", err)
	}
	instance.Preview = &ProvisionedArtifact{ManifestPath: "preview.json", Pin: pin}
	if err := inspector.InspectInstance(t.Context(), instance); err != nil {
		t.Fatalf("independently loaded bit-identical preview rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "started.marker")); !os.IsNotExist(err) {
		t.Fatalf("instance inspection started the plugin: %v", err)
	}
	instance.Preview.Pin.Digest = strings.Repeat("a", 64)
	if err := inspector.InspectInstance(t.Context(), instance); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("configured invalid preview silently omitted: %v", err)
	}
	instance.Preview = nil
	secret := true
	manifest.ConfigurationPolicy.Secret = &secret
	writeManifest(t, root, manifest)
	if err := inspector.InspectInstance(t.Context(), instance); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("unsupported Secret stored as ordinary configuration: %v", err)
	}
	noSecret := false
	manifest.ConfigurationPolicy.Secret = &noSecret
	manifest.ConfigurationSchema.Properties["personal"] = FieldSchema{Kind: StringValue, Optional: true}
	manifest.ConfigurationPolicy.Properties = map[string]ConfigurationPolicy{
		"personal": {Scope: "user", Secret: &noSecret},
	}
	writeManifest(t, root, manifest)
	personal, err := Literal(StringValue, "private")
	if err != nil {
		t.Fatal(err)
	}
	instance.Configuration, err = Literal(ObjectValue, map[string]Value{"personal": personal})
	if err != nil {
		t.Fatal(err)
	}
	if err := inspector.InspectInstance(t.Context(), instance); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("administrative write promoted personal configuration: %v", err)
	}
	instance.Configuration = configuration
	if err := inspector.InspectInstance(t.Context(), instance); err != nil {
		t.Fatalf("absent optional personal configuration blocked administrative instance: %v", err)
	}
}
