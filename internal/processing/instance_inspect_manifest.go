package processing

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// InspectManifest verifies the manifest and executable of a provisioned artifact without starting
// the plugin and returns the verified manifest. A manifest that declares access its capability
// does not grant fails with a CapabilityUndeclaredError.
func (i *LocalInstanceInspector) InspectManifest(ctx context.Context, artifact ProvisionedArtifact) (InstalledManifest, error) {
	if i == nil || ctx == nil {
		return InstalledManifest{}, invalid(errors.New("missing manifest inspection context"))
	}
	manifest, _, err := i.host.loadVerified(ctx, i.root, artifact.ManifestPath, artifact.Pin, uuid.Nil)
	return manifest, err
}
