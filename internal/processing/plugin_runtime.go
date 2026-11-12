package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/gopager/gopager/pluginsdk"
)

// pluginStartTimeout bounds the go-plugin handshake of a freshly started plugin process.
const pluginStartTimeout = 10 * time.Second

// installedInvocationTimeout bounds one installed invocation, including starting its plugin process.
// It is a variable only so tests can exercise the bound without waiting for the production value.
var installedInvocationTimeout = 30 * time.Second

// pluginIdleTimeout is how long a plugin process may stay unused before the host stops it.
// Five minutes keeps a busy instance warm between workflow runs yet releases the memory of
// instances nobody triggers. It is a variable only so tests can exercise it without waiting.
var pluginIdleTimeout = 5 * time.Minute

// errInvocationDeadline is the cancellation cause recorded when an invocation's own deadline
// expires, which distinguishes it from cancellation of the caller's context.
var errInvocationDeadline = errors.New("installed invocation deadline exceeded")

// invocationErr reports why an invocation context ended: expiry of the invocation's own deadline
// is the explicit resource failure, while caller cancellation or expiry is returned unchanged.
func invocationErr(ctx context.Context) error {
	err := ctx.Err()
	if err != nil && errors.Is(context.Cause(ctx), errInvocationDeadline) {
		return resource(errInvocationDeadline)
	}
	return err
}

// pluginEnvironment lists the only Core environment variables passed to plugin processes.
// Core configuration such as GOPAGER_DATABASE_URL is never inherited.
var pluginEnvironment = []string{"PATH", "HOME", "LANG", "LC_ALL", "TZ", "TMPDIR"}

// processKey identifies one pooled plugin process: a configured instance at an exact
// configuration revision running one exact artifact. Configuration is passed per invocation, but
// a new revision may pin another artifact and must never be served by a process of an older one.
type processKey struct {
	instance uuid.UUID
	revision int64
	digest   string
	mode     string
}

// PluginHost owns the plugin processes and the verified-artifact cache of plugin-runtime 2.0.
//
// A process is started on the first invocation of a configured instance revision, reused by every
// later run, and stopped after pluginIdleTimeout without use, when the instance changes, or when
// the host closes. Artifacts loaded without an instance are unpooled: each has a private process
// that stops with its last handle.
type PluginHost struct {
	mu        sync.Mutex
	closed    bool
	processes map[processKey]*pluginProcess
	unpooled  map[*pluginProcess]struct{}
	cache     map[artifactCacheKey]*cachedArtifact

	// statusMu guards statuses, the per-instance start/failure memory behind Status. It is a leaf
	// lock so it can be taken while holding mu or a process lock.
	statusMu sync.Mutex
	statuses map[uuid.UUID]*statusRecord
}

// NewPluginHost creates an empty host; no process starts until an artifact is invoked.
func NewPluginHost() *PluginHost {
	return &PluginHost{
		processes: make(map[processKey]*pluginProcess),
		unpooled:  make(map[*pluginProcess]struct{}),
		cache:     make(map[artifactCacheKey]*cachedArtifact),
	}
}

// acquire returns the process for key, creating it on first use, and takes one reference.
// A nil instance in key requests a private, unpooled process.
func (h *PluginHost) acquire(key processKey, cached *cachedArtifact) (*pluginProcess, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil, capabilityUnavailable(errors.New("plugin host is closed"))
	}
	pooled := key.instance != uuid.Nil
	if pooled {
		if process, exists := h.processes[key]; exists {
			process.mu.Lock()
			process.refs++
			process.mu.Unlock()
			return process, nil
		}
	}
	checksum, err := hex.DecodeString(key.digest)
	if err != nil {
		return nil, invalid(err)
	}
	process := &pluginProcess{host: h, key: key, pooled: pooled, cacheKey: cached.key, path: cached.path,
		dir: cached.dir, checksum: checksum, slot: make(chan struct{}, 1), refs: 1}
	if pooled {
		h.processes[key] = process
	} else {
		h.unpooled[process] = struct{}{}
	}
	return process, nil
}

