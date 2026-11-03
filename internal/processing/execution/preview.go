// Package execution binds transport identity to the store-neutral installed-execution boundary.
package execution

import (
	"context"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// PreviewAuthority retains a transport proof, not an earlier editor authority snapshot.
type PreviewAuthority struct {
	resolver *identity.Resolver
	proof    identity.Proof
}

// NewPreviewAuthority binds the actual requesting editor; HTTP Origin/CSRF checks remain transport-owned.
func NewPreviewAuthority(resolver *identity.Resolver, proof identity.Proof) (*PreviewAuthority, error) {
	if resolver == nil {
		return nil, authz.ErrInvalidInput
	}
	return &PreviewAuthority{resolver: resolver, proof: proof}, nil
}

// ResolvePreviewAuthority revalidates the editor's current credential/account/grants in the caller transaction.
func (p *PreviewAuthority) ResolvePreviewAuthority(ctx context.Context, tx platform.TransactionContext) (authz.Authority, error) {
	authentication, err := p.resolver.Resolve(ctx, tx, p.proof)
	if err != nil {
		return authz.Authority{}, err
	}
	return authentication.Authority, nil
}
