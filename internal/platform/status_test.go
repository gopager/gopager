package platform

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type probeStore struct {
	fakeStore
	pingErr      error
	authorityErr error
	migrationErr error
	txCalls      int
}

func (s *probeStore) Ping(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingErr
}

func (s *probeStore) CheckWriteAuthority(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authorityErr
}

func (s *probeStore) Migrate(context.Context, fs.FS, MigrationMode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.migrationErr
}

func (s *probeStore) WithinTransaction(_ context.Context, fn TxFunc) error {
	s.mu.Lock()
	s.txCalls++
	s.mu.Unlock()
	return fn(nil)
}

type runnerFunc func(context.Context) error

func (f runnerFunc) Run(ctx context.Context) error { return f(ctx) }

func TestReadinessRequiresEveryAuthorityCheck(t *testing.T) {
	t.Parallel()
	store := &probeStore{}
	h, err := Start(context.Background(), Config{
		Profile: "full", Roles: []Role{RoleWorker}, DatabaseURL: "synthetic",
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Shutdown(context.Background()) }()
	for _, code := range []ErrorCode{CodeStorageUnavailable, CodeAuthorityUnavailable, CodeIncompatibleVersion} {
		store.mu.Lock()
		failure := NewFailure(code, errors.New("synthetic-secret-dsn"))
		switch code {
		case CodeStorageUnavailable:
			store.pingErr = failure
		case CodeAuthorityUnavailable:
			store.authorityErr = failure
		case CodeIncompatibleVersion:
			store.migrationErr = failure
		}
		store.mu.Unlock()
		if err := h.Recheck(context.Background()); FailureCode(err) != code || h.ReadStatus().Ready {
			t.Fatalf("check did not fail closed for %s: %v", code, err)
		}
		for _, path := range []string{"/health/live", "/health/ready"} {
			rec := httptest.NewRecorder()
			HealthHandler(h.ReadStatus).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			wantCode, body := http.StatusOK, "{\"live\":true}\n"
			if path == "/health/ready" {
				wantCode, body = http.StatusServiceUnavailable, "{\"ready\":false}\n"
			}
			if rec.Code != wantCode || rec.Body.String() != body {
				t.Fatalf("unsafe status: %d %q", rec.Code, rec.Body.String())
			}
		}
		if err := h.WithinTransaction(context.Background(), func(TransactionContext) error { return nil }); FailureCode(err) != code {
			t.Fatalf("not-ready admitted work: %v", err)
		}
		store.mu.Lock()
		if store.txCalls != 0 {
			t.Error("failed readiness called transaction adapter")
		}
		store.pingErr, store.authorityErr, store.migrationErr = nil, nil, nil
		store.mu.Unlock()
		if err := h.Recheck(context.Background()); err != nil || !h.ReadStatus().Ready {
			t.Fatalf("recovery failed: %v", err)
		}
	}
}

func TestShutdownCancelsActiveOperationsAndRejectsNewOnes(t *testing.T) {
	t.Parallel()
	store := &cancellingStore{started: make(chan struct{})}
	h, err := Start(context.Background(), Config{
		Profile: "full", Roles: []Role{RoleWorker}, DatabaseURL: "synthetic",
	}, nil, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- h.WithinTransaction(context.Background(), func(TransactionContext) error { return nil })
	}()
	<-store.started
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("active operation not cancelled: %v", err)
	}
	if err := h.WithinTransaction(context.Background(), func(TransactionContext) error { return nil }); FailureCode(err) != CodeAuthorityUnavailable {
		t.Fatalf("shutdown admitted new work: %v", err)
	}
}

type cancellingStore struct {
	fakeStore
	started chan struct{}
}

func (s *cancellingStore) WithinTransaction(ctx context.Context, _ TxFunc) error {
	close(s.started)
	<-ctx.Done()
	return NewFailure(CodeStorageUnavailable, ctx.Err())
}

func TestShutdownDeadlineAndRunnerFailureAreVisible(t *testing.T) {
	t.Parallel()
	t.Run("deadline", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		exited := make(chan struct{})
		h, err := Start(context.Background(), Config{
			Profile: "full", Roles: []Role{RoleWorker}, DatabaseURL: "synthetic",
		}, nil, &fakeStore{}, []Runner{runnerFunc(func(context.Context) error {
			defer close(exited)
			<-release
			return nil
		})})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err = h.Shutdown(ctx)
		close(release)
		<-exited
		if !errors.Is(err, context.DeadlineExceeded) || FailureCode(err) != CodeStorageUnavailable {
			t.Fatalf("drain timeout concealed: %v", err)
		}
		if h.ReadStatus().Ready || h.ReadStatus().Live {
			t.Fatal("timed-out shutdown remains ready/live")
		}
		if h.Shutdown(context.Background()) != err {
			t.Fatal("repeated shutdown lost its failure result")
		}
	})
	t.Run("runner failure on cancellation", func(t *testing.T) {
		t.Parallel()
		h, err := Start(context.Background(), Config{
			Profile: "full", Roles: []Role{RoleWorker}, DatabaseURL: "synthetic",
		}, nil, &fakeStore{}, []Runner{runnerFunc(func(ctx context.Context) error {
			<-ctx.Done()
			return NewFailure(CodeStorageUnavailable, errors.New("synthetic-private-marker"))
		})})
		if err != nil {
			t.Fatal(err)
		}
		if err := h.Shutdown(context.Background()); FailureCode(err) != CodeStorageUnavailable {
			t.Fatalf("runner drain failure concealed: %v", err)
		}
	})
}

func TestSafeDiagnosticsDoNotDependOnDisplayPreferences(t *testing.T) { //nolint:paralleltest // sets the process environment variable LANG with t.Setenv
	for _, locale := range []string{"en", "de", "different-format"} {
		t.Run(locale, func(t *testing.T) {
			t.Setenv("LANG", locale)
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			failure := NewFailure(CodeCommitUncertain, errors.New("postgres://secret-marker:private@synthetic.invalid/db"))
			logger.Error("operation failed", "failure", failure, "code", failure.Code())
			if strings.Contains(output.String(), "secret-marker") || strings.Contains(output.String(), "synthetic.invalid") ||
				!strings.Contains(output.String(), "COMMIT_UNCERTAIN") || failure.TextKey() != "error.COMMIT_UNCERTAIN" {
				t.Fatalf("unsafe/unstable diagnostics: %s", output.String())
			}
		})
	}
}

func TestNewHTTPServerBoundsEveryConnectionPhase(t *testing.T) {
	t.Parallel()
	server := newHTTPServer(context.Background(), http.NotFoundHandler())
	if server.ReadHeaderTimeout != httpReadHeaderTimeout || server.ReadTimeout != httpReadTimeout ||
		server.WriteTimeout != httpWriteTimeout || server.IdleTimeout != httpIdleTimeout ||
		server.MaxHeaderBytes != httpMaxHeaderBytes {
		t.Fatalf("server limits not applied: %+v", server)
	}
	if httpReadHeaderTimeout <= 0 || httpReadTimeout <= 0 || httpWriteTimeout <= 0 || httpIdleTimeout <= 0 ||
		httpMaxHeaderBytes <= 0 || httpMaxHeaderBytes > 1<<20 {
		t.Fatal("a server limit is unset or unreasonably large")
	}
}
