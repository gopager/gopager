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

	"github.com/hashicorp/go-plugin"

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

// InstalledManifest describes an Operator-provisioned plugin executable (manifest schema 2, SDK 2.0).
type InstalledManifest struct {
	SchemaVersion       int                    `json:"schemaVersion"`
	SDKMajor            int                    `json:"sdkMajor"`
	SDKMinor            int                    `json:"sdkMinor"`
	PluginID            string                 `json:"pluginId"`
	Version             string                 `json:"version"`
	CapabilityID        string                 `json:"capabilityId"`
	Artifact            ArtifactReference      `json:"artifact"`
	Pure                bool                   `json:"pure"`
	PurePreview         bool                   `json:"purePreview"`
	InputSchema         map[string]FieldSchema `json:"inputSchema"`
	ResultSchema        map[string]FieldSchema `json:"resultSchema"`
	ConfigurationSchema FieldSchema            `json:"configurationSchema"`
	ConfigurationPolicy ConfigurationPolicy    `json:"configurationPolicy"`
	RequiredCoreFields  []FieldRef             `json:"requiredCoreFields"`
	RequiredOperations  []string               `json:"requiredOperations"`
	HostCapabilities    []string               `json:"hostCapabilities"`
}

// ArtifactPin is captured from current instance configuration, never supplied by a guest.
type ArtifactPin struct {
	PluginID     string `json:"pluginId"`
	Version      string `json:"version"`
	CapabilityID string `json:"capabilityId"`
	Digest       string `json:"digest"`
}

// InstalledArtifact is a verified plugin executable; loading never starts it. Its process starts on
// first invocation and belongs to this artifact (one per configured instance and run).
// Invocation still requires the separate current configuration/Principal/source authority participant.
type InstalledArtifact struct {
	manifest InstalledManifest
	host     *PluginHost
	path     string
	dir      string

	mu     sync.Mutex
	closed bool
	client *plugin.Client
	stub   *pluginsdk.Client
}

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
	if manifest.SchemaVersion != 2 || manifest.SDKMajor != pluginsdk.ProtocolVersion || manifest.SDKMinor != 0 {
		return platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("unsupported plugin manifest schema/SDK"))
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
	if !manifest.Pure || len(manifest.RequiredCoreFields) != 0 || len(manifest.RequiredOperations) != 0 || len(manifest.HostCapabilities) != 0 {
		return capabilityUnavailable(errors.New("installed command requires unavailable capabilities"))
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
	return validateConfigurationPolicy(manifest.ConfigurationSchema, manifest.ConfigurationPolicy, 1)
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
// executable. It does not start the plugin, grant authority or cache a directory scan.
func (h *PluginHost) LoadInstalled(ctx context.Context, root *os.Root, manifestPath string, pin ArtifactPin) (*InstalledArtifact, error) {
	if h == nil || ctx == nil || root == nil {
		return nil, invalid(errors.New("missing plugin host/root/context"))
	}
	encoded, err := readArtifactFile(ctx, root, manifestPath, MaxInputBytes)
	if err != nil {
		return nil, err
	}
	var manifest InstalledManifest
	if err := Decode(encoded, &manifest); err != nil {
		return nil, err
	}
	if err := validateManifest(manifest, pin); err != nil {
		return nil, err
	}
	executable := filepath.FromSlash(manifest.Artifact.Path)
	digest, err := hashExecutable(ctx, root, executable)
	if err != nil {
		return nil, err
	}
	if digest != pin.Digest {
		return nil, capabilityUnavailable(errors.New("installed artifact digest changed"))
	}
	base, err := filepath.Abs(root.Name())
	if err != nil {
		return nil, capabilityUnavailable(err)
	}
	path := filepath.Join(base, executable)
	artifact := &InstalledArtifact{manifest: manifest, host: h, path: path, dir: filepath.Dir(path)}
	if err := h.register(artifact); err != nil {
		return nil, err
	}
	return artifact, nil
}

// Close stops this artifact's plugin process after invocations drain; it does not close the host.
func (a *InstalledArtifact) Close(ctx context.Context) error {
	if a == nil || ctx == nil {
		return invalid(errors.New("missing installed artifact/context"))
	}
	a.mu.Lock()
	a.closed = true
	a.stopLocked()
	a.mu.Unlock()
	if a.host != nil {
		a.host.forget(a)
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
