package plugintest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/processing"
)

// DigestPlaceholder is replaced in the manifest template by the SHA-256 of the executable.
const DigestPlaceholder = "@DIGEST@"

// Options configure a Harness.
type Options struct {
	// Executable is the path of the built plugin, for example from BuildPlugin; required.
	Executable string
	// ManifestTemplate is the manifest with DigestPlaceholder ("@DIGEST@") where the executable's
	// digest belongs. It wins over ManifestTemplatePath.
	ManifestTemplate []byte
	// ManifestTemplatePath names a file holding the manifest template.
	ManifestTemplatePath string
	// TargetUse says who may use the instance as a Delivery target; nil admits everyone.
	TargetUse *processing.TargetUse
}

// Harness runs one Delivery plugin through the Core's plugin host and Delivery host with
// in-memory stores. Create it with New or Open; it releases everything when the test ends.
// Methods that cannot report an error fail the test instead.
type Harness struct {
	t          testing.TB
	manifest   processing.InstalledManifest
	manifestAt string
	pin        processing.ArtifactPin
	host       *processing.PluginHost
	delivery   *processing.DeliveryHost
	stores     *memoryStores
	cipher     *processing.SecretCipher
	secretDir  string
	instanceID uuid.UUID
	subscriber uuid.UUID
	targetUse  processing.TargetUse
	now        time.Time

	mu       sync.Mutex
	env      map[string]string
	revision int64
}

// templateIdentity is the part of the manifest the harness needs to lay out the plugin directory.
type templateIdentity struct {
	PluginID     string `json:"pluginId"`
	Version      string `json:"version"`
	CapabilityID string `json:"capabilityId"`
	Artifact     struct {
		Path string `json:"path"`
	} `json:"artifact"`
}

// New is Open that fails the test with the Core's error when the manifest or the executable is
// rejected.
func New(t testing.TB, options Options) *Harness {
	t.Helper()
	h, err := Open(t, options)
	if err != nil {
		t.Fatalf("plugintest: %v", err)
	}
	return h
}

// Open writes the plugin directory, validates the manifest with the Core's rules and prepares the
// hosts. The returned error is the Core's own, so errors.Is(err, processing.ErrCapabilityUndeclared)
// works for a manifest that asks for operations a Delivery plugin cannot have.
func Open(t testing.TB, options Options) (*Harness, error) {
	t.Helper()
	executable, err := os.ReadFile(options.Executable)
	if err != nil {
		return nil, fmt.Errorf("read executable: %w", err)
	}
	sum := sha256.Sum256(executable)
	digest := hex.EncodeToString(sum[:])
	template := options.ManifestTemplate
	if template == nil {
		if template, err = os.ReadFile(options.ManifestTemplatePath); err != nil {
			return nil, fmt.Errorf("read manifest template: %w", err)
		}
	}
	manifestBytes := bytes.ReplaceAll(template, []byte(DigestPlaceholder), []byte(digest))
	var identity templateIdentity
	if err := json.Unmarshal(manifestBytes, &identity); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	h := &Harness{t: t, stores: newMemoryStores(), instanceID: uuid.New(), subscriber: uuid.New(),
		targetUse: processing.TargetUse{Everyone: true}, env: map[string]string{},
		now: time.Now().UTC().Truncate(time.Microsecond),
		pin: processing.ArtifactPin{PluginID: identity.PluginID, Version: identity.Version,
			CapabilityID: identity.CapabilityID, Digest: digest}}
	if options.TargetUse != nil {
		h.targetUse = *options.TargetUse
	}
	root, err := h.install(executable, manifestBytes, identity.Artifact.Path)
	if err != nil {
		return nil, err
	}
	if err := h.start(root); err != nil {
		return nil, err
	}
	return h, nil
}