// drop removes an unreferenced process from the host so abandoned revisions do not accumulate.
func (h *PluginHost) drop(process *pluginProcess) {
	h.mu.Lock()
	defer h.mu.Unlock()
	process.mu.Lock()
	defer process.mu.Unlock()
	if process.refs > 0 && !process.retired {
		return
	}
	if h.processes[process.key] == process {
		delete(h.processes, process.key)
	}
	delete(h.unpooled, process)
}

// InstanceChanged stops the processes of every revision of the instance older than revision and
// drops the cached artifacts the instance used. Callers invoke it after a configuration commit;
// like run cancellation it is a best-effort signal, not execution authority.
func (h *PluginHost) InstanceChanged(id uuid.UUID, revision int64) {
	if h == nil {
		return
	}
	var stale []*pluginProcess
	h.mu.Lock()
	for key, process := range h.processes {
		if key.instance == id && key.revision < revision {
			delete(h.processes, key)
			stale = append(stale, process)
		}
	}
	for key, entry := range h.cache {
		if _, uses := entry.instances[id]; uses {
			delete(h.cache, key)
		}
	}
	h.mu.Unlock()
	for _, process := range stale {
		process.retire()
	}
}

// invalidate forgets a cached artifact whose bytes no longer match its pinned digest and retires
// the process that was meant to run it.
func (h *PluginHost) invalidate(process *pluginProcess) {
	h.mu.Lock()
	delete(h.cache, process.cacheKey)
	if h.processes[process.key] == process {
		delete(h.processes, process.key)
	}
	h.mu.Unlock()
	process.retire()
}

// Close stops every remaining plugin process; callers join runs first so this is a safety net.
func (h *PluginHost) Close(ctx context.Context) error {
	if h == nil || ctx == nil {
		return invalid(errors.New("missing plugin host/context"))
	}
	h.mu.Lock()
	h.closed = true
	remaining := make([]*pluginProcess, 0, len(h.processes)+len(h.unpooled))
	for _, process := range h.processes {
		remaining = append(remaining, process)
	}
	for process := range h.unpooled {
		remaining = append(remaining, process)
	}
	h.processes = map[processKey]*pluginProcess{}
	h.unpooled = map[*pluginProcess]struct{}{}
	h.cache = map[artifactCacheKey]*cachedArtifact{}
	h.mu.Unlock()
	for _, process := range remaining {
		process.retire()
	}
	return nil
}

func pluginCommand(path, dir string) *exec.Cmd {
	command := exec.Command(path)
	command.Dir = dir
	command.Env = []string{}
	for _, name := range pluginEnvironment {
		if value, ok := os.LookupEnv(name); ok {
			command.Env = append(command.Env, name+"="+value)
		}
	}
	return command
}

// pluginProcess is the long-lived child process of one pool key, shared by every InstalledArtifact
// handle that was loaded for that key.
type pluginProcess struct {
	host     *PluginHost
	key      processKey
	pooled   bool
	cacheKey artifactCacheKey
	path     string
	dir      string
	checksum []byte
	// slot serializes invocations: the plugin handler contract does not require concurrency
	// safety (pluginsdk.Serve would run each RPC on its own goroutine), so one process handles
	// one request at a time, which also bounds a misbehaving plugin to one stuck call.
	slot chan struct{}

	mu        sync.Mutex
	refs      int
	retired   bool
	client    *plugin.Client
	stub      *pluginsdk.Client
	busy      int
	lastUsed  time.Time
	idleAfter time.Duration
	idle      *time.Timer
}

