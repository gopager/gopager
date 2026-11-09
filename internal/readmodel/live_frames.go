package readmodel

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
)

// Bounds of one live batch.
const (
	// LiveBatchFacts is the most facts one batch reads from the log.
	LiveBatchFacts = 256
	// LiveBatchFrames is the most frames one batch may produce.
	LiveBatchFrames = 256
	// LiveBatchBytes is the most encoded frame data one batch may produce.
	LiveBatchBytes = 1 << 20
)

// Frame events of the live stream.
const (
	// FrameUpsert carries a feed item the client places or replaces.
	FrameUpsert = "upsert"
	// FrameRemove carries {"messageId"} of a Message the client drops.
	FrameRemove = "remove"
	// FrameResync tells the client to reload from a fresh page; the stream ends after it.
	FrameResync = "resync-required"
	// FrameError carries {"code"}; the stream ends after it.
	FrameError = "error"
	// FrameHeartbeat keeps the stream alive and carries the current position as its ID.
	FrameHeartbeat = "heartbeat"
)

// LiveFrame is one server-sent event. ID is the position a client resumes after; the frames of
// one fact carry it on the last frame only.
type LiveFrame struct {
	Event string
	ID    string
	Data  json.RawMessage
}

// ResyncFrame returns the frame that ends a stream whose view cannot be continued.
func ResyncFrame() LiveFrame { return LiveFrame{Event: FrameResync, Data: json.RawMessage(`{}`)} }

// ErrorFrame returns the frame that ends a stream with a public error code.
func ErrorFrame(code string) LiveFrame {
	data, _ := json.Marshal(struct {
		Code string `json:"code"`
	}{code})
	return LiveFrame{Event: FrameError, Data: data}
}

// HeartbeatFrame returns the keep-alive frame carrying position as its ID.
func HeartbeatFrame(position string) LiveFrame {
	return LiveFrame{Event: FrameHeartbeat, ID: position, Data: json.RawMessage(`{}`)}
}

// liveView is everything the evaluation of a batch's facts needs, read in the batch's
// transaction: which affected Messages match the view, which are eligible at all, and the
// current contexts of the before basis.
type liveView struct {
	matches  map[uuid.UUID]FeedItem
	eligible map[uuid.UUID]bool
	units    map[uuid.UUID]UnitContext
	networks map[uuid.UUID]uuid.UUID
	rows     authz.MessageAccess
	// now anchors the before-basis eligibility, so the age of a row is not reevaluated.
	now time.Time
}

// beforeEligible reports whether any context a fact names as before the change was readable for
// the caller. The age anchor is set to now, so a maximum age never excludes a context: the
// client already holds the Message, and the question is only whether the caller may be told that
// it is gone.
func (v liveView) beforeEligible(before liveBefore) bool {
	for _, id := range before.units {
		context, known := v.units[id]
		if known && v.rows.ReadEligible(authz.Target{AgencyID: context.AgencyID, UnitID: id,
			UnitCollections: context.Collections, AgeAnchor: v.now}) {
			return true
		}
	}
	for _, id := range before.networks {
		agency, known := v.networks[id]
		if known && v.rows.ReadEligible(authz.Target{AgencyID: agency, WithoutUnit: true, AgeAnchor: v.now}) {
			return true
		}
	}
	return false
}

// frames renders one fact. Hidden traffic produces none.
func (v liveView) frames(fact liveFact) []LiveFrame {
	item, matches := v.matches[fact.messageID]
	switch fact.kind {
	case kindAdd:
		if matches {
			return []LiveFrame{upsertFrame(item)}
		}
	case kindChange:
		switch {
		case matches:
			return []LiveFrame{upsertFrame(item)}
		case v.eligible[fact.messageID] || v.beforeEligible(fact.before):
			return []LiveFrame{removeFrame(fact.messageID)}
		}
	case kindMerge:
		if !v.eligible[fact.messageID] && !matches {
			return nil
		}
		var out []LiveFrame
		if matches {
			out = append(out, upsertFrame(item))
		}
		for _, id := range fact.absorbed {
			out = append(out, removeFrame(id))
		}
		return out
	}
	return nil
}

func upsertFrame(item FeedItem) LiveFrame {
	data, _ := json.Marshal(item)
	return LiveFrame{Event: FrameUpsert, Data: data}
}

func removeFrame(id uuid.UUID) LiveFrame {
	data, _ := json.Marshal(struct {
		MessageID uuid.UUID `json:"messageId"`
	}{id})
	return LiveFrame{Event: FrameRemove, Data: data}
}

// buildFrames renders the facts in log order and stamps each fact's last frame with its
// position, issued at now. It reports false when the frames exceed a batch bound, in which case
// the stream must resynchronize instead of delivering a partial view.
func (v liveView) buildFrames(facts []liveFact, now time.Time) ([]LiveFrame, bool) {
	var out []LiveFrame
	size := 0
	for _, fact := range facts {
		frames := v.frames(fact)
		if len(frames) == 0 {
			continue
		}
		for _, frame := range frames {
			size += len(frame.Data)
		}
		if len(out)+len(frames) > LiveBatchFrames || size > LiveBatchBytes {
			return nil, false
		}
		frames[len(frames)-1].ID = Position{TxID: int64(fact.position.TxID), Seq: fact.position.Seq, Issued: now}.String()
		out = append(out, frames...)
	}
	return out, true
}
