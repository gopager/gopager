package attributes

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// ManageAction creates, changes and deletes Attribute schemas. It is checked in every Agency a
// schema is assigned to.
const ManageAction = "attribute-schema.manage"

// readerActions are the actions of which any one makes a schema visible: the rights whose
// holders pick or fill attributes. Their scopes are mapped to Agencies; a schema carries no
// secret, so account-scoped rights see every schema.
var readerActions = []string{"message.read", "workflow.read", "presentation.manage", "subscription.manage-own",
	"subscription.manage-global", "saved-view.manage-global", "unit.update", ManageAction}

const createOperation = "attribute-schema.create"

// Service manages Attribute schemas. Create it with NewService; it is safe for concurrent use.
type Service struct {
	transactions identity.Transactions
	resolver     identity.AuthorityResolver
	clock        platform.Clock
	store        Store
	receipts     commands.Repository
}

// NewService composes the schema service. It fails with authz.ErrInvalidInput on a missing
// dependency.
func NewService(transactions identity.Transactions, resolver identity.AuthorityResolver, clock platform.Clock,
	store Store, receipts commands.Repository) (*Service, error) {
	if transactions == nil || resolver == nil || clock == nil || store == nil || receipts == nil {
		return nil, authz.ErrInvalidInput
	}
	return &Service{transactions: transactions, resolver: resolver, clock: clock, store: store, receipts: receipts}, nil
}

// SchemaInput is the editable content of a schema. The key is fixed at creation.
type SchemaInput struct {
	Key         string
	Label       string
	Description string
	Subject     Subject
	Fields      map[string]Field
	Agencies    []uuid.UUID
	Deprecated  bool
}

// schema builds the schema a valid input describes, with its Agencies sorted.
func (i SchemaInput) schema(key string) Schema {
	return Schema{Key: key, Label: i.Label, Description: i.Description, Subject: i.Subject, Fields: i.Fields,
		Agencies: sortedAgencies(i.Agencies), Deprecated: i.Deprecated}
}

// sortedAgencies returns a sorted copy, the order the store returns and fingerprints use.
func sortedAgencies(agencies []uuid.UUID) []uuid.UUID {
	out := slices.Clone(agencies)
	slices.SortFunc(out, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return out
}

// caller is what a schema operation needs to know about its caller.
type caller struct {
	principal uuid.UUID
	authority authz.Authority
}

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
			if mutation && authentication.Kind == identity.AnonymousProof {
				return identity.ErrLoginRequired
			}
			return operation(tx, caller{principal: principal, authority: authority}, now)
		})
}

// requireManage requires ManageAction in every one of the Agencies, so nobody can change a
// schema that also belongs to an Agency they do not manage.
func requireManage(authority authz.Authority, now time.Time, agencies ...[]uuid.UUID) error {
	for _, set := range agencies {
		for _, agency := range set {
			if err := authz.Authorize(authority, ManageAction, nil, authz.Target{AgencyID: agency}, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// visibleSchema loads a schema the caller may see; an invisible schema does not exist for the
// caller (authz.ErrNotFound).
func (s *Service) visibleSchema(ctx context.Context, tx platform.TransactionContext, who caller, key string) (Schema, error) {
	if !ValidKey(key) {
		return Schema{}, authz.ErrInvalidInput
	}
	visibility, err := authz.VisibleAgencies(who.authority, readerActions, true)
	if err != nil {
		return Schema{}, err
	}
	schema, err := s.store.Get(ctx, tx, key)
	if err != nil {
		return Schema{}, err
	}
	visible, err := s.store.Visible(ctx, tx, key, visibility)
	if err != nil {
		return Schema{}, err
	}
	if !visible {
		return Schema{}, authz.ErrNotFound
	}
	return schema, nil
}

// List returns one page of the visible schemas ordered by key, optionally only those assigned
// to the Agency. A schema is visible when any right that picks or fills attributes reaches one
// of its Agencies.
func (s *Service) List(ctx context.Context, proof identity.Proof, agency *uuid.UUID, page units.PageInput) (units.Page[Schema], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Schema]{}, err
	}
	result := units.Page[Schema]{Items: []Schema{}}
	err = s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who caller, _ time.Time) error {
		visibility, err := authz.VisibleAgencies(who.authority, readerActions, true)
		if err != nil {
			return err
		}
		items, total, err := s.store.List(tx.Context(), tx, visibility, agency, page.Offset, page.Limit)
		if err != nil {
			return err
		}
		result.Items, result.Total = append(result.Items, items...), total
		return nil
	})
	return result, err
}

