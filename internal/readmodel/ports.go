package readmodel

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/platform"
)

// Store is the persistence port of the read side. Every method runs in the caller's
// transaction, in which the caller's authority was resolved, and evaluates the row
// eligibility carried by its query inside the database before it filters, orders or pages.
type Store interface {
	// Feed returns one page of Messages, or exactly the listed Messages when query.IDs is set.
	Feed(ctx context.Context, tx platform.TransactionContext, query FeedQuery) (FeedResult, error)
	// Detail returns the Message with its supporting Transmissions and visible targets, or
	// authz.ErrNotFound when no appearance is eligible for detail.
	Detail(ctx context.Context, tx platform.TransactionContext, query DetailQuery) (DetailRecord, error)
	// Options returns the filter option objects the caller's message.read scopes reach.
	Options(ctx context.Context, tx platform.TransactionContext, query OptionsQuery) (OptionsRecord, error)
	// Eligible returns those of the Messages that have at least one eligible appearance.
	Eligible(ctx context.Context, tx platform.TransactionContext, access authz.MessageAccess, ids []uuid.UUID) ([]uuid.UUID, error)
}

// TokenKeys supplies the secret that signs page tokens.
type TokenKeys interface {
	// TokenKey returns the installation's 32-byte secret, creating it on first use. Callers run it
	// in a transaction of its own, so that a created secret is committed before it is used.
	TokenKey(ctx context.Context, tx platform.TransactionContext) ([]byte, error)
}

// FeedQuery selects Messages for the feed or, with IDs, for a live re-evaluation.
type FeedQuery struct {
	Access  authz.MessageAccess
	Filters Filters
	// Selection is the compiled predicate of Filters.Filter, or nil without one.
	Selection *fields.Filter
	// After is the keyset cursor; nil starts at the newest Message.
	After *Cursor
	// Limit is the page size; the store reads one more row to learn whether another page exists.
	Limit int
	// IDs, when non-nil, evaluates exactly these Messages with the filters and returns them in
	// feed order without paging or a position.
	IDs []uuid.UUID
	// DerivedGroups are the field groups the caller may read, or nil when the caller cannot read
	// derived results. A result contributes attributes, shown or matched by the predicate, only
	// when all of its stored field groups are among them.
	DerivedGroups []string
	// WithPublishedResults asks for FeedRecord.PublishedResults, which the feed itself does not need.
	WithPublishedResults bool
}

// FeedResult is one page of the feed.
type FeedResult struct {
	Items   []FeedRecord
	HasMore bool
	// TxID and Seq are the cutoff in the page statement's own snapshot.
	TxID, Seq int64
}

// DetailQuery selects one Message.
type DetailQuery struct {
	Access        authz.MessageAccess
	MessageID     uuid.UUID
	DerivedGroups []string
}

// OptionsQuery selects the option families to load: Units, Organizations and Agencies need
// the unit group, Networks the gateway group.
type OptionsQuery struct {
	Access        authz.MessageAccess
	Units         bool
	Organizations bool
	Agencies      bool
	Networks      bool
}

// PayloadRecord is the original Payload of a Transmission with its Protocol.
type PayloadRecord struct {
	Payload    json.RawMessage
	Protocol   string
	Incomplete bool
}

// UnitRecord is an eligible Unit appearance.
type UnitRecord struct {
	UnitID          uuid.UUID
	Name            string
	AgencyID        uuid.UUID
	OrganizationID  *uuid.UUID
	Encrypted       bool
	FirstObservedAt time.Time
}

// WithoutUnitRecord is an eligible Unit-less appearance.
type WithoutUnitRecord struct {
	AgencyID        uuid.UUID
	FirstObservedAt time.Time
}

// AttributeRecord is the effective value of one attribute field: the present value of the
// latest published result, among the readable results of the supporting Transmissions, for the
// schema key, the field and the target position (nil for a Transmission-level result).
type AttributeRecord struct {
	SchemaKey string
	Field     string
	// TargetPosition is the original target position of a target-level result.
	TargetPosition *int
	// UnitID is the Unit the target is assigned to.
	UnitID *uuid.UUID
	// Value is the typed value as stored.
	Value               json.RawMessage
	WorkflowID          uuid.UUID
	DefinitionVersionID uuid.UUID
	ResultID            uuid.UUID
	PublishedAt         time.Time
}

// FeedRecord is everything the feed knows about one Message before field groups are applied.
type FeedRecord struct {
	MessageID        uuid.UUID
	Time             time.Time
	ProcessingStatus string
	CorrectionStatus string
	// Representative is nil when no readable supporting Transmission exists.
	Representative *PayloadRecord
	Units          []UnitRecord
	WithoutUnit    []WithoutUnitRecord
	// UnresolvedAgencies are the Agencies of the Networks of supporting readable Transmissions
	// whose Network could not be resolved.
	UnresolvedAgencies []uuid.UUID
	// Attributes are the effective attribute values the caller's readable field groups allow.
	Attributes []AttributeRecord
	// PublishedResults are the IDs of the published results of the supporting Transmissions that
	// the caller's readable field groups allow, whether or not they hold an effective value.
	// They are filled only when FeedQuery.WithPublishedResults is set.
	PublishedResults []uuid.UUID
}

// TargetRecord is a visible, unsuppressed target of a Transmission.
type TargetRecord struct {
	Position      int
	Resolution    string
	UnitID        *uuid.UUID
	Encrypted     bool
	AddressNumber *int64
	AddressText   *string
	Function      *int
}

// TransmissionRecord is a supporting readable Transmission with its visible targets.
type TransmissionRecord struct {
	ID                uuid.UUID
	AcceptedAt        time.Time
	ObservedAt        time.Time
	ProcessingStatus  string
	NetworkID         uuid.UUID
	NetworkName       string
	NetworkAgencyID   uuid.UUID
	NetworkUnresolved bool
	PayloadRecord
	QualityMetrics json.RawMessage
	Targets        []TargetRecord
}

// DetailRecord is a Message with its supporting Transmissions.
type DetailRecord struct {
	FeedRecord
	Transmissions []TransmissionRecord
}

// UnitOption is a Unit the caller's scopes reach.
type UnitOption struct {
	ID             uuid.UUID
	Name           string
	AgencyID       uuid.UUID
	OrganizationID *uuid.UUID
}

// NamedOption is an Organization, Agency or Network the caller's scopes reach.
type NamedOption struct {
	ID   uuid.UUID
	Name string
}

// OptionsRecord holds the requested option families.
type OptionsRecord struct {
	Units         []UnitOption
	Organizations []NamedOption
	Agencies      []NamedOption
	Networks      []NamedOption
}
