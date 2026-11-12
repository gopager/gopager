package processing

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

// loadableTTL is how long Loadable reuses its answer.
const loadableTTL = 5 * time.Second

// DeliveryInstances reads instance configuration for the Delivery host. Implementations run inside
// the caller's transaction.
type DeliveryInstances interface {
	// ReadInstance returns the exact immutable revision of an instance.
	ReadInstance(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, revision int64) (InstanceConfiguration, error)
	// ListCurrentInstances returns the current revision of every instance.
	ListCurrentInstances(ctx context.Context, tx platform.TransactionContext) ([]InstanceConfiguration, error)
}

// DeliveryHostConfig wires the Delivery host to its participants.
type DeliveryHostConfig struct {
	// Host owns the plugin processes; required.
	Host *PluginHost
	// Root is this node's plugin directory; nil makes every instance unavailable.
	Root *os.Root
	// Transactions, Instances, Secrets and Personal are the storage participants; required.
	Transactions ExecutionTransactions
	Instances    DeliveryInstances
	Secrets      SecretStore
	Personal     PersonalConfigurations
	// Cipher decrypts stored Secrets; nil means no key is configured, which makes every instance
	// holding any Secret reference unavailable.
	Cipher *SecretCipher
	// SecretFiles is the operator's Secret file root; nil rejects file references at use.
	SecretFiles *os.Root
	// LookupEnv reads Secret environment variables; nil selects os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// Clock supplies the current time; nil selects time.Now.
	Clock func() time.Time
}

// DeliveryHost runs Delivery plugin attempts for the Core. It adapts configured instances with the
// delivery capability to describe, attempt and placement operations.
type DeliveryHost struct {
	cfg DeliveryHostConfig

	mu         sync.Mutex
	loadableAt time.Time
	loadable   []uuid.UUID
	cached     bool
}

// NewDeliveryHost validates the configuration and returns a host.
func NewDeliveryHost(cfg DeliveryHostConfig) (*DeliveryHost, error) {
	if cfg.Host == nil || cfg.Transactions == nil || cfg.Instances == nil || cfg.Secrets == nil || cfg.Personal == nil {
		return nil, invalid(errors.New("missing delivery host dependency"))
	}
	if cfg.LookupEnv == nil {
		cfg.LookupEnv = os.LookupEnv
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	return &DeliveryHost{cfg: cfg}, nil
}

// DeliveryDescription is what an instance offers as a Delivery target. Personal reports that the
// manifest has user-scoped properties.
type DeliveryDescription struct {
	OutputKinds    []string
	Inputs         []DeliveryInputDeclaration
	Dependencies   []DeliveryDependency
	SendsPerMinute int
	Idempotent     bool
	Personal       bool
}

// load verifies the instance's pinned manifest and executable without starting or acquiring a
// process and requires the delivery capability.
func (d *DeliveryHost) load(ctx context.Context, instance InstanceConfiguration) (InstalledManifest, *cachedArtifact, error) {
	if d.cfg.Root == nil {
		return InstalledManifest{}, nil, capabilityUnavailable(errors.New("no plugin directory on this node"))
	}
	manifest, cached, err := d.cfg.Host.loadVerified(ctx, d.cfg.Root, instance.Main.ManifestPath, instance.Main.Pin, instance.ID)
	if err != nil {
		return InstalledManifest{}, nil, err
	}
	if manifest.EffectiveCapability() != CapabilityDelivery {
		return InstalledManifest{}, nil, capabilityUnavailable(errors.New("plugin is not a delivery capability"))
	}
	return manifest, cached, nil
}

// Describe verifies the instance for use as a Delivery target and reports what it offers. It
// starts no process and performs no SQL, so it is safe inside a primary transaction. The
// administrative configuration must satisfy the schema and policy and hold only valid Secret
// references; any Secret reference without a configured key is AUTHORITY_UNAVAILABLE.
func (d *DeliveryHost) Describe(ctx context.Context, instance InstanceConfiguration) (DeliveryDescription, error) {
	if d == nil || ctx == nil {
		return DeliveryDescription{}, invalid(errors.New("missing delivery host/context"))
	}
	if err := instance.Validate(); err != nil {
		return DeliveryDescription{}, err
	}
	manifest, _, err := d.load(ctx, instance)
	if err != nil {
		return DeliveryDescription{}, err
	}
	if err := manifest.validateInstance(instance); err != nil {
		return DeliveryDescription{}, err
	}
	references, err := manifest.ConfigurationSecretReferences(instance.Configuration)
	if err != nil {
		return DeliveryDescription{}, err
	}
	if len(references) != 0 && d.cfg.Cipher == nil {
		return DeliveryDescription{}, capabilityUnavailable(errors.New("secret key is not configured"))
	}
	declared := manifest.Delivery
	return DeliveryDescription{OutputKinds: slices.Clone(declared.OutputKinds), Inputs: slices.Clone(declared.Inputs),
		Dependencies: slices.Clone(declared.Dependencies), SendsPerMinute: declared.SendsPerMinute,
		Idempotent: declared.Idempotent, Personal: manifest.HasPersonalProperties()}, nil
}

// Loadable lists the current, enabled Delivery instances whose pinned manifest and executable verify
// from this node's plugin directory. No process starts. The answer is reused for five seconds;
// without a plugin directory it is empty.
func (d *DeliveryHost) Loadable(ctx context.Context) ([]uuid.UUID, error) {
	if d == nil || ctx == nil {
		return nil, invalid(errors.New("missing delivery host/context"))
	}
	if d.cfg.Root == nil {
		return []uuid.UUID{}, nil
	}
	now := d.cfg.Clock()
	d.mu.Lock()
	if d.cached && now.Sub(d.loadableAt) < loadableTTL {
		cached := slices.Clone(d.loadable)
		d.mu.Unlock()
		return cached, nil
	}
	d.mu.Unlock()
	var instances []InstanceConfiguration
	if err := d.cfg.Transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		instances, err = d.cfg.Instances.ListCurrentInstances(tx.Context(), tx)
		return err
	}); err != nil {
		return nil, err
	}
	loadable := []uuid.UUID{}
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		if _, _, err := d.load(ctx, instance); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		loadable = append(loadable, instance.ID)
	}
	d.mu.Lock()
	d.loadable, d.loadableAt, d.cached = slices.Clone(loadable), now, true
	d.mu.Unlock()
	return loadable, nil
}
