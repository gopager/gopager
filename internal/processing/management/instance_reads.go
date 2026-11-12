package management

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// InstanceConfigurationProjector applies the current installed manifest's field classifications.
type InstanceConfigurationProjector interface {
	ProjectInstanceConfiguration(context.Context, processing.InstanceConfiguration) (processing.InstanceConfigurationView, error)
}

// InstanceArtifactView omits local manifest paths and artifact digests.
type InstanceArtifactView struct {
	PluginID     string `json:"pluginId"`
	Version      string `json:"version"`
	CapabilityID string `json:"capabilityId"`
}

// InstanceSummary is the redacted administrative instance listing projection.
type InstanceSummary struct {
	ID                   uuid.UUID            `json:"id"`
	Revision             int64                `json:"revision,string"`
	Enabled              bool                 `json:"enabled"`
	ExecutionPrincipalID uuid.UUID            `json:"executionPrincipalId"`
	Main                 InstanceArtifactView `json:"main"`
	// Capability is the capability of the pinned manifest; empty when the artifact cannot be loaded.
	Capability string    `json:"capability,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// InstanceView adds manifest-classified configuration, the target use, the lifecycle and the
// status of this node to a redacted instance summary. Configuration is absent when the artifact
// cannot be loaded.
type InstanceView struct {
	InstanceSummary
	TargetUse     *processing.TargetUse                 `json:"targetUse,omitempty"`
	Lifecycle     string                                `json:"lifecycle"`
	Status        InstanceStatus                        `json:"status"`
	Configuration *processing.InstanceConfigurationView `json:"configuration,omitempty"`
}

func validateInstancePage(input processing.PageInput) error {
	if input.Offset < 0 || input.Limit <= 0 || input.Limit > processing.MaxCollection ||
		input.Offset > math.MaxInt-input.Limit {
		return authz.ErrInvalidInput
	}
	return nil
}

func instanceSummary(configuration processing.InstanceConfiguration) (InstanceSummary, error) {
	if err := configuration.Validate(); err != nil {
		return InstanceSummary{}, err
	}
	summary := InstanceSummary{
		ID: configuration.ID, Revision: configuration.Revision, Enabled: configuration.Enabled,
		ExecutionPrincipalID: configuration.ExecutionPrincipalID,
		Main: InstanceArtifactView{
			PluginID: configuration.Main.Pin.PluginID, Version: configuration.Main.Pin.Version,
			CapabilityID: configuration.Main.Pin.CapabilityID,
		},
		CreatedAt: configuration.CreatedAt,
	}
	return summary, nil
}

func (s *InstanceService) withConfigureRead(ctx context.Context, proof identity.Proof,
	read func(platform.TransactionContext, authz.Authority, time.Time) error) error {
	if s == nil || s.management == nil || s.repository == nil || ctx == nil || read == nil {
		return authz.ErrInvalidInput
	}
	return s.management.within(ctx, proof, false, nil, func(tx platform.TransactionContext,
		authority authz.Authority, now time.Time) error {
		if err := authz.Authorize(authority, "plugin.configure", nil, authz.Target{}, now); err != nil {
			return err
		}
		return read(tx, authority, now)
	})
}

// ReadInstance returns the current redacted administrative view under global plugin.configure.
func (s *InstanceService) ReadInstance(ctx context.Context, proof identity.Proof, id uuid.UUID) (InstanceView, error) {
	if id == uuid.Nil {
		return InstanceView{}, authz.ErrInvalidInput
	}
	var configuration processing.InstanceConfiguration
	if err := s.withConfigureRead(ctx, proof, func(tx platform.TransactionContext, authority authz.Authority,
		now time.Time) error {
		var err error
		configuration, err = s.repository.ReadCurrentInstance(tx.Context(), tx, authority, id, now)
		return err
	}); err != nil {
		return InstanceView{}, err
	}
	summary, err := instanceSummary(configuration)
	if err != nil {
		return InstanceView{}, err
	}
	view := InstanceView{InstanceSummary: summary, TargetUse: configuration.TargetUse,
		Status: s.pluginStatus(configuration.ID)}
	manifest, projected, loadErr := s.loadForView(ctx, configuration)
	view.Lifecycle = lifecycleOf(configuration.Enabled, loadErr)
	if loadErr == nil {
		view.Capability, view.Configuration = manifest.EffectiveCapability(), &projected
	}
	if err := s.withConfigureRead(ctx, proof, func(tx platform.TransactionContext, authority authz.Authority,
		now time.Time) error {
		current, err := s.repository.ReadCurrentInstance(tx.Context(), tx, authority, id, now)
		if err != nil {
			return err
		}
		if current.Revision != configuration.Revision {
			return authz.ErrRevisionConflict
		}
		return nil
	}); err != nil {
		return InstanceView{}, err
	}
	return view, nil
}

// ListInstances returns a bounded page of redacted current-instance summaries.
func (s *InstanceService) ListInstances(ctx context.Context, proof identity.Proof,
	input processing.PageInput) (processing.Page[InstanceSummary], error) {
	if err := validateInstancePage(input); err != nil {
		return processing.Page[InstanceSummary]{}, err
	}
	var configurations processing.Page[processing.InstanceConfiguration]
	if err := s.withConfigureRead(ctx, proof, func(tx platform.TransactionContext, authority authz.Authority,
		now time.Time) error {
		var err error
		configurations, err = s.repository.ListInstances(tx.Context(), tx, authority, input, now)
		if err != nil {
			return err
		}
		if configurations.Total < len(configurations.Items) || len(configurations.Items) > input.Limit {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid instance page"))
		}
		return nil
	}); err != nil {
		return processing.Page[InstanceSummary]{}, err
	}
	result := processing.Page[InstanceSummary]{Total: configurations.Total,
		Items: make([]InstanceSummary, len(configurations.Items))}
	seen := make(map[uuid.UUID]struct{}, len(configurations.Items))
	for index, configuration := range configurations.Items {
		if _, exists := seen[configuration.ID]; exists {
			return processing.Page[InstanceSummary]{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
				errors.New("duplicate instance in page"))
		}
		seen[configuration.ID] = struct{}{}
		summary, err := instanceSummary(configuration)
		if err != nil {
			return processing.Page[InstanceSummary]{}, err
		}
		if manifest, err := s.inspector.InspectManifest(ctx, configuration.Main); err == nil {
			summary.Capability = manifest.EffectiveCapability()
		}
		result.Items[index] = summary
	}
	return result, nil
}
