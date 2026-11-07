package assignments

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// TargetContext is the stored state of one target position of a transmission:
// its message, Network, current Resolution and observation/acceptance times.
type TargetContext struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	Position       int
	Resolution     Resolution
	ObservedAt     time.Time
	AcceptedAt     time.Time
}

// Suppression excludes one target position of a transmission from assignment.
// Active is false once lifted, and LiftedAt is then set.
type Suppression struct {
	ID             uuid.UUID  `json:"id"`
	TransmissionID uuid.UUID  `json:"transmissionId"`
	TargetPosition int        `json:"targetPosition"`
	Revision       int64      `json:"revision,string"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"createdAt"`
	LiftedAt       *time.Time `json:"liftedAt"`
}

// CorrectionScope selects what a correction re-resolves: the targets of an address selector on
// one Network and Protocol with an optional validity window (a rule change), of one Unit (a Unit
// deletion) or one target (a suppression change), or the Network and targets of every
// Transmission of one Gateway and Protocol (a Gateway mapping change). Exactly one of the four
// forms is set; Kind names it. Functions of a rule selector are not part of the scope, so a
// scope is a superset of the rule's targets, which is safe because re-resolution is idempotent.
type CorrectionScope struct {
	NetworkID      *uuid.UUID         `json:"networkId,omitempty"`
	Protocol       protocols.Protocol `json:"protocol,omitempty"`
	AddressFrom    *uint64            `json:"addressFrom,omitempty"`
	AddressTo      *uint64            `json:"addressTo,omitempty"`
	AddressText    *string            `json:"addressText,omitempty"`
	ValidFrom      *time.Time         `json:"validFrom,omitempty"`
	ValidUntil     *time.Time         `json:"validUntil,omitempty"`
	UnitID         *uuid.UUID         `json:"unitId,omitempty"`
	TransmissionID *uuid.UUID         `json:"transmissionId,omitempty"`
	TargetPosition *int               `json:"targetPosition,omitempty"`
	GatewayID      *uuid.UUID         `json:"gatewayId,omitempty"`
}

// Kind returns address-rules, unit, target or gateway, or an empty string for an invalid scope.
func (s CorrectionScope) Kind() string {
	switch {
	case s.NetworkID != nil && s.UnitID == nil && s.TransmissionID == nil && s.GatewayID == nil:
		return "address-rules"
	case s.UnitID != nil && s.NetworkID == nil && s.TransmissionID == nil && s.GatewayID == nil:
		return "unit"
	case s.TransmissionID != nil && s.TargetPosition != nil && s.NetworkID == nil && s.UnitID == nil &&
		s.GatewayID == nil:
		return "target"
	case s.GatewayID != nil && s.NetworkID == nil && s.UnitID == nil && s.TransmissionID == nil:
		return "gateway"
	}
	return ""
}

// Validate rejects a scope that is not exactly one of the four forms or carries a nil ID.
func (s CorrectionScope) Validate() error {
	switch s.Kind() {
	case "address-rules":
		if *s.NetworkID == uuid.Nil || !s.Protocol.Valid() ||
			(s.AddressText == nil) == (s.AddressFrom == nil || s.AddressTo == nil) ||
			(s.AddressFrom != nil && *s.AddressFrom > *s.AddressTo) ||
			(s.ValidFrom != nil && s.ValidUntil != nil && !s.ValidFrom.Before(*s.ValidUntil)) {
			return authz.ErrInvalidInput
		}
	case "unit":
		if *s.UnitID == uuid.Nil {
			return authz.ErrInvalidInput
		}
	case "target":
		if *s.TransmissionID == uuid.Nil || *s.TargetPosition < 0 {
			return authz.ErrInvalidInput
		}
	case "gateway":
		if *s.GatewayID == uuid.Nil || !s.Protocol.Valid() || s.TargetPosition != nil ||
			s.AddressFrom != nil || s.AddressTo != nil || s.AddressText != nil || s.ValidFrom != nil ||
			s.ValidUntil != nil {
			return authz.ErrInvalidInput
		}
	default:
		return authz.ErrInvalidInput
	}
	return nil
}

// MarshalJSON adds the scope kind to the API view.
func (s CorrectionScope) MarshalJSON() ([]byte, error) {
	type plain CorrectionScope
	return json.Marshal(struct {
		Kind string `json:"kind"`
		plain
	}{s.Kind(), plain(s)})
}

