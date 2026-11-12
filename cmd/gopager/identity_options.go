package main

import "github.com/gopager/gopager/internal/identity"

// identityOptions adjusts the identity services the process composes. Production leaves it
// empty, so passwords use the production Argon2id cost; the test binary sets a reduced-cost
// hasher once in TestMain, because every login service it starts would otherwise pay the full
// cost for its dummy hash and every login.
var identityOptions []identity.Option
