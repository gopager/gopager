package platform

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	DefaultCallTimeout     = 30 * time.Second
	DefaultShutdownTimeout = 30 * time.Second
	statusCheckInterval    = 5 * time.Second
)

// Store supplies platform infrastructure without exposing SQL or driver handles.
type Store interface {
	Ping(context.Context) error
	CheckWriteAuthority(context.Context) error
	Migrate(context.Context, fs.FS, MigrationMode) error
	WithinTransaction(context.Context, TxFunc) error
	Close() error
}

// Runner performs bounded internal work and must drain when its context is cancelled.
type Runner interface {
	Run(context.Context) error
}

type Status struct {
	Live           bool
	Ready          bool
	Profile        string
	Roles          []Role
	MigrationState string
	FailureCode    ErrorCode
}

// RunningHandle owns the composed roles and their total shutdown lifecycle.
type RunningHandle struct {
	mu           sync.RWMutex
	status       Status
	migrations   fs.FS
	store        Store
	server       *http.Server
	cancel       context.CancelFunc
	root         context.Context
	workers      sync.WaitGroup
	operations   sync.WaitGroup
	stopping     bool
	roleFailed   bool
	roleErr      error
	shutdownOnce sync.Once
	shutdownErr  error
}

// Start validates configuration and store compatibility before composing the roles.
// After valid registration it owns the store, including cleanup on startup failure.
func Start(ctx context.Context, config Config, migrations fs.FS, store Store, runners []Runner) (_ *RunningHandle, result error) {
	config = config.WithDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if store == nil || (len(runners) > 0 && !includesRole(config.Roles, RoleWorker)) {
		return nil, NewFailure(CodeInvalidInput, errors.New("invalid store or runner role registration"))
	}
	for _, runner := range runners {
		if runner == nil {
			return nil, NewFailure(CodeInvalidInput, errors.New("nil worker registration"))
		}
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, store.Close())
		}
	}()
	startCtx, cancelStart := context.WithTimeout(ctx, DefaultCallTimeout)
	defer cancelStart()
	if err := checkStore(startCtx, store, migrations, config.MigrationMode); err != nil {
		return nil, err
	}

	root, cancel := context.WithCancel(ctx)
	handle := &RunningHandle{
		status: Status{
			Live: true, Ready: true, Profile: config.Profile,
			Roles: append([]Role(nil), config.Roles...), MigrationState: "current",
		},
		migrations: migrations,
		store:      store,
		root:       root,
		cancel:     cancel,
	}
	if includesRole(config.Roles, RoleAPI) {
		listener, err := (&net.ListenConfig{}).Listen(startCtx, "tcp", config.ListenAddress)
		if err != nil {
			cancel()
			return nil, NewFailure(CodeInternalError, err)
		}
		handle.server = &http.Server{
			Handler: HealthHandler(handle.ReadStatus), ReadHeaderTimeout: 5 * time.Second,
			BaseContext: func(net.Listener) context.Context { return root },
		}
		handle.workers.Add(1)
		go func() {
			defer handle.workers.Done()
			if err := handle.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
				handle.failRole(err)
			}
		}()
	}
	for _, runner := range runners {
		handle.workers.Add(1)
		go func() {
			defer handle.workers.Done()
			err := runner.Run(root)
			if root.Err() == nil || (err != nil && !errors.Is(err, root.Err())) {
				if err == nil {
					err = NewFailure(CodeInternalError, errors.New("registered runner exited before shutdown"))
				}
				handle.failRole(err)
			}
		}()
	}
	handle.workers.Add(1)
	go handle.monitor()
	return handle, nil
}

// ReadStatus returns an independent safe snapshot, never private configuration.
func (h *RunningHandle) ReadStatus() Status {
	h.mu.RLock()
	defer h.mu.RUnlock()
	status := h.status
	status.Roles = append([]Role(nil), status.Roles...)
	if h.root.Err() != nil {
		status.Ready = false
	}
	return status
}

