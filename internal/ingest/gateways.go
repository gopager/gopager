package ingest

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// GatewayNetworkInput is one requested Gateway-to-Network assignment: the
// Gateway's local Identifier (1..128 bytes of non-blank UTF-8) for the Network,
// and whether it is the default Network for that Network's protocol.
type GatewayNetworkInput struct {
	Identifier string    `json:"gatewayNetworkIdentifier"`
	NetworkID  uuid.UUID `json:"networkId"`
	Default    bool      `json:"default"`
}

// GatewayCredentialView is the non-secret status of one Gateway credential.
// Status is "current", "pending" or "revoked"; timestamps are preformatted
// strings and the optional ones are nil when the event has not happened.
type GatewayCredentialView struct {
	ID         uuid.UUID `json:"credentialId"`
	Status     string    `json:"status"`
	CreatedAt  string    `json:"createdAt"`
	LastUsedAt *string   `json:"lastUsedAt,omitempty"`
	RevokedAt  *string   `json:"revokedAt,omitempty"`
}

// GatewayCredentialIssueResult is the outcome of IssueCredential. Secret is
// returned only on first issue and is never stored or replayed; Replayed is
// true (and Secret empty) when the result came from an existing command
// receipt. Status is "current" or "pending".
type GatewayCredentialIssueResult struct {
	CredentialID uuid.UUID `json:"credentialId"`
	Secret       string    `json:"secret,omitempty"`
	Status       string    `json:"status"`
	Revision     int64     `json:"revision,string"`
	Replayed     bool      `json:"-"`
}

// GatewayAdministrationRepository persists Gateway Network assignments and
// credentials. All methods participate in the caller's transaction.
// Mutating methods take the expected revision and must fail with
// authz.ErrRevisionConflict when it is stale; missing Gateways, Networks or
// credentials are reported as authz.ErrNotFound. IssueGatewayCredential stores
// only the secret hash and reports whether the new credential is pending.
type GatewayAdministrationRepository interface {
	GatewayNetworks(context.Context, platform.TransactionContext, uuid.UUID) ([]GatewayNetwork, error)
	GatewayNetworksForAuthorization(context.Context, platform.TransactionContext, uuid.UUID) ([]GatewayNetwork, error)
	ReadNetwork(context.Context, platform.TransactionContext, uuid.UUID) (protocols.Protocol, uuid.UUID, error)
	ReplaceGatewayNetworks(context.Context, platform.TransactionContext, uuid.UUID, int64,
		[]GatewayNetwork) (int64, error)
	GatewayCredentialStatus(context.Context, platform.TransactionContext, uuid.UUID) ([]GatewayCredentialView, error)
	IssueGatewayCredential(context.Context, platform.TransactionContext, uuid.UUID, int64, uuid.UUID,
		[32]byte, time.Time) (bool, int64, error)
	RevokeGatewayCredential(context.Context, platform.TransactionContext, uuid.UUID, uuid.UUID,
		int64, time.Time) (int64, error)
}

// GatewayAdministration implements authorized administrative operations on
// Gateways: Network assignments and credential issue/revocation. Each
// operation runs in one transaction that resolves the caller's authority,
// reads the database clock, authorizes against every Network the Gateway is
// (or will be) assigned to and applies the change. It is safe for concurrent
// use.
type GatewayAdministration struct {
	transactions identity.Transactions
	resolver     *identity.Resolver
	clock        platform.Clock
	repository   GatewayAdministrationRepository
	receipts     commands.Repository
	facts        events.Appender
}

// NewGatewayAdministration returns a GatewayAdministration, failing with
// CodeInvalidInput if any dependency is nil.
func NewGatewayAdministration(transactions identity.Transactions, resolver *identity.Resolver, clock platform.Clock,
	repository GatewayAdministrationRepository, receipts commands.Repository, facts events.Appender) (*GatewayAdministration, error) {
	if transactions == nil || resolver == nil || clock == nil || repository == nil || receipts == nil || facts == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Gateway administration dependency"))
	}
	return &GatewayAdministration{transactions: transactions, resolver: resolver, clock: clock,
		repository: repository, receipts: receipts, facts: facts}, nil
}