// connection starts the plugin process on first use and returns its invocation stub.
// The started binary is verified against the pinned digest by go-plugin itself (SecureConfig), so
// a binary replaced after the load-time check is refused instead of executed.
func (p *pluginProcess) connection() (*pluginsdk.Client, *plugin.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.retired {
		return nil, nil, capabilityUnavailable(errors.New("plugin process was retired"))
	}
	if p.client != nil && !p.client.Exited() && p.stub != nil {
		return p.stub, p.client, nil
	}
	p.stopLocked()
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pluginsdk.Handshake,
		Plugins:          plugin.PluginSet{pluginsdk.PluginName: &pluginsdk.GRPCPlugin{}},
		Cmd:              pluginCommand(p.path, p.dir),
		SecureConfig:     &plugin.SecureConfig{Checksum: p.checksum, Hash: sha256.New()},
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
		SkipHostEnv:      true,
		StartTimeout:     pluginStartTimeout,
		Logger:           hclog.NewNullLogger(),
		SyncStdout:       io.Discard,
		SyncStderr:       io.Discard,
	})
	protocol, err := client.Client()
	if err != nil {
		client.Kill()
		if errors.Is(err, plugin.ErrChecksumsDoNotMatch) {
			// The host lock precedes the process lock, so invalidate runs after this call returns.
			go p.host.invalidate(p)
			p.host.recordFailure(p.key.instance, StatusChecksumMismatch)
			return nil, nil, capabilityUnavailable(errors.New("plugin executable differs from its pinned digest"))
		}
		p.host.recordFailure(p.key.instance, StatusStartFailed)
		return nil, nil, capabilityUnavailable(errors.New("plugin process did not complete its handshake"))
	}
	raw, err := protocol.Dispense(pluginsdk.PluginName)
	stub, ok := raw.(*pluginsdk.Client)
	if err != nil || !ok {
		client.Kill()
		p.host.recordFailure(p.key.instance, StatusStartFailed)
		return nil, nil, capabilityUnavailable(errors.New("plugin process does not offer the invocation service"))
	}
	p.client, p.stub = client, stub
	p.host.recordStarted(p.key.instance)
	return stub, client, nil
}

func (p *pluginProcess) stopLocked() {
	if p.idle != nil {
		p.idle.Stop()
		p.idle = nil
	}
	if p.client != nil {
		p.client.Kill()
	}
	p.client, p.stub = nil, nil
}

// stopIf kills used only when it is still the current process, so a concurrent restart survives.
func (p *pluginProcess) stopIf(used *plugin.Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == used {
		p.stopLocked()
	}
}

// retire stops the process for good; later invocations are refused instead of restarting it.
func (p *pluginProcess) retire() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.retired = true
	p.stopLocked()
}

// release drops one handle reference. An unpooled process stops with its last handle; a pooled one
// stays warm for the next run until it idles out or its instance changes, unless it never started,
// in which case there is nothing to keep warm and the pool entry is dropped.
func (p *pluginProcess) release() {
	p.mu.Lock()
	p.refs--
	unreferenced := p.refs <= 0
	if unreferenced && !p.pooled {
		p.retired = true
		p.stopLocked()
	}
	removable := unreferenced && (!p.pooled || p.client == nil)
	p.mu.Unlock()
	if removable {
		p.host.drop(p)
	}
}

// beginUse marks the process busy so the idle timer cannot stop it under a running call.
func (p *pluginProcess) beginUse() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.busy++
	if p.idle != nil {
		p.idle.Stop()
		p.idle = nil
	}
}

// endUse records the end of a call and arms the idle timer once the process is unused.
func (p *pluginProcess) endUse() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.busy--
	p.lastUsed = time.Now()
	if p.busy > 0 || p.retired || p.client == nil {
		return
	}
	p.idleAfter = pluginIdleTimeout
	p.idle = time.AfterFunc(p.idleAfter, p.idleExpired)
}

// idleExpired stops a process that stayed unused for the whole idle period. The lastUsed check
// discards a timer that fired concurrently with a newer call.
func (p *pluginProcess) idleExpired() {
	p.mu.Lock()
	if p.busy > 0 || p.retired || time.Since(p.lastUsed) < p.idleAfter {
		p.mu.Unlock()
		return
	}
	p.stopLocked()
	unreferenced := p.refs <= 0
	p.mu.Unlock()
	if unreferenced {
		p.host.drop(p)
	}
}

// running reports whether a plugin child process is currently alive.
func (p *pluginProcess) running() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client != nil && !p.client.Exited()
}

// unsentError marks a failure that happened before the request reached the plugin, so the plugin
// cannot have acted on it. Its text and chain are those of the wrapped error.
type unsentError struct{ cause error }

