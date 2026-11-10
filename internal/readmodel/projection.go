package readmodel

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

// FeedItem is the feed projection of a Message. The identifying metadata is always present;
// every other property appears only when the caller may read its field group, so an
// unreadable group is absent rather than empty.
type FeedItem struct {
	MessageID         uuid.UUID       `json:"messageId"`
	MessageTime       time.Time       `json:"messageTime"`
	ProcessingStatus  string          `json:"processingStatus"`
	CorrectionStatus  string          `json:"correctionStatus"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Protocol          *string         `json:"protocol,omitempty"`
	Incomplete        *bool           `json:"incomplete,omitempty"`
	Units             *[]UnitItem     `json:"units,omitempty"`
	WithoutUnit       *bool           `json:"withoutUnit,omitempty"`
	Derived           *[]DerivedItem  `json:"derived,omitempty"`
	NetworkUnresolved *bool           `json:"networkUnresolved,omitempty"`
}

// UnitItem is an eligible Unit appearance. FirstObservedAt appears in details only.
type UnitItem struct {
	UnitID          uuid.UUID  `json:"unitId"`
	Name            string     `json:"name"`
	AgencyID        uuid.UUID  `json:"agencyId"`
	OrganizationID  *uuid.UUID `json:"organizationId,omitempty"`
	Encrypted       bool       `json:"encrypted"`
	FirstObservedAt *time.Time `json:"firstObservedAt,omitempty"`
}

// WithoutUnitItem is an eligible Unit-less appearance of a detail.
type WithoutUnitItem struct {
	AgencyID        uuid.UUID `json:"agencyId"`
	FirstObservedAt time.Time `json:"firstObservedAt"`
}

// DerivedItem is the latest published Processing result of one Workflow. Fields are the
// outputs of a Transmission-level Workflow, TargetResults those of a target-level one,
// limited to the targets the caller may see.
type DerivedItem struct {
	WorkflowID    uuid.UUID               `json:"workflowId"`
	ResultID      uuid.UUID               `json:"resultId"`
	Fields        map[string]DerivedField `json:"fields,omitempty"`
	TargetResults []DerivedTargetResult   `json:"targetResults,omitempty"`
}

// DerivedField is one output: its state and typed value.
type DerivedField struct {
	State string          `json:"state"`
	Value json.RawMessage `json:"value,omitempty"`
}

// DerivedTargetResult is the outputs for one original target position.
type DerivedTargetResult struct {
	TargetPosition int                     `json:"targetPosition"`
	Fields         map[string]DerivedField `json:"fields"`
}

// DetailItem is the detail projection: the feed item plus per-appearance first times, the
// Unit-less appearances and the supporting readable Transmissions.
type DetailItem struct {
	FeedItem
	WithoutUnitAppearances *[]WithoutUnitItem `json:"withoutUnitAppearances,omitempty"`
	Transmissions          []TransmissionItem `json:"transmissions"`
}

// TransmissionItem is a supporting readable Transmission of a detail.
type TransmissionItem struct {
	TransmissionID    uuid.UUID       `json:"transmissionId"`
	AcceptedAt        time.Time       `json:"acceptedAt"`
	ProcessingStatus  string          `json:"processingStatus"`
	ObservedAt        *time.Time      `json:"observedAt,omitempty"`
	NetworkID         *uuid.UUID      `json:"networkId,omitempty"`
	NetworkName       *string         `json:"networkName,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Protocol          *string         `json:"protocol,omitempty"`
	Incomplete        *bool           `json:"incomplete,omitempty"`
	QualityMetrics    json.RawMessage `json:"qualityMetrics,omitempty"`
	NetworkUnresolved *bool           `json:"networkUnresolved,omitempty"`
	Targets           []TargetItem    `json:"targets"`
}

// TargetItem is a visible, unsuppressed target. Resolution tells a failed attempt from a
// missing match: assigned, unassigned, pending, failed or unresolved.
type TargetItem struct {
	Position   int        `json:"position"`
	Resolution string     `json:"resolution"`
	UnitID     *uuid.UUID `json:"unitId,omitempty"`
	Encrypted  *bool      `json:"encrypted,omitempty"`
	Address    any        `json:"address,omitempty"`
	Function   *int       `json:"function,omitempty"`
}

// OptionsItem lists the filter options per family; a family the caller's field groups do not
// allow is absent.
type OptionsItem struct {
	Units         *[]UnitOptionItem `json:"units,omitempty"`
	Organizations *[]NamedItem      `json:"organizations,omitempty"`
	Agencies      *[]NamedItem      `json:"agencies,omitempty"`
	Networks      *[]NamedItem      `json:"networks,omitempty"`
	Protocols     *[]string         `json:"protocols,omitempty"`
}

// UnitOptionItem is a selectable Unit.
type UnitOptionItem struct {
	ID             uuid.UUID  `json:"id"`
	Name           string     `json:"name"`
	AgencyID       uuid.UUID  `json:"agencyId"`
	OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
}

// NamedItem is a selectable Organization, Agency or Network.
type NamedItem struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// FeedPage is the response of a feed page.
type FeedPage struct {
	Items         []FeedItem `json:"items"`
	NextPageToken string     `json:"nextPageToken,omitempty"`
	// Position is the cutoff for resuming live updates.
	Position string `json:"position"`
}

// viewer carries what projection needs to know about the caller.
type viewer struct {
	groups       map[string]bool
	networkReach authz.AgencyReach
}

func (v viewer) can(group string) bool { return v.groups[group] }

