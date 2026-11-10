package readmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// ManageGlobalAction is the action that maintains global saved views.
const ManageGlobalAction = "saved-view.manage-global"

// SavedViewRecord is a stored saved view.
type SavedViewRecord struct {
	ID               uuid.UUID
	Revision         int64
	Ownership        string
	OwnerPrincipalID *uuid.UUID
	Name             string
	// Definition is the canonical definition JSON.
	Definition []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SavedViewListQuery selects the views a caller may list.
type SavedViewListQuery struct {
	// Owner selects the personal views of this Principal; uuid.Nil selects none.
	Owner uuid.UUID
	// Global adds the global views.
	Global bool
	Offset int
	Limit  int
}

// SavedViewStore is the persistence port of saved views. Every method runs in the caller's
// transaction.
type SavedViewStore interface {
	// InsertSavedView stores a new view.
	InsertSavedView(ctx context.Context, tx platform.TransactionContext, record SavedViewRecord) error
	// SavedView returns the view or authz.ErrNotFound.
	SavedView(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (SavedViewRecord, error)
	// ListSavedViews returns one page ordered by name then ID and the total of the selection.
	ListSavedViews(ctx context.Context, tx platform.TransactionContext, query SavedViewListQuery) ([]SavedViewRecord, int, error)
	// UpdateSavedView replaces name and definition and increments the revision, only at the
	// expected revision; otherwise it returns authz.ErrRevisionConflict.
	UpdateSavedView(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64,
		name string, definition []byte, now time.Time) error
	// DeleteSavedView removes the view at the expected revision, else authz.ErrRevisionConflict.
	DeleteSavedView(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, expected int64) error
}

// SavedViews manages personal and global saved views. Views write no fact and no audit record.
// Create it with NewSavedViews; it is safe for concurrent use.
type SavedViews struct {
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	store        SavedViewStore
	receipts     commands.Repository
}

// NewSavedViews composes the saved view service. It fails with authz.ErrInvalidInput on a
// missing dependency.
func NewSavedViews(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store SavedViewStore, receipts commands.Repository) (*SavedViews, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil || receipts == nil {
		return nil, authz.ErrInvalidInput
	}
	return &SavedViews{transactions: transactions, resolver: resolver, clock: clock, store: store, receipts: receipts}, nil
}

// actor is what a saved view operation needs to know about its caller.
type actor struct {
	principal uuid.UUID
	guest     bool
	// reader holds any message.read grant.
	reader bool
	// manager may manage global views.
	manager bool
	groups  map[string]bool
}

func (a actor) can(group string) bool { return a.groups[group] }

// within resolves the caller and runs operation in the same transaction. A mutation needs the
// transport's state-change check and is never available to the guest.
func (s *SavedViews) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error,
	operation func(tx platform.TransactionContext, who actor, now time.Time) error) error {
	if mutation && check == nil {
		return authz.ErrInvalidInput
	}
	var authentication identity.Authentication
	remember := func(value identity.Authentication) error {
		authentication = value
		if check != nil {
			return check(value)
		}
		return nil
	}
	return identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, mutation, remember,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			// PostgreSQL keeps microseconds; a response must equal what a later read returns.
			now = now.Truncate(time.Microsecond)
			who, err := newActor(authentication, authority, now)
			if err != nil {
				return err
			}
			if mutation && who.guest {
				return identity.ErrLoginRequired
			}
			return operation(tx, who, now)
		})
}

// newActor derives the caller's saved view capabilities from its current authority.
func newActor(authentication identity.Authentication, authority authz.Authority, now time.Time) (actor, error) {
	principal, err := uuid.Parse(authority.PrincipalID)
	if err != nil || principal == uuid.Nil {
		return actor{}, authz.ErrForbidden
	}
	rows, err := authz.NewMessageAccess(authority, now)
	if err != nil {
		return actor{}, err
	}
	readable, err := authz.ReadableFieldGroups(authority)
	if err != nil {
		return actor{}, err
	}
	who := actor{principal: principal, guest: authentication.Kind == identity.AnonymousProof,
		reader: rows.Read.Granted(), groups: make(map[string]bool, len(readable))}
	for _, group := range readable {
		who.groups[group] = true
	}
	switch err := authz.Authorize(authority, ManageGlobalAction, nil, authz.Target{}, now); {
	case err == nil:
		who.manager = true
	case !errors.Is(err, authz.ErrForbidden):
		return actor{}, err
	}
	return who, nil
}

