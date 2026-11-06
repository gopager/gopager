package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

type gatewayCredentialFixture struct {
	authentication GatewayAuthentication
	err            error
	hash           [32]byte
}

func (f *gatewayCredentialFixture) AuthenticateGatewayCredential(_ context.Context,
	_ platform.TransactionContext, hash [32]byte) (GatewayAuthentication, error) {
	f.hash = hash
	return f.authentication, f.err
}

func TestGatewayAuthenticatorKeepsGatewayProofSeparate(t *testing.T) {
	t.Parallel()
	secret, hash, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	expected := GatewayAuthentication{GatewayID: uuid.New(), CredentialID: uuid.New()}
	repository := &gatewayCredentialFixture{authentication: expected}
	authenticator, err := NewGatewayAuthenticator(repository)
	if err != nil {
		t.Fatal(err)
	}
	authentication, err := authenticator.Authenticate(t.Context(), platformFixtureTransaction{}, secret)
	if err != nil || authentication != expected || repository.hash != hash {
		t.Fatalf("authentication=%+v err=%v", authentication, err)
	}
	if _, err := authenticator.Authenticate(t.Context(), platformFixtureTransaction{}, "invalid"); !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("invalid proof error=%v", err)
	}
}

func TestGatewayAuthenticatorRejectsMissingRepositoryIdentity(t *testing.T) {
	t.Parallel()
	authenticator, err := NewGatewayAuthenticator(&gatewayCredentialFixture{})
	if err != nil {
		t.Fatal(err)
	}
	secret, _, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authenticator.Authenticate(t.Context(), platformFixtureTransaction{}, secret); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("missing identity error=%v", err)
	}
}

type platformFixtureTransaction struct{}

func (platformFixtureTransaction) Context() context.Context { return context.Background() }
func (platformFixtureTransaction) ID() uuid.UUID            { return uuid.New() }
