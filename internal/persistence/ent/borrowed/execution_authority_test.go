package borrowed_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/entql"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/account"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/platform"
)

func TestExecutionPrincipalHoldsPrimaryAuthorityLockAndRejectsSuspension(t *testing.T) {
	f := newMatcherFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	protected, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	workerResult, suspendResult := make(chan error, 1), make(chan error, 1)
	go func() {
		workerResult <- f.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			_, err := f.resolver.ResolvePrincipalAuthority(tx.Context(), tx, f.principal)
			close(protected)
			if err != nil {
				return err
			}
			select {
			case <-release:
				return nil
			case <-tx.Context().Done():
				return tx.Context().Err()
			}
		})
	}()
	<-protected
	pids := make(chan int, 1)
	go func() {
		suspendResult <- f.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			executor, err := f.store.Resolve(tx)
			if err != nil {
				return err
			}
			var pid int
			if err := executor.QueryRowContext(tx.Context(), "SELECT pg_backend_pid()").Scan(&pid); err != nil {
				return err
			}
			pids <- pid
			if _, err := f.owners.identity.LockAccessForChange(tx.Context(), tx); err != nil {
				return err
			}
			driver, err := borrowed.New(f.store, tx)
			if err != nil {
				return err
			}
			scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
				Predicates: map[string]entql.P{"Account": entql.FieldEQ("id", f.principal)}})
			return ent.NewClient(ent.Driver(driver)).Account.UpdateOneID(f.principal).
				SetState(account.StateSuspended).Exec(scope)
		})
	}()
	select {
	case pid := <-pids:
		waitForAuthorityLock(t, f.store, pid)
	case err := <-suspendResult:
		t.Fatalf("suspender failed before lock: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release <- struct{}{}
	if err := <-workerResult; err != nil {
		t.Fatal(err)
	}
	if err := <-suspendResult; err != nil {
		t.Fatal(err)
	}
	err := f.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		_, err := f.resolver.ResolvePrincipalAuthority(tx.Context(), tx, f.principal)
		return err
	})
	if !errors.Is(err, identity.ErrLoginRequired) {
		t.Fatalf("suspended execution Principal retained authority: %v", err)
	}
	if _, err := f.resolver.ResolvePrincipalAuthority(ctx, nil, f.principal); err == nil {
		t.Fatal("internal authority escaped a live primary transaction")
	}
}