// ConfigureNetworks atomically replaces the Gateway's Network assignments,
// guarded by the expected revision, and returns the new revision. Input must
// hold 1..1024 entries with unique identifiers and Network IDs, and every
// protocol present must have exactly one default; a referenced Network that
// does not exist is reported as authz.ErrInvalidInput. check is called with
// the resolved authentication before any change (e.g. to require a fresh
// session); its error aborts the call. The caller needs "gateway.update" on
// every currently and newly assigned Network.
func (s *GatewayAdministration) ConfigureNetworks(ctx context.Context, proof identity.Proof, gateway uuid.UUID,
	expected int64, input []GatewayNetworkInput, check func(identity.Authentication) error) (int64, error) {
	if gateway == uuid.Nil || expected <= 0 || len(input) == 0 || len(input) > 1024 || check == nil {
		return 0, authz.ErrInvalidInput
	}
	seenIdentifiers, seenNetworks := make(map[string]bool), make(map[uuid.UUID]bool)
	assignments := make([]GatewayNetwork, 0, len(input))
	defaults := make(map[protocols.Protocol]int)
	for _, item := range input {
		if item.Identifier == "" || !utf8.ValidString(item.Identifier) || len([]byte(item.Identifier)) > 128 ||
			strings.TrimSpace(item.Identifier) == "" || item.NetworkID == uuid.Nil ||
			seenIdentifiers[item.Identifier] || seenNetworks[item.NetworkID] {
			return 0, authz.ErrInvalidInput
		}
		seenIdentifiers[item.Identifier], seenNetworks[item.NetworkID] = true, true
		assignments = append(assignments, GatewayNetwork{Identifier: item.Identifier, NetworkID: item.NetworkID, Default: item.Default})
	}
	var revision int64
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if err := check(authentication); err != nil {
			return err
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		current, err := s.repository.GatewayNetworksForAuthorization(tx.Context(), tx, gateway)
		if err != nil {
			return err
		}
		scope := make(map[uuid.UUID]bool)
		for _, network := range current {
			scope[network.AgencyID] = true
		}
		for index := range assignments {
			protocol, agency, err := s.repository.ReadNetwork(tx.Context(), tx, assignments[index].NetworkID)
			if err != nil {
				if errors.Is(err, authz.ErrNotFound) {
					return authz.ErrInvalidInput
				}
				return err
			}
			if !protocol.Valid() {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid current Network Protocol"))
			}
			assignments[index].Protocol = protocol
			scope[agency] = true
			if assignments[index].Default {
				defaults[protocol]++
			}
		}
		protocolsInSet := make(map[protocols.Protocol]bool)
		for _, assignment := range assignments {
			protocolsInSet[assignment.Protocol] = true
		}
		for protocol := range protocolsInSet {
			if defaults[protocol] != 1 {
				return authz.ErrInvalidInput
			}
		}
		if err := authorizeGatewayNetworks(authentication.Authority, "gateway.update", scope, now); err != nil {
			return err
		}
		revision, err = s.repository.ReplaceGatewayNetworks(tx.Context(), tx, gateway, expected, assignments)
		return err
	})
	return revision, err
}

