package assignments

import (
	"context"
	"time"

	"github.com/google/uuid"
	units "github.com/gopager/gopager/internal/Units"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// NetworkOwner is implemented by the Radio network owner (transmission intake) and
// reports a Network's current existence, Protocol and home Agency inside the
// caller's transaction.
type NetworkOwner interface {
	ReadNetwork(context.Context, platform.TransactionContext, uuid.UUID) (protocols.Protocol, uuid.UUID, error)
}

// Repository owns current rules and durable invalidation, not historical Message/support tables.
type Repository interface {
	LockRevision(context.Context, platform.TransactionContext) (int64, error)
	LockRevisionForChange(context.Context, platform.TransactionContext) (int64, error)
	CurrentRules(context.Context, platform.TransactionContext, []uuid.UUID) ([]Rule, error)
	ReadRule(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, time.Time) (Rule, error)
	ListRules(context.Context, platform.TransactionContext, authz.Authority, units.PageInput, time.Time) (units.Page[Rule], error)
	SaveRule(context.Context, platform.TransactionContext, authz.Authority, Rule, int64, time.Time) error
	DeleteRule(context.Context, platform.TransactionContext, authz.Authority, Rule, time.Time) error
	MarkPending(context.Context, platform.TransactionContext, []CorrectionScope, string) (int64, error)
	RecordChange(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID, string, time.Time) error
}

// RuleInput omits server-owned ID/revision and preserves the explicit selector/interval.
type RuleInput struct {
	NetworkID  uuid.UUID          `json:"networkId"`
	UnitID     uuid.UUID          `json:"unitId"`
	Protocol   protocols.Protocol `json:"protocol"`
	Selector   Selector           `json:"selector"`
	ValidFrom  *time.Time         `json:"validFrom"`
	ValidUntil *time.Time         `json:"validUntil"`
	Enabled    bool               `json:"enabled"`
}

// Rule constructs the complete server-owned candidate before ordinary validation.
func (input RuleInput) Rule(id uuid.UUID, revision int64) Rule {
	return Rule{ID: id, Revision: revision, NetworkID: input.NetworkID, UnitID: input.UnitID,
		Protocol: input.Protocol, Selector: input.Selector, ValidFrom: input.ValidFrom, ValidUntil: input.ValidUntil, Enabled: input.Enabled}
}

// MatchResult is the resolution of one transmission's targets at assignment
// Revision. Resolutions has one entry per target position; UnitContext holds
// the current context of every assigned Unit.
type MatchResult struct {
	Revision    int64
	Resolutions []Resolution
	UnitContext map[uuid.UUID]units.Context
}
