package ingest

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/platform"
)

// GatewayNetworkReader returns a Gateway's current Network mapping, locking the Gateway row for the
// rest of the caller's transaction so a mapping replacement cannot interleave; the intake store
// implements it.
type GatewayNetworkReader interface {
	GatewayNetworks(context.Context, platform.TransactionContext, uuid.UUID) ([]GatewayNetwork, error)
}

// NetworkReResolver re-resolves the Network of an accepted Transmission for the correction runner
// with the same rules as intake (ResolveNetwork): its stored declaration, the Gateway's current
// Networks and the current Address rules at its Observation time. It implements
// assignments.NetworkResolver and is safe for concurrent use.
type NetworkReResolver struct {
	networks GatewayNetworkReader
	matcher  NetworkMatcher
}

// NewNetworkReResolver returns a NetworkReResolver, failing with CodeInvalidInput if a dependency
// is nil.
func NewNetworkReResolver(networks GatewayNetworkReader, matcher NetworkMatcher) (*NetworkReResolver, error) {
	if networks == nil || matcher == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Network re-resolution dependency"))
	}
	return &NetworkReResolver{networks: networks, matcher: matcher}, nil
}

// ResolveNetwork resolves item against the Gateway's current mapping. A mapping that no longer
// resolves it (unmapped declaration, no Network of its Protocol) is reported as unresolved, not as
// an error; storage and matcher failures are returned.
func (r *NetworkReResolver) ResolveNetwork(ctx context.Context, tx platform.TransactionContext,
	item assignments.CorrectionBatchItem) (assignments.NetworkResolution, error) {
	assigned, err := r.networks.GatewayNetworks(ctx, tx, item.GatewayID)
	if err != nil {
		return assignments.NetworkResolution{}, err
	}
	submission := ValidatedSubmission{Submission: Submission{Protocol: item.Protocol,
		GatewayNetworkIdentifier: item.DeclaredNetwork, TransmissionTargets: item.Targets},
		ObservedAt: item.ObservedAt}
	matches, err := addressMatches(tx, r.matcher, submission, assigned)
	if err != nil {
		return assignments.NetworkResolution{}, err
	}
	network, method, err := ResolveNetwork(submission, assigned, matches)
	if errors.Is(err, ErrInvalidSubmission) {
		return assignments.NetworkResolution{}, nil
	}
	if err != nil {
		return assignments.NetworkResolution{}, err
	}
	if method == "address-priority" {
		slog.Warn("Gateway Network re-resolution was ambiguous; highest-priority matching Network applied",
			"transmissionId", item.TransmissionID)
	}
	return assignments.NetworkResolution{NetworkID: network, Method: method, Resolved: true}, nil
}

var _ assignments.NetworkResolver = (*NetworkReResolver)(nil)
