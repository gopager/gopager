package attributestore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// TestCatalogLockHoldsSchemaDeletionUntilTheReferenceCommits expects a schema deletion to wait
// for a write that locked the catalog before validating its references, and then to see the
// committed reference and refuse, instead of passing its reference check first.
func TestCatalogLockHoldsSchemaDeletionUntilTheReferenceCommits(t *testing.T) {
	t.Parallel()
	w := newWorld(t)
	ctx := context.Background()
	if _, err := w.manager.Create(ctx, identity.Proof{}, uuid.New(), w.input("held"), allow); err != nil {
		t.Fatal(err)
	}
	locked, release := make(chan struct{}), make(chan struct{})
	writer := make(chan error, 1)
	go func() {
		writer <- w.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			if err := w.schemas.LockCatalog(tx.Context(), tx); err != nil {
				return err
			}
			executor, err := w.store.Resolve(tx)
			if err != nil {
				return err
			}
			if _, err := executor.ExecContext(tx.Context(), `INSERT INTO subscriptions (id, revision, kind, subscriber_id,
				created_by, state, event_types, filter, split_by_unit, created_at, updated_at)
				VALUES ($1, 1, 'global', $2, $2, 'active', ARRAY['message.created'], $3::jsonb, false, now(), now())`,
				uuid.New(), uuid.New(), `{"op":"present","field":{"namespace":"attributes","path":["held","code"]},
				"value":{"kind":"boolean","value":true}}`); err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case err := <-writer:
		t.Fatalf("writer ended early: %v", err)
	}
	deletion := make(chan error, 1)
	go func() {
		_, err := w.manager.Delete(ctx, identity.Proof{}, "held", 1, allow)
		deletion <- err
	}()
	select {
	case err := <-deletion:
		t.Fatalf("deletion did not wait for the referencing write: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	close(release)
	if err := <-writer; err != nil {
		t.Fatalf("referencing write: %v", err)
	}
	if err := <-deletion; !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("deletion after the reference committed: %v, want revision conflict", err)
	}
}
