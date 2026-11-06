package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// ProvisionedArtifact binds a local manifest to exact Operator-provisioned module bytes.
type ProvisionedArtifact struct {
	ManifestPath string      `json:"manifestPath"`
	Pin          ArtifactPin `json:"pin"`
}

// InstanceConfiguration is an immutable administrative revision, separate from Workflow versions.
// Unsupported Secret and personal configuration cannot be supplied through this administrative record.
type InstanceConfiguration struct {
	ID                   uuid.UUID           `json:"id"`
	Revision             int64               `json:"revision,string"`
	Enabled              bool                `json:"enabled"`
	ExecutionPrincipalID uuid.UUID           `json:"executionPrincipalId"`
	Main                 ProvisionedArtifact `json:"main"`
	Configuration        Value               `json:"configuration"`
	CreatedAt            time.Time           `json:"createdAt"`
	CreatedBy            uuid.UUID           `json:"createdBy"`
}

// ConfigurationFieldView describes one declared administrative configuration value.
type ConfigurationFieldView struct {
	Scope      string                            `json:"scope"`
	Secret     bool                              `json:"secret"`
	Configured bool                              `json:"configured"`
	Redacted   bool                              `json:"redacted"`
	Value      json.RawMessage                   `json:"value,omitempty"`
	Properties map[string]ConfigurationFieldView `json:"properties,omitempty"`
	Items      []ConfigurationFieldView          `json:"items,omitempty"`
}

// InstanceConfigurationView contains manifest-classified values safe for administrative display.
type InstanceConfigurationView struct {
	Fields map[string]ConfigurationFieldView `json:"fields"`
}

// Validate checks persisted identity and bounds without pretending to inspect a supplied module.
func (i InstanceConfiguration) Validate() error {
	if i.ID == uuid.Nil || i.Revision <= 0 || i.ExecutionPrincipalID == uuid.Nil ||
		i.CreatedBy == uuid.Nil || i.CreatedAt.IsZero() || i.CreatedAt.Nanosecond()%1000 != 0 ||
		i.Main.ManifestPath == "" {
		return invalid(errors.New("invalid installed instance identity"))
	}
	artifact := i.Main
	if !utf8.ValidString(artifact.ManifestPath) || strings.ContainsRune(artifact.ManifestPath, '\x00') ||
		!validLocalName(artifact.Pin.PluginID) || !validLocalName(artifact.Pin.Version) ||
		!validLocalName(artifact.Pin.CapabilityID) {
		return invalid(errors.New("invalid provisioned instance artifact"))
	}
	digest, err := hex.DecodeString(artifact.Pin.Digest)
	if err != nil || len(digest) != sha256.Size || artifact.Pin.Digest != strings.ToLower(artifact.Pin.Digest) {
		return invalid(errors.New("invalid instance artifact digest"))
	}
	encoded, err := json.Marshal(i)
	if err != nil {
		return err
	}
	var detached InstanceConfiguration
	return Decode(encoded, &detached)
}

// InstanceRepository keeps immutable snapshots and a current revision under shared configuration locks.
// Private execution reads require current source/Principal authorization from their coordinating owner.
type InstanceRepository interface {
	ReadInstance(context.Context, platform.TransactionContext, uuid.UUID, int64) (InstanceConfiguration, error)
	ReadCurrentInstance(context.Context, platform.TransactionContext, authz.Authority, uuid.UUID,
		time.Time) (InstanceConfiguration, error)
	ListInstances(context.Context, platform.TransactionContext, authz.Authority, PageInput,
		time.Time) (Page[InstanceConfiguration], error)
	SaveInstance(context.Context, platform.TransactionContext, authz.Authority, InstanceConfiguration, int64) error
}
