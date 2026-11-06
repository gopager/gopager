package processing

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// WorkflowConfiguration is revisioned metadata, separate from saved immutable definitions.
type WorkflowConfiguration struct {
	ID              uuid.UUID   `json:"id"`
	Revision        int64       `json:"revision,string"`
	Name            string      `json:"name"`
	Scope           authz.Scope `json:"scope"`
	Enabled         bool        `json:"enabled"`
	ActiveVersionID *uuid.UUID  `json:"activeVersionId,omitempty"`
	CreatedAt       time.Time   `json:"createdAt"`
	UpdatedAt       time.Time   `json:"updatedAt"`
	Deleted         bool        `json:"-"`
}

// Validate checks metadata without inventing scope/enable defaults or coercing operator names.
func (w WorkflowConfiguration) Validate() error {
	if w.ID == uuid.Nil || w.Revision <= 0 || !utf8.ValidString(w.Name) ||
		strings.TrimSpace(w.Name) == "" || strings.ContainsRune(w.Name, '\x00') ||
		!authz.ValidScope(w.Scope) || !slices.Contains([]string{"all", "network", "unit"}, w.Scope.Kind) ||
		(w.ActiveVersionID != nil && *w.ActiveVersionID == uuid.Nil) ||
		(w.Deleted && (w.Enabled || w.ActiveVersionID != nil)) {
		return invalid(errors.New("invalid Workflow configuration"))
	}
	return nil
}

// Version records an immutable saved definition; saving alone does not activate it.
type Version struct {
	ID         uuid.UUID  `json:"id"`
	WorkflowID uuid.UUID  `json:"workflowId"`
	Definition Definition `json:"definition"`
	CreatedAt  time.Time  `json:"createdAt"`
	CreatedBy  uuid.UUID  `json:"createdBy"`
}

// PageInput selects a page of a list read. Offset must be non-negative and Limit within
// 1..MaxCollection; services reject other values as invalid input.
type PageInput struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// Page is one page of a list read; Total counts all matching items, not only Items.
type Page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

// ConfigurationRepository keeps native filtered reads and expected-revision writes in caller transactions.
// Services own proof/CSRF/scope validation and atomically participate in receipts/audit/facts.
type ConfigurationRepository interface {
	LockRevision(context.Context, platform.TransactionContext) (int64, error)
	LockRevisionForChange(context.Context, platform.TransactionContext) (int64, error)
	ReadWorkflow(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, time.Time) (WorkflowConfiguration, error)
	ListWorkflows(context.Context, platform.TransactionContext, authz.Authority, PageInput, time.Time) (Page[WorkflowConfiguration], error)
	ReadVersion(context.Context, platform.TransactionContext, authz.Authority, uuid.UUID, uuid.UUID, time.Time) (Version, error)
	ReadVersionForAction(context.Context, platform.TransactionContext, authz.Authority, string, uuid.UUID, uuid.UUID, time.Time) (Version, error)
	ListVersions(context.Context, platform.TransactionContext, authz.Authority, uuid.UUID, PageInput, time.Time) (Page[Version], error)
	SaveWorkflow(context.Context, platform.TransactionContext, authz.Authority, string, WorkflowConfiguration, int64, time.Time) error
	SaveVersion(context.Context, platform.TransactionContext, Version) error
	RecordChange(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID, string, time.Time) error
}
