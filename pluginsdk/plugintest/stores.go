package plugintest

import (
	"context"
	"sync"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// transaction is the trivial transaction of the in-memory stores.
type transaction struct{ ctx context.Context }

// Context returns the context the transaction runs in.
func (t transaction) Context() context.Context { return t.ctx }

// memoryStores implements every storage port of the Core's Delivery host in memory. A harness
// holds exactly one instance, so its history is the list of revisions.
type memoryStores struct {
	mu        sync.Mutex
	revisions map[int64]processing.InstanceConfiguration
	current   *processing.InstanceConfiguration
	secrets   map[uuid.UUID]processing.StoredSecret
	personal  map[uuid.UUID]processing.PersonalConfiguration
}

func newMemoryStores() *memoryStores {
	return &memoryStores{revisions: map[int64]processing.InstanceConfiguration{},
		secrets:  map[uuid.UUID]processing.StoredSecret{},
		personal: map[uuid.UUID]processing.PersonalConfiguration{}}
}

// WithinTransaction runs fn; the in-memory stores need no isolation.
func (s *memoryStores) WithinTransaction(ctx context.Context, fn platform.TxFunc) error {
	return fn(transaction{ctx: ctx})
}

// ReadInstance returns an exact revision, or authz.ErrNotFound.
func (s *memoryStores) ReadInstance(_ context.Context, _ platform.TransactionContext, id uuid.UUID,
	revision int64) (processing.InstanceConfiguration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	instance, found := s.revisions[revision]
	if !found || instance.ID != id {
		return processing.InstanceConfiguration{}, authz.ErrNotFound
	}
	return instance, nil
}

// ListCurrentInstances returns the current revision of the harness's instance.
func (s *memoryStores) ListCurrentInstances(context.Context, platform.TransactionContext) ([]processing.InstanceConfiguration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return nil, nil
	}
	return []processing.InstanceConfiguration{*s.current}, nil
}

// ReadSecrets returns the stored rows that exist for ids.
func (s *memoryStores) ReadSecrets(_ context.Context, _ platform.TransactionContext,
	ids []uuid.UUID) (map[uuid.UUID]processing.StoredSecret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows := make(map[uuid.UUID]processing.StoredSecret, len(ids))
	for _, id := range ids {
		if row, found := s.secrets[id]; found {
			rows[id] = row
		}
	}
	return rows, nil
}

// ReadPersonalConfiguration returns the owner's values, or authz.ErrNotFound.
func (s *memoryStores) ReadPersonalConfiguration(_ context.Context, _ platform.TransactionContext,
	_, ownerID uuid.UUID) (processing.PersonalConfiguration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	personal, found := s.personal[ownerID]
	if !found {
		return processing.PersonalConfiguration{}, authz.ErrNotFound
	}
	return personal, nil
}

// putInstance records a new revision and makes it current.
func (s *memoryStores) putInstance(instance processing.InstanceConfiguration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revisions[instance.Revision] = instance
	s.current = &instance
}

// putSecret stores a sealed row.
func (s *memoryStores) putSecret(row processing.StoredSecret) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[row.ID] = row
}

// putPersonal stores the owner's values as the next revision.
func (s *memoryStores) putPersonal(personal processing.PersonalConfiguration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	personal.Revision = s.personal[personal.OwnerID].Revision + 1
	s.personal[personal.OwnerID] = personal
}
