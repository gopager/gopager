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
	// now is the batch time; a before context admitted at it is covered by some grant.
	now time.Time
	// key signs the positions of the frames.
	key []byte
}

// beforeBasis is what the contexts a fact names as before the change say about a Message the
// caller can no longer read.
type beforeBasis int

const (
	// basisNone: no before context was readable for the caller, so it never held the Message.
	basisNone beforeBasis = iota
	// basisRemove: a grant without a maximum age covers a before context, so the caller could
	// have loaded the Message and may be told that it is gone.
	basisRemove
	// basisResync: only grants with a maximum age cover a before context. The fact does not say
	// how old the Message's appearance was, so a remove could name a Message too old for the
	// caller ever to have loaded; the view is resynchronized without naming it.
	basisResync
)

// longAgo is an age anchor no maximum age admits, so only grants without one match it.
var longAgo = time.Date(1, time.January, 2, 0, 0, 0, 0, time.UTC)

// beforeBasis evaluates the contexts a fact names as before the change against the caller's
// current grants: first with an anchor no age limit admits, then with an anchor every age
// limit admits.
func (v liveView) beforeBasis(before liveBefore) beforeBasis {
	var targets []authz.Target
	for _, id := range before.units {
		if context, known := v.units[id]; known {
			targets = append(targets, authz.Target{AgencyID: context.AgencyID, UnitID: id, UnitCollections: context.Collections})
		}
	}
	for _, id := range before.networks {
		if agency, known := v.networks[id]; known {
			targets = append(targets, authz.Target{AgencyID: agency, WithoutUnit: true})
		}
	}
	basis := basisNone
	for _, target := range targets {
		target.AgeAnchor = longAgo
		if v.rows.ReadEligible(target) {
			return basisRemove
		}
		target.AgeAnchor = v.now
		if v.rows.ReadEligible(target) {
			basis = basisResync
		}
	}
	return basis
}

// needsResync reports whether a fact can only be reconciled by resynchronizing the view.
func (v liveView) needsResync(fact liveFact) bool {
	if fact.kind != kindChange {
		return false
	}
	if _, matches := v.matches[fact.messageID]; matches || v.eligible[fact.messageID] {
		return false
	}
	return v.beforeBasis(fact.before) == basisResync
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
		case v.eligible[fact.messageID] || v.beforeBasis(fact.before) == basisRemove:
			return []LiveFrame{removeFrame(fact.messageID)}
		}
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
// position, issued at now. It reports false when the frames exceed a batch bound or a fact can
// only be reconciled by a resync, in which case the stream must resynchronize instead of
// delivering a partial view.
func (v liveView) buildFrames(facts []liveFact, now time.Time) ([]LiveFrame, bool) {
	var out []LiveFrame
	size := 0
	for _, fact := range facts {
		if v.needsResync(fact) {
			return nil, false
		}
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
		frames[len(frames)-1].ID = Position{TxID: int64(fact.position.TxID), Seq: fact.position.Seq, Issued: now}.Sign(v.key)
		out = append(out, frames...)
	}
	return out, true
}
