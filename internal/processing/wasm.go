package processing

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
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

//go:generate env GOOS=wasip1 GOARCH=wasm CGO_ENABLED=0 go build -trimpath -buildvcs=false -o regex.wasm ./regexguest

//go:embed regex.wasm
var coreRegex []byte

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
	regex   wazero.CompiledModule
	close   sync.Once
	err     error
}

// NewWASMHost prepares the supplied Core guest once, outside source transactions.
func NewWASMHost(ctx context.Context) (*WASMHost, error) {
	if ctx == nil {
		return nil, invalid(errors.New("missing WASI context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runtime := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler().
		WithMemoryLimitPages(1024).WithCloseOnContextDone(true))
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return nil, errors.Join(err, runtime.Close(context.WithoutCancel(ctx)))
	}
	compiled, err := runtime.CompileModule(ctx, coreRegex)
	if err != nil {
		return nil, errors.Join(err, runtime.Close(context.WithoutCancel(ctx)))
	}
	return &WASMHost{runtime: runtime, regex: compiled}, nil
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

type RegexCapture struct {
	Name  string  `json:"name"`
	Index int     `json:"index"`
	Text  *string `json:"text,omitempty"`
}

type RegexResult struct {
	Matched  bool           `json:"matched"`
	Captures []RegexCapture `json:"captures,omitempty"`
	Names    []string       `json:"names"`
	Failure  string         `json:"failure,omitempty"`
}

// Regex delegates both syntax inspection and matching to the embedded bounded guest.
func (h *WASMHost) Regex(ctx context.Context, pattern string, text *string) (RegexResult, error) {
	if h == nil {
		return RegexResult{}, invalid(errors.New("missing WASI host"))
	}
	if len(pattern) > 4096 {
		return RegexResult{}, resource(errors.New("regex pattern boundary"))
	}
	if !utf8.ValidString(pattern) || (text != nil && !utf8.ValidString(*text)) {
		return RegexResult{}, invalid(errors.New("regex data is not UTF-8"))
	}
	input := struct {
		Mode    string `json:"mode"`
		Pattern string `json:"pattern"`
		Text    string `json:"text,omitempty"`
	}{Mode: "inspect", Pattern: pattern}
	if text != nil {
		input.Mode, input.Text = "match", *text
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return RegexResult{}, invalid(err)
	}
	var result RegexResult
	if err := h.invoke(ctx, h.regex, encoded, &result, time.Second); err != nil {
		return RegexResult{}, err
	}
	if result.Failure != "" {
		return RegexResult{}, invalid(errors.New("Core regex rejected its input"))
	}
	return result, nil
}

var _ io.Writer = (*boundedOutput)(nil)
