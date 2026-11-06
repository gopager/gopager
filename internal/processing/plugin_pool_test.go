package processing

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

// poolFixture is one provisioned command plugin that tests load repeatedly, like successive runs.
type poolFixture struct {
	t         *testing.T
	host      *PluginHost
	root      *os.Root
	directory string
	pin       ArtifactPin
}

func newPoolFixture(t *testing.T) *poolFixture {
	t.Helper()
	host := NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	directory, root, manifest, pin := artifactFixture(t, pluginfixture.Binary(t, "command"))
	schema := map[string]FieldSchema{"mode": {Kind: StringValue}, "text": {Kind: StringValue}}
	manifest.InputSchema, manifest.ResultSchema = schema, schema
	writeManifest(t, root, manifest)
	return &poolFixture{t: t, host: host, root: root, directory: directory, pin: pin}
}

// load returns a handle for the instance revision, as one run's preparation would.
func (f *poolFixture) load(instance uuid.UUID, revision int64) *InstalledArtifact {
	f.t.Helper()
	artifact, err := f.host.LoadInstalledFor(context.Background(), f.root, "manifest.json", f.pin, instance, revision)
	if err != nil {
		f.t.Fatal(err)
	}
	return artifact
}

// pid asks the fixture plugin for its operating system process ID.
func (f *poolFixture) pid(artifact *InstalledArtifact) (int, error) {
	f.t.Helper()
	mode, err := Literal(StringValue, "pid")
	if err != nil {
		f.t.Fatal(err)
	}
	envelope := installedEnvelope(f.t, f.pin, map[string]Value{"mode": mode, "text": mode})
	fields, err := artifact.invokeProjected(context.Background(), envelope)
	if err != nil {
		return 0, err
	}
	var text string
	if err := Decode(fields["text"].Value, &text); err != nil {
		f.t.Fatal(err)
	}
	return strconv.Atoi(text)
}

func (f *poolFixture) mustPID(artifact *InstalledArtifact) int {
	f.t.Helper()
	pid, err := f.pid(artifact)
	if err != nil {
		f.t.Fatalf("invocation failed: %v", err)
	}
	return pid
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPluginPoolReusesOneProcessAcrossRuns(t *testing.T) {
	f := newPoolFixture(t)
	instance := uuid.New()
	first := f.load(instance, 1)
	pid := f.mustPID(first)
	if again := f.mustPID(first); again != pid {
		t.Fatalf("second invocation started a new process: %d != %d", again, pid)
	}
	if err := first.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	second := f.load(instance, 1)
	defer func() { _ = second.Close(context.Background()) }()
	if next := f.mustPID(second); next != pid {
		t.Fatalf("a later run did not reuse the warm process: %d != %d", next, pid)
	}
	other := f.load(uuid.New(), 1)
	defer func() { _ = other.Close(context.Background()) }()
	if next := f.mustPID(other); next == pid {
		t.Fatal("two instances shared one process")
	}
}

func TestPluginPoolSerializesConcurrentInvocationsOnOneProcess(t *testing.T) {
	f := newPoolFixture(t)
	instance := uuid.New()
	var group sync.WaitGroup
	pids := make([]int, 8)
	for i := range pids {
		group.Add(1)
		go func() {
			defer group.Done()
			artifact := f.load(instance, 1)
			defer func() { _ = artifact.Close(context.Background()) }()
			pid, err := f.pid(artifact)
			if err != nil {
				t.Errorf("concurrent invocation failed: %v", err)
			}
			pids[i] = pid
		}()
	}
	group.Wait()
	for _, pid := range pids {
		if pid != pids[0] {
			t.Fatalf("concurrent runs used different processes: %v", pids)
		}
	}
}

func TestPluginPoolIdleTimeoutStopsProcess(t *testing.T) {
	original := pluginIdleTimeout
	pluginIdleTimeout = 150 * time.Millisecond
	t.Cleanup(func() { pluginIdleTimeout = original })
	f := newPoolFixture(t)
	artifact := f.load(uuid.New(), 1)
	defer func() { _ = artifact.Close(context.Background()) }()
	pid := f.mustPID(artifact)
	if !artifact.process.running() {
		t.Fatal("process is not running right after an invocation")
	}
	waitFor(t, "idle timeout to stop the process", func() bool { return !artifact.process.running() })
	if next := f.mustPID(artifact); next == pid {
		t.Fatal("the process was not restarted after it idled out")
	}
}

func TestPluginPoolBusyProcessIsNotStoppedByIdleTimeout(t *testing.T) {
	original := pluginIdleTimeout
	pluginIdleTimeout = 50 * time.Millisecond
	t.Cleanup(func() { pluginIdleTimeout = original })
	f := newPoolFixture(t)
	artifact := f.load(uuid.New(), 1)
	defer func() { _ = artifact.Close(context.Background()) }()
	pid := f.mustPID(artifact)
	for range 5 {
		time.Sleep(20 * time.Millisecond)
		if next := f.mustPID(artifact); next != pid {
			t.Fatalf("a process in regular use was restarted: %d != %d", next, pid)
		}
	}
}

func TestPluginPoolInstanceChangedStopsOlderRevisions(t *testing.T) {
	f := newPoolFixture(t)
	instance := uuid.New()
	artifact := f.load(instance, 1)
	defer func() { _ = artifact.Close(context.Background()) }()
	pid := f.mustPID(artifact)
	f.host.InstanceChanged(instance, 1)
	f.host.InstanceChanged(uuid.New(), 5)
	if !artifact.process.running() || f.mustPID(artifact) != pid {
		t.Fatal("a signal for the same revision or another instance stopped the process")
	}
	f.host.InstanceChanged(instance, 2)
	if artifact.process.running() {
		t.Fatal("InstanceChanged did not stop the older revision's process")
	}
	if _, err := f.pid(artifact); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("a stale revision's process was resurrected: %v", err)
	}
	current := f.load(instance, 2)
	defer func() { _ = current.Close(context.Background()) }()
	if next := f.mustPID(current); next == pid {
		t.Fatal("the new revision reused the stopped process")
	}
}

