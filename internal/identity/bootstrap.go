package identity

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
)

// BootstrapAccount is an internal local-Operator command, never an HTTP input.
// Initial permissions are explicitly supplied, not inferred from an administrator role.
type BootstrapAccount struct {
	ID           uuid.UUID
	GuestID      uuid.UUID
	Username     string
	UsernameKey  string
	PasswordHash string
	Permissions  BootstrapPermissions
	CreatedAt    time.Time
}

// BootstrapPermissions makes every initial capability an explicit local Operator selection.
type BootstrapPermissions struct {
	Actions   []string
	Fields    []string
	Delegable bool
}

// LogValue excludes initial account and credential material.
func (b BootstrapAccount) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", b.ID.String()))
}

type BootstrapRepository interface {
	InitializeIdentity(context.Context, platform.TransactionContext, BootstrapAccount) error
}

// Bootstrap initializes a fresh installation through local Operator composition only.
// Existing installations are rejected; there is no remote first-administrator route.
func Bootstrap(ctx context.Context, transactions Transactions, repository BootstrapRepository,
	clock platform.Clock, facts events.Appender, username, password string, permissions BootstrapPermissions,
) (uuid.UUID, error) {
	if transactions == nil || repository == nil || clock == nil || facts == nil {
		return uuid.Nil, ErrInvalidInput
	}
	key, err := UsernameKey(username)
	if err != nil {
		return uuid.Nil, err
	}
	input := BootstrapAccount{ID: uuid.New(), GuestID: uuid.New(), Username: username,
		UsernameKey: key, Permissions: BootstrapPermissions{Actions: slices.Clone(permissions.Actions),
			Fields: slices.Clone(permissions.Fields), Delegable: permissions.Delegable}}
	if err := authz.ValidateGrant(authz.Grant{SubjectID: input.ID.String(), SubjectKind: "principal",
		Actions: input.Permissions.Actions, Scope: authz.Scope{Kind: "all"}}); err != nil {
		return uuid.Nil, err
	}
	if len(input.Permissions.Fields) > 0 {
		if err := authz.ValidateFieldPermission(authz.FieldPermission{ID: uuid.NewString(), Revision: 1,
			SubjectID: input.ID.String(), SubjectKind: "principal", Fields: input.Permissions.Fields}); err != nil {
			return uuid.Nil, err
		}
	}
	input.PasswordHash, err = HashPassword(password)
	if err != nil {
		return uuid.Nil, err
	}
	err = transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		input.CreatedAt = now
		if err := repository.InitializeIdentity(tx.Context(), tx, input); err != nil {
			return err
		}
		_, err = facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
			ID: uuid.New(), Type: events.ConfigurationChanged,
			Aggregate: events.EntityRef{Type: "account", ID: input.ID}, Revision: 1,
			OccurredAt: now, ChangeKind: events.BootstrapCompleted,
		})
		return err
	})
	if err != nil {
		return uuid.Nil, err
	}
	return input.ID, nil
}
