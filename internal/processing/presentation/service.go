package presentation

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/units"
)

// The actions that gate layouts.
const (
	// ManageAction maintains layouts.
	ManageAction = "presentation.manage"
	// SubscriptionManageOwnAction and SubscriptionManageGlobalAction also allow reading layouts,
	// so Subscription authors can pick one.
	SubscriptionManageOwnAction    = "subscription.manage-own"
	SubscriptionManageGlobalAction = "subscription.manage-global"
)

// maxNameBytes bounds a layout name.
const maxNameBytes = 256

const createOperation = "layout.create"

// Service manages layouts. Layouts write no Domain event. Create it with NewService; it is safe
// for concurrent use.
type Service struct {
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	store        Store
	receipts     commands.Repository
	fields       processing.Declarations
}

// NewService composes the layout service. fields resolves the field references of a layout
// definition when it is saved. It fails with authz.ErrInvalidInput on a missing dependency.
func NewService(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store Store, receipts commands.Repository, fields processing.Declarations) (*Service, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil || receipts == nil || fields == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Service{transactions: transactions, resolver: resolver, clock: clock, store: store,
		receipts: receipts, fields: fields}, nil
}

// caller is what a layout operation needs to know about its caller.
type caller struct {
	principal uuid.UUID
	guest     bool
	manager   bool
	reader    bool
}

// has reports whether the authority holds the action, passing other failures on.
func has(authority authz.Authority, action string, now time.Time) (bool, error) {
	err := authz.Authorize(authority, action, nil, authz.Target{}, now)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, authz.ErrForbidden):
		return false, nil
	}
	return false, err
}

// within resolves the caller and runs operation in the same transaction.
func (s *Service) within(ctx context.Context, proof identity.Proof, mutation bool, check func(identity.Authentication) error,
	operation func(tx platform.TransactionContext, who caller, now time.Time) error) error {
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
			principal, err := uuid.Parse(authority.PrincipalID)
			if err != nil || principal == uuid.Nil {
				return authz.ErrForbidden
			}
			who := caller{principal: principal, guest: authentication.Kind == identity.AnonymousProof}
			if mutation && who.guest {
				return identity.ErrLoginRequired
			}
			if who.manager, err = has(authority, ManageAction, now); err != nil {
				return err
			}
			who.reader = who.manager
			for _, action := range []string{SubscriptionManageOwnAction, SubscriptionManageGlobalAction} {
				if who.reader {
					break
				}
				if who.reader, err = has(authority, action, now); err != nil {
					return err
				}
			}
			if mutation && !who.manager || !mutation && !who.reader {
				return authz.ErrForbidden
			}
			return operation(tx, who, now)
		})
}

// validateName requires valid UTF-8 without NUL, non-blank after trimming and at most 256 bytes.
func validateName(name string) error {
	if !utf8.ValidString(name) || strings.ContainsRune(name, 0) || strings.TrimSpace(name) == "" || len(name) > maxNameBytes {
		return authz.ErrInvalidInput
	}
	return nil
}

// canonical validates a definition structurally and returns its canonical JSON.
func (s *Service) canonical(ctx context.Context, name string, definition processing.Layout) ([]byte, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}
	encoded, err := encode(definition)
	if err != nil {
		return nil, authz.ErrInvalidInput
	}
	layout, err := processing.DecodeLayout(encoded)
	if err != nil {
		return nil, err
	}
	if _, err := processing.CompileLayout(ctx, layout, s.fields); err != nil {
		return nil, err
	}
	return encoded, nil
}

// view builds the response of a stored layout.
func view(record Record) (Layout, error) {
	definition, err := processing.DecodeLayout(record.Definition)
	if err != nil {
		return Layout{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	return Layout{ID: record.ID, Revision: record.Revision, Name: record.Name, OutputKind: record.OutputKind,
		Definition: definition, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}

// List returns one page of layouts ordered by name then ID. The caller needs presentation.manage,
// subscription.manage-own or subscription.manage-global.
func (s *Service) List(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[Layout], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Layout]{}, err
	}
	result := units.Page[Layout]{Items: []Layout{}}
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, _ caller, _ time.Time) error {
		records, total, err := s.store.List(tx.Context(), tx, page.Offset, page.Limit)
		if err != nil {
			return err
		}
		result.Items, result.Total = make([]Layout, 0, len(records)), total
		for _, record := range records {
			item, err := view(record)
			if err != nil {
				return err
			}
			result.Items = append(result.Items, item)
		}
		return nil
	})
	return result, err
}

