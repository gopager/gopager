package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/pluginsdk"
)

// MaxArtifactBytes bounds the streamed SHA-256 verification of a plugin executable.
const MaxArtifactBytes = 256 << 20

// ConfigurationPolicy mirrors every declared container, property and list element.
// Secret is a pointer so omission is never interpreted as an explicit public classification.
type ConfigurationPolicy struct {
	Scope      string                         `json:"configurationScope"`
	Secret     *bool                          `json:"secret"`
	Properties map[string]ConfigurationPolicy `json:"properties,omitempty"`
	Element    *ConfigurationPolicy           `json:"element,omitempty"`
}

// ArtifactReference identifies exact provisioned bytes, not a remote download location.
type ArtifactReference struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// InstalledManifest describes an Operator-provisioned plugin executable (manifest schema 2, SDK 2.0
// or 2.1). Capability selects the pure Workflow step (processing, the default) or a Delivery target.
type InstalledManifest struct {
	SchemaVersion       int                    `json:"schemaVersion"`
	SDKMajor            int                    `json:"sdkMajor"`
	SDKMinor            int                    `json:"sdkMinor"`
	Capability          string                 `json:"capability,omitempty"`
	PluginID            string                 `json:"pluginId"`
	Version             string                 `json:"version"`
	CapabilityID        string                 `json:"capabilityId"`
	Artifact            ArtifactReference      `json:"artifact"`
	Pure                bool                   `json:"pure"`
	SupportsPreview     bool                   `json:"supportsPreview"`
	InputSchema         map[string]FieldSchema `json:"inputSchema"`
	ResultSchema        map[string]FieldSchema `json:"resultSchema"`
	ConfigurationSchema FieldSchema            `json:"configurationSchema"`
	ConfigurationPolicy ConfigurationPolicy    `json:"configurationPolicy"`
	RequiredCoreFields  []FieldRef             `json:"requiredCoreFields"`
	RequiredOperations  []string               `json:"requiredOperations"`
	HostCapabilities    []string               `json:"hostCapabilities"`
	Delivery            *DeliveryCapability    `json:"delivery,omitempty"`
	Resources           *PluginResources       `json:"resources,omitempty"`
}

// ArtifactPin is captured from current instance configuration, never supplied by a guest.
type ArtifactPin struct {
	PluginID     string `json:"pluginId"`
	Version      string `json:"version"`
	CapabilityID string `json:"capabilityId"`
	Digest       string `json:"digest"`
}

// InstalledArtifact is a handle to a verified plugin executable; loading never starts it. The
// process starts on first invocation and is shared, through the host's pool, by every handle loaded
// for the same instance revision (see PluginHost). Closing a handle releases its reference; it does
// not stop a pooled process, which the idle timeout, InstanceChanged and host Close do.
// Invocation still requires the separate current configuration/Principal/source authority participant.
type InstalledArtifact struct {
	manifest InstalledManifest
	host     *PluginHost
	process  *pluginProcess

	mu     sync.Mutex
	closed bool
}

// isClosed reports whether Close was called on this handle.
func (a *InstalledArtifact) isClosed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.closed
}

// artifactCacheKey identifies a verified executable by its location below the artifact root and its
// pinned digest.
type artifactCacheKey struct {
	root       string
	executable string
	digest     string
}

// cachedArtifact records that the executable at key hashed to key.digest when it had this size,
// modification time and mode, so later loads skip re-hashing up to MaxArtifactBytes. It is only a
// load-time shortcut: SecureConfig re-verifies the bytes whenever a process starts.
type cachedArtifact struct {
	key       artifactCacheKey
	path      string
	dir       string
	size      int64
	modTime   time.Time
	mode      os.FileMode
	instances map[uuid.UUID]struct{}
}

// maxCachedArtifacts bounds the verification cache; it is cleared when exceeded because entries
// are cheap to rebuild and instances rarely pin more than a handful of distinct executables.
const maxCachedArtifacts = 256

func capabilityUnavailable(cause error) error {
	return platform.NewFailure(platform.CodeAuthorityUnavailable, cause)
}

