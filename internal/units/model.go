// Package units owns Unit master data: Units, their current collection
// membership, Unit collections, Organizations with their setting defaults, and
// standalone Unit templates.
//
// Service (built with NewService) is the entry point for authorized reads and
// revision-controlled mutations; each mutation runs in one platform
// transaction that also records an audit change and appends an
// events.MasterDataFact. Repository, ContextReader and RuleOwner are the ports
// implemented by persistence and the assignment rules owner. Settings,
// Defaults, ResolveSettings and MaterializeSettings implement the Unit setting
// inheritance from Organization defaults, Unit attributes included; AttributeCatalog
// supplies the `unit` Attribute schemas that attribute values are checked against.
package units

import (
	"context"
	"errors"
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
	AgencyID          uuid.UUID         `json:"agencyId"`
	OrganizationID    *uuid.UUID        `json:"organizationId"`
	CollectionIDs     []uuid.UUID       `json:"collectionIds"`
	Settings          Settings          `json:"settings"`
	EffectiveSettings EffectiveSettings `json:"effectiveSettings"`
	Deleted           bool              `json:"deleted"`
}

// BulkUnitItem is one Unit in a bulk operation. Settings is required for
// "update" and must be nil for "delete"; ExpectedRevision must be positive.
type BulkUnitItem struct {
	ID               uuid.UUID `json:"id"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Settings         *Settings `json:"settings,omitempty"`
}

// BulkUnitInput is a bulk Unit operation: Operation is "update" or "delete"
// and Items holds 1 to 100 distinct Units.
type BulkUnitInput struct {
	Operation string         `json:"operation"`
	Items     []BulkUnitItem `json:"items"`
}

// BulkUnitResult is the per-Unit outcome of BulkUnits.
type BulkUnitResult = commands.ResultItem

// BulkItemPreview is the dry-run outcome for one bulk item. When Valid is
// false, Code is NOT_FOUND, REVISION_CONFLICT or INVALID_INPUT.
type BulkItemPreview struct {
	ID               uuid.UUID `json:"id"`
	ExpectedRevision int64     `json:"expectedRevision,string"`
	Valid            bool      `json:"valid"`
	Code             string    `json:"code,omitempty"`
}

// BulkPreview is the result of PreviewBulkUnits; Valid is true only when every
// item is valid. Items are ordered by Unit ID.
type BulkPreview struct {
	Valid bool              `json:"valid"`
	Items []BulkItemPreview `json:"items"`
}

// Context is the internal matcher/authority view, without names or presentation data.
type Context struct {
	ID            uuid.UUID
	Revision      int64
	AgencyID      uuid.UUID
	CollectionIDs []uuid.UUID
	Deleted       bool
}

// Agency is the administrative area (for example a county or a city) that owns Units and
// Radio networks. It is the normal grant scope above a Unit; Organizations and Unit
// collections stay independent of it.
type Agency struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Name     string    `json:"name"`
}

// AgencyInput is the requested state of an Agency. Call Canonical to validate it.
type AgencyInput struct {
	Name string `json:"name"`
}

// Canonical validates the name and returns a canonical copy; it returns
// authz.ErrInvalidInput on invalid input.
func (input AgencyInput) Canonical() (AgencyInput, error) {
	if err := ValidateName(input.Name); err != nil {
		return AgencyInput{}, err
	}
	return input, nil
}

// ErrAgencyInUse reports an Agency deletion while Units or Radio networks still belong to it.
var ErrAgencyInUse = errors.New("the agency still has units or radio networks; reassign them first")

// Collection is a named Unit collection DTO. It carries no member list.
type Collection struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Name     string    `json:"name"`
}

// UnitInput is the requested state of a Unit for create or change. At most one
// of OrganizationID and NewOrganization may be set; NewOrganization (create
// only) creates the Organization in the same transaction. Call Canonical to
// validate it.
type UnitInput struct {
	Name            string             `json:"name"`
	AgencyID        uuid.UUID          `json:"agencyId"`
	OrganizationID  *uuid.UUID         `json:"organizationId"`
	CollectionIDs   []uuid.UUID        `json:"collectionIds"`
	Settings        Settings           `json:"settings"`
	NewOrganization *OrganizationInput `json:"newOrganization,omitempty"`
}

// FieldSetting is one Unit or template setting. Mode is "inherit" (the
// default when empty; use the Organization default), "unset", or "value";
// Value must be set exactly when Mode is "value".
type FieldSetting[T any] struct {
	Mode  string `json:"mode"`
	Value *T     `json:"value,omitempty"`
}

// DefaultSetting is one Organization default. Mode is "unset" or "value" (no
// inheritance); Value must be set exactly when Mode is "value".
type DefaultSetting[T any] struct {
	Mode  string `json:"mode"`
	Value *T     `json:"value,omitempty"`
}

// Settings are the stored per-Unit (or per-template) settings: presentation settings and Unit
// attributes.
type Settings struct {
	Color            FieldSetting[string]         `json:"color"`
	Icon             FieldSetting[string]         `json:"icon"`
	FunctionMeanings FieldSetting[map[int]string] `json:"functionMeanings"`
	Attributes       AttributeSettings            `json:"attributes"`
}

// Defaults are an Organization's setting defaults, inherited by Unit settings
// in "inherit" mode.
type Defaults struct {
	Color            DefaultSetting[string]         `json:"color"`
	Icon             DefaultSetting[string]         `json:"icon"`
	FunctionMeanings DefaultSetting[map[int]string] `json:"functionMeanings"`
	Attributes       AttributeDefaults              `json:"attributes"`
}

// EffectiveValue is a resolved setting value. Source is "unit",
// "organization" or "unset" (Value is nil when unset).
type EffectiveValue[T any] struct {
	Value  *T     `json:"value"`
	Source string `json:"source"`
}

// EffectiveSettings are the resolved settings computed by ResolveSettings.
type EffectiveSettings struct {
	Color            EffectiveValue[string]         `json:"color"`
	Icon             EffectiveValue[string]         `json:"icon"`
	FunctionMeanings EffectiveValue[map[int]string] `json:"functionMeanings"`
	Attributes       EffectiveAttributes            `json:"attributes"`
}

// Organization is an Organization DTO with the setting defaults its Units
// inherit.
type Organization struct {
	ID       uuid.UUID `json:"id"`
	Revision int64     `json:"revision,string"`
	Name     string    `json:"name"`
	Defaults Defaults  `json:"defaults"`
	Deleted  bool      `json:"deleted"`
}

// OrganizationInput is the requested state of an Organization. Call Canonical
// to validate it; all Defaults must have an explicit Mode.
type OrganizationInput struct {
	Name     string   `json:"name"`
	Defaults Defaults `json:"defaults"`
}

// Canonical validates the name and defaults and returns a copy with canonical
// defaults. It returns authz.ErrInvalidInput on invalid input.
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

// UnitTemplate is a standalone Unit template DTO. Units created from it copy
// its current settings and keep no link to it.
type UnitTemplate struct {
	ID                uuid.UUID         `json:"id"`
	Revision          int64             `json:"revision,string"`
	Name              string            `json:"name"`
	OrganizationID    *uuid.UUID        `json:"organizationId"`
	Settings          Settings          `json:"settings"`
	EffectiveSettings EffectiveSettings `json:"effectiveSettings"`
	Deleted           bool              `json:"deleted"`
}

// UnitTemplateInput is the requested state of a Unit template. Call Canonical
// to validate it.
type UnitTemplateInput struct {
	Name           string     `json:"name"`
	OrganizationID *uuid.UUID `json:"organizationId"`
	Settings       Settings   `json:"settings"`
}

// Canonical validates the name, settings and optional Organization reference
// (which must not be the nil UUID) and returns a canonical copy. It returns
// authz.ErrInvalidInput on invalid input.
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

// PageInput is a list page request; call Validate to apply defaults and bounds.
type PageInput struct {
	Offset int
	Limit  int
}

// Page is one page of authorized list results; Total counts all permitted
// items, not only those on the page.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// Validate returns the page with a zero Limit replaced by the default of 50. It
// returns authz.ErrInvalidInput when Offset is negative or Limit is outside
// 1..200.
func (p PageInput) Validate() (PageInput, error) {
	if p.Limit == 0 {
		p.Limit = 50
	}
	if p.Offset < 0 || p.Limit < 1 || p.Limit > 200 {
		return PageInput{}, authz.ErrInvalidInput
	}
	return p, nil
}

// ValidateName returns authz.ErrInvalidInput when name is not valid UTF-8, is
// blank, or exceeds 256 runes. It does not trim or otherwise alter the name.
func ValidateName(name string) error {
	if !utf8.ValidString(name) || strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 256 {
		return authz.ErrInvalidInput
	}
	return nil
}

// Canonical validates Unit input and returns a copy with canonical settings and
// CollectionIDs sorted (never nil). It returns authz.ErrInvalidInput for an
// invalid name or settings, nil-UUID references, both OrganizationID and
// NewOrganization set, duplicate IDs, or more than 1024 collections.
func (input UnitInput) Canonical() (UnitInput, error) {
	if err := ValidateName(input.Name); err != nil {
		return UnitInput{}, err
	}
	if input.AgencyID == uuid.Nil {
		return UnitInput{}, authz.ErrInvalidInput
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

// RuleOwner durably invalidates a deleted Unit's Address rules, never foreign writes.
type RuleOwner interface {
	InvalidateUnit(context.Context, platform.TransactionContext, uuid.UUID) error
}

// Repository applies native authority filters before public read/count/page or mutation.
type Repository interface {
	ContextReader
	ReadAgency(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, time.Time) (Agency, error)
	ListAgencies(context.Context, platform.TransactionContext, authz.Authority, PageInput, time.Time) (Page[Agency], error)
	SaveAgency(context.Context, platform.TransactionContext, authz.Authority, Agency, int64, time.Time) error
	// DeleteAgency fails with ErrAgencyInUse while Units or Radio networks belong to the Agency.
	DeleteAgency(context.Context, platform.TransactionContext, authz.Authority, Agency, time.Time) error
	// LockAgency holds the Agency against deletion for the transaction; a missing one is authz.ErrInvalidInput.
	LockAgency(context.Context, platform.TransactionContext, uuid.UUID) error
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
