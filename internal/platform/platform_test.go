package platform

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{
			name: "full api and worker",
			cfg: Config{
				Profile:       "full",
				Roles:         []Role{RoleAPI, RoleWorker},
				MigrationMode: MigrationCheck,
				DatabaseURL:   "postgres://user:secret@localhost/db",
				ListenAddress: "127.0.0.1:8080",
			},
			ok: true,
		},
		{
			name: "worker only",
			cfg: Config{
				Profile:       "full",
				Roles:         []Role{RoleWorker},
				MigrationMode: MigrationApply,
				DatabaseURL:   "postgres://user:secret@localhost/db",
			},
			ok: true,
		},
		{
			name: "migration check defaults",
			cfg: Config{
				Profile:     "full",
				Roles:       []Role{RoleWorker},
				DatabaseURL: "postgres://user:secret@localhost/db",
			},
			ok: true,
		},
		{
			name: "lite rejected",
			cfg: Config{
				Profile:       "lite",
				Roles:         []Role{RoleWorker},
				MigrationMode: MigrationCheck,
				DatabaseURL:   "postgres://user:secret@localhost/db",
			},
		},
		{
			name: "duplicate role rejected",
			cfg: Config{
				Profile:       "full",
				Roles:         []Role{RoleAPI, RoleAPI},
				MigrationMode: MigrationCheck,
				DatabaseURL:   "postgres://user:secret@localhost/db",
				ListenAddress: "127.0.0.1:8080",
			},
		},
		{
			name: "api requires address",
			cfg: Config{
				Profile:       "full",
				Roles:         []Role{RoleAPI},
				MigrationMode: MigrationCheck,
				DatabaseURL:   "postgres://user:secret@localhost/db",
			},
		},
		{
			name: "connection required",
			cfg: Config{
				Profile:       "full",
				Roles:         []Role{RoleWorker},
				MigrationMode: MigrationCheck,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cfg := test.cfg.WithDefaults()
			err := cfg.Validate()
			if (err == nil) != test.ok {
				t.Fatalf("Validate() error = %v, want success %v", err, test.ok)
			}
			if test.name == "migration check defaults" && cfg.MigrationMode != MigrationCheck {
				t.Fatalf("default migration mode = %q, want %q", cfg.MigrationMode, MigrationCheck)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatalf("safe validation error exposed a secret: %q", err)
			}
		})
	}
}

func TestHealthHandlerHasBooleanOnlyBodies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		path       string
		status     Status
		wantStatus int
		wantBody   string
	}{
		{"live", "/health/live", Status{Live: true}, http.StatusOK, "{\"live\":true}\n"},
		{"ready", "/health/ready", Status{Ready: true}, http.StatusOK, "{\"ready\":true}\n"},
		{"not ready", "/health/ready", Status{Ready: false}, http.StatusServiceUnavailable, "{\"ready\":false}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			HealthHandler(func() Status { return test.status }).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.wantStatus || recorder.Body.String() != test.wantBody {
				t.Fatalf("response = %d %q, want %d %q", recorder.Code, recorder.Body.String(), test.wantStatus, test.wantBody)
			}
		})
	}
}

func TestHealthHandlerRejectsOtherMethods(t *testing.T) {
	t.Parallel()
	recorder := httptest.NewRecorder()
	HealthHandler(func() Status { return Status{Live: true} }).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodPost, "/health/live", nil),
	)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusMethodNotAllowed)
	}
}

func TestNextFencingToken(t *testing.T) {
	t.Parallel()
	next, err := NextFencingToken(7)
	if err != nil || next != 8 {
		t.Fatalf("NextFencingToken(7) = %d, %v; want 8, nil", next, err)
	}
	for _, value := range []int64{-1, int64(^uint64(0) >> 1)} {
		if _, err := NextFencingToken(value); FailureCode(err) != CodeResourceLimit {
			t.Fatalf("NextFencingToken(%d) error = %v, want RESOURCE_LIMIT", value, err)
		}
	}
}

func TestStartWorkerOnlyAndShutdown(t *testing.T) {
	t.Parallel()
	store := &fakeStore{}
	worker := &blockingRunner{done: make(chan struct{})}
	handle, err := Start(context.Background(), Config{
		Profile:       "full",
		Roles:         []Role{RoleWorker},
		MigrationMode: MigrationCheck,
		DatabaseURL:   "postgres://localhost/gopager",
	}, nil, store, []Runner{worker})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !handle.ReadStatus().Ready {
		t.Fatal("worker-only composition did not become ready")
	}
	if err := handle.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if handle.ReadStatus().Live || handle.ReadStatus().Ready {
		t.Fatalf("status after shutdown = %+v", handle.ReadStatus())
	}
	if !store.closed || !worker.cancelled {
		t.Fatalf("store closed=%t worker cancelled=%t", store.closed, worker.cancelled)
	}
}

type fakeStore struct {
	mu     sync.Mutex
	closed bool
}

func (s *fakeStore) Ping(context.Context) error                { return nil }
func (s *fakeStore) CheckWriteAuthority(context.Context) error { return nil }
func (s *fakeStore) Migrate(context.Context, fs.FS, MigrationMode) error {
	return nil
}
func (s *fakeStore) WithinTransaction(context.Context, TxFunc) error { return nil }
func (s *fakeStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

type blockingRunner struct {
	done      chan struct{}
	cancelled bool
}

func (r *blockingRunner) Run(ctx context.Context) error {
	<-ctx.Done()
	r.cancelled = true
	close(r.done)
	return ctx.Err()
}

func TestFailureCodeAndEnglishFallbackAreStable(t *testing.T) {
	t.Parallel()
	err := NewFailure(CodeStorageUnavailable, fmt.Errorf("DSN=secret-marker"))
	if err.Error() != englishFallbacks[CodeStorageUnavailable] || strings.Contains(err.Error(), "secret-marker") {
		t.Fatalf("unsafe fallback: %q", err)
	}
	if FailureCode(fmt.Errorf("wrapped: %w", err)) != CodeStorageUnavailable {
		t.Fatal("wrapped error code was not preserved")
	}
	expectedFallbacks := []string{
		englishFallbacks[CodeInvalidInput],
		englishFallbacks[CodeIncompatibleVersion],
		englishFallbacks[CodeResourceLimit],
		englishFallbacks[CodeStorageUnavailable],
		englishFallbacks[CodeAuthorityUnavailable],
		englishFallbacks[CodeCommitUncertain],
		englishFallbacks[CodeInternalError],
	}
	wantFallbacks := []string{
		"Check the supplied values.",
		"This version is not compatible.",
		"The operation exceeds a configured limit.",
		"Storage is not available for this operation.",
		"Current write or access authority is not available.",
		"The operation may have been committed. Check its identifier.",
		"The operation could not be completed.",
	}
	for i := range expectedFallbacks {
		if expectedFallbacks[i] != wantFallbacks[i] {
			t.Fatalf("fallback %d = %q, want %q", i, expectedFallbacks[i], wantFallbacks[i])
		}
	}
}
