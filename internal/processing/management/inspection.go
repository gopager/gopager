package management

import (
	"context"
	"time"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// ManifestInspection is what inspecting a provisioned artifact reveals: the manifest's identity,
// capability, schemas, policy, Delivery declaration and informational resources. The artifact's
// location and digest are not part of it.
type ManifestInspection struct {
	PluginID            string                         `json:"pluginId"`
	Version             string                         `json:"version"`
	CapabilityID        string                         `json:"capabilityId"`
	Capability          string                         `json:"capability"`
	SupportsPreview     bool                           `json:"supportsPreview"`
	ConfigurationSchema processing.FieldSchema         `json:"configurationSchema"`
	ConfigurationPolicy processing.ConfigurationPolicy `json:"configurationPolicy"`
	Delivery            *processing.DeliveryCapability `json:"delivery,omitempty"`
	Resources           *processing.PluginResources    `json:"resources,omitempty"`
	SecretProperties    []InspectedSecretProperty      `json:"secretProperties"`
}

// InspectedSecretProperty names a Secret property by dotted path and the scope that owns it.
type InspectedSecretProperty struct {
	Path     string `json:"path"`
	Scope    string `json:"scope"`
	Optional bool   `json:"optional"`
}

// InspectArtifact verifies a provisioned manifest and executable without starting the plugin,
// under global plugin.configure. A manifest that declares access its capability does not grant
// fails with processing.ErrCapabilityUndeclared.
func (s *InstanceService) InspectArtifact(ctx context.Context, proof identity.Proof,
	artifact processing.ProvisionedArtifact, check func(identity.Authentication) error) (ManifestInspection, error) {
	if artifact.ManifestPath == "" {
		return ManifestInspection{}, authz.ErrInvalidInput
	}
	if err := s.withConfigureRead(ctx, proof, check, func(platform.TransactionContext, authz.Authority, time.Time) error {
		return nil
	}); err != nil {
		return ManifestInspection{}, err
	}
	manifest, err := s.inspector.InspectManifest(ctx, artifact)
	if err != nil {
		return ManifestInspection{}, err
	}
	inspection := ManifestInspection{PluginID: manifest.PluginID, Version: manifest.Version,
		CapabilityID: manifest.CapabilityID, Capability: manifest.EffectiveCapability(),
		SupportsPreview: manifest.SupportsPreview, ConfigurationSchema: manifest.ConfigurationSchema,
		ConfigurationPolicy: manifest.ConfigurationPolicy, Delivery: manifest.Delivery, Resources: manifest.Resources,
		SecretProperties: []InspectedSecretProperty{}}
	for _, property := range manifest.SecretProperties() {
		inspection.SecretProperties = append(inspection.SecretProperties,
			InspectedSecretProperty{Path: property.Path, Scope: property.Scope, Optional: property.Optional})
	}
	return inspection, nil
}

// requireDelivery fails with not found unless the artifact is a Delivery capability.
func requireDelivery(manifest processing.InstalledManifest) error {
	if manifest.EffectiveCapability() != processing.CapabilityDelivery {
		return authz.ErrNotFound
	}
	return nil
}
