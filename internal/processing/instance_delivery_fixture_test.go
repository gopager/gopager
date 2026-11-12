package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

type fakeTx struct{ ctx context.Context }

func (t fakeTx) Context() context.Context { return t.ctx }

// fakeStores implements every storage port of the Delivery host in memory.
type fakeStores struct {
	mu                sync.Mutex
	transacts         int
	revisions         map[[2]any]InstanceConfiguration
	current           []InstanceConfiguration
	secrets           map[uuid.UUID]StoredSecret
	personal          map[[2]uuid.UUID]PersonalConfiguration
	readSecretCalls   int
	readPersonalCalls int
}

func newFakeStores() *fakeStores {
	return &fakeStores{revisions: map[[2]any]InstanceConfiguration{}, secrets: map[uuid.UUID]StoredSecret{},
		personal: map[[2]uuid.UUID]PersonalConfiguration{}}
}

func (f *fakeStores) WithinTransaction(ctx context.Context, fn platform.TxFunc) error {
	f.mu.Lock()
	f.transacts++
	f.mu.Unlock()
	return fn(fakeTx{ctx: ctx})
}

func (f *fakeStores) put(instance InstanceConfiguration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.revisions[[2]any{instance.ID, instance.Revision}] = instance
	f.current = append(f.current, instance)
}

func (f *fakeStores) ReadInstance(_ context.Context, _ platform.TransactionContext, id uuid.UUID, revision int64) (InstanceConfiguration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	instance, found := f.revisions[[2]any{id, revision}]
	if !found {
		return InstanceConfiguration{}, authz.ErrNotFound
	}
	return instance, nil
}

func (f *fakeStores) ListCurrentInstances(context.Context, platform.TransactionContext) ([]InstanceConfiguration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]InstanceConfiguration(nil), f.current...), nil
}

func (f *fakeStores) ReadSecrets(_ context.Context, _ platform.TransactionContext, ids []uuid.UUID) (map[uuid.UUID]StoredSecret, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readSecretCalls++
	rows := map[uuid.UUID]StoredSecret{}
	for _, id := range ids {
		if row, found := f.secrets[id]; found {
			rows[id] = row
		}
	}
	return rows, nil
}

func (f *fakeStores) ReadPersonalConfiguration(_ context.Context, _ platform.TransactionContext, instanceID, ownerID uuid.UUID) (PersonalConfiguration, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readPersonalCalls++
	personal, found := f.personal[[2]uuid.UUID{instanceID, ownerID}]
	if !found {
		return PersonalConfiguration{}, authz.ErrNotFound
	}
	return personal, nil
}

// deliveryEnv is a provisioned delivery plugin directory with in-memory stores.
type deliveryEnv struct {
	t         *testing.T
	host      *PluginHost
	root      *os.Root
	directory string
	pin       ArtifactPin
	stores    *fakeStores
	cipher    *SecretCipher
	files     string
	filesRoot *os.Root
	env       map[string]string
	now       time.Time
}

// deliveryManifestBytes renders the shared manifest template for an executable digest.
func deliveryManifestBytes(t *testing.T, digest string) []byte {
	t.Helper()
	template, err := os.ReadFile(filepath.Join("testdata", "delivery", "manifest.template.json"))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.ReplaceAll(string(template), "{{digest}}", digest))
}

