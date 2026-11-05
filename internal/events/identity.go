// Package events defines the internal domain-fact vocabulary that producing
// domains append to the event store: identity, master-data, configuration,
// transmission-intake, historical-correction and processing facts.
//
// A producer builds a typed fact value (IdentityFact, MasterDataFact,
// ConfigurationFact, IntakeFact, CorrectionFact, ProcessingConfigurationFact,
// ProcessingInstanceFact, ProcessingCompletionFact) and hands it to the matching
// appender port (Appender, MasterDataAppender, ConfigurationAppender,
// IntakeAppender, CorrectionAppender, ProcessingAppender,
// ProcessingInstanceAppender) inside its own platform transaction. The store
// calls the fact's Envelope method with the primary database time, which
// validates the fact against the frozen vocabulary and produces the versioned
// JSON envelope that is persisted. Facts are written in the producer's
// transaction, so they become visible to consumers only if it commits.
package events

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// Type is the envelope event type shared by identity, master-data and
// configuration facts.
type Type string

// Event types accepted by IdentityFact.Envelope. ConfigurationChanged is also
// the type of every MasterDataFact and ConfigurationFact envelope.
const (
	// ConfigurationChanged records that an administered aggregate (an identity
	// object, master data or an address rule) was created, changed or deleted.
	ConfigurationChanged Type = "configuration.changed"
	// PrincipalAccessChanged records that what a principal may access changed,
	// for example suspension, grants, memberships or credentials.
	PrincipalAccessChanged Type = "principal.access-changed"
	// PrincipalDeleted records that an account is being deleted; it is only
	// valid with ChangeKind AccountDeleting.
	PrincipalDeleted Type = "principal.deleted"
)

// ChangeKind names the concrete identity change an IdentityFact records; it is
// serialized as data.changeKind.
type ChangeKind string

// Identity change kinds accepted by IdentityFact.Envelope.
const (
	AccountCreated     ChangeKind = "account-created"
	ProfileChanged     ChangeKind = "profile-changed"
	AccountSuspended   ChangeKind = "account-suspended"
	AccountReactivated ChangeKind = "account-reactivated"
	GrantsChanged      ChangeKind = "grants-changed"
	MembershipChanged  ChangeKind = "membership-changed"
	CredentialIssued   ChangeKind = "credential-issued"
	CredentialRevoked  ChangeKind = "credential-revoked"
	PasswordChanged    ChangeKind = "password-changed"
	AccountDeleting    ChangeKind = "account-deleting"
	BootstrapCompleted ChangeKind = "bootstrap-completed"
)

// ErrInvalidFact is returned (possibly wrapped or joined with detail) by every
// Envelope method when a fact has missing identifiers, a non-positive revision,
// zero or sub-microsecond timestamps, nil-UUID causation/correlation IDs, or a
// type, aggregate type or change kind outside the frozen vocabulary.
var ErrInvalidFact = errors.New("invalid identity fact")

// EntityRef identifies an aggregate by its type name and ID; it is serialized
// as the envelope's aggregateRef.
type EntityRef struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

// Producer identifies the producing domain and its fact-vocabulary version in
// an envelope. Values are fixed by each Envelope method, not by callers.
type Producer struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
}

// IdentityData is the data payload of an identity envelope.
type IdentityData struct {
	ChangeKind ChangeKind `json:"changeKind"`
}

// IdentityFact is the producer-owned input for an identity fact (accounts, user
// groups, grants, credentials, installation bootstrap). The recorded time is
// assigned by the primary store at append, not by the producer.
type IdentityFact struct {
	ID            uuid.UUID
	Type          Type
	Aggregate     EntityRef
	Revision      int64
	OccurredAt    time.Time
	CausationID   *uuid.UUID
	CorrelationID *uuid.UUID
	ChangeKind    ChangeKind
}

// Envelope is the internal, classified identity-facts 1.0 serialization.
// It contains no authorization to publish its contents to an external consumer.
type Envelope struct {
	SchemaVersion     int          `json:"schemaVersion"`
	DomainEventID     uuid.UUID    `json:"domainEventId"`
	Type              Type         `json:"type"`
	Producer          Producer     `json:"producer"`
	AggregateRef      EntityRef    `json:"aggregateRef"`
	AggregateRevision int64        `json:"aggregateRevision,string"`
	OccurredAt        time.Time    `json:"occurredAt"`
	RecordedAt        time.Time    `json:"recordedAt"`
	CausationID       *uuid.UUID   `json:"causationId,omitempty"`
	CorrelationID     *uuid.UUID   `json:"correlationId,omitempty"`
	Data              IdentityData `json:"data"`
	Classification    string       `json:"classification"`
	ReplayMode        string       `json:"replayMode"`
}

// Appender records identity facts. Implementations write through the caller's
// active platform transaction and never dispatch a fact before that transaction
// commits. AppendIdentityFact returns the fact ID, or ErrInvalidFact when the
// fact fails Envelope validation.
type Appender interface {
	AppendIdentityFact(context.Context, platform.TransactionContext, IdentityFact) (uuid.UUID, error)
}

// Envelope validates the fact against the identity vocabulary and returns the
// envelope stamped with recordedAt (both times normalized to UTC, producer
// identity-core 1.1, classification "authorization"). It returns ErrInvalidFact
// when validation fails.
func (f IdentityFact) Envelope(recordedAt time.Time) (Envelope, error) {
	if f.ID == uuid.Nil || f.Aggregate.ID == uuid.Nil || f.Revision <= 0 ||
		!slices.Contains([]string{"account", "userGroup", "grant", "credential", "installation"}, f.Aggregate.Type) ||
		!slices.Contains([]Type{ConfigurationChanged, PrincipalAccessChanged, PrincipalDeleted}, f.Type) ||
		!slices.Contains([]ChangeKind{AccountCreated, ProfileChanged, AccountSuspended, AccountReactivated,
			GrantsChanged, MembershipChanged, CredentialIssued, CredentialRevoked, PasswordChanged,
			AccountDeleting, BootstrapCompleted}, f.ChangeKind) ||
		(f.Type == PrincipalDeleted && f.ChangeKind != AccountDeleting) ||
		f.OccurredAt.IsZero() || recordedAt.IsZero() ||
		f.OccurredAt.Nanosecond()%1000 != 0 || recordedAt.Nanosecond()%1000 != 0 ||
		(f.CausationID != nil && *f.CausationID == uuid.Nil) ||
		(f.CorrelationID != nil && *f.CorrelationID == uuid.Nil) {
		return Envelope{}, ErrInvalidFact
	}
	return Envelope{
		SchemaVersion: 1, DomainEventID: f.ID, Type: f.Type,
		Producer:     Producer{Domain: "identity", Version: "identity-core1.1"},
		AggregateRef: f.Aggregate, AggregateRevision: f.Revision,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: f.CausationID, CorrelationID: f.CorrelationID,
		Data: IdentityData{ChangeKind: f.ChangeKind}, Classification: "authorization", ReplayMode: "normal",
	}, nil
}
