// Package events defines the frozen internal identity-fact append boundary.
package events

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type Type string

const (
	ConfigurationChanged   Type = "configuration.changed"
	PrincipalAccessChanged Type = "principal.access-changed"
	PrincipalDeleted       Type = "principal.deleted"
)

type ChangeKind string

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

var ErrInvalidFact = errors.New("invalid identity fact")

type EntityRef struct {
	Type string    `json:"type"`
	ID   uuid.UUID `json:"id"`
}

type Producer struct {
	Domain  string `json:"domain"`
	Version string `json:"version"`
}

type IdentityData struct {
	ChangeKind ChangeKind `json:"changeKind"`
}

// IdentityFact is producer-owned input; recorded time is assigned by the primary store.
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

// Appender participates in the existing transaction and never dispatches before commit.
type Appender interface {
	AppendIdentityFact(context.Context, platform.TransactionContext, IdentityFact) (uuid.UUID, error)
}

// Envelope validates the bounded identity vocabulary and stamps a primary recorded time.
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
		Producer:     Producer{Domain: "DOM-05", Version: "identity-core1.1"},
		AggregateRef: f.Aggregate, AggregateRevision: f.Revision,
		OccurredAt: f.OccurredAt.UTC(), RecordedAt: recordedAt.UTC(),
		CausationID: f.CausationID, CorrelationID: f.CorrelationID,
		Data: IdentityData{ChangeKind: f.ChangeKind}, Classification: "authorization", ReplayMode: "normal",
	}, nil
}