// newDeliveryEnvFor provisions executable as the delivery plugin.
func newDeliveryEnvFor(t *testing.T, executable []byte) *deliveryEnv {
	t.Helper()
	host := NewPluginHost()
	t.Cleanup(func() { _ = host.Close(context.Background()) })
	directory, root, _, _ := artifactFixture(t, executable)
	sum := sha256.Sum256(executable)
	digest := hex.EncodeToString(sum[:])
	if err := root.WriteFile("manifest.json", deliveryManifestBytes(t, digest), 0o600); err != nil {
		t.Fatal(err)
	}
	filesDir := t.TempDir()
	filesRoot, err := os.OpenRoot(filesDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = filesRoot.Close() })
	return &deliveryEnv{t: t, host: host, root: root, directory: directory, stores: newFakeStores(),
		pin:    ArtifactPin{PluginID: "test-delivery", Version: "1", CapabilityID: "send", Digest: digest},
		cipher: testCipher(t, 5), files: filesDir, filesRoot: filesRoot, env: map[string]string{},
		now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func newDeliveryEnv(t *testing.T) *deliveryEnv {
	t.Helper()
	return newDeliveryEnvFor(t, pluginfixture.Binary(t, "delivery"))
}

// hostFor builds a DeliveryHost; mutate adjusts the configuration.
func (e *deliveryEnv) hostFor(mutate func(*DeliveryHostConfig)) *DeliveryHost {
	e.t.Helper()
	cfg := DeliveryHostConfig{Host: e.host, Root: e.root, Transactions: e.stores, Instances: e.stores, Secrets: e.stores,
		Personal: e.stores, Cipher: e.cipher, SecretFiles: e.filesRoot,
		LookupEnv: func(name string) (string, bool) { value, ok := e.env[name]; return value, ok },
		Clock:     func() time.Time { return e.now }}
	if mutate != nil {
		mutate(&cfg)
	}
	host, err := NewDeliveryHost(cfg)
	if err != nil {
		e.t.Fatal(err)
	}
	return host
}

func stringLeaf(t *testing.T, text string) Value {
	t.Helper()
	value, err := Literal(StringValue, text)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func objectOf(t *testing.T, members map[string]Value) Value {
	t.Helper()
	value, err := encodeObject(members)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// sealed stores a Secret row and returns its reference string.
func (e *deliveryEnv) sealed(instance uuid.UUID, owner *uuid.UUID, plaintext string) string {
	e.t.Helper()
	id := uuid.New()
	row, err := e.cipher.Seal(id, instance, owner, []byte(plaintext))
	if err != nil {
		e.t.Fatal(err)
	}
	e.stores.secrets[id] = row
	return "secret:" + id.String()
}

// instance stores and returns a delivery instance revision with the given title and token reference.
func (e *deliveryEnv) instance(title, tokenReference string) InstanceConfiguration {
	e.t.Helper()
	id := uuid.New()
	if tokenReference == "" {
		tokenReference = e.sealed(id, nil, "admin-token-"+title)
	}
	return e.instanceWithID(id, title, tokenReference)
}

func (e *deliveryEnv) instanceWithID(id uuid.UUID, title, tokenReference string) InstanceConfiguration {
	e.t.Helper()
	instance := InstanceConfiguration{ID: id, Revision: 1, Enabled: true, ExecutionPrincipalID: uuid.New(),
		Main: ProvisionedArtifact{ManifestPath: "manifest.json", Pin: e.pin}, CreatedAt: e.now, CreatedBy: uuid.New(),
		Configuration: objectOf(e.t, map[string]Value{"title": stringLeaf(e.t, title), "apiToken": stringLeaf(e.t, tokenReference)}),
		TargetUse:     &TargetUse{Everyone: true}}
	e.stores.put(instance)
	return instance
}

// setPersonal stores a subscriber's personal values.
func (e *deliveryEnv) setPersonal(instance, owner uuid.UUID, members map[string]Value) {
	e.stores.personal[[2]uuid.UUID{instance, owner}] = PersonalConfiguration{InstanceID: instance, OwnerID: owner,
		Revision: 1, Configuration: objectOf(e.t, members), UpdatedAt: e.now}
}

// request builds an attempt for the instance with the given body.
func (e *deliveryEnv) request(instance InstanceConfiguration, subscriber uuid.UUID, body string) DeliveryAttemptRequest {
	return DeliveryAttemptRequest{InstanceID: instance.ID, InstanceRevision: instance.Revision, DeliveryID: uuid.New(),
		AttemptID: uuid.New(), AttemptNumber: 1, SubscriberID: subscriber, CausationID: uuid.New(), OutputKind: "text",
		Body: body, Deadline: time.Now().Add(20 * time.Second)}
}

func mustJSON(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("receipt %q: %v", raw, err)
	}
	return decoded
}