// RuleScope is the correction scope of one rule version: its Network, Protocol, address
// selector and validity window.
func RuleScope(rule Rule) CorrectionScope {
	network := rule.NetworkID
	scope := CorrectionScope{NetworkID: &network, Protocol: rule.Protocol, ValidFrom: rule.ValidFrom, ValidUntil: rule.ValidUntil}
	if rule.Selector.Value != nil {
		text := *rule.Selector.Value
		scope.AddressText = &text
		return scope
	}
	if rule.Selector.From != nil {
		from := *rule.Selector.From
		to := from
		if rule.Selector.To != nil {
			to = *rule.Selector.To
		}
		scope.AddressFrom, scope.AddressTo = &from, &to
	}
	return scope
}

// UnitScope is the correction scope of every target currently resolved to the Unit.
func UnitScope(id uuid.UUID) CorrectionScope {
	unit := id
	return CorrectionScope{UnitID: &unit}
}

// TargetScope is the correction scope of exactly one target.
func TargetScope(transmission uuid.UUID, position int) CorrectionScope {
	id, index := transmission, position
	return CorrectionScope{TransmissionID: &id, TargetPosition: &index}
}

// GatewayScope is the correction scope of the Network resolution of every Transmission of one
// Gateway and Protocol.
func GatewayScope(gateway uuid.UUID, protocol protocols.Protocol) CorrectionScope {
	id := gateway
	return CorrectionScope{GatewayID: &id, Protocol: protocol}
}

// CorrectionStatus is a correction scope queued at one assignment revision. State is
// pending, running, completed, failed or superseded; FailureCode is set for failures;
// Processed and Remaining count the Transmissions inside the scope; Unresolved counts, for a
// gateway scope, its Transmissions the current mapping and rules could not resolve (0 otherwise). Fields
// tagged json:"-" are internal cursor data that are not exposed to API clients.
type CorrectionStatus struct {
	ID            uuid.UUID       `json:"id"`
	AssignmentRev int64           `json:"assignmentRevision,string"`
	State         string          `json:"state"`
	Scope         CorrectionScope `json:"scope"`
	Processed     int             `json:"processed"`
	Remaining     int             `json:"remaining"`
	Unresolved    int             `json:"unresolved"`
	FailureCode   string          `json:"failureCode,omitempty"`
	Reason        string          `json:"-"`
	CursorAt      *time.Time      `json:"-"`
	CursorID      *uuid.UUID      `json:"-"`
	Attempts      int             `json:"-"`
}

// CorrectionRepository is the persistence port for suppressions and
// correction status used by Service. Reads return authz.ErrNotFound for
// unknown IDs. RetryCorrection returns authz.ErrRevisionConflict unless the
// scope is failed, and otherwise supersedes it with a new queued scope at the
// current assignment revision. GatewayCorrectionAgencies lists, without
// duplicates, the Agencies of the Gateway's current Networks of the Protocol and
// of the Networks its Transmissions of that Protocol reference.
type CorrectionRepository interface {
	ReadTargetContext(context.Context, platform.TransactionContext, uuid.UUID, int) (TargetContext, error)
	ReadSuppression(context.Context, platform.TransactionContext, uuid.UUID) (Suppression, error)
	FindActiveSuppression(context.Context, platform.TransactionContext, uuid.UUID, int) (Suppression, bool, error)
	ListSuppressions(context.Context, platform.TransactionContext) ([]Suppression, error)
	CreateSuppression(context.Context, platform.TransactionContext, Suppression, uuid.UUID) error
	LiftSuppression(context.Context, platform.TransactionContext, uuid.UUID, int64, uuid.UUID, time.Time) (Suppression, error)
	ReadCorrection(context.Context, platform.TransactionContext, uuid.UUID) (CorrectionStatus, error)
	CorrectionForRevision(context.Context, platform.TransactionContext, int64) (CorrectionStatus, error)
	RetryCorrection(context.Context, platform.TransactionContext, uuid.UUID) (CorrectionStatus, error)
	GatewayCorrectionAgencies(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol) ([]uuid.UUID, error)
}