func TestPluginPoolHostCloseStopsPooledAndUnpooledProcesses(t *testing.T) {
	f := newPoolFixture(t)
	pooled := f.load(uuid.New(), 1)
	unpooled := f.load(uuid.Nil, 0)
	f.mustPID(pooled)
	f.mustPID(unpooled)
	if err := f.host.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if pooled.process.running() || unpooled.process.running() {
		t.Fatal("host Close left a plugin process running")
	}
	if _, err := f.host.LoadInstalledFor(t.Context(), f.root, "manifest.json", f.pin, uuid.New(), 1); err == nil {
		t.Fatal("a closed host loaded an artifact")
	}
}

func TestPluginPoolClosingIdleHandlesReleasesProcesses(t *testing.T) {
	f := newPoolFixture(t)
	unpooled := f.load(uuid.Nil, 0)
	f.mustPID(unpooled)
	process := unpooled.process
	if err := unpooled.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if process.running() {
		t.Fatal("closing the last handle of an unpooled artifact left its process running")
	}
	unused := f.load(uuid.New(), 1)
	if err := unused.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.host.mu.Lock()
	defer f.host.mu.Unlock()
	if len(f.host.processes) != 0 || len(f.host.unpooled) != 0 {
		t.Fatalf("pool kept entries for handles that never started a process: %d pooled, %d unpooled",
			len(f.host.processes), len(f.host.unpooled))
	}
}

func TestPluginSecureConfigRefusesTamperedBinary(t *testing.T) {
	f := newPoolFixture(t)
	artifact := f.load(uuid.New(), 1)
	defer func() { _ = artifact.Close(context.Background()) }()
	// The replacement would record its start; SecureConfig must refuse it before it runs.
	if err := f.root.WriteFile("plugin", markerScript, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pid(artifact); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("tampered binary was not refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.directory, "started.marker")); !os.IsNotExist(err) {
		t.Fatalf("tampered binary was executed: %v", err)
	}
	waitFor(t, "digest mismatch to evict the cache entry", func() bool {
		f.host.mu.Lock()
		defer f.host.mu.Unlock()
		return len(f.host.cache) == 0
	})
	if _, err := f.host.LoadInstalledFor(t.Context(), f.root, "manifest.json", f.pin, uuid.New(), 1); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("tampered binary was loaded: %v", err)
	}
}

func TestPluginArtifactCacheSkipsRehashAndDetectsChange(t *testing.T) {
	f := newPoolFixture(t)
	instance := uuid.New()
	first := f.load(instance, 1)
	defer func() { _ = first.Close(context.Background()) }()
	second := f.load(instance, 1)
	defer func() { _ = second.Close(context.Background()) }()
	f.host.mu.Lock()
	cached := len(f.host.cache)
	f.host.mu.Unlock()
	if cached != 1 {
		t.Fatalf("loads of one digest created %d cache entries", cached)
	}
	// Same size but different bytes and a newer modification time must be rehashed and refused.
	data, err := f.root.ReadFile("plugin")
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 0xff
	if err := f.root.WriteFile("plugin", data, 0o755); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(f.directory, "plugin"), later, later); err != nil {
		t.Fatal(err)
	}
	if _, err := f.host.LoadInstalledFor(t.Context(), f.root, "manifest.json", f.pin, instance, 1); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("changed executable was served from the cache: %v", err)
	}
}

func TestPluginPoolInstanceChangedDropsCachedArtifacts(t *testing.T) {
	f := newPoolFixture(t)
	instance := uuid.New()
	artifact := f.load(instance, 1)
	defer func() { _ = artifact.Close(context.Background()) }()
	f.host.InstanceChanged(instance, 2)
	f.host.mu.Lock()
	defer f.host.mu.Unlock()
	if len(f.host.cache) != 0 {
		t.Fatal("InstanceChanged kept the instance's cached artifact")
	}
}

func TestInstalledPreviewModeReachesPlugin(t *testing.T) {
	f := newPoolFixture(t)
	artifact := f.load(uuid.New(), 1)
	defer func() { _ = artifact.Close(context.Background()) }()
	mode, err := Literal(StringValue, "fail")
	if err != nil {
		t.Fatal(err)
	}
	text, err := Literal(StringValue, "hello")
	if err != nil {
		t.Fatal(err)
	}
	envelope := installedEnvelope(t, f.pin, map[string]Value{"mode": mode, "text": text})
	envelope.Mode = InvocationModePreview
	fields, err := artifact.invokeProjected(t.Context(), envelope)
	if err != nil || string(fields["text"].Value) != `"PREVIEW:hello"` {
		t.Fatalf("plugin did not see mode preview: %+v, %v", fields, err)
	}
	envelope.Mode = InvocationModeLive
	if _, err := artifact.invokeProjected(t.Context(), envelope); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("live mode did not run the fixture's live failure: %v", err)
	}
	envelope.Mode = "dry"
	if _, err := artifact.invokeProjected(t.Context(), envelope); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("unknown mode was accepted: %v", err)
	}
	artifact.manifest.SupportsPreview = false
	envelope.Mode = InvocationModePreview
	if _, err := artifact.invokeProjected(t.Context(), envelope); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("a plugin without supportsPreview received a preview invocation: %v", err)
	}
}
