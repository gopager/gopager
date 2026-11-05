package processing

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"

	"github.com/gopager/gopager/pluginsdk"
)

// pluginStartTimeout bounds the go-plugin handshake of a freshly started plugin process.
const pluginStartTimeout = 10 * time.Second

// installedInvocationTimeout bounds one installed invocation, including starting its plugin process.
// It is a variable only so tests can exercise the bound without waiting for the production value.
var installedInvocationTimeout = 30 * time.Second

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

// PluginHost owns the plugin processes started for loaded artifacts (plugin-runtime 2.0).
// Each loaded artifact, and therefore each configured instance, gets its own process.
type PluginHost struct {
	mu        sync.Mutex
	closed    bool
	artifacts map[*InstalledArtifact]struct{}
}

// NewPluginHost creates an empty host; no process starts until an artifact is invoked.
func NewPluginHost() *PluginHost {
	return &PluginHost{artifacts: make(map[*InstalledArtifact]struct{})}
}

func (h *PluginHost) register(artifact *InstalledArtifact) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return capabilityUnavailable(errors.New("plugin host is closed"))
	}
	h.artifacts[artifact] = struct{}{}
	return nil
}

func (h *PluginHost) forget(artifact *InstalledArtifact) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.artifacts, artifact)
}

// Close stops every remaining plugin process; callers join runs first so this is a safety net.
func (h *PluginHost) Close(ctx context.Context) error {
	if h == nil || ctx == nil {
		return invalid(errors.New("missing plugin host/context"))
	}
	h.mu.Lock()
	h.closed = true
	remaining := make([]*InstalledArtifact, 0, len(h.artifacts))
	for artifact := range h.artifacts {
		remaining = append(remaining, artifact)
	}
	h.artifacts = map[*InstalledArtifact]struct{}{}
	h.mu.Unlock()
	for _, artifact := range remaining {
		artifact.stop()
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

// connection starts the plugin process on first use and returns its invocation stub.
func (a *InstalledArtifact) connection() (*pluginsdk.Client, *plugin.Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, nil, capabilityUnavailable(errors.New("installed artifact is closed"))
	}
	if a.client != nil && !a.client.Exited() && a.stub != nil {
		return a.stub, a.client, nil
	}
	a.stopLocked()
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  pluginsdk.Handshake,
		Plugins:          plugin.PluginSet{pluginsdk.PluginName: &pluginsdk.GRPCPlugin{}},
		Cmd:              pluginCommand(a.path, a.dir),
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
		return nil, nil, capabilityUnavailable(errors.New("plugin process did not complete its handshake"))
	}
	raw, err := protocol.Dispense(pluginsdk.PluginName)
	stub, ok := raw.(*pluginsdk.Client)
	if err != nil || !ok {
		client.Kill()
		return nil, nil, capabilityUnavailable(errors.New("plugin process does not offer the invocation service"))
	}
	a.client, a.stub = client, stub
	return stub, client, nil
}

func (a *InstalledArtifact) stopLocked() {
	if a.client != nil {
		a.client.Kill()
	}
	a.client, a.stub = nil, nil
}

// stopIf kills used only when it is still the current process, so a concurrent restart survives.
func (a *InstalledArtifact) stopIf(used *plugin.Client) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.client == used {
		a.stopLocked()
	}
}

func (a *InstalledArtifact) stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopLocked()
}

// call performs one bounded invocation. A deadline, cancellation or transport failure kills the
// process so an uncooperative plugin cannot keep running; the next call starts a new one.
// Expiry of the invocation's own deadline is RESOURCE_LIMIT; caller cancellation stays a
// cancellation and any other transport failure is INVALID_INPUT.
func (a *InstalledArtifact) call(ctx context.Context, input []byte, output any, duration time.Duration, budget *executionBudget) error {
	if a == nil || ctx == nil || output == nil || duration <= 0 {
		return invalid(errors.New("missing plugin invocation participant"))
	}
	if err := invocationErr(ctx); err != nil {
		return err
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
		stub, client, err := a.connection()
		if err != nil {
			return err
		}
		response, err = stub.Invoke(ctx, input, MaxOutputBytes)
		if err != nil {
			a.stopIf(client)
			if err := invocationErr(ctx); err != nil {
				return err
			}
			return invalid(errors.New("plugin invocation failed"))
		}
		return nil
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
