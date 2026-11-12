package readmodel

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/events"
)

// liveKind classifies how a fact changes the Messages a view shows.
type liveKind int

const (
	// kindAdd facts can only make a Message appear in a view: the client upserts it when it
	// matches and otherwise has nothing to change.
	kindAdd liveKind = iota
	// kindChange facts can also make a Message leave a view, which the client learns through
	// a remove frame.
	kindChange
	// kindMerge facts replace Messages. They never produce Message frames: which absorbed IDs a
	// caller had loaded is unknowable without disclosing hidden IDs, so a merge the caller may see
	// resynchronizes the view.
	kindMerge
	// kindResync facts can change membership, names or Agencies of every view.
	kindResync
)

// liveSchemaVersion is the only envelope schema this reader understands.
const liveSchemaVersion = 1

// liveEnvelope is the part of a domain-event envelope that live updates read. Every Message
// related fact type shares these field names, so one decoder serves them all.
type liveEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	Type          string `json:"type"`
	AggregateRef  struct {
		Type string `json:"type"`
	} `json:"aggregateRef"`
	Data struct {
		MessageID           uuid.UUID    `json:"messageId"`
		TransmissionID      uuid.UUID    `json:"transmissionId"`
		MessageTime         *time.Time   `json:"messageTime"`
		PreviousMessageTime *time.Time   `json:"previousMessageTime"`
		Absorbed            []uuid.UUID  `json:"absorbedMessageIds"`
		NetworkID           uuid.UUID    `json:"networkId"`
		PreviousNetworkID   *uuid.UUID   `json:"previousNetworkId"`
		UnitID              *uuid.UUID   `json:"unitId"`
		ChangeKind          string       `json:"changeKind"`
		Reason              string       `json:"reason"`
		Targets             []liveTarget `json:"targets"`
	} `json:"data"`
}

// liveTarget is one target of an assignment fact. Facts of a correction carry the resolution
// before and after; the first assignment of an intake carries the resolution inline, which has
// no before state.
type liveTarget struct {
	Before *liveResolution `json:"before"`
}

// liveResolution is a target resolution as far as the before basis needs it.
type liveResolution struct {
	Status string     `json:"status"`
	UnitID *uuid.UUID `json:"unitId"`
}

// liveBefore names the appearance contexts a fact says the Message had before the change: Unit
// rows by Unit and Unit-less rows by the Network whose Agency owned them.
type liveBefore struct {
	units    []uuid.UUID
	networks []uuid.UUID
}

// liveFact is a decoded fact that concerns Message views.
type liveFact struct {
	position  events.Position
	kind      liveKind
	messageID uuid.UUID
	// transmissionID is the joining Transmission of a merge; its current Message is the final
	// survivor even when later merges absorbed the fact's survivor.
	transmissionID uuid.UUID
	absorbed       []uuid.UUID
	before         liveBefore
}

// liveOutcome is the result of decoding one fact record.
type liveOutcome int

const (
	// outcomeIgnore means the fact needs no frame and does not concern the loaded range.
	outcomeIgnore liveOutcome = iota
	// outcomeFact means the fact is relevant and must be evaluated.
	outcomeFact
	// outcomeResync means the view cannot be kept consistent incrementally.
	outcomeResync
)

// configurationResyncAggregates are the aggregates of configuration.changed whose change can
// alter every view.
var configurationResyncAggregates = map[string]bool{"unit": true, "unitCollection": true, "organization": true, "agency": true, "attributeSchema": true}

// decodeLiveFact reads one fact for a view that loaded the range [from, until). Types outside
// the handled set are ignored; a handled type with a schema this reader does not know, or an
// undecodable envelope, requires a resync because its effect cannot be derived.
func decodeLiveFact(record events.Record, from, until *time.Time) (liveFact, liveOutcome) {
	var kind liveKind
	switch record.Type {
	case "message.created", "message.transmission-joined", "assignment.changed", "suppression.changed",
		"message.merged", "processing.completed", "processing.retracted":
	case "configuration.changed":
		kind = kindResync
	default:
		return liveFact{}, outcomeIgnore
	}
	var envelope liveEnvelope
	if err := json.Unmarshal(record.Envelope, &envelope); err != nil || envelope.SchemaVersion != liveSchemaVersion {
		return liveFact{}, outcomeResync
	}
	if kind == kindResync {
		if configurationResyncAggregates[envelope.AggregateRef.Type] {
			return liveFact{}, outcomeResync
		}
		return liveFact{}, outcomeIgnore
	}
	data := envelope.Data
	if !intersectsRange(data.MessageTime, data.PreviousMessageTime, from, until) {
		return liveFact{}, outcomeIgnore
	}
	fact := liveFact{position: record.Position(), messageID: data.MessageID}
	switch record.Type {
	case "message.created", "message.transmission-joined":
		fact.kind = kindAdd
	case "message.merged":
		fact.kind, fact.absorbed, fact.transmissionID = kindMerge, data.Absorbed, data.TransmissionID
	case "processing.completed", "processing.retracted":
		fact.kind = kindChange
	case "assignment.changed":
		if data.Reason == "created" {
			fact.kind = kindAdd
			break
		}
		fact.kind, fact.before = kindChange, assignmentBefore(data.Targets, data.NetworkID, data.PreviousNetworkID)
	case "suppression.changed":
		if data.ChangeKind == "lifted" {
			fact.kind = kindAdd
			break
		}
		fact.kind = kindChange
		if data.UnitID != nil {
			fact.before.units = []uuid.UUID{*data.UnitID}
		} else if data.NetworkID != uuid.Nil {
			fact.before.networks = []uuid.UUID{data.NetworkID}
		}
	}
	if fact.messageID == uuid.Nil {
		return liveFact{}, outcomeResync
	}
	return fact, outcomeFact
}

// assignmentBefore derives the contexts a corrected assignment came from: the Unit of each
// target that was assigned, and the Unit-less context of the Network the Transmission had
// before the fact for each unassigned target. A Transmission that only moved Networks had no
// target change, so its before context is the Unit-less one of the previous Network.
func assignmentBefore(targets []liveTarget, network uuid.UUID, previous *uuid.UUID) liveBefore {
	var before liveBefore
	origin := network
	if previous != nil {
		origin = *previous
	}
	unassigned := previous != nil && len(targets) == 0
	for _, target := range targets {
		if target.Before == nil {
			continue
		}
		switch {
		case target.Before.Status == "assigned" && target.Before.UnitID != nil:
			before.units = append(before.units, *target.Before.UnitID)
		case target.Before.Status == "unassigned":
			unassigned = true
		}
	}
	if unassigned && origin != uuid.Nil {
		before.networks = append(before.networks, origin)
	}
	return before
}

// intersectsRange reports whether the closed interval a fact spans, from its Message time to the
// previous Message time of a repositioning, meets the loaded range [from, until). A fact without
// a Message time may concern any range.
func intersectsRange(messageTime, previous, from, until *time.Time) bool {
	if messageTime == nil {
		return true
	}
	last := *messageTime
	if previous != nil && previous.After(last) {
		last = *previous
	}
	if until != nil && !messageTime.Before(*until) {
		return false
	}
	return from == nil || !last.Before(*from)
}