// Read returns one layout at its current version, or authz.ErrNotFound.
func (s *Service) Read(ctx context.Context, proof identity.Proof, id uuid.UUID) (Layout, error) {
	if id == uuid.Nil {
		return Layout{}, authz.ErrInvalidInput
	}
	var result Layout
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, _ caller, _ time.Time) error {
		record, err := s.store.Get(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		result, err = view(record)
		return err
	})
	return result, err
}

// ReadVersion returns one immutable version of a layout, or authz.ErrNotFound.
func (s *Service) ReadVersion(ctx context.Context, proof identity.Proof, id uuid.UUID, version int64) (LayoutVersion, error) {
	if id == uuid.Nil || version <= 0 {
		return LayoutVersion{}, authz.ErrInvalidInput
	}
	var result LayoutVersion
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, _ caller, _ time.Time) error {
		record, err := s.store.Version(tx.Context(), tx, id, version)
		if err != nil {
			return err
		}
		definition, err := processing.DecodeLayout(record.Definition)
		if err != nil {
			return platform.NewFailure(platform.CodeInternalError, err)
		}
		result = LayoutVersion{LayoutID: record.LayoutID, Version: record.Version, Definition: definition,
			CreatedAt: record.CreatedAt}
		return nil
	})
	return result, err
}

// Create stores a layout at revision 1 with actor-bound command idempotency; its output kind is
// the definition's. An identical replay of the command returns the stored layout as it is now,
// or authz.ErrNotFound if it was deleted since.
func (s *Service) Create(ctx context.Context, proof identity.Proof, command uuid.UUID, input LayoutInput,
	check func(identity.Authentication) error) (Layout, error) {
	if command == uuid.Nil {
		return Layout{}, authz.ErrInvalidInput
	}
	encoded, err := s.canonical(ctx, input.Name, input.Definition)
	if err != nil {
		return Layout{}, err
	}
	fingerprint, err := commands.Fingerprint(struct {
		Name       string `json:"name"`
		Definition string `json:"definition"`
	}{input.Name, string(encoded)})
	if err != nil {
		return Layout{}, err
	}
	var result Layout
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, who.principal, command)
		if err != nil {
			return err
		}
		if found {
			stored, err := receipt.Check(createOperation, fingerprint)
			if err != nil {
				return err
			}
			record, err := s.store.Get(tx.Context(), tx, stored.ID)
			if err != nil {
				return err
			}
			result, err = view(record)
			return err
		}
		record := Record{ID: uuid.New(), Revision: 1, Name: input.Name, OutputKind: input.Definition.OutputKind,
			Definition: encoded, CreatedBy: who.principal, CreatedAt: now, UpdatedAt: now}
		if err := s.store.Insert(tx.Context(), tx, record); err != nil {
			return err
		}
		if err := s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: who.principal,
			CommandID: command, Operation: createOperation, Fingerprint: fingerprint,
			Result: commands.Result{ID: record.ID, Revision: 1}, RecordedAt: now}); err != nil {
			return err
		}
		result, err = view(record)
		return err
	})
	return result, err
}

// Update replaces name and definition at the expected revision and appends a version. The
// definition's output kind must equal the layout's (authz.ErrInvalidInput).
func (s *Service) Update(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64, input LayoutInput,
	check func(identity.Authentication) error) (Layout, error) {
	if id == uuid.Nil || expected <= 0 {
		return Layout{}, authz.ErrInvalidInput
	}
	encoded, err := s.canonical(ctx, input.Name, input.Definition)
	if err != nil {
		return Layout{}, err
	}
	var result Layout
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		record, err := s.store.Get(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(record.Revision, expected)
		if err != nil {
			return err
		}
		if input.Definition.OutputKind != record.OutputKind {
			return authz.ErrInvalidInput
		}
		if err := s.store.Update(tx.Context(), tx, id, expected, input.Name, encoded, who.principal, now); err != nil {
			return err
		}
		record.Name, record.Definition, record.Revision, record.UpdatedAt = input.Name, encoded, next, now
		result, err = view(record)
		return err
	})
	return result, err
}

// Delete removes a layout at the expected revision and returns its ID with the revision it
// reached. A layout that a subscription references is authz.ErrRevisionConflict.
func (s *Service) Delete(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, _ caller, _ time.Time) error {
		record, err := s.store.Get(tx.Context(), tx, id)
		if err != nil {
			return err
		}
		next, err := commands.NextRevision(record.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.store.Delete(tx.Context(), tx, id, expected); err != nil {
			return err
		}
		result = commands.Result{ID: id, Revision: next}
		return nil
	})
	return result, err
}
