package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type GatewayAuthentication struct {
	GatewayID    uuid.UUID
	CredentialID uuid.UUID
}

// GatewayCredentialRepository authenticates and performs any pending-credential cutover atomically.
type GatewayCredentialRepository interface {
	AuthenticateGatewayCredential(context.Context, platform.TransactionContext, [32]byte) (GatewayAuthentication, error)
}

type GatewayAuthenticator struct {
	repository GatewayCredentialRepository
}

// NewGatewayAuthenticator creates a proof boundary separate from Principal resolution.
func NewGatewayAuthenticator(repository GatewayCredentialRepository) (*GatewayAuthenticator, error) {
	if repository == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Gateway credential repository"))
	}
	return &GatewayAuthenticator{repository: repository}, nil
}

// Authenticate resolves only the Gateway identified by a valid reusable bearer secret.
func (a *GatewayAuthenticator) Authenticate(ctx context.Context, tx platform.TransactionContext, secret string) (GatewayAuthentication, error) {
	if a == nil || a.repository == nil || ctx == nil || tx == nil {
		return GatewayAuthentication{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Gateway authentication context"))
	}
	hash, err := SecretHash(secret)
	if err != nil {
		return GatewayAuthentication{}, ErrLoginRequired
	}
	authentication, err := a.repository.AuthenticateGatewayCredential(ctx, tx, hash)
	if err != nil {
		return GatewayAuthentication{}, err
	}
	if authentication.GatewayID == uuid.Nil || authentication.CredentialID == uuid.Nil {
		return GatewayAuthentication{}, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("Gateway credential owner returned incomplete identity"))
	}
	return authentication, nil
}