// install writes the executable and manifest.json at the places the manifest names and opens the
// directory as the artifact root.
func (h *Harness) install(executable, manifest []byte, artifactPath string) (*os.Root, error) {
	directory := h.t.TempDir()
	target := filepath.Join(directory, filepath.FromSlash(artifactPath))
	if !filepath.IsLocal(filepath.FromSlash(artifactPath)) {
		return nil, fmt.Errorf("manifest artifact path %q leaves the plugin directory", artifactPath)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return nil, err
	}
	h.manifestAt = path.Join(path.Dir(artifactPath), "manifest.json")
	if err := os.WriteFile(target, executable, 0o700); err != nil { //nolint:gosec // the plugin must be executable
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(directory, filepath.FromSlash(h.manifestAt)), manifest, 0o600); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	h.t.Cleanup(func() { _ = root.Close() })
	return root, nil
}

// start builds the Core's hosts around the plugin directory and loads the manifest once, which
// applies the Core's manifest and executable checks.
func (h *Harness) start(root *os.Root) error {
	h.host = processing.NewPluginHost()
	h.t.Cleanup(func() { _ = h.host.Close(context.Background()) })
	artifact, err := h.host.LoadInstalled(context.Background(), root, h.manifestAt, h.pin)
	if err != nil {
		return err
	}
	defer func() { _ = artifact.Close(context.Background()) }()
	if h.manifest, err = artifact.Manifest(); err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	if h.cipher, err = processing.NewSecretCipher(key); err != nil {
		return err
	}
	h.secretDir = h.t.TempDir()
	secretFiles, err := os.OpenRoot(h.secretDir)
	if err != nil {
		return err
	}
	h.t.Cleanup(func() { _ = secretFiles.Close() })
	h.delivery, err = processing.NewDeliveryHost(processing.DeliveryHostConfig{Host: h.host, Root: root,
		Transactions: h.stores, Instances: h.stores, Secrets: h.stores, Personal: h.stores, Cipher: h.cipher,
		SecretFiles: secretFiles, LookupEnv: h.lookupEnv})
	return err
}

// Manifest returns the manifest as the Core validated and decoded it, with the real digest.
func (h *Harness) Manifest() processing.InstalledManifest { return h.manifest }

// InstanceID returns the ID of the configured instance.
func (h *Harness) InstanceID() uuid.UUID { return h.instanceID }

// Revision returns the current instance configuration revision; 0 before the first Configure.
func (h *Harness) Revision() int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.revision
}

// Subscriber returns the subscriber that Attempt uses when AttemptInput names none.
func (h *Harness) Subscriber() uuid.UUID { return h.subscriber }

// NewSubscriber returns a fresh subscriber ID.
func (h *Harness) NewSubscriber() uuid.UUID { return uuid.New() }

// SetEnv defines an environment variable that SecretEnv references can read. The name is the part
// after the plugin's namespace, see SecretEnv.
func (h *Harness) SetEnv(name, value string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.env[h.secretEnvName(name)] = value
}

// secretEnvName is the full name of a variable in the plugin's namespace.
func (h *Harness) secretEnvName(name string) string {
	return processing.SecretEnvPluginPrefix(h.pin.PluginID) + name
}

// secretFilePath is the path below the Secret file root of a file in the plugin's directory.
func (h *Harness) secretFilePath(name string) string {
	return h.pin.PluginID + "/" + name
}

// lookupEnv is the injected environment of the Delivery host.
func (h *Harness) lookupEnv(name string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	value, found := h.env[name]
	return value, found
}

// WriteSecretFile creates or replaces a file in the plugin's directory below the Secret file root,
// which SecretFile references name relative to it. Content is written as given, so include a trailing newline to
// mimic an editor.
func (h *Harness) WriteSecretFile(name, content string) {
	h.t.Helper()
	target := filepath.Join(h.secretDir, filepath.FromSlash(h.secretFilePath(name)))
	if !filepath.IsLocal(filepath.FromSlash(name)) {
		h.t.Fatalf("plugintest: secret file %q leaves the secret file root", name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		h.t.Fatalf("plugintest: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		h.t.Fatalf("plugintest: %v", err)
	}
}
