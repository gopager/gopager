package readmodel

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// CheckTransmissionReadable returns the original positions of the Transmission's targets that the
// authority sees (the visible, unsuppressed ones the detail lists) when the Transmission is a supporting readable
// Transmission of the Message for the authority: one of the Transmissions the Message's detail
// lists under the authority's current grants. Everything else, a Message without an eligible
// appearance included, is authz.ErrNotFound, so a hidden Transmission is indistinguishable from
// a missing one. It runs in the caller's transaction, in which authority was resolved at now.
func (s *Service) CheckTransmissionReadable(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	now time.Time, messageID, transmissionID uuid.UUID) ([]int, error) {
	if messageID == uuid.Nil || transmissionID == uuid.Nil {
		return nil, authz.ErrInvalidInput
	}
	current, err := newAccess(authority, now)
	if err != nil {
		return nil, err
	}
	if !current.rows.Read.Granted() || !current.rows.Detail.Granted() {
		return nil, authz.ErrNotFound
	}
	record, err := s.store.Detail(ctx, tx, DetailQuery{Access: current.rows, MessageID: messageID})
	if err != nil {
		return nil, err
	}
	for _, transmission := range record.Transmissions {
		if transmission.ID != transmissionID {
			continue
		}
		positions := make([]int, 0, len(transmission.Targets))
		for _, target := range transmission.Targets {
			positions = append(positions, target.Position)
		}
		return positions, nil
	}
	return nil, authz.ErrNotFound
}
