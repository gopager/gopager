package management

import (
	"context"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/processing"
)

// completeConfiguration finishes the configuration of a save outside SQL: it checks the supplied
// configuration against the pinned manifest, plans the Secret writes against the previous
// revision's references and sets the resulting reference strings into the configuration. A
// processing instance has no Secrets.
func (s *InstanceService) completeConfiguration(ctx context.Context, configuration *processing.InstanceConfiguration,
	previous *processing.InstanceConfiguration, input InstanceInput) (secretPlan, error) {
	manifest, err := s.inspector.InspectManifest(ctx, input.Main)
	if err != nil {
		return secretPlan{}, err
	}
	if manifest.EffectiveCapability() != processing.CapabilityDelivery && len(input.Secrets) != 0 {
		return secretPlan{}, authz.ErrInvalidInput
	}
	if err := manifest.RejectSecretValues(input.Configuration); err != nil {
		return secretPlan{}, err
	}
	var kept map[string]string
	if previous != nil {
		if earlier, err := s.inspector.InspectManifest(ctx, previous.Main); err == nil {
			kept = previousReferences(earlier, previous.Configuration)
		}
	}
	plan, err := planSecrets(s.secrets, manifest, secretScope{scope: "admin", instanceID: configuration.ID,
		allowReference: true}, kept, input.Secrets)
	if err != nil {
		return secretPlan{}, err
	}
	configuration.Configuration, err = manifest.WithSecretReferences(input.Configuration, plan.references)
	return plan, err
}
