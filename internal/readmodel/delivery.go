package readmodel

import (
	"context"
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

// WarmClassifications classifies every Workflow definition version not cached yet. Callers that
// project inside their own transaction (ProjectForDelivery) call it beforehand and outside any
// transaction; a version it could not classify stays hidden.
func (s *Service) WarmClassifications(ctx context.Context) error { return s.warmClassifications(ctx) }

// ProjectForDelivery returns the Message as the holder of authority may read it, inside the
// caller's transaction, in which authority was resolved at now. The boolean is false when the
// authority holds no message.read grant or no appearance of the Message is eligible. Results of
// Workflow versions not classified by an earlier WarmClassifications are hidden.
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
	if err := s.readableVersions(ctx, tx, &current); err != nil {
		return DeliveryProjection{}, false, err
	}
	result, err := s.store.Feed(tx.Context(), tx, FeedQuery{Access: current.rows, IDs: []uuid.UUID{messageID},
		ReadableVersions: current.version})
	if err != nil {
		return DeliveryProjection{}, false, err
	}
	if len(result.Items) == 0 {
		return DeliveryProjection{}, false, nil
	}
	record := result.Items[0]
	groups := make([]string, 0, len(current.viewer.groups))
	for group := range current.viewer.groups {
		groups = append(groups, group)
	}
	slices.Sort(groups)
	return DeliveryProjection{Item: current.viewer.feedItem(record, false), Units: slices.Clone(record.Units),
		WithoutUnit: slices.Clone(record.WithoutUnit), Groups: groups}, true, nil
}