// WithinTransaction refuses new work after shutdown or a failed readiness check.
func (h *RunningHandle) WithinTransaction(ctx context.Context, callback TxFunc) error {
	h.mu.Lock()
	if h.stopping || !h.status.Ready || h.root.Err() != nil {
		code := h.status.FailureCode
		if code == "" {
			code = CodeAuthorityUnavailable
		}
		h.mu.Unlock()
		return NewFailure(code, errors.New("platform is not accepting work"))
	}
	h.operations.Add(1)
	h.mu.Unlock()
	defer h.operations.Done()
	ctx, cancel := context.WithTimeout(ctx, DefaultCallTimeout)
	stop := context.AfterFunc(h.root, cancel)
	defer stop()
	defer cancel()
	if err := h.store.WithinTransaction(ctx, callback); err != nil {
		code := FailureCode(err)
		if code == CodeStorageUnavailable || code == CodeAuthorityUnavailable || code == CodeIncompatibleVersion {
			h.setReadiness(err)
		}
		return err
	}
	return nil
}

func (h *RunningHandle) failRole(err error) {
	h.mu.Lock()
	h.roleFailed = true
	h.roleErr = errors.Join(h.roleErr, NewFailure(FailureCode(err), err))
	h.status.Ready = false
	h.status.FailureCode = FailureCode(err)
	h.mu.Unlock()
	slog.Error("platform role stopped", "code", FailureCode(err))
}

func (h *RunningHandle) monitor() {
	defer h.workers.Done()
	ticker := time.NewTicker(statusCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-h.root.Done():
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(h.root, DefaultCallTimeout)
			err := h.Recheck(ctx)
			cancel()
			if err != nil && h.root.Err() == nil {
				slog.Error("platform readiness check failed", "code", FailureCode(err))
			}
		}
	}
}

// Recheck requires connectivity, write authority and the registered schema together.
func (h *RunningHandle) Recheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, DefaultCallTimeout)
	defer cancel()
	err := checkStore(ctx, h.store, h.migrations, MigrationCheck)
	h.setReadiness(err)
	return err
}

func checkStore(ctx context.Context, store Store, migrations fs.FS, mode MigrationMode) error {
	if err := store.Ping(ctx); err != nil {
		return err
	}
	if err := store.CheckWriteAuthority(ctx); err != nil {
		return err
	}
	return store.Migrate(ctx, migrations, mode)
}

func (h *RunningHandle) setReadiness(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopping {
		return
	}
	if err != nil {
		h.status.Ready = false
		h.status.FailureCode = FailureCode(err)
		if h.status.FailureCode == CodeIncompatibleVersion {
			h.status.MigrationState = "incompatible"
		}
	} else if !h.roleFailed && h.root.Err() == nil {
		h.status.Ready = true
		h.status.FailureCode = ""
		h.status.MigrationState = "current"
	}
}

// Shutdown closes admission, cancels and drains work, then closes listener and store.
// Repeated callers receive the same outcome; the caller's shorter deadline is honored.
func (h *RunningHandle) Shutdown(ctx context.Context) error {
	h.shutdownOnce.Do(func() {
		h.mu.Lock()
		h.stopping = true
		h.status.Ready = false
		h.mu.Unlock()
		h.cancel()
		ctx, cancel := context.WithTimeout(ctx, DefaultShutdownTimeout)
		defer cancel()
		var result error
		if h.server != nil {
			if err := h.server.Shutdown(ctx); err != nil {
				result = errors.Join(result, NewFailure(CodeStorageUnavailable, err))
				if closeErr := h.server.Close(); closeErr != nil {
					result = errors.Join(result, NewFailure(CodeInternalError, closeErr))
				}
			}
		}
		drained := make(chan struct{})
		go func() {
			h.workers.Wait()
			h.operations.Wait()
			close(drained)
		}()
		select {
		case <-drained:
		case <-ctx.Done():
			result = errors.Join(result, NewFailure(CodeStorageUnavailable, ctx.Err()))
		}
		closed := make(chan error, 1)
		go func() { closed <- h.store.Close() }()
		select {
		case err := <-closed:
			result = errors.Join(result, err)
		case <-ctx.Done():
			result = errors.Join(result, NewFailure(CodeStorageUnavailable, ctx.Err()))
		}
		h.mu.Lock()
		result = errors.Join(result, h.roleErr)
		h.status.Live = false
		h.shutdownErr = result
		h.mu.Unlock()
	})
	return h.shutdownErr
}

func includesRole(roles []Role, wanted Role) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
