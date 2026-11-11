package delivery

import (
	"context"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

// eventMemo holds what the Subscriptions of one subscriber share while one fact is fanned out in
// one transaction: the resolved authority with its readable field groups and the authorized
// Message projection. Resolving the authority once per event keeps its access revision the same
// for all of that subscriber's Subscriptions. It lives for one FanOut.Handle call and is never
// kept across transactions, so a later access or Message change is always seen.
type eventMemo struct {
	subjects map[uuid.UUID]*subjectMemo
}

// subjectMemo is the shared state of one subscriber within an event.
type subjectMemo struct {
	resolved bool
	// base is the resolved subject; nil when the subscriber cannot act now.
	base *subject
	// projected reports that projection holds the outcome; a nil projection means the subscriber
	// may not see the Message.
	projected  bool
	projection *readmodel.DeliveryProjection
}

func newEventMemo() *eventMemo {
	return &eventMemo{subjects: map[uuid.UUID]*subjectMemo{}}
}

// subject returns a copy of the subscriber's resolved subject that shares the subscriber's
// projection memo, or nil without error when the subscriber cannot act now. The copy lets each
// Subscription add its own splits without affecting the others.
func (m *eventMemo) subject(ctx context.Context, tx platform.TransactionContext, deps Dependencies, event eventView,
	subscriber uuid.UUID) (*subject, error) {
	entry := m.subjects[subscriber]
	if entry == nil {
		entry = &subjectMemo{}
		m.subjects[subscriber] = entry
	}
	if !entry.resolved {
		base, err := resolveSubject(ctx, tx, deps, event, subscriber)
		if err != nil {
			return nil, err
		}
		entry.base, entry.resolved = base, true
	}
	if entry.base == nil {
		return nil, nil
	}
	shared := *entry.base
	shared.memo = entry
	return &shared, nil
}