// feedItem applies the caller's field groups to a record.
func (v viewer) feedItem(record FeedRecord, detail bool) FeedItem {
	item := FeedItem{MessageID: record.MessageID, MessageTime: record.Time.UTC(),
		ProcessingStatus: record.ProcessingStatus, CorrectionStatus: record.CorrectionStatus}
	if v.can("payload") && record.Representative != nil {
		protocol, incomplete := record.Representative.Protocol, record.Representative.Incomplete
		item.Payload, item.Protocol, item.Incomplete = record.Representative.Payload, &protocol, &incomplete
	}
	if v.can("unit") {
		units := make([]UnitItem, 0, len(record.Units))
		for _, unit := range record.Units {
			entry := UnitItem{UnitID: unit.UnitID, Name: unit.Name, AgencyID: unit.AgencyID,
				OrganizationID: unit.OrganizationID, Encrypted: unit.Encrypted}
			if detail {
				first := unit.FirstObservedAt.UTC()
				entry.FirstObservedAt = &first
			}
			units = append(units, entry)
		}
		withoutUnit := len(record.WithoutUnit) > 0
		item.Units, item.WithoutUnit = &units, &withoutUnit
	}
	if v.can("derived") {
		derived := v.derivedItems(record.Derived)
		item.Derived = &derived
	}
	if slices.ContainsFunc(record.UnresolvedAgencies, v.networkReach.Contains) {
		unresolved := true
		item.NetworkUnresolved = &unresolved
	}
	return item
}

// derivedItems shapes the derived results the store already limited to the caller's readable
// field groups, keeping only the targets the caller may see.
func (v viewer) derivedItems(records []DerivedRecord) []DerivedItem {
	items := make([]DerivedItem, 0, len(records))
	for _, record := range records {
		var evaluation struct {
			Fields        map[string]DerivedField `json:"fields"`
			TargetResults []DerivedTargetResult   `json:"targetResults"`
		}
		if json.Unmarshal(record.Evaluation, &evaluation) != nil {
			continue
		}
		item := DerivedItem{WorkflowID: record.WorkflowID, ResultID: record.ResultID, Fields: evaluation.Fields}
		for _, target := range evaluation.TargetResults {
			if slices.Contains(record.VisibleTargets, target.TargetPosition) {
				item.TargetResults = append(item.TargetResults, target)
			}
		}
		items = append(items, item)
	}
	return items
}

// detailItem applies the caller's field groups to a detail record.
func (v viewer) detailItem(record DetailRecord) DetailItem {
	item := DetailItem{FeedItem: v.feedItem(record.FeedRecord, true), Transmissions: make([]TransmissionItem, 0, len(record.Transmissions))}
	if v.can("unit") {
		appearances := make([]WithoutUnitItem, 0, len(record.WithoutUnit))
		for _, entry := range record.WithoutUnit {
			appearances = append(appearances, WithoutUnitItem{AgencyID: entry.AgencyID, FirstObservedAt: entry.FirstObservedAt.UTC()})
		}
		item.WithoutUnitAppearances = &appearances
	}
	for _, transmission := range record.Transmissions {
		item.Transmissions = append(item.Transmissions, v.transmissionItem(transmission))
	}
	return item
}

func (v viewer) transmissionItem(record TransmissionRecord) TransmissionItem {
	item := TransmissionItem{TransmissionID: record.ID, AcceptedAt: record.AcceptedAt.UTC(),
		ProcessingStatus: record.ProcessingStatus, Targets: make([]TargetItem, 0, len(record.Targets))}
	if v.can("gateway") {
		observed, network, name := record.ObservedAt.UTC(), record.NetworkID, record.NetworkName
		item.ObservedAt, item.NetworkID, item.NetworkName = &observed, &network, &name
	}
	if v.can("payload") {
		protocol, incomplete := record.Protocol, record.Incomplete
		item.Payload, item.Protocol, item.Incomplete = record.Payload, &protocol, &incomplete
	}
	if v.can("quality") {
		item.QualityMetrics = record.QualityMetrics
	}
	if record.NetworkUnresolved && v.networkReach.Contains(record.NetworkAgencyID) {
		unresolved := true
		item.NetworkUnresolved = &unresolved
	}
	for _, target := range record.Targets {
		entry := TargetItem{Position: target.Position, Resolution: target.Resolution}
		if v.can("unit") {
			entry.UnitID = target.UnitID
			if target.UnitID != nil {
				encrypted := target.Encrypted
				entry.Encrypted = &encrypted
			}
		}
		if v.can("address") {
			switch {
			case target.AddressNumber != nil:
				entry.Address = *target.AddressNumber
			case target.AddressText != nil:
				entry.Address = *target.AddressText
			}
			entry.Function = target.Function
		}
		item.Targets = append(item.Targets, entry)
	}
	return item
}

// optionsItem applies the caller's field groups to option families.
func (v viewer) optionsItem(record OptionsRecord) OptionsItem {
	var item OptionsItem
	if v.can("unit") {
		units := make([]UnitOptionItem, 0, len(record.Units))
		for _, unit := range record.Units {
			units = append(units, UnitOptionItem(unit))
		}
		item.Units = &units
		item.Organizations = namedItems(record.Organizations)
		item.Agencies = namedItems(record.Agencies)
	}
	if v.can("gateway") {
		item.Networks = namedItems(record.Networks)
	}
	if v.can("payload") {
		list := slices.Clone(protocols)
		item.Protocols = &list
	}
	return item
}

func namedItems(options []NamedOption) *[]NamedItem {
	items := make([]NamedItem, 0, len(options))
	for _, option := range options {
		items = append(items, NamedItem(option))
	}
	return &items
}