// ReadCredentialStatus lists the Gateway's credentials without secrets. The
// caller needs "gateway.read" on every Network the Gateway is assigned to, or
// an installation-wide ("all" scope) grant when it has none. A repository row with an unknown status
// fails with CodeAuthorityUnavailable.
func (s *GatewayAdministration) ReadCredentialStatus(ctx context.Context, proof identity.Proof,
	gateway uuid.UUID) ([]GatewayCredentialView, error) {
	if gateway == uuid.Nil {
		return nil, authz.ErrInvalidInput
	}
	var result []GatewayCredentialView
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		networks, err := s.repository.GatewayNetworksForAuthorization(tx.Context(), tx, gateway)
		if err != nil {
			return err
		}
		scope := make(map[uuid.UUID]bool)
		for _, network := range networks {
			scope[network.AgencyID] = true
		}
		if err := authorizeGatewayNetworks(authentication.Authority, "gateway.read", scope, now); err != nil {
			return err
		}
		result, err = s.repository.GatewayCredentialStatus(tx.Context(), tx, gateway)
		if err == nil && !validGatewayCredentialViews(result) {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid Gateway credential status"))
		}
		return err
	})
	return result, err
}

// IssueCredential creates a new Gateway credential and returns its one-time
// secret. command is the caller-chosen idempotency key: repeating it with the
// same Gateway and expected revision replays the recorded result without the
// secret (Replayed=true), while reusing it with different input fails with
// commands.ErrIdempotencyConflict. The credential, an identity fact and the command
// receipt are written in one transaction. check and authorization behave as
// in ConfigureNetworks.
func (s *GatewayAdministration) IssueCredential(ctx context.Context, proof identity.Proof, gateway, command uuid.UUID,
	expected int64, check func(identity.Authentication) error) (GatewayCredentialIssueResult, error) {
	if gateway == uuid.Nil || command == uuid.Nil || expected <= 0 || check == nil {
		return GatewayCredentialIssueResult{}, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(struct {
		GatewayID uuid.UUID `json:"gatewayId"`
		Revision  int64     `json:"expectedRevision"`
	}{gateway, expected})
	if err != nil {
		return GatewayCredentialIssueResult{}, err
	}
	var result GatewayCredentialIssueResult
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if err := check(authentication); err != nil {
			return err
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		networks, err := s.repository.GatewayNetworksForAuthorization(tx.Context(), tx, gateway)
		if err != nil {
			return err
		}
		scope := make(map[uuid.UUID]bool)
		for _, network := range networks {
			scope[network.AgencyID] = true
		}
		if err := authorizeGatewayNetworks(authentication.Authority, "gateway.update", scope, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authentication.Authority.PrincipalID)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			recorded, err := receipt.Check("gateway.credential.issue", fingerprint)
			if err != nil {
				return err
			}
			views, err := s.repository.GatewayCredentialStatus(tx.Context(), tx, gateway)
			if err != nil {
				return err
			}
			if !validGatewayCredentialViews(views) {
				return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid Gateway credential status"))
			}
			for _, view := range views {
				if view.ID == recorded.ID {
					result = GatewayCredentialIssueResult{CredentialID: recorded.ID, Status: view.Status,
						Revision: recorded.Revision, Replayed: true}
					return nil
				}
			}
			return authz.ErrNotFound
		}
		secret, hash, err := identity.NewSecret()
		if err != nil {
			return err
		}
		credentialID := uuid.New()
		pending, revision, err := s.repository.IssueGatewayCredential(tx.Context(), tx, gateway,
			expected, credentialID, hash, now)
		if err != nil {
			return err
		}
		if _, err := s.facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
			ID: uuid.New(), Type: events.ConfigurationChanged,
			Aggregate: events.EntityRef{Type: "credential", ID: credentialID}, Revision: credentialIssuedRevision,
			OccurredAt: now, ChangeKind: events.CredentialIssued,
		}); err != nil {
			return err
		}
		if err := s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: actor, CommandID: command, Operation: "gateway.credential.issue", Fingerprint: fingerprint,
			Result: commands.Result{ID: credentialID, Revision: revision}, RecordedAt: now,
		}); err != nil {
			return err
		}
		status := "current"
		if pending {
			status = "pending"
		}
		result = GatewayCredentialIssueResult{CredentialID: credentialID, Secret: secret,
			Status: status, Revision: revision}
		return nil
	})
	return result, err
}