func (e *unsentError) Error() string { return e.cause.Error() }
func (e *unsentError) Unwrap() error { return e.cause }

// isUnsent reports whether err happened before the request was sent to the plugin.
func isUnsent(err error) bool {
	var unsent *unsentError
	return errors.As(err, &unsent)
}

// invoke performs one request on the process, one at a time. A transport failure or an expired
// deadline kills the process so an uncooperative plugin cannot keep running; the next call starts
// a new one. Expiry of the invocation's own deadline is RESOURCE_LIMIT; caller cancellation stays
// a cancellation and any other transport failure is INVALID_INPUT. A failure before the request
// was sent (waiting for the call slot, starting the process) is marked with isUnsent.
func (p *pluginProcess) invoke(ctx context.Context, input []byte) ([]byte, error) {
	select {
	case p.slot <- struct{}{}:
	case <-ctx.Done():
		return nil, &unsentError{cause: invocationErr(ctx)}
	}
	defer func() { <-p.slot }()
	p.beginUse()
	defer p.endUse()
	stub, client, err := p.connection()
	if err != nil {
		return nil, &unsentError{cause: err}
	}
	response, err := stub.Invoke(ctx, input, MaxOutputBytes)
	if err != nil {
		p.stopIf(client)
		failure := invokeFailure(ctx, err)
		p.host.recordFailure(p.key.instance, invocationFailureCode(failure))
		return nil, failure
	}
	return response, nil
}

// remoteDeadlineSkew is how close to the caller's deadline a deadline or cancellation reported by
// the plugin side counts as that same deadline rather than as the plugin's own failure.
const remoteDeadlineSkew = 250 * time.Millisecond

// invokeFailure classifies a failed plugin invocation. gRPC hands the caller's deadline to the
// plugin process, whose side can expire a moment before the caller's context does and report
// DeadlineExceeded (or Canceled) first; within remoteDeadlineSkew of the caller's deadline the
// caller waits the remaining moment for its own context and reports the deadline. Earlier, such a
// status comes from the plugin itself (for example a shorter internal operation the SDK passes
// through), so it is the plugin's failure and returns at once, freeing the process slot.
func invokeFailure(ctx context.Context, err error) error {
	if err := invocationErr(ctx); err != nil {
		return err
	}
	if code := status.Code(err); code == codes.DeadlineExceeded || code == codes.Canceled {
		if deadline, bounded := ctx.Deadline(); bounded && time.Until(deadline) <= remoteDeadlineSkew {
			<-ctx.Done()
			return invocationErr(ctx)
		}
	}
	return invalid(errors.New("plugin invocation failed"))
}

// call performs one bounded invocation on the artifact's process.
func (a *InstalledArtifact) call(ctx context.Context, input []byte, output any, duration time.Duration, budget *executionBudget) error {
	if a == nil || ctx == nil || output == nil || duration <= 0 {
		return invalid(errors.New("missing plugin invocation participant"))
	}
	if err := invocationErr(ctx); err != nil {
		return err
	}
	if a.isClosed() {
		return capabilityUnavailable(errors.New("installed artifact is closed"))
	}
	if len(input) > MaxInputBytes {
		return resource(errors.New("plugin encoded input boundary"))
	}
	if !utf8.Valid(input) {
		return invalid(errors.New("plugin input is not UTF-8"))
	}
	ctx, cancel := context.WithTimeoutCause(ctx, duration, errInvocationDeadline)
	defer cancel()
	var response []byte
	invoke := func(context.Context) error {
		var err error
		response, err = a.process.invoke(ctx, input)
		return err
	}
	var err error
	if budget == nil {
		err = invoke(ctx)
	} else {
		err = budget.guestWait(invoke)
	}
	if err != nil {
		return err
	}
	if err := invocationErr(ctx); err != nil {
		return err
	}
	if budget != nil {
		if err := budget.computeContext().Err(); err != nil {
			return budget.bound(err)
		}
	}
	if len(response) > MaxOutputBytes {
		return resource(errors.New("plugin output boundary"))
	}
	if !utf8.Valid(response) {
		return invalid(errors.New("plugin output is not UTF-8"))
	}
	return Decode(response, output)
}
