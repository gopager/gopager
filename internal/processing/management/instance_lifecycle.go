package management

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// Lifecycle values of an instance, computed on read.
const (
	// LifecycleActive means the instance is enabled and its artifact loads.
	LifecycleActive = "active"
	// LifecycleDisabled means the instance is switched off.
	LifecycleDisabled = "disabled"
	// LifecycleIncompatible means the artifact needs another SDK or manifest version.
	LifecycleIncompatible = "incompatible"
	// LifecycleFailed means the artifact or the configuration cannot be loaded for another reason.
	LifecycleFailed = "failed"
)

// InstanceStatus is what this node knows about the instance's plugin processes. It is not shared
// between nodes and resets with the process.
type InstanceStatus struct {
	Running         bool       `json:"running"`
	LastStartedAt   *time.Time `json:"lastStartedAt,omitempty"`
	LastFailureAt   *time.Time `json:"lastFailureAt,omitempty"`
	LastFailureCode string     `json:"lastFailureCode,omitempty"`
}

// pluginStatus returns the node-local status; without a source nothing is running.
func (s *InstanceService) pluginStatus(id uuid.UUID) InstanceStatus {
	if s.status == nil {
		return InstanceStatus{}
	}
	status := s.status.Status(id)
	return InstanceStatus{Running: status.Running, LastStartedAt: status.LastStartedAt,
		LastFailureAt: status.LastFailureAt, LastFailureCode: status.LastFailureCode}
}

// lifecycleOf classifies an instance: a load failure outranks the enabled flag, because it tells
// the administrator what to fix first.
func lifecycleOf(enabled bool, loadErr error) string {
	switch {
	case loadErr != nil && platform.FailureCode(loadErr) == platform.CodeIncompatibleVersion:
		return LifecycleIncompatible
	case loadErr != nil:
		return LifecycleFailed
	case !enabled:
		return LifecycleDisabled
	}
	return LifecycleActive
}

// loadForView verifies the pinned artifact and projects the configuration; it fails with the
// reason the instance cannot be shown in full.
func (s *InstanceService) loadForView(ctx context.Context, configuration processing.InstanceConfiguration) (
	processing.InstalledManifest, processing.InstanceConfigurationView, error) {
	manifest, err := s.inspector.InspectManifest(ctx, configuration.Main)
	if err != nil {
		return processing.InstalledManifest{}, processing.InstanceConfigurationView{}, err
	}
	projector, ok := s.inspector.(InstanceConfigurationProjector)
	if !ok {
		return processing.InstalledManifest{}, processing.InstanceConfigurationView{},
			platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("instance configuration view owner unavailable"))
	}
	projected, err := projector.ProjectInstanceConfiguration(ctx, configuration)
	return manifest, projected, err
}
