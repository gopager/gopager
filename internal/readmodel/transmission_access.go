package readmodel

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// CheckTransmissionReadable returns nil when the Transmission is a supporting readable
// Transmission of the Message for the authority: one of the Transmissions the Message's detail
// lists under the authority's current grants. Everything else, a Message without an eligible
// appearance included, is authz.ErrNotFound, so a hidden Transmission is indistinguishable from
// a missing one. It runs in the caller's transaction, in which authority was resolved at now.
func (s *Service) CheckTransmissionReadable(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	now time.Time, messageID, transmissionID uuid.UUID) error {
	if messageID == uuid.Nil || transmissionID == uuid.Nil {
		return authz.ErrInvalidInput
	}
	current, err := newAccess(authority, now)
	if err != nil {
		return err
	}
	if !current.rows.Read.Granted() || !current.rows.Detail.Granted() {
		return authz.ErrNotFound
	}
	record, err := s.store.Detail(ctx, tx, DetailQuery{Access: current.rows, MessageID: messageID})
	if err != nil {
		return err
	}
	for _, transmission := range record.Transmissions {
		if transmission.ID == transmissionID {
			return nil
		}
	}
	return authz.ErrNotFound
}
