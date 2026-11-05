package processing

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

type boundedOutput struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *boundedOutput) Write(data []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if len(data) > w.limit-w.Len() {
		w.err = resource(errors.New("WASI output boundary"))
		return 0, w.err
	}
	return w.Buffer.Write(data)
}

// WASMHost executes isolated data-in/data-out commands with no filesystem or network mounts.
// The fixed guest memory bound is not an OS-wide Go host-memory guarantee.
type WASMHost struct {
	runtime wazero.Runtime
	close   sync.Once
	err     error
}

// compilation shares compiled guest code between hosts so each module is compiled once per process.
var compilation struct {
	sync.Mutex
	cache wazero.CompilationCache
}

func compilationCache() wazero.CompilationCache {
	compilation.Lock()
	defer compilation.Unlock()
	if compilation.cache == nil {
		compilation.cache = wazero.NewCompilationCache()
	}
	return compilation.cache
}

// UseCompilationCacheDir persists compiled guest code in dir for every host created afterwards.
// Cache entries are trusted native code, so dir must be writable only by the operator; production
// composition keeps the in-memory default. It fails once a host has already used the cache.
func UseCompilationCacheDir(dir string) error {
	if dir == "" {
		return invalid(errors.New("missing WASI compilation cache directory"))
	}
	compilation.Lock()
	defer compilation.Unlock()
	if compilation.cache != nil {
		return invalid(errors.New("WASI compilation cache already in use"))
	}
	cache, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		return invalid(err)
	}
	compilation.cache = cache
	return nil
}

// NewWASMHost prepares the runtime for installed artifacts, outside source transactions.
func NewWASMHost(ctx context.Context) (*WASMHost, error) {
	if ctx == nil {
		return nil, invalid(errors.New("missing WASI context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
		WithMemoryLimitPages(1024).WithCloseOnContextDone(true).WithCompilationCache(compilationCache()))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return nil, errors.Join(err, runtime.Close(context.WithoutCancel(ctx)))
	}
	return &WASMHost{runtime: runtime}, nil
}

// Close releases prepared artifacts/runtime after dependent invocations have drained.
func (h *WASMHost) Close(ctx context.Context) error {
	if h == nil || h.runtime == nil || ctx == nil {
		return invalid(errors.New("missing WASI host/context"))
	}
	h.close.Do(func() { h.err = h.runtime.Close(ctx) })
	return h.err
}

func (h *WASMHost) invoke(ctx context.Context, module wazero.CompiledModule, input []byte, output any, duration time.Duration) (failure error) {
	return h.invokeMeasured(ctx, module, input, output, duration, nil)
}

func (h *WASMHost) invokeMeasured(ctx context.Context, module wazero.CompiledModule, input []byte, output any, duration time.Duration, budget *executionBudget) (failure error) {
	if h == nil || h.runtime == nil || module == nil || ctx == nil || output == nil || duration <= 0 {
		return invalid(errors.New("missing WASI invocation participant"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(input) > MaxInputBytes {
		return resource(errors.New("WASI encoded input boundary"))
	}
	if !utf8.Valid(input) {
		return invalid(errors.New("WASI input is not UTF-8"))
	}
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	stdout := &boundedOutput{limit: MaxOutputBytes}
	stderr := &boundedOutput{limit: MaxOutputBytes}
	config := wazero.NewModuleConfig().WithName(uuid.NewString()).WithStdin(bytes.NewReader(input)).WithStdout(stdout).WithStderr(stderr)
	var instance api.Module
	instantiate := func(context.Context) error {
		var err error
		instance, err = h.runtime.InstantiateModule(ctx, module, config)
		return err
	}
	var err error
	if budget == nil {
		err = instantiate(ctx)
	} else {
		err = budget.guestWait(instantiate)
	}
	if instance != nil {
		defer func() {
			if err := instance.Close(context.WithoutCancel(ctx)); err != nil {
				failure = errors.Join(failure, invalid(err))
			}
		}()
	}
	if stdout.err != nil || stderr.err != nil {
		return errors.Join(stdout.err, stderr.err)
	}
	if err != nil {
		var exit *sys.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 0 {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return invalid(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if budget != nil {
		if err := budget.computeContext().Err(); err != nil {
			return err
		}
	}
	if !utf8.Valid(stdout.Bytes()) {
		return invalid(errors.New("WASI output is not UTF-8"))
	}
	return Decode(stdout.Bytes(), output)
}

var _ io.Writer = (*boundedOutput)(nil)
