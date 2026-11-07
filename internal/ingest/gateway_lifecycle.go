package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/units"
)

// GatewayNetworkView is one Network assignment of a Gateway in the management view.
type GatewayNetworkView struct {
	Identifier string    `json:"gatewayNetworkIdentifier"`
	NetworkID  uuid.UUID `json:"networkId"`
	Priority   int       `json:"priority"`
}

// Gateway is the management view of an undeleted Gateway: its configuration and
// current state with the server-recorded contact times (nil when never recorded).
// It never contains credentials.
type Gateway struct {
	ID                 uuid.UUID            `json:"id"`
	Revision           int64                `json:"revision,string"`
	Name               string               `json:"name"`
	Enabled            bool                 `json:"enabled"`
	State              string               `json:"state"`
	LastContactAt      *time.Time           `json:"lastContactAt"`
	LastHeartbeatAt    *time.Time           `json:"lastHeartbeatAt"`
	LastTransmissionAt *time.Time           `json:"lastTransmissionAt"`
	Networks           []GatewayNetworkView `json:"networks"`
}

// GatewayLastReport is the most recent state report of a Gateway.
type GatewayLastReport struct {
	Operation  string `json:"operation"`
	ReasonCode string `json:"reasonCode,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// GatewayDiagnostics is the delegable diagnostic view of a Gateway: state, contact
// times, the last report and the history retention marker. It contains no Network,
// credential or Payload data. A nil LastContactAt means never seen; a non-nil
// HistoryRemovedBefore means older transitions were removed by retention.
type GatewayDiagnostics struct {
	ID                   uuid.UUID          `json:"id"`
	Name                 string             `json:"name"`
	Enabled              bool               `json:"enabled"`
	State                string             `json:"state"`
	LastContactAt        *time.Time         `json:"lastContactAt"`
	LastHeartbeatAt      *time.Time         `json:"lastHeartbeatAt"`
	LastTransmissionAt   *time.Time         `json:"lastTransmissionAt"`
	LastReport           *GatewayLastReport `json:"lastReport"`
	HistoryRemovedBefore *time.Time         `json:"historyRemovedBefore"`
}

// GatewayInput is the caller-supplied content of a new Gateway: a name and the
// initial Network assignments under the ConfigureNetworks rules, which may be empty.
type GatewayInput struct {
	Name     string                `json:"name"`
	Networks []GatewayNetworkInput `json:"networks"`
}

// GatewayChange is the caller-supplied new name and enablement of a Gateway.
type GatewayChange struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// GatewayLifecycleRepository persists the Gateway lifecycle in the caller's
// transaction. Deleted or missing Gateways are authz.ErrNotFound. CreateGateway
// validates the assigned Networks as ReplaceGatewayNetworks does. ListGateways
// returns only Gateways whose every assigned Network belongs to a reachable
// Agency (any Gateway, including one without Networks, when reach.All).
// DeleteGateway soft-deletes, removes the Network assignments, revokes every
// unrevoked credential at the given time and returns the revoked credential IDs.
// RecordChange writes the audit record of a configuration change.
type GatewayLifecycleRepository interface {
	CreateGateway(context.Context, platform.TransactionContext, uuid.UUID, string, []GatewayNetwork) error
	ReadGatewayRecord(context.Context, platform.TransactionContext, uuid.UUID) (Gateway, error)
	ListGateways(context.Context, platform.TransactionContext, authz.AgencyReach, units.PageInput) (units.Page[Gateway], error)
	ChangeGateway(context.Context, platform.TransactionContext, uuid.UUID, int64, GatewayChange) (int64, error)
	DeleteGateway(context.Context, platform.TransactionContext, uuid.UUID, int64, time.Time) ([]uuid.UUID, int64, error)
	ReadGatewayDiagnostics(context.Context, platform.TransactionContext, uuid.UUID) (GatewayDiagnostics, error)
	ListGatewayTransitions(context.Context, platform.TransactionContext, uuid.UUID, units.PageInput) (units.Page[GatewayTransition], error)
	RecordChange(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID, string, time.Time) error
}

// CreateGateway creates an enabled, never-seen Gateway without credentials and with
// the given initial Network assignments. The caller needs gateway.create on the
// Agency of every proposed Network, or an installation-wide grant without Networks.
// command is the actor-bound idempotency key.
func (s *GatewayAdministration) CreateGateway(ctx context.Context, proof identity.Proof, command uuid.UUID,
	input GatewayInput, check func(identity.Authentication) error) (commands.Result, error) {
	if units.ValidateName(input.Name) != nil || command == uuid.Nil || check == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	assignments, err := validateAssignments(input.Networks, true)
	if err != nil {
		return commands.Result{}, err
	}
	fingerprint, err := commands.Fingerprint(input)
	if err != nil {
		return commands.Result{}, err
	}
	var result commands.Result
	err = identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, true, check,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			scope, err := s.resolveAssignments(tx, assignments, nil)
			if err != nil {
				return err
			}
			if err := authorizeGateway(authority, uuid.Nil, scope, now, "gateway.create"); err != nil {
				return err
			}
			actor, err := uuid.Parse(authority.PrincipalID)
			if err != nil {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
			}
			receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
			if err != nil {
				return err
			}
			if found {
				result, err = receipt.Check("gateway.create", fingerprint)
				return err
			}
			id := uuid.New()
			if err := s.lifecycle.CreateGateway(tx.Context(), tx, id, input.Name, assignments); err != nil {
				return err
			}
			if err := s.lifecycle.RecordChange(tx.Context(), tx, actor, id, "gateway.create", now); err != nil {
				return err
			}
			result = commands.Result{ID: id, Revision: 1}
			return s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{ActorID: actor, CommandID: command,
				Operation: "gateway.create", Fingerprint: fingerprint, Result: result, RecordedAt: now})
		})
	return result, err
}

// ReadGateway returns the management view of a Gateway (gateway.read on the Agency
// of every assigned Network). A denied caller gets authz.ErrForbidden when it holds
// another right on the Gateway and authz.ErrNotFound otherwise.
func (s *GatewayAdministration) ReadGateway(ctx context.Context, proof identity.Proof, id uuid.UUID) (Gateway, error) {
	if id == uuid.Nil {
		return Gateway{}, authz.ErrInvalidInput
	}
	var result Gateway
	err := identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, nil,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			if err := s.authorizeExisting(tx, authority, id, now, "gateway.read"); err != nil {
				return err
			}
			var err error
			result, err = s.lifecycle.ReadGatewayRecord(tx.Context(), tx, id)
			return err
		})
	return result, err
}

// ListGateways pages the Gateways whose every assigned Network's Agency the caller
// may read with gateway.read; Gateways without Networks need an installation-wide grant.
func (s *GatewayAdministration) ListGateways(ctx context.Context, proof identity.Proof, page units.PageInput) (units.Page[Gateway], error) {
	page, err := page.Validate()
	if err != nil {
		return units.Page[Gateway]{}, err
	}
	var result units.Page[Gateway]
	err = identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, nil,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			reach, err := authz.ReachableAgencies(authority, "gateway.read", now)
			if err != nil {
				return err
			}
			result, err = s.lifecycle.ListGateways(tx.Context(), tx, reach, page)
			return err
		})
	return result, err
}

// ChangeGateway sets the Gateway's name and enablement at the expected revision
// (gateway.update on every assigned Agency). A disabled Gateway authenticates
// nothing until it is enabled again.
func (s *GatewayAdministration) ChangeGateway(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	change GatewayChange, check func(identity.Authentication) error) (commands.Result, error) {
	if units.ValidateName(change.Name) != nil || id == uuid.Nil || expected <= 0 || check == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, true, check,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			if err := s.authorizeExisting(tx, authority, id, now, "gateway.update"); err != nil {
				return err
			}
			actor, err := uuid.Parse(authority.PrincipalID)
			if err != nil {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
			}
			revision, err := s.lifecycle.ChangeGateway(tx.Context(), tx, id, expected, change)
			if err != nil {
				return err
			}
			result = commands.Result{ID: id, Revision: revision}
			if revision == expected {
				return nil
			}
			return s.lifecycle.RecordChange(tx.Context(), tx, actor, id, "gateway.update", now)
		})
	return result, err
}

// DeleteGateway soft-deletes the Gateway at the expected revision (gateway.delete on
// every assigned Agency): its Network assignments are removed and every unrevoked
// credential is revoked with one identity fact each, in one transaction. Accepted
// Transmissions and the state history are unchanged.
func (s *GatewayAdministration) DeleteGateway(ctx context.Context, proof identity.Proof, id uuid.UUID, expected int64,
	check func(identity.Authentication) error) (commands.Result, error) {
	if id == uuid.Nil || expected <= 0 || check == nil {
		return commands.Result{}, authz.ErrInvalidInput
	}
	var result commands.Result
	err := identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, true, check,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			if err := s.authorizeExisting(tx, authority, id, now, "gateway.delete"); err != nil {
				return err
			}
			actor, err := uuid.Parse(authority.PrincipalID)
			if err != nil {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
			}
			revoked, revision, err := s.lifecycle.DeleteGateway(tx.Context(), tx, id, expected, now)
			if err != nil {
				return err
			}
			for _, credential := range revoked {
				if _, err := s.facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
					ID: uuid.New(), Type: events.ConfigurationChanged,
					Aggregate: events.EntityRef{Type: "credential", ID: credential}, Revision: credentialRevokedRevision,
					OccurredAt: now, ChangeKind: events.CredentialRevoked,
				}); err != nil {
					return err
				}
			}
			result = commands.Result{ID: id, Revision: revision}
			return s.lifecycle.RecordChange(tx.Context(), tx, actor, id, "gateway.delete", now)
		})
	return result, err
}

// ReadDiagnostics returns the Gateway's diagnostic view. The caller needs gateway.read
// on every assigned Agency or gateway.diagnostics.read on the Gateway. A denied
// caller gets authz.ErrForbidden when it holds another right on the Gateway and
// authz.ErrNotFound otherwise.
func (s *GatewayAdministration) ReadDiagnostics(ctx context.Context, proof identity.Proof, id uuid.UUID) (GatewayDiagnostics, error) {
	if id == uuid.Nil {
		return GatewayDiagnostics{}, authz.ErrInvalidInput
	}
	var result GatewayDiagnostics
	err := identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, nil,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			if err := s.authorizeExisting(tx, authority, id, now, "gateway.read", "gateway.diagnostics.read"); err != nil {
				return err
			}
			var err error
			result, err = s.lifecycle.ReadGatewayDiagnostics(tx.Context(), tx, id)
			return err
		})
	return result, err
}

// ListTransitions pages the Gateway's recorded state transitions, newest first,
// with the authorization of ReadDiagnostics.
func (s *GatewayAdministration) ListTransitions(ctx context.Context, proof identity.Proof, id uuid.UUID,
	page units.PageInput) (units.Page[GatewayTransition], error) {
	page, err := page.Validate()
	if err != nil || id == uuid.Nil {
		return units.Page[GatewayTransition]{}, authz.ErrInvalidInput
	}
	var result units.Page[GatewayTransition]
	err = identity.WithAuthority(ctx, s.transactions, s.clock, s.resolver, proof, false, nil,
		func(tx platform.TransactionContext, authority authz.Authority, now time.Time) error {
			if err := s.authorizeExisting(tx, authority, id, now, "gateway.read", "gateway.diagnostics.read"); err != nil {
				return err
			}
			var err error
			result, err = s.lifecycle.ListGatewayTransitions(tx.Context(), tx, id, page)
			return err
		})
	return result, err
}

// gatewayVisibility lists the actions that let a caller know an existing Gateway: any
// right on it. A caller without any of them cannot tell it from a missing Gateway.
var gatewayVisibility = []string{"gateway.read", "gateway.update", "gateway.delete",
	"gateway.diagnostics.read", "gateway.credential.rotate"}

// authorizeExisting locks the Gateway's assignments and requires one of actions on
// the Gateway (see authorizeGateway). A denied caller gets authz.ErrForbidden when it
// holds any right on the Gateway and authz.ErrNotFound otherwise, so Gateway IDs
// cannot be probed.
func (s *GatewayAdministration) authorizeExisting(tx platform.TransactionContext, authority authz.Authority,
	gateway uuid.UUID, now time.Time, actions ...string) error {
	agencies, err := s.currentAgencies(tx, gateway)
	if err != nil {
		return err
	}
	return denyByVisibility(authorizeGateway(authority, gateway, agencies, now, actions...), authority, gateway, agencies, now)
}

// currentAgencies locks the Gateway's assignments and returns the Agencies of its Networks.
func (s *GatewayAdministration) currentAgencies(tx platform.TransactionContext, gateway uuid.UUID) (map[uuid.UUID]bool, error) {
	networks, err := s.repository.GatewayNetworksForAuthorization(tx.Context(), tx, gateway)
	if err != nil {
		return nil, err
	}
	agencies := make(map[uuid.UUID]bool)
	for _, network := range networks {
		agencies[network.AgencyID] = true
	}
	return agencies, nil
}

// denyByVisibility turns a forbidden decision on an existing Gateway into not found
// unless the caller holds any right on the Gateway; other results pass through.
func denyByVisibility(err error, authority authz.Authority, gateway uuid.UUID, agencies map[uuid.UUID]bool,
	now time.Time) error {
	if !errors.Is(err, authz.ErrForbidden) {
		return err
	}
	if visible := authorizeGateway(authority, gateway, agencies, now, gatewayVisibility...); visible != nil {
		if errors.Is(visible, authz.ErrForbidden) {
			return authz.ErrNotFound
		}
		return visible
	}
	return authz.ErrForbidden
}

// resolveAssignments completes each assignment's Protocol and adds its Network's
// Agency to scope (created when nil), reporting an unknown Network as invalid input.
func (s *GatewayAdministration) resolveAssignments(tx platform.TransactionContext, assignments []GatewayNetwork,
	scope map[uuid.UUID]bool) (map[uuid.UUID]bool, error) {
	if scope == nil {
		scope = make(map[uuid.UUID]bool)
	}
	for index := range assignments {
		protocol, agency, err := s.repository.ReadNetwork(tx.Context(), tx, assignments[index].NetworkID)
		if err != nil {
			if errors.Is(err, authz.ErrNotFound) {
				return nil, authz.ErrInvalidInput
			}
			return nil, err
		}
		if !protocol.Valid() {
			return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid current Network Protocol"))
		}
		assignments[index].Protocol = protocol
		scope[agency] = true
	}
	return scope, nil
}

// authorizeGateway succeeds when one of actions is permitted for the Gateway on the
// Agency of every Network it is or will be assigned to, each evaluated with the
// target {AgencyID, GatewayID}; a Gateway without Networks is evaluated with
// {GatewayID} alone, which only "all" and "gateway" scopes match. gateway is
// uuid.Nil for a Gateway that does not exist yet. A non-forbidden failure of any
// evaluation is returned as is.
func authorizeGateway(authority authz.Authority, gateway uuid.UUID, agencies map[uuid.UUID]bool, now time.Time,
	actions ...string) error {
	targets := make([]authz.Target, 0, len(agencies))
	for agency := range agencies {
		targets = append(targets, authz.Target{AgencyID: agency, GatewayID: gateway})
	}
	if len(targets) == 0 {
		targets = append(targets, authz.Target{GatewayID: gateway})
	}
	for _, action := range actions {
		err := authorizeTargets(authority, action, targets, now)
		if err == nil || !errors.Is(err, authz.ErrForbidden) {
			return err
		}
	}
	return authz.ErrForbidden
}

func authorizeTargets(authority authz.Authority, action string, targets []authz.Target, now time.Time) error {
	for _, target := range targets {
		if err := authz.Authorize(authority, action, nil, target, now); err != nil {
			return err
		}
	}
	return nil
}
