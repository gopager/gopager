// Package readmodel owns the authorized Message read side: feed pages, details, filter options
// and the filter, token and projection rules they share with live updates and saved views.
// It holds no SQL; a Store port supplies the rows that the caller's authority makes eligible.
package readmodel

import (
	"crypto/sha256"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

// Limits of the filter vocabulary.
const (
	// MaxFilterValues bounds the values of one repeatable dimension.
	MaxFilterValues = 100
	// MaxTextBytes bounds the text search term in bytes.
	MaxTextBytes = 1024
	// MaxFilterBytes bounds the encoded `filter` predicate in bytes.
	MaxFilterBytes = 32 * 1024
)

// protocols are the Protocols a Network can carry.
var protocols = []string{"pocsag", "flex", "zvei", "eas"}

// Filters are the Message feed filters shared by the feed, the live stream and saved views.
// Values of one dimension are alternatives; the dimensions combine with AND. The zero value
// selects every eligible Message. Construct them with ParseFilters or normalize hand-built
// ones with Validate.
type Filters struct {
	// From and Until bound the Message time as [From, Until).
	From, Until   *time.Time
	Text          string
	Units         []uuid.UUID
	Organizations []uuid.UUID
	Agencies      []uuid.UUID
	Networks      []uuid.UUID
	Protocols     []string
	// Filter is the typed predicate over field-registry fields; it combines with the other
	// dimensions by AND. It is checked against a registry by CompileSelection.
	Filter *processing.Predicate
	// WithoutUnit adds Unit-less appearances as an alternative of the Unit dimension.
	WithoutUnit bool
}

// filterParameters are the query parameters that carry filters.
var filterParameters = []string{"from", "until", "text", "unit", "organization", "agency", "network",
	"protocol", "filter", "withoutUnit"}

// scalarParameters may appear at most once.
var scalarParameters = []string{"from", "until", "text", "filter", "withoutUnit", "pageSize", "pageToken"}

// ParseFilters reads the filter parameters of a query. Any parameter outside the filter
// vocabulary and the names in extra is rejected, as is a repeated scalar. Failures are
// authz.ErrInvalidInput.
func ParseFilters(values url.Values, extra ...string) (Filters, error) {
	for name, entries := range values {
		if !slices.Contains(filterParameters, name) && !slices.Contains(extra, name) {
			return Filters{}, authz.ErrInvalidInput
		}
		if slices.Contains(scalarParameters, name) && len(entries) != 1 {
			return Filters{}, authz.ErrInvalidInput
		}
	}
	var filters Filters
	var err error
	if filters.From, err = parseTime(values, "from"); err != nil {
		return Filters{}, err
	}
	if filters.Until, err = parseTime(values, "until"); err != nil {
		return Filters{}, err
	}
	filters.Text = values.Get("text")
	switch values.Get("withoutUnit") {
	case "", "false":
	case "true":
		filters.WithoutUnit = true
	default:
		return Filters{}, authz.ErrInvalidInput
	}
	for name, target := range map[string]*[]uuid.UUID{"unit": &filters.Units, "organization": &filters.Organizations,
		"agency": &filters.Agencies, "network": &filters.Networks} {
		for _, entry := range values[name] {
			id, err := uuid.Parse(entry)
			if err != nil || id == uuid.Nil {
				return Filters{}, authz.ErrInvalidInput
			}
			*target = append(*target, id)
		}
	}
	filters.Protocols = values["protocol"]
	if values.Has("filter") {
		if filters.Filter, err = parsePredicate(values.Get("filter")); err != nil {
			return Filters{}, err
		}
	}
	return filters.Validate()
}

// parsePredicate decodes the syntax of a `filter` parameter. Which fields exist and how they
// compare is decided later, against a registry.
func parsePredicate(encoded string) (*processing.Predicate, error) {
	if len(encoded) > MaxFilterBytes {
		return nil, authz.ErrInvalidInput
	}
	var predicate processing.Predicate
	if err := processing.Decode([]byte(encoded), &predicate); err != nil {
		return nil, authz.ErrInvalidInput
	}
	return &predicate, nil
}

// parseTime reads an optional RFC 3339 instant.
func parseTime(values url.Values, name string) (*time.Time, error) {
	if !values.Has(name) {
		return nil, nil
	}
	instant, err := time.Parse(time.RFC3339Nano, values.Get(name))
	if err != nil {
		return nil, authz.ErrInvalidInput
	}
	instant = instant.UTC()
	return &instant, nil
}

// Validate checks the limits and the vocabulary and returns the filters in canonical form:
// UTC times, sorted and de-duplicated lists. It does not mutate its receiver.
func (f Filters) Validate() (Filters, error) {
	out := f
	if f.From != nil {
		from := f.From.UTC()
		out.From = &from
	}
	if f.Until != nil {
		until := f.Until.UTC()
		out.Until = &until
	}
	if len(f.Text) > MaxTextBytes || !utf8.ValidString(f.Text) || strings.ContainsRune(f.Text, 0) {
		return Filters{}, authz.ErrInvalidInput
	}
	var err error
	if out.Units, err = canonicalIDs(f.Units); err != nil {
		return Filters{}, err
	}
	if out.Organizations, err = canonicalIDs(f.Organizations); err != nil {
		return Filters{}, err
	}
	if out.Agencies, err = canonicalIDs(f.Agencies); err != nil {
		return Filters{}, err
	}
	if out.Networks, err = canonicalIDs(f.Networks); err != nil {
		return Filters{}, err
	}
	if len(f.Protocols) > MaxFilterValues {
		return Filters{}, authz.ErrInvalidInput
	}
	out.Protocols = slices.Clone(f.Protocols)
	slices.Sort(out.Protocols)
	out.Protocols = slices.Compact(out.Protocols)
	for _, protocol := range out.Protocols {
		if !slices.Contains(protocols, protocol) {
			return Filters{}, authz.ErrInvalidInput
		}
	}
	if f.Filter != nil {
		encoded, err := json.Marshal(f.Filter)
		if err != nil || len(encoded) > MaxFilterBytes {
			return Filters{}, authz.ErrInvalidInput
		}
	}
	return out, nil
}

// canonicalIDs rejects nil IDs and oversized lists, then sorts and de-duplicates.
func canonicalIDs(ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) > MaxFilterValues {
		return nil, authz.ErrInvalidInput
	}
	out := slices.Clone(ids)
	for _, id := range out {
		if id == uuid.Nil {
			return nil, authz.ErrInvalidInput
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return slices.Compact(out), nil
}

// WithoutTime returns the filters without the Message-time bounds: the live stream evaluates
// a Message against the view's other filters because the client places it in its range.
func (f Filters) WithoutTime() Filters {
	f.From, f.Until = nil, nil
	return f
}

// canonicalFilters is the stable encoding that page tokens and saved views bind to.
type canonicalFilters struct {
	Version       int             `json:"v"`
	From          string          `json:"from,omitempty"`
	Until         string          `json:"until,omitempty"`
	Text          string          `json:"text,omitempty"`
	Units         []string        `json:"unit,omitempty"`
	Organizations []string        `json:"organization,omitempty"`
	Agencies      []string        `json:"agency,omitempty"`
	Networks      []string        `json:"network,omitempty"`
	Protocols     []string        `json:"protocol,omitempty"`
	Filter        json.RawMessage `json:"filter,omitempty"`
	WithoutUnit   bool            `json:"withoutUnit,omitempty"`
}

// Canonical returns the deterministic encoding of validated filters.
func (f Filters) Canonical() []byte {
	canonical := canonicalFilters{Version: 2, Text: f.Text, Protocols: f.Protocols, WithoutUnit: f.WithoutUnit,
		Units: idStrings(f.Units), Organizations: idStrings(f.Organizations),
		Agencies: idStrings(f.Agencies), Networks: idStrings(f.Networks)}
	if f.From != nil {
		canonical.From = f.From.UTC().Format(time.RFC3339Nano)
	}
	if f.Until != nil {
		canonical.Until = f.Until.UTC().Format(time.RFC3339Nano)
	}
	if f.Filter != nil {
		canonical.Filter, _ = json.Marshal(f.Filter)
	}
	encoded, _ := json.Marshal(canonical)
	return encoded
}

// Hash is the SHA-256 of the canonical encoding.
func (f Filters) Hash() [32]byte { return sha256.Sum256(f.Canonical()) }

func idStrings(ids []uuid.UUID) []string {
	var out []string
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

// RequiredFieldGroups lists the field groups a caller must be able to read to use the filters:
// text and Protocol need payload, Unit, Organization, Agency and Unit-less need unit, Network
// needs gateway, and the compiled predicate (nil without one) needs the groups of its fields.
func (f Filters) RequiredFieldGroups(selection *fields.Filter) []string {
	var groups []string
	add := func(needed bool, group string) {
		if needed {
			groups = append(groups, group)
		}
	}
	add(f.Text != "" || len(f.Protocols) > 0, "payload")
	add(len(f.Units) > 0 || len(f.Organizations) > 0 || len(f.Agencies) > 0 || f.WithoutUnit, "unit")
	add(len(f.Networks) > 0, "gateway")
	if selection != nil {
		groups = append(groups, selection.Groups()...)
	}
	return groups
}
