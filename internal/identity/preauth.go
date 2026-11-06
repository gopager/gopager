package identity

import (
	"context"
	"errors"

	"github.com/gopager/gopager/internal/platform"
)

// Preauthenticator runs Resolver.Authenticate in its own short transaction. HTTP adapters use it to
// answer LOGIN_REQUIRED before they read a request body. It does not replace the authority
// resolution inside the service transaction (see Resolver.Authenticate for why both exist).
// It is safe for concurrent use.
type Preauthenticator struct {
	transactions Transactions
	resolver     *Resolver
}

// NewPreauthenticator returns a Preauthenticator, failing with CodeInvalidInput if a dependency is nil.
func NewPreauthenticator(transactions Transactions, resolver *Resolver) (*Preauthenticator, error) {
	if transactions == nil || resolver == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing pre-authentication dependency"))
	}
	return &Preauthenticator{transactions: transactions, resolver: resolver}, nil
}

// Authenticate checks the proof's credential in one short transaction. An anonymous proof has no
// credential and passes without touching the database; the operation then resolves the guest
// account itself.
func (p *Preauthenticator) Authenticate(ctx context.Context, proof Proof) error {
	if proof.Kind == AnonymousProof {
		return nil
	}
	return p.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		return p.resolver.Authenticate(tx.Context(), tx, proof)
	})
}
