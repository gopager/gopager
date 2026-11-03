// Package units owns Unit identity, current collection membership and master-data ports.
package units

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// Unit is the early master-data DTO, not a replacement for later presentation configuration.
type Unit struct {
	ID            uuid.UUID   `json:"id"`
	Revision      int64       `json:"revision,string"`
	Name          string      `json:"name"`
	CollectionIDs []uuid.UUID `json:"collectionIds"`
	Deleted       bool        `json:"deleted"`
}

// Context is the internal matcher/authority view, without names or presentation data.
type Context struct {
	ID            uuid.UUID
	Revision      int64
	CollectionIDs []uuid.UUID
	Deleted       bool
}

type Collection struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Name     string    `json:"name"`
}

type UnitInput struct {
	Name          string      `json:"name"`
	CollectionIDs []uuid.UUID `json:"collectionIds"`
}

type PageInput struct {
	Offset int
	Limit  int
}

type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Validate accepts the shared bounded page shape; zero Limit selects the existing default.
func (p PageInput) Validate() (PageInput, error) {
	if p.Limit == 0 {
		p.Limit = 50
	}
	if p.Offset < 0 || p.Limit < 1 || p.Limit > 200 {
		return PageInput{}, authz.ErrInvalidInput
	}
	return p, nil
}

// ValidateName preserves spelling and rejects only invalid/empty/over-limit names.
func ValidateName(name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 256 {
		return authz.ErrInvalidInput
	}
	return nil
}

// Canonical validates Unit input and canonicalizes its set-valued membership list.
func (input UnitInput) Canonical() (UnitInput, error) {
	if err := ValidateName(input.Name); err != nil {
		return UnitInput{}, err
	}
	if len(input.CollectionIDs) > 1024 {
		return UnitInput{}, authz.ErrInvalidInput
	}
	input.CollectionIDs = slices.Clone(input.CollectionIDs)
	slices.SortFunc(input.CollectionIDs, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	for i, id := range input.CollectionIDs {
		if id == uuid.Nil || (i > 0 && id == input.CollectionIDs[i-1]) {
			return UnitInput{}, authz.ErrInvalidInput
		}
	}
	if input.CollectionIDs == nil {
		input.CollectionIDs = []uuid.UUID{}
	}
	return input, nil
}

// ContextReader supplies current owner context inside the caller's live transaction.
type ContextReader interface {
	ReadUnitContext(context.Context, platform.TransactionContext, uuid.UUID) (Context, error)
}

// RuleOwner coordinates current Network uses and durable invalidation, never foreign writes.
type RuleOwner interface {
	NetworksForUnit(context.Context, platform.TransactionContext, uuid.UUID) ([]uuid.UUID, error)
	InvalidateUnit(context.Context, platform.TransactionContext, uuid.UUID) error
}

// Repository applies native authority filters before public read/count/page or mutation.
type Repository interface {
	ContextReader
	ReadUnit(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, time.Time) (Unit, error)
	ListUnits(context.Context, platform.TransactionContext, authz.Authority, PageInput, time.Time) (Page[Unit], error)
	SaveUnit(context.Context, platform.TransactionContext, authz.Authority, Unit, int64, time.Time) error
	ReadCollection(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, time.Time) (Collection, error)
	ListCollections(context.Context, platform.TransactionContext, authz.Authority, PageInput, time.Time) (Page[Collection], error)
	SaveCollection(context.Context, platform.TransactionContext, authz.Authority, Collection, int64, time.Time) error
	CollectionUnits(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID) ([]Unit, error)
	DeleteCollection(context.Context, platform.TransactionContext, authz.Authority, Collection, time.Time) error
	RecordChange(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID, string, time.Time) error
}
