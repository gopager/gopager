package management

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/processing"
)

// manifestInspector serves one manifest to configuration completion.
type manifestInspector struct{ manifest processing.InstalledManifest }

func (manifestInspector) InspectInstance(context.Context, processing.InstanceConfiguration) error {
	return nil
}

func (i manifestInspector) InspectManifest(context.Context, processing.ProvisionedArtifact) (processing.InstalledManifest, error) {
	return i.manifest, nil
}

// optionalBranchManifest declares an optional object "auth" holding a required administrative Secret.
func optionalBranchManifest() processing.InstalledManifest {
	return processing.InstalledManifest{
		Capability: processing.CapabilityDelivery,
		ConfigurationSchema: processing.FieldSchema{Kind: processing.ObjectValue, Properties: map[string]processing.FieldSchema{
			"auth": {Kind: processing.ObjectValue, Optional: true, Properties: map[string]processing.FieldSchema{
				"token": {Kind: processing.StringValue}}}}},
		ConfigurationPolicy: processing.ConfigurationPolicy{Scope: "admin", Secret: truth(false),
			Properties: map[string]processing.ConfigurationPolicy{
				"auth": {Scope: "admin", Secret: truth(false), Properties: map[string]processing.ConfigurationPolicy{
					"token": {Scope: "admin", Secret: truth(true)}}}}},
	}
}

// TestUpdateOmittingOptionalBranchDropsItsSecret checks that a previous Secret below an optional
// object is carried over only while the submitted configuration still holds that object.
func TestUpdateOmittingOptionalBranchDropsItsSecret(t *testing.T) {
	t.Parallel()
	manifest := optionalBranchManifest()
	service := &InstanceService{inspector: manifestInspector{manifest}, secrets: secretSettings{cipher: newCipher(t, 1)}}
	previousValue, err := manifest.WithSecretReferences(processing.EmptyConfiguration(), map[string]string{"auth.token": "file:token"})
	if err != nil {
		t.Fatal(err)
	}
	previous := &processing.InstanceConfiguration{ID: uuid.New(), Configuration: previousValue}

	omitted := processing.InstanceConfiguration{ID: previous.ID}
	if _, err := service.completeConfiguration(t.Context(), &omitted, previous,
		InstanceInput{Configuration: processing.EmptyConfiguration()}); err != nil {
		t.Fatalf("omitting the optional branch: %v", err)
	}
	if strings.Contains(string(omitted.Configuration.Value), "auth") {
		t.Fatalf("the omitted branch was recreated: %s", omitted.Configuration.Value)
	}

	kept := processing.InstanceConfiguration{ID: previous.ID}
	var empty processing.Value
	if err := processing.Decode([]byte(`{"kind":"object","value":{"auth":{"kind":"object","value":{}}}}`), &empty); err != nil {
		t.Fatal(err)
	}
	if _, err := service.completeConfiguration(t.Context(), &kept, previous, InstanceInput{Configuration: empty}); err != nil {
		t.Fatalf("keeping the branch: %v", err)
	}
	if !strings.Contains(string(kept.Configuration.Value), "file:token") {
		t.Fatalf("the branch lost its Secret: %s", kept.Configuration.Value)
	}
}