func validateConfigurationPolicy(schema FieldSchema, policy ConfigurationPolicy, depth int) error {
	if depth > MaxDepth {
		return resource(errors.New("configuration policy nesting boundary"))
	}
	if !slices.Contains([]string{"admin", "user"}, policy.Scope) || policy.Secret == nil {
		return invalid(errors.New("unclassified configuration property"))
	}
	if schema.Kind == ObjectValue {
		if policy.Element != nil || len(policy.Properties) != len(schema.Properties) {
			return invalid(errors.New("configuration object policy is incomplete"))
		}
		for name, child := range schema.Properties {
			declaration, exists := policy.Properties[name]
			if !exists {
				return invalid(errors.New("configuration property has no policy"))
			}
			if err := validateConfigurationPolicy(child, declaration, depth+1); err != nil {
				return err
			}
		}
	} else if schema.Kind == ListValue {
		if len(policy.Properties) != 0 || policy.Element == nil {
			return invalid(errors.New("configuration element policy is incomplete"))
		}
		return validateConfigurationPolicy(*schema.Element, *policy.Element, depth+1)
	} else if len(policy.Properties) != 0 || policy.Element != nil {
		return invalid(errors.New("configuration scalar has container policy"))
	}
	return nil
}

func validateManifest(manifest InstalledManifest, pin ArtifactPin) error {
	if manifest.SchemaVersion != 2 || manifest.SDKMajor != pluginsdk.ProtocolVersion ||
		(manifest.SDKMinor != 0 && manifest.SDKMinor != sdkMinorDelivery) {
		return platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("unsupported plugin manifest schema/SDK"))
	}
	capability, err := manifest.capabilityKind()
	if err != nil {
		return err
	}
	if !filepath.IsLocal(filepath.FromSlash(manifest.Artifact.Path)) {
		return invalid(errors.New("plugin executable path must stay inside the artifact root"))
	}
	if !validLocalName(manifest.PluginID) || !validLocalName(manifest.Version) || !validLocalName(manifest.CapabilityID) ||
		manifest.PluginID != pin.PluginID || manifest.Version != pin.Version ||
		manifest.CapabilityID != pin.CapabilityID || manifest.Artifact.Digest != pin.Digest {
		return capabilityUnavailable(errors.New("installed artifact identity/pin mismatch"))
	}
	digest, err := hex.DecodeString(pin.Digest)
	if err != nil || len(digest) != sha256.Size || pin.Digest != strings.ToLower(pin.Digest) {
		return invalid(errors.New("artifact digest must be lowercase SHA-256"))
	}
	if err := validateCapabilityDeclaration(manifest, capability); err != nil {
		return err
	}
	for _, schema := range []map[string]FieldSchema{manifest.InputSchema, manifest.ResultSchema} {
		if len(schema) > MaxCollection {
			return resource(errors.New("installed schema collection boundary"))
		}
		for name, field := range schema {
			if !validLocalName(name) {
				return invalid(errors.New("invalid installed field name"))
			}
			if err := field.Validate(); err != nil {
				return err
			}
		}
	}
	if schemaContainsBytes(manifest.ConfigurationSchema) {
		return invalid(errors.New("bytes are unsupported in installed configuration"))
	}
	if manifest.ConfigurationSchema.Kind != ObjectValue {
		return invalid(errors.New("installed configuration must have a closed object schema"))
	}

	if err := manifest.ConfigurationSchema.Validate(); err != nil {
		return err
	}
	if err := validateConfigurationPolicy(manifest.ConfigurationSchema, manifest.ConfigurationPolicy, 1); err != nil {
		return err
	}
	if capability == CapabilityDelivery {
		return validateDeliveryConfiguration(manifest.ConfigurationSchema, manifest.ConfigurationPolicy)
	}
	return nil
}

func schemaContainsBytes(schema FieldSchema) bool {
	if schema.Kind == BytesValue {
		return true
	}
	if schema.Element != nil && schemaContainsBytes(*schema.Element) {
		return true
	}
	for _, property := range schema.Properties {
		if schemaContainsBytes(property) {
			return true
		}
	}
	return false
}

