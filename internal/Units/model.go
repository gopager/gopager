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
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/platform"
)

// Unit is the early master-data DTO, not a replacement for later presentation configuration.
type Unit struct {
	ID                uuid.UUID         `json:"id"`
	Revision          int64             `json:"revision,string"`
	Name              string            `json:"name"`
	OrganizationID    *uuid.UUID        `json:"organizationId"`
	CollectionIDs     []uuid.UUID       `json:"collectionIds"`
	Settings          Settings          `json:"settings"`
	EffectiveSettings EffectiveSettings `json:"effectiveSettings"`
	Deleted           bool              `json:"deleted"`
}

type BulkUnitItem struct {
	ID               uuid.UUID `json:"id"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Settings         *Settings `json:"settings,omitempty"`
}

type BulkUnitInput struct {
	Operation string         `json:"operation"`
	Items     []BulkUnitItem `json:"items"`
}

type BulkUnitResult = commands.ResultItem

type BulkItemPreview struct {
	ID               uuid.UUID `json:"id"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Valid            bool      `json:"valid"`
	Code             string    `json:"code,omitempty"`
}

type BulkPreview struct {
	Valid bool              `json:"valid"`
	Items []BulkItemPreview `json:"items"`
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
	Name            string             `json:"name"`
	OrganizationID  *uuid.UUID         `json:"organizationId"`
	CollectionIDs   []uuid.UUID        `json:"collectionIds"`
	Settings        Settings           `json:"settings"`
	NewOrganization *OrganizationInput `json:"newOrganization,omitempty"`
}

type FieldSetting[T any] struct {
	Mode  string `json:"mode"`
	Value *T     `json:"value,omitempty"`
}

type DefaultSetting[T any] struct {
	Mode  string `json:"mode"`
	Value *T     `json:"value,omitempty"`
}

type Settings struct {
	Color            FieldSetting[string]         `json:"color"`
	Icon             FieldSetting[string]         `json:"icon"`
	FunctionMeanings FieldSetting[map[int]string] `json:"functionMeanings"`
}

type Defaults struct {
	Color            DefaultSetting[string]         `json:"color"`
	Icon             DefaultSetting[string]         `json:"icon"`
	FunctionMeanings DefaultSetting[map[int]string] `json:"functionMeanings"`
}

type EffectiveValue[T any] struct {
	Value  *T     `json:"value"`
	Source string `json:"source"`
}

type EffectiveSettings struct {
	Color            EffectiveValue[string]         `json:"color"`
	Icon             EffectiveValue[string]         `json:"icon"`
	FunctionMeanings EffectiveValue[map[int]string] `json:"functionMeanings"`
}

type Organization struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Name     string    `json:"name"`
	Defaults Defaults  `json:"defaults"`
	Deleted  bool      `json:"deleted"`
}

type OrganizationInput struct {
	Name     string   `json:"name"`
	Defaults Defaults `json:"defaults"`
}

func (input OrganizationInput) Canonical() (OrganizationInput, error) {
	if err := ValidateName(input.Name); err != nil {
		return OrganizationInput{}, err
	}
	defaults, err := input.Defaults.Canonical()
	if err != nil {
		return OrganizationInput{}, err
	}
	input.Defaults = defaults
	return input, nil
}

type UnitTemplate struct {
	ID                uuid.UUID         `json:"id"`
	Revision          int64             `json:"revision,string"`
	Name              string            `json:"name"`
	OrganizationID    *uuid.UUID        `json:"organizationId"`
	Settings          Settings          `json:"settings"`
	EffectiveSettings EffectiveSettings `json:"effectiveSettings"`
	Deleted           bool              `json:"deleted"`
}

type UnitTemplateInput struct {
	Name           string     `json:"name"`
	OrganizationID *uuid.UUID `json:"organizationId"`
	Settings       Settings   `json:"settings"`
}

func (input UnitTemplateInput) Canonical() (UnitTemplateInput, error) {
	if err := ValidateName(input.Name); err != nil {
		return UnitTemplateInput{}, err
	}
	settings, err := input.Settings.Canonical()
	if err != nil {
		return UnitTemplateInput{}, err
	}
	input.Settings = settings
	if input.OrganizationID != nil {
		if *input.OrganizationID == uuid.Nil {
			return UnitTemplateInput{}, authz.ErrInvalidInput
		}
		id := *input.OrganizationID
		input.OrganizationID = &id
	}
	return input, nil
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
	settings, err := input.Settings.Canonical()
	if err != nil {
		return UnitInput{}, err
	}
	input.Settings = settings
	if input.OrganizationID != nil {
		if *input.OrganizationID == uuid.Nil {
			return UnitInput{}, authz.ErrInvalidInput
		}
		organizationID := *input.OrganizationID
		input.OrganizationID = &organizationID
	}
	if input.NewOrganization != nil {
		if input.OrganizationID != nil {
			return UnitInput{}, authz.ErrInvalidInput
		}
		organization, err := input.NewOrganization.Canonical()
		if err != nil {
			return UnitInput{}, err
		}
		input.NewOrganization = &organization
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
	ReadOrganizationContext(context.Context, platform.TransactionContext, uuid.UUID) (Organization, []Unit, []UnitTemplate, error)
	ReadOrganization(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, time.Time) (Organization, error)
	ListOrganizations(context.Context, platform.TransactionContext, authz.Authority, PageInput, time.Time) (Page[Organization], error)
	SaveOrganization(context.Context, platform.TransactionContext, authz.Authority, Organization, int64, time.Time) error
	DeleteOrganization(context.Context, platform.TransactionContext, authz.Authority, Organization, int64, time.Time) error
	ReadUnitTemplate(context.Context, platform.TransactionContext, uuid.UUID) (UnitTemplate, error)
	ListUnitTemplates(context.Context, platform.TransactionContext, PageInput) (Page[UnitTemplate], error)
	SaveUnitTemplate(context.Context, platform.TransactionContext, UnitTemplate, int64) error
	LockUnits(context.Context, platform.TransactionContext, authz.Authority, string, []uuid.UUID, time.Time) ([]Unit, error)
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
