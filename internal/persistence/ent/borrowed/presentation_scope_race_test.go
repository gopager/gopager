package borrowed_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// TestAScopeDeletedDuringAnAssignmentLeavesNoAssignment runs a scope deletion while an assignment
// write that already found the scope is still open: the deletion must wait for that write, so its
// trigger removes the new assignment instead of leaving one behind for a deleted scope.
func TestAScopeDeletedDuringAnAssignmentLeavesNoAssignment(t *testing.T) {
	t.Parallel()
	w := newPresentationWorld(t)
	agency, org := w.agency(), w.organization()
	unit := w.unit(agency, uuid.Nil)
	layout := w.layout("text", agency)

	for _, scope := range []struct {
		kind, table string
		id          uuid.UUID
	}{{presentation.ScopeUnit, "units", unit}, {presentation.ScopeOrganization, "organizations", org}} {
		found, deleted := make(chan struct{}), make(chan error, 1)
		written := make(chan error, 1)
		go func() {
			written <- w.within(func(tx platform.TransactionContext) error {
				var err error
				if scope.kind == presentation.ScopeUnit {
					_, err = w.layers.ReadUnit(tx.Context(), tx, scope.id)
				} else {
					var exists bool
					if exists, err = w.layers.OrganizationExists(tx.Context(), tx, scope.id); err == nil && !exists {
						t.Errorf("%s not found", scope.kind)
					}
				}
				close(found)
				if err != nil {
					return err
				}
				// Give a concurrent deletion the chance to commit before the write.
				time.Sleep(300 * time.Millisecond)
				return w.layers.Put(tx.Context(), tx, presentation.Assignment{ScopeKind: scope.kind, ScopeID: scope.id,
					OutputKind: "text", LayoutID: layout, Revision: 1, UpdatedAt: time.Now()}, 0)
			})
		}()
		<-found
		go func() {
			deleted <- w.within(func(tx platform.TransactionContext) error {
				executor, err := w.store.Resolve(tx)
				if err == nil {
					_, err = executor.ExecContext(tx.Context(), `UPDATE `+scope.table+` SET deleted = true WHERE id = $1`, scope.id)
				}
				return err
			})
		}()
		if err := <-written; err != nil {
			t.Fatalf("%s assignment: %v", scope.kind, err)
		}
		if err := <-deleted; err != nil {
			t.Fatalf("%s deletion: %v", scope.kind, err)
		}
		var left []presentation.ListedAssignment
		if err := w.within(func(tx platform.TransactionContext) error {
			var err error
			left, err = w.layers.List(tx.Context(), tx, scope.kind, scope.id)
			return err
		}); err != nil || len(left) != 0 {
			t.Fatalf("assignments of the deleted %s: %+v, %v", scope.kind, left, err)
		}
	}
}
