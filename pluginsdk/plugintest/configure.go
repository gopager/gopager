package plugintest

import (
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/processing"
)

// Configure saves a new revision of the instance's administrative configuration, as an
// administrator saving the instance would. values holds the plain properties (see typedValue for
// the accepted Go types) and secrets the Secret properties by dotted path. Each call increments
// the revision, retires the plugin process of the previous revision and keeps the personal values
// of subscribers. The Core checks the configuration against the manifest when the instance is
// described or used, so a mismatch surfaces from Describe or Attempt.
func (h *Harness) Configure(values map[string]any, secrets map[string]Secret) {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configureLocked(values, secrets)
}

// configureLocked saves the next revision; h.mu is held.
func (h *Harness) configureLocked(values map[string]any, secrets map[string]Secret) {
	h.t.Helper()
	merged := withSecretReferences(values, h.references(secrets, nil))
	configuration, err := typedObject(merged)
	if err != nil {
		h.t.Fatalf("plugintest: configuration: %v", err)
	}
	targetUse := h.targetUse
	h.revision++
	h.stores.putInstance(processing.InstanceConfiguration{ID: h.instanceID, Revision: h.revision, Enabled: true,
		ExecutionPrincipalID: uuid.New(), CreatedBy: uuid.New(), CreatedAt: h.now,
		Main:          processing.ProvisionedArtifact{ManifestPath: h.manifestAt, Pin: h.pin},
		Configuration: configuration, TargetUse: &targetUse})
	h.host.InstanceChanged(h.instanceID, h.revision)
}

// Personal saves the user-scoped values of one subscriber, as the subscriber would through their
// personal configuration. It replaces the subscriber's previous values and never touches the
// administrative configuration or other subscribers. Secret properties are sealed for this owner.
func (h *Harness) Personal(subscriber uuid.UUID, values map[string]any, secrets map[string]Secret) {
	h.t.Helper()
	owner := subscriber
	merged := withSecretReferences(values, h.references(secrets, &owner))
	configuration, err := typedObject(merged)
	if err != nil {
		h.t.Fatalf("plugintest: personal configuration: %v", err)
	}
	h.stores.putPersonal(processing.PersonalConfiguration{InstanceID: h.instanceID, OwnerID: subscriber,
		Configuration: configuration, UpdatedAt: time.Now().UTC().Truncate(time.Microsecond)})
}

// current returns the revision the next call uses, saving an empty configuration first when
// Configure was never called.
func (h *Harness) current() processing.InstanceConfiguration {
	h.t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.revision == 0 {
		h.configureLocked(nil, nil)
	}
	instances, err := h.stores.ListCurrentInstances(h.t.Context(), nil)
	if err != nil || len(instances) != 1 {
		h.t.Fatalf("plugintest: current instance unavailable: %v", err)
	}
	return instances[0]
}