// Read returns one visible schema, or authz.ErrNotFound.
func (s *Service) Read(ctx context.Context, proof identity.Proof, key string) (Schema, error) {
	var result Schema
	err := s.within(ctx, proof, false, nil, func(tx platform.TransactionContext, who caller, _ time.Time) error {
		var err error
		result, err = s.visibleSchema(tx.Context(), tx, who, key)
		return err
	})
	return result, err
}

// Create stores a schema at revision 1 with actor-bound command idempotency. The caller needs
// ManageAction in every Agency of the schema. An existing key is authz.ErrRevisionConflict. An
// identical replay of the command returns the stored schema as it is now, or authz.ErrNotFound
// if it was deleted since.
func (s *Service) Create(ctx context.Context, proof identity.Proof, command uuid.UUID, input SchemaInput,
	check func(identity.Authentication) error) (Schema, error) {
	if command == uuid.Nil {
		return Schema{}, authz.ErrInvalidInput
	}
	schema := input.schema(input.Key)
	if err := schema.Validate(); err != nil {
		return Schema{}, err
	}
	fingerprint, err := commands.Fingerprint(schema)
	if err != nil {
		return Schema{}, err
	}
	var result Schema
	err = s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		if err := requireManage(who.authority, now, schema.Agencies); err != nil {
			return err
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, who.principal, command)
		if err != nil {
			return err
		}
		if found {
			if _, err := receipt.Check(createOperation, fingerprint); err != nil {
				return err
			}
			result, err = s.store.Get(tx.Context(), tx, schema.Key)
			return err
		}
		schema.Revision, schema.CreatedAt, schema.UpdatedAt = 1, now, now
		if err := s.store.Insert(tx.Context(), tx, schema, who.principal); err != nil {
			return err
		}
		// The receipt keeps a UUID; a key is a string, and a replay carries the same key.
		if err := s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: who.principal,
			CommandID: command, Operation: createOperation, Fingerprint: fingerprint,
			Result: commands.Result{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(schema.Key)), Revision: 1}, RecordedAt: now}); err != nil {
			return err
		}
		result = schema
		return nil
	})
	return result, err
}

// Update replaces label, description, fields, deprecation and Agencies at the expected revision.
// The change must keep the schema compatible (CheckChange), and the caller needs ManageAction in
// the stored and the new Agencies.
func (s *Service) Update(ctx context.Context, proof identity.Proof, key string, expected int64, input SchemaInput,
	check func(identity.Authentication) error) (Schema, error) {
	if !ValidKey(key) || expected <= 0 {
		return Schema{}, authz.ErrInvalidInput
	}
	next := input.schema(key)
	if err := next.Validate(); err != nil {
		return Schema{}, err
	}
	var result Schema
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		current, err := s.visibleSchema(tx.Context(), tx, who, key)
		if err != nil {
			return err
		}
		if err := requireManage(who.authority, now, current.Agencies, next.Agencies); err != nil {
			return err
		}
		revision, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := CheckChange(current, next); err != nil {
			return err
		}
		next.Revision, next.CreatedAt, next.UpdatedAt = revision, current.CreatedAt, now
		if err := s.store.Update(tx.Context(), tx, next, expected); err != nil {
			return err
		}
		result = next
		return nil
	})
	return result, err
}

// Delete removes a schema at the expected revision and returns its revision reached. The caller
// needs ManageAction in every Agency of the schema. A schema that is still referenced is
// authz.ErrRevisionConflict.
func (s *Service) Delete(ctx context.Context, proof identity.Proof, key string, expected int64,
	check func(identity.Authentication) error) (Deleted, error) {
	if !ValidKey(key) || expected <= 0 {
		return Deleted{}, authz.ErrInvalidInput
	}
	var result Deleted
	err := s.within(ctx, proof, true, check, func(tx platform.TransactionContext, who caller, now time.Time) error {
		current, err := s.visibleSchema(tx.Context(), tx, who, key)
		if err != nil {
			return err
		}
		if err := requireManage(who.authority, now, current.Agencies); err != nil {
			return err
		}
		revision, err := commands.NextRevision(current.Revision, expected)
		if err != nil {
			return err
		}
		if err := s.store.Delete(tx.Context(), tx, key, expected); err != nil {
			return err
		}
		result = Deleted{Key: key, Revision: revision}
		return nil
	})
	return result, err
}

// Deleted is the result of a delete: the key and the revision the schema reached.
type Deleted struct {
	Key      string `json:"key"`
	Revision int64  `json:"revision,string"`
}
