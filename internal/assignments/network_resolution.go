package assignments

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// NetworkResolution is the outcome of re-resolving a stored Transmission's Network against the
// current Gateway mapping. Resolved is false when the mapping no longer resolves it (its declared
// identifier is unmapped or no Network of its Protocol is left); NetworkID and Method are then
// empty and the Transmission keeps its Network.
type NetworkResolution struct {
	NetworkID uuid.UUID
	Method    string
	Resolved  bool
}

// NetworkResolver re-resolves the Network of a stored Transmission with its declared identifier,
// the Gateway's current Networks of its Protocol and the current Address rules at its Observation
// time, inside the caller's transaction. It locks the Gateway before reading the mapping, so a
// concurrent mapping replacement either precedes or follows the whole batch. The Radio network
// owner (transmission intake) implements it.
type NetworkResolver interface {
	ResolveNetwork(context.Context, platform.TransactionContext, CorrectionBatchItem) (NetworkResolution, error)
}
