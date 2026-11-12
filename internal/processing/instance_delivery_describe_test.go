package processing

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

func TestDescribeDoesNotStartPlugin(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnvFor(t, markerScript)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	description, err := host.Describe(t.Context(), instance)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Loadable(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.directory, "started.marker")); !os.IsNotExist(err) {
		t.Fatalf("describing started the plugin: %v", err)
	}
	if len(description.OutputKinds) != 3 || len(description.Inputs) != 2 || description.Inputs[1].MaxBytes != 64 ||
		len(description.Dependencies) != 2 || description.SendsPerMinute != 600 || !description.Idempotent || !description.Personal {
		t.Fatalf("wrong description: %+v", description)
	}
	description.OutputKinds[0] = "changed"
	again, _ := host.Describe(t.Context(), instance)
	if again.OutputKinds[0] != "text" {
		t.Fatal("description aliases the manifest")
	}
}

func TestDescribeRules(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	instance := env.instance("T", "")
	noKey := env.hostFor(func(c *DeliveryHostConfig) { c.Cipher = nil })
	if _, err := noKey.Describe(t.Context(), instance); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("Secret-bearing instance without a key: %v", err)
	}
	host := env.hostFor(nil)
	for name, mutate := range map[string]func(*InstanceConfiguration){
		"no target use": func(i *InstanceConfiguration) { i.TargetUse = nil },
		"invalid reference": func(i *InstanceConfiguration) {
			i.Configuration = objectOf(t, map[string]Value{"title": stringLeaf(t, "T"), "apiToken": stringLeaf(t, "plain-text-token")})
		},
		"missing Secret": func(i *InstanceConfiguration) {
			i.Configuration = objectOf(t, map[string]Value{"title": stringLeaf(t, "T")})
		},
		"personal value": func(i *InstanceConfiguration) {
			i.Configuration = objectOf(t, map[string]Value{"title": stringLeaf(t, "T"), "apiToken": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__A"), "userKey": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__B")})
		},
		"bad pin": func(i *InstanceConfiguration) { i.Main.Pin.Version = "2" },
	} {
		changed := instance
		mutate(&changed)
		if _, err := host.Describe(t.Context(), changed); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	noRoot := env.hostFor(func(c *DeliveryHostConfig) { c.Root = nil })
	if _, err := noRoot.Describe(t.Context(), instance); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("no plugin directory: %v", err)
	}
}

// writeProcessingManifest provisions a processing manifest for the same executable.
func writeProcessingManifest(t *testing.T, env *deliveryEnv, name string) {
	t.Helper()
	manifest, _ := deliveryManifestFixture()
	manifest.Capability, manifest.Delivery, manifest.Pure, manifest.SDKMinor = "", nil, true, 0
	manifest.Artifact.Digest = env.pin.Digest
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.root.WriteFile(name, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadableFiltersAndCaches(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	good := env.instance("Good", "")
	disabled := env.instance("Disabled", "")
	disabled.Enabled = false
	env.stores.current[len(env.stores.current)-1] = disabled
	processing := env.instance("Processing", "")
	writeProcessingManifest(t, env, "processing.json")
	processing.Main.ManifestPath = "processing.json"
	env.stores.current[len(env.stores.current)-1] = processing
	broken := env.instance("Broken", "")
	broken.Main.ManifestPath = "missing.json"
	env.stores.current[len(env.stores.current)-1] = broken
	host := env.hostFor(nil)
	ids, err := host.Loadable(t.Context())
	if err != nil || len(ids) != 1 || ids[0] != good.ID {
		t.Fatalf("loadable %v %v", ids, err)
	}
	transactions := env.stores.transacts
	extra := env.instance("Late", "")
	env.now = env.now.Add(4 * time.Second)
	if ids, _ = host.Loadable(t.Context()); len(ids) != 1 || env.stores.transacts != transactions {
		t.Fatalf("cache not used within 5s: %v", ids)
	}
	ids[0] = uuid.Nil
	env.now = env.now.Add(2 * time.Second)
	ids, err = host.Loadable(t.Context())
	if err != nil || len(ids) != 2 || ids[0] != good.ID || ids[1] != extra.ID || env.stores.transacts == transactions {
		t.Fatalf("cache not refreshed after 5s: %v %v", ids, err)
	}
	empty := env.hostFor(func(c *DeliveryHostConfig) { c.Root = nil })
	before := env.stores.transacts
	if ids, err := empty.Loadable(t.Context()); err != nil || ids == nil || len(ids) != 0 || env.stores.transacts != before {
		t.Fatalf("no plugin directory: %v %v", ids, err)
	}
}

func TestPluginStatus(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	subscriber := uuid.New()
	if status := env.host.Status(instance.ID); status.Running || status.LastStartedAt != nil || status.LastFailureAt != nil {
		t.Fatalf("fresh status %+v", status)
	}
	if _, err := host.Attempt(t.Context(), env.request(instance, subscriber, "accept")); err != nil {
		t.Fatal(err)
	}
	status := env.host.Status(instance.ID)
	if !status.Running || status.LastStartedAt == nil || status.LastFailureAt != nil || status.LastFailureCode != "" {
		t.Fatalf("after start %+v", status)
	}
	if _, err := host.Attempt(t.Context(), env.request(instance, subscriber, "crash")); err == nil {
		t.Fatal("crash returned no error")
	}
	status = env.host.Status(instance.ID)
	if status.LastFailureCode != StatusInvocationFailed || status.LastFailureAt == nil {
		t.Fatalf("after crash %+v", status)
	}
	other := env.instance("Other", "")
	if status := env.host.Status(other.ID); status.LastFailureAt != nil {
		t.Fatalf("status leaked to another instance %+v", status)
	}
}

func TestPluginStatusStartFailure(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnvFor(t, markerScript)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	outcome, err := host.Attempt(t.Context(), env.request(instance, uuid.New(), "accept"))
	if err != nil || outcome.Code != CodePluginUnavailable {
		t.Fatalf("%+v %v", outcome, err)
	}
	status := env.host.Status(instance.ID)
	if status.Running || status.LastFailureCode != StatusStartFailed {
		t.Fatalf("%+v", status)
	}
}

func TestPluginStatusChecksumMismatch(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnvFor(t, markerScript)
	host := env.hostFor(nil)
	instance := env.instance("T", "")
	if _, err := host.Describe(t.Context(), instance); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(env.directory, "plugin")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := make([]byte, info.Size())
	copy(replacement, "#!/bin/sh\nexit 1\n")
	if err := os.WriteFile(path, replacement, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	outcome, err := host.Attempt(t.Context(), env.request(instance, uuid.New(), "accept"))
	if err != nil || outcome.Code != CodePluginUnavailable {
		t.Fatalf("%+v %v", outcome, err)
	}
	if status := env.host.Status(instance.ID); status.LastFailureCode != StatusChecksumMismatch {
		t.Fatalf("%+v", status)
	}
}
