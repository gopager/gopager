package readmodel

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// Saved view ownership values.
const (
	// OwnershipPersonal marks a view that belongs to exactly one Principal.
	OwnershipPersonal = "personal"
	// OwnershipGlobal marks a view that every Message reader sees and a global manager maintains.
	OwnershipGlobal = "global"
)

// maxViewNameRunes bounds a saved view name in Unicode scalars.
const maxViewNameRunes = 256

// DefinitionVersion is the only supported schemaVersion of a saved view definition.
const DefinitionVersion = 1

// DefinitionFilters are the feed filter dimensions of a saved view, as arrays (Text as a string,
// WithoutUnit as a boolean). Their limits are those of the feed filters.
type DefinitionFilters struct {
	Text          string      `json:"text,omitempty"`
	Units         []uuid.UUID `json:"units,omitempty"`
	WithoutUnit   bool        `json:"withoutUnit,omitempty"`
	Agencies      []uuid.UUID `json:"agencies,omitempty"`
	Organizations []uuid.UUID `json:"organizations,omitempty"`
	Networks      []uuid.UUID `json:"networks,omitempty"`
	Protocols     []string    `json:"protocols,omitempty"`
	// Filter is the typed predicate over Message fields.
	Filter *processing.Predicate `json:"filter,omitempty"`
}

// Definition is the stored filter set of a saved view: the feed filters plus an optional
// Message time range.
type Definition struct {
	SchemaVersion int               `json:"schemaVersion"`
	Filters       DefinitionFilters `json:"filters"`
	From          *time.Time        `json:"from,omitempty"`
	Until         *time.Time        `json:"until,omitempty"`
}

// ParseDefinition decodes a definition strictly: unknown fields, trailing data and an
// unsupported schemaVersion are authz.ErrInvalidInput. It does not validate the filter limits;
// call Normalize on the result.
func ParseDefinition(raw []byte) (Definition, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var definition Definition
	if err := decoder.Decode(&definition); err != nil || decoder.More() {
		return Definition{}, authz.ErrInvalidInput
	}
	if definition.SchemaVersion != DefinitionVersion {
		return Definition{}, authz.ErrInvalidInput
	}
	return definition, nil
}

// FeedFilters converts the definition to feed filters without validating them.
func (d Definition) FeedFilters() Filters {
	filters := Filters{From: d.From, Until: d.Until, Text: d.Filters.Text, Units: d.Filters.Units,
		Organizations: d.Filters.Organizations, Agencies: d.Filters.Agencies, Networks: d.Filters.Networks,
		Protocols: d.Filters.Protocols, WithoutUnit: d.Filters.WithoutUnit}
	filters.Filter = d.Filters.Filter
	return filters
}

// Normalize validates the definition with the limits of the feed filters and returns it in
// canonical form (sorted, de-duplicated lists, UTC times) with the validated filters. Failures
// are authz.ErrInvalidInput.
func (d Definition) Normalize() (Definition, Filters, error) {
	if d.SchemaVersion != DefinitionVersion {
		return Definition{}, Filters{}, authz.ErrInvalidInput
	}
	filters, err := d.FeedFilters().Validate()
	if err != nil {
		return Definition{}, Filters{}, err
	}
	return definitionOf(filters), filters, nil
}

// definitionOf is the canonical definition of validated filters.
func definitionOf(filters Filters) Definition {
	definition := Definition{SchemaVersion: DefinitionVersion, From: filters.From, Until: filters.Until,
		Filters: DefinitionFilters{Text: filters.Text, Units: filters.Units, WithoutUnit: filters.WithoutUnit,
			Agencies: filters.Agencies, Organizations: filters.Organizations, Networks: filters.Networks,
			Protocols: filters.Protocols, Filter: filters.Filter}}
	return definition
}

// ValidateViewName rejects a name that is not valid UTF-8, contains NUL, is blank after trimming
// or exceeds 256 Unicode scalars. The name is stored as given.
func ValidateViewName(name string) error {
	if !utf8.ValidString(name) || strings.ContainsRune(name, 0) || strings.TrimSpace(name) == "" ||
		utf8.RuneCountInString(name) > maxViewNameRunes {
		return authz.ErrInvalidInput
	}
	return nil
}

// dimensionGroups maps each filter dimension (named like its feed query parameter) to the field
// group that must be readable to use it.
var dimensionGroups = []struct{ dimension, group string }{
	{"agency", "unit"}, {"network", "gateway"}, {"organization", "unit"},
	{"protocol", "payload"}, {"text", "payload"}, {"unit", "unit"}, {"withoutUnit", "unit"},
}

// UnavailableDimensions lists, sorted, the filter dimensions in use whose field group
// the can predicate reports as unreadable. A client marks such a saved view unavailable instead
// of dropping the dimensions. The `filter` dimension is unavailable when any field group of the
// compiled predicate is unreadable; selection is nil when the predicate does not compile now,
// which makes the dimension unavailable too.
func (f Filters) UnavailableDimensions(can func(group string) bool, selection *fields.Filter) []string {
	used := map[string]bool{"agency": len(f.Agencies) > 0,
		"network": len(f.Networks) > 0, "organization": len(f.Organizations) > 0,
		"protocol": len(f.Protocols) > 0, "text": f.Text != "", "unit": len(f.Units) > 0,
		"withoutUnit": f.WithoutUnit}
	unavailable := []string{}
	for _, entry := range dimensionGroups {
		if used[entry.dimension] && !can(entry.group) {
			unavailable = append(unavailable, entry.dimension)
		}
	}
	if f.Filter != nil && (selection == nil || slices.ContainsFunc(selection.Groups(), func(group string) bool { return !can(group) })) {
		unavailable = append(unavailable, "filter")
	}
	slices.Sort(unavailable)
	return unavailable
}

// SavedView is a saved view as returned to a caller.
type SavedView struct {
	ID        uuid.UUID `json:"id"`
	Revision  int64     `json:"revision,string"`
	Ownership string    `json:"ownership"`
	Name      string    `json:"name"`
	// Definition is the stored filter set.
	Definition Definition `json:"definition"`
	// UnavailableFilters names the filter dimensions whose field group the caller cannot read now.
	UnavailableFilters []string  `json:"unavailableFilters"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// SavedViewInput is the writable part of a saved view. Ownership is fixed at creation.
type SavedViewInput struct {
	Ownership  string
	Name       string
	Definition Definition
}
