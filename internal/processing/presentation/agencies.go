package presentation

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// maxAgencies bounds the Agencies of one layout.
const maxAgencies = 256

// validateAgencies requires 1 to 256 distinct, non-nil Agencies.
func validateAgencies(agencies []uuid.UUID) error {
	if len(agencies) == 0 || len(agencies) > maxAgencies {
		return authz.ErrInvalidInput
	}
	seen := make(map[uuid.UUID]bool, len(agencies))
	for _, agency := range agencies {
		if agency == uuid.Nil || seen[agency] {
			return authz.ErrInvalidInput
		}
		seen[agency] = true
	}
	return nil
}

// sortedAgencies returns a sorted copy, the order the store returns and fingerprints use.
func sortedAgencies(agencies []uuid.UUID) []uuid.UUID {
	out := slices.Clone(agencies)
	slices.SortFunc(out, compareUUID)
	return out
}

// requireManage requires ManageAction in every one of the Agencies, so nobody can change a
// layout that also belongs to an Agency they do not manage.
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

// visibleRecord loads a layout the caller may see; a layout in no visible Agency does not
// exist for the caller (authz.ErrNotFound).
func (s *Service) visibleRecord(ctx context.Context, tx platform.TransactionContext, who caller, id uuid.UUID) (Record, error) {
	visibility, err := who.visibility()
	if err != nil {
		return Record{}, err
	}
	record, err := s.store.Get(ctx, tx, id)
	if err != nil {
		return Record{}, err
	}
	visible, err := s.store.Visible(ctx, tx, id, visibility)
	if err != nil {
		return Record{}, err
	}
	if !visible {
		return Record{}, authz.ErrNotFound
	}
	return record, nil
}

// CheckUse reports, as authz.ErrInvalidInput, that the layout does not exist or that the
// authority may not use it: it must be visible in the way Read requires. A Subscription save
// calls it in its own transaction. The time is the decision time of the caller's transaction.
func (s *Service) CheckUse(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	layoutID uuid.UUID, _ time.Time) error {
	return checkUse(ctx, tx, s.store, authority, layoutID)
}

// CheckUse reports, as authz.ErrInvalidInput, that the layout does not exist or that the
// authority may not use it, like Service.CheckUse.
func (r *Resolver) CheckUse(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	layoutID uuid.UUID, _ time.Time) error {
	return checkUse(ctx, tx, r.store, authority, layoutID)
}

func checkUse(ctx context.Context, tx platform.TransactionContext, store Store, authority authz.Authority,
	layoutID uuid.UUID) error {
	visibility, err := authz.VisibleAgencies(authority, readerActions, false)
	if err != nil {
		return authz.ErrInvalidInput
	}
	visible, err := store.Visible(ctx, tx, layoutID, visibility)
	if err != nil {
		return err
	}
	if !visible {
		return authz.ErrInvalidInput
	}
	return nil
}
