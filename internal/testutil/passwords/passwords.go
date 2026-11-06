// Package passwords provides a reduced-cost password hasher so tests do not pay the
// production Argon2id cost on every hash and login.
package passwords

import "github.com/gopager/gopager/internal/identity"

// Params are the reduced Argon2id parameters used by tests. They are far below
// identity.ProductionPasswordParams and must never be used outside tests.
var Params = identity.PasswordParams{MemoryKiB: 8, Iterations: 1, Parallelism: 1}

// Hasher returns a hasher with the reduced test parameters. It still verifies hashes
// made with the production parameters.
func Hasher() identity.PasswordHasher {
	hasher, err := identity.NewPasswordHasher(Params)
	if err != nil {
		panic(err)
	}
	return hasher
}

// Option makes Bootstrap or NewLoginService use Hasher.
func Option() identity.Option {
	return identity.WithPasswordHasher(Hasher())
}
