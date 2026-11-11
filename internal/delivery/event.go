package delivery

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

// GatewayView is the part of a Gateway a Delivery may use, with the Agencies its Networks
// belong to for authorization.
type GatewayView struct {
	ID        uuid.UUID
	Name      string
	AgencyIDs []uuid.UUID
}

// eventView is the classified part of a fact a Delivery may use.
type eventView struct {
	ID            uuid.UUID
	Type          string
	OccurredAt    time.Time
	RecordedAt    time.Time
	ReplayMode    string
	CorrelationID *uuid.UUID
	CausationID   *uuid.UUID
	Data          eventData
}

// eventData holds the data fields Delivery reads from the deliverable fact types.
type eventData struct {
	MessageID      uuid.UUID  `json:"messageId"`
	TransmissionID uuid.UUID  `json:"transmissionId"`
	ResultID       uuid.UUID  `json:"resultId"`
	GatewayID      uuid.UUID  `json:"gatewayId"`
	OldState       string     `json:"oldState"`
	NewState       string     `json:"newState"`
	ContactTime    *time.Time `json:"contactTime"`
	LateJoin       *bool      `json:"lateJoin"`
	Reason         string     `json:"reason"`
}

// parseEvent decodes the envelope fields Delivery needs from a stored record; a Gateway fact's
// Gateway comes from its aggregate reference.
func parseEvent(id uuid.UUID, envelope json.RawMessage) (eventView, error) {
	var decoded struct {
		Type          string     `json:"type"`
		OccurredAt    time.Time  `json:"occurredAt"`
		RecordedAt    time.Time  `json:"recordedAt"`
		ReplayMode    string     `json:"replayMode"`
		CorrelationID *uuid.UUID `json:"correlationId"`
		CausationID   *uuid.UUID `json:"causationId"`
		AggregateRef  struct {
			Type string    `json:"type"`
			ID   uuid.UUID `json:"id"`
		} `json:"aggregateRef"`
		Data eventData `json:"data"`
	}
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return eventView{}, platform.NewFailure(platform.CodeInternalError, err)
	}
	// Gateway facts name their Gateway as the aggregate, not in data.
	if decoded.AggregateRef.Type == "gateway" && decoded.Data.GatewayID == uuid.Nil {
		decoded.Data.GatewayID = decoded.AggregateRef.ID
	}
	return eventView{ID: id, Type: decoded.Type, OccurredAt: decoded.OccurredAt.UTC(), RecordedAt: decoded.RecordedAt.UTC(),
		ReplayMode: decoded.ReplayMode, CorrelationID: decoded.CorrelationID, CausationID: decoded.CausationID,
		Data: decoded.Data}, nil
}