func readArtifactFile(ctx context.Context, root *os.Root, path string, limit int) (data []byte, failure error) {
	if root == nil || ctx == nil || path == "" {
		return nil, invalid(errors.New("missing artifact-directory participant"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := root.Stat(path)
	if err != nil {
		return nil, capabilityUnavailable(err)
	}
	if !info.Mode().IsRegular() {
		return nil, invalid(errors.New("artifact is not a regular file"))
	}
	if info.Size() > int64(limit) {
		return nil, resource(errors.New("installed artifact size boundary"))
	}
	file, err := root.Open(path)
	if err != nil {
		return nil, capabilityUnavailable(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			data = nil
			failure = errors.Join(failure, capabilityUnavailable(err))
		}
	}()
	info, err = file.Stat()
	if err != nil {
		return nil, capabilityUnavailable(err)
	}
	if !info.Mode().IsRegular() {
		return nil, invalid(errors.New("opened artifact is not a regular file"))
	}
	if info.Size() > int64(limit) {
		return nil, resource(errors.New("opened artifact size boundary"))
	}
	data, err = io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, capabilityUnavailable(err)
	}
	if len(data) > limit {
		return nil, resource(errors.New("growing artifact size boundary"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// hashExecutable verifies a regular, executable artifact inside root by streaming its SHA-256.
func hashExecutable(ctx context.Context, root *os.Root, path string) (digest string, failure error) {
	info, err := root.Stat(path)
	if err != nil {
		return "", capabilityUnavailable(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", capabilityUnavailable(errors.New("plugin artifact is not a regular executable file"))
	}
	if info.Size() > MaxArtifactBytes {
		return "", resource(errors.New("plugin artifact size boundary"))
	}
	file, err := root.Open(path)
	if err != nil {
		return "", capabilityUnavailable(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			failure = errors.Join(failure, capabilityUnavailable(err))
		}
	}()
	hash := sha256.New()
	read, err := io.Copy(hash, io.LimitReader(file, MaxArtifactBytes+1))
	if err != nil {
		return "", capabilityUnavailable(err)
	}
	if read > MaxArtifactBytes {
		return "", resource(errors.New("growing plugin artifact size boundary"))
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// LoadInstalled reads only Operator-provisioned files below root and verifies the exact pinned
// executable. It does not start the plugin or grant authority. The artifact is not tied to an
// instance, so it gets a private process that stops when the handle is closed; use LoadInstalledFor
// for pooled, reusable processes.
func (h *PluginHost) LoadInstalled(ctx context.Context, root *os.Root, manifestPath string, pin ArtifactPin) (*InstalledArtifact, error) {
	return h.LoadInstalledFor(ctx, root, manifestPath, pin, uuid.Nil, 0, InvocationModeLive)
}

// LoadInstalledFor is LoadInstalled for a configured instance revision. Handles loaded for the
// same instance, revision, digest and invocation mode share one long-lived plugin process, which
// the host stops after pluginIdleTimeout, on InstanceChanged for the instance, or on Close. Live
// and preview invocations never share a process, so an editor's preview cannot occupy the call
// slot a live run waits for. The executable hash is cached per digest and revalidated by size,
// modification time and file mode, so repeated loads do not re-read the binary; the manifest is
// always re-read so edits are never served stale.
func (h *PluginHost) LoadInstalledFor(ctx context.Context, root *os.Root, manifestPath string, pin ArtifactPin,
	instance uuid.UUID, revision int64, mode string) (*InstalledArtifact, error) {
	manifest, cached, err := h.loadVerified(ctx, root, manifestPath, pin, instance)
	if err != nil {
		return nil, err
	}
	process, err := h.acquire(processKey{instance: instance, revision: revision, digest: pin.Digest, mode: mode}, cached)
	if err != nil {
		return nil, err
	}
	return &InstalledArtifact{manifest: manifest, host: h, process: process}, nil
}

// loadVerified reads and validates the manifest and verifies the pinned executable without
// acquiring or starting a process. A non-nil instance is remembered by the cache entry so
// InstanceChanged can drop it.
func (h *PluginHost) loadVerified(ctx context.Context, root *os.Root, manifestPath string, pin ArtifactPin,
	instance uuid.UUID) (InstalledManifest, *cachedArtifact, error) {
	if h == nil || ctx == nil || root == nil {
		return InstalledManifest{}, nil, invalid(errors.New("missing plugin host/root/context"))
	}
	encoded, err := readArtifactFile(ctx, root, manifestPath, MaxInputBytes)
	if err != nil {
		return InstalledManifest{}, nil, err
	}
	var manifest InstalledManifest
	if err := Decode(encoded, &manifest); err != nil {
		return InstalledManifest{}, nil, err
	}
	if err := validateManifest(manifest, pin); err != nil {
		return InstalledManifest{}, nil, err
	}
	base, err := filepath.Abs(root.Name())
	if err != nil {
		return InstalledManifest{}, nil, capabilityUnavailable(err)
	}
	cached, err := h.verifiedExecutable(ctx, root, base, filepath.FromSlash(manifest.Artifact.Path), pin.Digest, instance)
	if err != nil {
		return InstalledManifest{}, nil, err
	}
	return manifest, cached, nil
}

// verifiedExecutable returns the cache entry for an executable whose digest equals digest, hashing
// the file only when no entry matches its current size, modification time and mode. A digest
// mismatch is an explicit capability failure and leaves no entry behind.
func (h *PluginHost) verifiedExecutable(ctx context.Context, root *os.Root, base, executable, digest string,
	instance uuid.UUID) (*cachedArtifact, error) {
	key := artifactCacheKey{root: base, executable: executable, digest: digest}
	info, err := root.Stat(executable)
	if err != nil {
		return nil, capabilityUnavailable(err)
	}
	h.mu.Lock()
	entry, hit := h.cache[key]
	if hit && (entry.size != info.Size() || !entry.modTime.Equal(info.ModTime()) || entry.mode != info.Mode()) {
		delete(h.cache, key)
		hit = false
	}
	if hit {
		rememberInstance(entry, instance)
		h.mu.Unlock()
		return entry, nil
	}
	h.mu.Unlock()
	actual, err := hashExecutable(ctx, root, executable)
	if err != nil {
		return nil, err
	}
	if actual != digest {
		return nil, capabilityUnavailable(errors.New("installed artifact digest changed"))
	}
	path := filepath.Join(base, executable)
	entry = &cachedArtifact{key: key, path: path, dir: filepath.Dir(path), size: info.Size(),
		modTime: info.ModTime(), mode: info.Mode(), instances: make(map[uuid.UUID]struct{})}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.cache) >= maxCachedArtifacts {
		clear(h.cache)
	}
	h.cache[key] = entry
	rememberInstance(entry, instance)
	return entry, nil
}

// rememberInstance records which instance loaded an entry so InstanceChanged can drop it.
func rememberInstance(entry *cachedArtifact, instance uuid.UUID) {
	if instance != uuid.Nil {
		entry.instances[instance] = struct{}{}
	}
}

// Close releases this handle's reference to its plugin process. A pooled process keeps running for
// later runs of the same instance revision; an unpooled one stops with its last handle. It is
// idempotent and does not close the host.
func (a *InstalledArtifact) Close(ctx context.Context) error {
	if a == nil || ctx == nil {
		return invalid(errors.New("missing installed artifact/context"))
	}
	a.mu.Lock()
	already := a.closed
	a.closed = true
	a.mu.Unlock()
	if !already && a.process != nil {
		a.process.release()
	}
	return nil
}

// Manifest returns detached inspected metadata, not current activation or execution authority.
func (a *InstalledArtifact) Manifest() (InstalledManifest, error) {
	if a == nil {
		return InstalledManifest{}, invalid(errors.New("missing installed artifact"))
	}
	encoded, err := json.Marshal(a.manifest)
	if err != nil {
		return InstalledManifest{}, invalid(err)
	}
	var detached InstalledManifest
	if err := Decode(encoded, &detached); err != nil {
		return InstalledManifest{}, err
	}
	return detached, nil
}