// RevokeCredential revokes one credential of the Gateway, guarded by the
// expected revision, and returns the resulting revision. command is an
// idempotency key with the same replay/conflict semantics as in
// IssueCredential. check and authorization behave as in ConfigureNetworks.
func (s *GatewayAdministration) RevokeCredential(ctx context.Context, proof identity.Proof, gateway, credential,
	command uuid.UUID, expected int64, check func(identity.Authentication) error) (int64, error) {
	if gateway == uuid.Nil || credential == uuid.Nil || command == uuid.Nil || expected <= 0 || check == nil {
		return 0, authz.ErrInvalidInput
	}
	fingerprint, err := commands.Fingerprint(struct {
		GatewayID    uuid.UUID `json:"gatewayId"`
		CredentialID uuid.UUID `json:"credentialId"`
		Revision     int64     `json:"expectedRevision"`
	}{gateway, credential, expected})
	if err != nil {
		return 0, err
	}
	var revision int64
	err = s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		authentication, err := s.resolver.Resolve(tx.Context(), tx, proof)
		if err != nil {
			return err
		}
		if err := check(authentication); err != nil {
			return err
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		networks, err := s.repository.GatewayNetworksForAuthorization(tx.Context(), tx, gateway)
		if err != nil {
			return err
		}
		scope := make(map[uuid.UUID]bool)
		for _, network := range networks {
			scope[network.AgencyID] = true
		}
		if err := authorizeGatewayNetworks(authentication.Authority, "gateway.update", scope, now); err != nil {
			return err
		}
		actor, err := uuid.Parse(authentication.Authority.PrincipalID)
		if err != nil {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, err)
		}
		receipt, found, err := s.receipts.ReadCommandReceipt(tx.Context(), tx, actor, command)
		if err != nil {
			return err
		}
		if found {
			recorded, err := receipt.Check("gateway.credential.revoke", fingerprint)
			if err != nil {
				return err
			}
			revision = recorded.Revision
			return nil
		}
		revision, err = s.repository.RevokeGatewayCredential(tx.Context(), tx, gateway, credential, expected, now)
		if err != nil {
			return err
		}
		if _, err := s.facts.AppendIdentityFact(tx.Context(), tx, events.IdentityFact{
			ID: uuid.New(), Type: events.ConfigurationChanged,
			Aggregate: events.EntityRef{Type: "credential", ID: credential}, Revision: credentialRevokedRevision,
			OccurredAt: now, ChangeKind: events.CredentialRevoked,
		}); err != nil {
			return err
		}
		return s.receipts.SaveCommandReceipt(tx.Context(), tx, commands.Receipt{
			ActorID: actor, CommandID: command, Operation: "gateway.credential.revoke", Fingerprint: fingerprint,
			Result: commands.Result{ID: credential, Revision: revision}, RecordedAt: now,
		})
	})
	return revision, err
}

// A Gateway credential's identity facts form a two-step lifecycle on the
// credential aggregate: issuance is revision 1 and the single, final revocation
// (explicit or by rotation cutover) is revision 2.
const (
	credentialIssuedRevision  int64 = 1
	credentialRevokedRevision int64 = 2
)

func validGatewayCredentialViews(views []GatewayCredentialView) bool {
	for _, view := range views {
		if view.ID == uuid.Nil || view.Status != "current" && view.Status != "pending" && view.Status != "revoked" {
			return false
		}
	}
	return true
}

// authorizeGatewayNetworks requires action on the Agency of every Network the Gateway is or will be
// assigned to; a Gateway without Networks needs an installation-wide grant.
func authorizeGatewayNetworks(authority authz.Authority, action string, agencies map[uuid.UUID]bool, now time.Time) error {
	if len(agencies) == 0 {
		return authz.Authorize(authority, action, nil, authz.Target{}, now)
	}
	for agency := range agencies {
		if err := authz.Authorize(authority, action, nil, authz.Target{AgencyID: agency}, now); err != nil {
			return err
		}
	}
	return nil
}