// visible reports whether the caller may read the view; everything else is not found. A global
// view is visible to every Message reader and to its managers, who could not maintain a view
// they cannot see; seeing a definition grants no Message access.
func (a actor) visible(record SavedViewRecord) bool {
	if record.Ownership == OwnershipPersonal {
		return !a.guest && a.reader && record.OwnerPrincipalID != nil && *record.OwnerPrincipalID == a.principal
	}
	return a.reader || a.manager
}

// writable decides whether the caller may change or delete the view: its own personal view, or
// a global view as a manager. A global view that a reader sees but cannot manage is forbidden;
// one that the caller can neither see nor manage is not found.
func (a actor) writable(record SavedViewRecord) error {
	if record.Ownership == OwnershipPersonal {
		if a.visible(record) {
			return nil
		}
		return authz.ErrNotFound
	}
	switch {
	case a.manager:
		return nil
	case a.reader:
		return authz.ErrForbidden
	}
	return authz.ErrNotFound
}

// requireAuthorGroups rejects filters that need a field group the author cannot read.
func (a actor) requireAuthorGroups(filters Filters) error {
	for _, group := range filters.RequiredFieldGroups() {
		if !a.can(group) {
			return authz.ErrForbidden
		}
	}
	return nil
}

// view builds the response of a stored view for this caller.
func (a actor) view(record SavedViewRecord) (SavedView, error) {
	definition, err := ParseDefinition(record.Definition)
	if err != nil {
		return SavedView{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	return SavedView{ID: record.ID, Revision: record.Revision, Ownership: record.Ownership, Name: record.Name,
		Definition: definition, UnavailableFilters: definition.FeedFilters().UnavailableDimensions(a.can),
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

// canonicalInput validates an input and returns the canonical definition JSON with its filters.
func canonicalInput(name string, definition Definition) (Definition, Filters, []byte, error) {
	if err := ValidateViewName(name); err != nil {
		return Definition{}, Filters{}, nil, err
	}
	normalized, filters, err := definition.Normalize()
	if err != nil {
		return Definition{}, Filters{}, nil, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return Definition{}, Filters{}, nil, err
	}
	return normalized, filters, encoded, nil
}

// sameDefinition compares a stored definition with canonical definition JSON. PostgreSQL's
// jsonb does not keep key order or spacing, so the stored form is canonicalized first.
func sameDefinition(stored, canonical []byte) bool {
	definition, err := ParseDefinition(stored)
	if err != nil {
		return false
	}
	normalized, _, err := definition.Normalize()
	if err != nil {
		return false
	}
	encoded, err := json.Marshal(normalized)
	return err == nil && bytes.Equal(encoded, canonical)
}

// List returns one page of the views the caller may read: its own personal views and, for a
// Message reader, the global views, ordered by name then ID.
func (s *SavedViews) List(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[SavedView], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[SavedView]{}, err
	}
	result := units.Page[SavedView]{Items: []SavedView{}}
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who actor, _ time.Time) error {
		result.Items, result.Total = []SavedView{}, 0
		query := SavedViewListQuery{Global: who.reader || who.manager, Offset: page.Offset, Limit: page.Limit}
		if who.reader && !who.guest {
			query.Owner = who.principal
		}
		if !query.Global && query.Owner == uuid.Nil {
			return nil
		}
		records, total, err := s.store.ListSavedViews(tx.Context(), tx, query)
		if err != nil {
			return err
		}
		result.Total = total
		for _, record := range records {
			item, err := who.view(record)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, item)
		}
		return nil
	})
	return result, err
}

// Read returns one view the caller may see; any other view is authz.ErrNotFound.
func (s *SavedViews) Read(ctx context.Context, proof identity.Proof, id uuid.UUID) (SavedView, error) {
	if id == uuid.Nil {
		return SavedView{}, authz.ErrInvalidInput
	}
	var result SavedView
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who actor, _ time.Time) error {
		record, err := s.store.SavedView(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		if !who.visible(record) {
			return authz.ErrNotFound
		}
		result, err = who.view(record)
		return err
	})
	return result, err
}