// CorrectionLease is a claimed correction scope. FencingToken guards every
// write made under the lease, which expires at LeaseUntil; AssignmentRev is the
// revision the scope was recorded at (targets below it are stale). Correction is system work,
// so no access revision is pinned: the fence and the lease are the only guards.
type CorrectionLease struct {
	ID            uuid.UUID
	AssignmentRev int64
	Scope         CorrectionScope
	Reason        string
	CursorAt      *time.Time
	CursorID      *uuid.UUID
	FencingToken  int64
	LeaseUntil    time.Time
	Attempts      int
	// Exhausted marks the lease ClaimCorrection returns, unclaimed, for a scope it just failed
	// with RETRY_LIMIT; only ID, Reason and Attempts are set.
	Exhausted bool
}

// CorrectionBatchItem is one stored transmission within a correction scope,
// with the original targets and observation time needed to re-match it and, for
// Network re-resolution, its Gateway and declared Gateway Network identifier
// (empty when none was declared).
type CorrectionBatchItem struct {
	TransmissionID  uuid.UUID
	MessageID       uuid.UUID
	NetworkID       uuid.UUID
	GatewayID       uuid.UUID
	DeclaredNetwork string
	Protocol        protocols.Protocol
	Targets         []protocols.Target
	ObservedAt      time.Time
	AcceptedAt      time.Time
	SourceRevision  int64
}

// CorrectionUpdate pairs a batch item with its fresh match result. Network is set for a
// gateway scope: the item's re-resolved Network, matched by Match.
type CorrectionUpdate struct {
	Item    CorrectionBatchItem
	Match   MatchResult
	Network *NetworkResolution
}

// CorrectionChange is one target whose stored resolution was changed by
// ApplyCorrectionBatch; MessageRev is the message revision after the change.
type CorrectionChange struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	NetworkID      uuid.UUID
	TargetPosition int
	MessageRev     int64
	Before         events.IntakeResolution
	After          events.IntakeResolution
}

// CorrectionApplied is the outcome of ApplyCorrectionBatch: the targets whose resolution changed,
// the Transmissions whose Network moved, the number of Transmissions the current Gateway mapping
// could not resolve, and every Transmission that needs Processing readmission: those whose
// targets were restamped at a newer assignment revision (with or without a resolution change,
// because a Transmission's processing context is the highest revision stamped on its targets)
// and those that moved.
type CorrectionApplied struct {
	Changes    []CorrectionChange
	Moves      []NetworkMove
	Unresolved int
	Restamped  []uuid.UUID
}

// NetworkMove is one Transmission that a gateway scope moved From one Network To another;
// MessageRev is the message revision after the move.
type NetworkMove struct {
	TransmissionID uuid.UUID
	MessageID      uuid.UUID
	From           uuid.UUID
	To             uuid.UUID
	MessageRev     int64
}

// CorrectionWorkRepository is the persistence port used by CorrectionRunner.
// ClaimCorrection leases the oldest queued (or expired running) scope; it
// reports false when there is none. A scope that has no attempt left is failed instead
// and returned with Exhausted set, so the runner records its final failure. ApplyCorrectionBatch and
// CompleteCorrection must be called with a still-valid lease and fail with a
// revision conflict when the fence is lost; a newer assignment revision or
// access revision is fine, because every scope runs against current rules.
// CompleteCorrection returns false while targets in the scope still carry an
// older assignment revision. FailCorrection reschedules the scope with a delay
// or marks it failed after repeated attempts and reports that final failure;
// it is a no-op for a lost lease.
type CorrectionWorkRepository interface {
	ClaimCorrection(context.Context, platform.TransactionContext, time.Time, time.Duration) (CorrectionLease, bool, error)
	ReadCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, int) ([]CorrectionBatchItem, error)
	ApplyCorrectionBatch(context.Context, platform.TransactionContext, CorrectionLease, []CorrectionUpdate) (CorrectionApplied, error)
	CompleteCorrection(context.Context, platform.TransactionContext, CorrectionLease) (bool, error)
	FailCorrection(context.Context, platform.TransactionContext, CorrectionLease, string, time.Duration) (bool, error)
}

// ProcessingReadmission re-admits Processing for a Transmission whose target
// resolution a correction changed: it supersedes candidates bound to the old
// Assignment context, admits current ones and refreshes the Transmission's
// processing status in the correction's batch transaction.
type ProcessingReadmission interface {
	ReadmitProcessing(context.Context, platform.TransactionContext, uuid.UUID) error
}

// SuppressionResult is a suppression together with the correction scope its
// change queued. Correction is zero for ReadSuppression.
type SuppressionResult struct {
	Suppression Suppression      `json:"suppression"`
	Correction  CorrectionStatus `json:"correction"`
}
