// Package presentationstore implements the PostgreSQL side of the installation settings, layout
// assignments and the Layout layers in raw SQL, inside the caller's primary transaction.
package presentationstore

import (
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing/presentation"
)

// Store reads and writes presentation settings. It holds no transaction of its own.
type Store struct {
	primary *postgres.Store
}

var (
	_ presentation.SettingsStore   = (*Store)(nil)
	_ presentation.AssignmentStore = (*Store)(nil)
	_ presentation.LayerStore      = (*Store)(nil)
	_ presentation.ReferenceGuard  = (*Store)(nil)
)

// New binds the store to the primary store whose transactions it joins. It fails with
// authz.ErrInvalidInput on a nil store.
func New(primary *postgres.Store) (*Store, error) {
	if primary == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Store{primary: primary}, nil
}
