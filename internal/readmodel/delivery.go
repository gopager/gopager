package readmodel

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// DeliveryProjection is the view of one Message that a Delivery may be materialized from: the
// feed item under the subscriber's current authority, plus the eligible appearances with their
// first observations, which serve as age anchors and split identities but are not output.
type DeliveryProjection struct {
	// Item is the Message as the subscriber's feed shows it.
	Item FeedItem
	// Units are the eligible Unit appearances.
	Units []UnitRecord
	// WithoutUnit are the eligible Unit-less appearances.
	WithoutUnit []WithoutUnitRecord
	// Groups are the field groups the subscriber may read, sorted.
	Groups []string
}

// ErrClassificationUnavailable reports that the Message has a published result of a Workflow
// version whose classification is not known yet (created after the last warm-up, or its
// classification failed). A Delivery projection is then retried instead of treating the result
// as hidden, which would lose a notification for good.
var ErrClassificationUnavailable error = platform.NewFailure(platform.CodeStorageUnavailable,
	errors.New("workflow version classification unavailable"))

// WarmClassifications classifies every Workflow definition version not cached yet. Callers that
// project inside their own transaction (ProjectForDelivery) call it beforehand and outside any
// transaction; a version it could not classify stays hidden.
func (s *Service) WarmClassifications(ctx context.Context) error { return s.warmClassifications(ctx) }

// ProjectForDelivery returns the Message as the holder of authority may read it, inside the
// caller's transaction, in which authority was resolved at now. The boolean is false when the
// authority holds no message.read grant or no appearance of the Message is eligible. A published
// result of a Workflow version that an earlier WarmClassifications could not classify fails with
// ErrClassificationUnavailable when the subscriber reads `derived`.
func (s *Service) ProjectForDelivery(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	now time.Time, messageID uuid.UUID) (DeliveryProjection, bool, error) {
	if messageID == uuid.Nil {
		return DeliveryProjection{}, false, authz.ErrInvalidInput
	}
	current, err := newAccess(authority, now)
	if err != nil {
		return DeliveryProjection{}, false, err
	}
	if !current.rows.Read.Granted() {
		return DeliveryProjection{}, false, nil
	}
	unknown, err := s.deliveryVersions(ctx, tx, &current)
	if err != nil {
		return DeliveryProjection{}, false, err
	}
	// Results of unclassified versions are loaded only to detect them; the projection never shows
	// them, because viewer.versions holds only readable versions.
	result, err := s.store.Feed(tx.Context(), tx, FeedQuery{Access: current.rows, IDs: []uuid.UUID{messageID},
		ReadableVersions: append(slices.Clone(current.version), unknown...)})
	if err != nil {
		return DeliveryProjection{}, false, err
	}
	if len(result.Items) == 0 {
		return DeliveryProjection{}, false, nil
	}
	record := result.Items[0]
	for _, derived := range record.Derived {
		if slices.Contains(unknown, derived.VersionID) {
			return DeliveryProjection{}, false, ErrClassificationUnavailable
		}
	}
	groups := make([]string, 0, len(current.viewer.groups))
	for group := range current.viewer.groups {
		groups = append(groups, group)
	}
	slices.Sort(groups)
	return DeliveryProjection{Item: current.viewer.feedItem(record, false), Units: slices.Clone(record.Units),
		WithoutUnit: slices.Clone(record.WithoutUnit), Groups: groups}, true, nil
}

// deliveryVersions sets the readable Workflow versions of current like readableVersions and
// returns the versions whose classification is not known. Without the derived group nothing is
// readable and nothing needs to be known.
func (s *Service) deliveryVersions(ctx context.Context, tx platform.TransactionContext, current *access) ([]uuid.UUID, error) {
	if !current.viewer.can("derived") {
		return nil, nil
	}
	versions, err := s.store.WorkflowVersions(tx.Context(), tx)
	if err != nil {
		return nil, err
	}
	var unknown []uuid.UUID
	current.viewer.versions = make(map[uuid.UUID]bool, len(versions))
	for _, version := range versions {
		groups := s.cachedClassification(version)
		if groups == nil {
			unknown = append(unknown, version)
			continue
		}
		if !slices.ContainsFunc(groups, func(group string) bool { return !current.viewer.can(group) }) {
			current.viewer.versions[version] = true
			current.version = append(current.version, version)
		}
	}
	return unknown, nil
}