// Create stores a view with actor-bound command idempotency. A personal view needs a non-guest
// caller with a message.read grant, a global view needs saved-view.manage-global; either is
// authz.ErrForbidden otherwise. The definition must only use filters whose field groups the
// author can read (authz.ErrForbidden). An identical replay of the command returns the stored
// view as it is now, or authz.ErrNotFound if it was deleted since.
func (s *SavedViews) Create(ctx context.Context, proof identity.Proof, command uuid.UUID, input SavedViewInput,
	check func(identity.Authentication) error) (SavedView, error) {
	if command == uuid.Nil || (input.Ownership != OwnershipPersonal && input.Ownership != OwnershipGlobal) {
		return SavedView{}, authz.ErrInvalidInput
	}
	_, filters, encoded, err := canonicalInput(input.Name, input.Definition)
	if err != nil {
		return SavedView{}, err
	}
	fingerprint, err := commands.Fingerprint(struct {
		Ownership  string `json:"ownership"`
		Name       string `json:"name"`
		Definition string `json:"definition"`
	}{input.Ownership, input.Name, string(encoded)})
	if err != nil {
		return SavedView{}, err
	}
	var result SavedView
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who actor, now time.Time) error {
		allowed := who.manager
		if input.Ownership == OwnershipPersonal {
			allowed = !who.guest && who.reader
		}
		if !allowed {
			return authz.ErrForbidden
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, who.principal, command)
		if err != nil {
			return err
		}
		if found {
			stored, err := receipt.Check("saved-view.create", fingerprint)
			if err != nil {
				return err
			}
			record, err := s.store.SavedView(tx.Context(), tx, stored.ID)
			if err != nil {
				return err
			}
			result, err = who.view(record)
			return err
		}
		if err := who.requireAuthorGroups(filters); err != nil {
			return err
		}
		record := SavedViewRecord{ID: uuid.New(), Revision: 1, Ownership: input.Ownership, Name: input.Name,
			Definition: encoded, CreatedAt: now, UpdatedAt: now}
		if input.Ownership == OwnershipPersonal {
			owner := who.principal
			record.OwnerPrincipalID = &owner
		}
		if err := s.store.InsertSavedView(tx.Context(), tx, record); err != nil {
			return err
		}
		if err := s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: who.principal,
			CommandID: command, Operation: "saved-view.create", Fingerprint: fingerprint,
			Result: commands.Result{ID: record.ID, Revision: 1}, RecordedAt: now}); err != nil {
			return err
		}
		result, err = who.view(record)
		return err
	})
	return result, err
}

// Update replaces name and definition of a view at the expected revision. The view's ownership
// does not change. A change that stores the same name and definition keeps the revision.
func (s *SavedViews) Update(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, name string,
	definition Definition, check func(identity.Authentication) error) (SavedView, error) {
	if id == uuid.Nil || expected <= 0 {
		return SavedView{}, authz.ErrInvalidInput
	}
	_, filters, encoded, err := canonicalInput(name, definition)
	if err != nil {
		return SavedView{}, err
	}
	var result SavedView
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who actor, now time.Time) error {
		record, err := s.store.SavedView(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := who.writable(record); err != nil {
			return err
		}
		next, err := commands.NextRevision(record.Revision, expected)
		if err != nil {
			return err
		}
		if err := who.requireAuthorGroups(filters); err != nil {
			return err
		}
		if record.Name != name || !sameDefinition(record.Definition, encoded) {
			if err := s.store.UpdateSavedView(tx.Context(), tx, id, expected, name, encoded, now); err != nil {
				return err
			}
			record.Name, record.Definition, record.Revision, record.UpdatedAt = name, encoded, next, now
		}
		result, err = who.view(record)
		return err
	})
	return result, err
}

// Delete removes a view at the expected revision and returns its ID with the revision it
// reached.
func (s *SavedViews) Delete(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who actor, _ time.Time) error {
		record, err := s.store.SavedView(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		if err := who.writable(record); err != nil {
			return err
		}
		next, err := commands.NextRevision(record.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.store.DeleteSavedView(tx.Context(), tx, id, expected); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return nil
	})
	return result, err
}
