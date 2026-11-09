package readmodel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
)

func liveRows(clauses ...authz.AccessClause) authz.MessageAccess {
	read := authz.ActionAccess{Owner: clauses}
	return authz.MessageAccess{Read: read, Detail: read, WithoutUnit: authz.ActionAccess{Owner: []authz.AccessClause{{Kind: "all"}}}}
}

func newView(rows authz.MessageAccess, now time.Time) liveView {
	return liveView{matches: map[uuid.UUID]FeedItem{}, eligible: map[uuid.UUID]bool{}, units: map[uuid.UUID]UnitContext{},
		networks: map[uuid.UUID]uuid.UUID{}, rows: rows, now: now}
}

func decodeFrame(t *testing.T, frame LiveFrame) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(frame.Data, &out); err != nil {
		t.Fatalf("%s data %q: %v", frame.Event, frame.Data, err)
	}
	return out
}

func TestFramesPerFactKind(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	message, other := uuid.New(), uuid.New()
	view := newView(liveRows(authz.AccessClause{Kind: "all"}), now)
	view.matches[message] = FeedItem{MessageID: message}
	view.eligible[message] = true
	view.eligible[other] = true

	if frames := view.frames(liveFact{kind: kindAdd, messageID: message}); len(frames) != 1 || frames[0].Event != FrameUpsert {
		t.Errorf("an added Message that matches is upserted: %v", frames)
	}
	if frames := view.frames(liveFact{kind: kindAdd, messageID: other}); frames != nil {
		t.Errorf("an added Message that does not match yields nothing: %v", frames)
	}
	if frames := view.frames(liveFact{kind: kindChange, messageID: other}); len(frames) != 1 || frames[0].Event != FrameRemove ||
		decodeFrame(t, frames[0])["messageId"] != other.String() {
		t.Errorf("an eligible Message that stopped matching is removed: %v", frames)
	}
	if frames := view.frames(liveFact{kind: kindChange, messageID: uuid.New()}); frames != nil {
		t.Errorf("hidden traffic yields nothing: %v", frames)
	}
	absorbed := []uuid.UUID{uuid.New(), uuid.New()}
	frames := view.frames(liveFact{kind: kindMerge, messageID: message, absorbed: absorbed})
	if len(frames) != 3 || frames[0].Event != FrameUpsert || frames[1].Event != FrameRemove || frames[2].Event != FrameRemove {
		t.Errorf("a merge upserts the survivor and removes absorbed Messages: %v", frames)
	}
	if frames := view.frames(liveFact{kind: kindMerge, messageID: uuid.New(), absorbed: absorbed}); frames != nil {
		t.Errorf("a merge into a Message the caller cannot read yields nothing: %v", frames)
	}
	removeOnly := view.frames(liveFact{kind: kindMerge, messageID: other, absorbed: absorbed})
	if len(removeOnly) != 2 {
		t.Errorf("a merge into an eligible Message that does not match only removes: %v", removeOnly)
	}
}

func TestBeforeBasisIgnoresAgeAndUsesCurrentContexts(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	agency, unit, collection, network := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	cutoff := now.Add(-time.Hour)
	rows := liveRows(authz.AccessClause{Kind: "unitCollection", References: []uuid.UUID{collection}, Cutoff: &cutoff})
	view := newView(rows, now)
	view.units[unit] = UnitContext{AgencyID: agency, Collections: []uuid.UUID{collection}}
	if !view.beforeEligible(liveBefore{units: []uuid.UUID{unit}}) {
		t.Error("a before Unit in a readable collection is eligible although the Message is older than the maximum age")
	}
	if view.beforeEligible(liveBefore{units: []uuid.UUID{uuid.New()}}) {
		t.Error("an unknown Unit is not eligible")
	}
	view.units[unit] = UnitContext{AgencyID: agency}
	if view.beforeEligible(liveBefore{units: []uuid.UUID{unit}}) {
		t.Error("a Unit that left the readable collection is not eligible")
	}

	agencyRows := liveRows(authz.AccessClause{Kind: "agency", References: []uuid.UUID{agency}})
	less := newView(agencyRows, now)
	less.networks[network] = agency
	if !less.beforeEligible(liveBefore{networks: []uuid.UUID{network}}) {
		t.Error("the Unit-less context of a readable Agency's Network is eligible")
	}
	less.networks[network] = uuid.New()
	if less.beforeEligible(liveBefore{networks: []uuid.UUID{network}}) {
		t.Error("the Unit-less context of another Agency is not eligible")
	}
	noWithoutUnit := agencyRows
	noWithoutUnit.WithoutUnit = authz.ActionAccess{}
	denied := newView(noWithoutUnit, now)
	denied.networks[network] = agency
	if denied.beforeEligible(liveBefore{networks: []uuid.UUID{network}}) {
		t.Error("a Unit-less context needs without-unit.read")
	}
}

func TestBeforeBasisDrivesRemove(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	message, unit, agency := uuid.New(), uuid.New(), uuid.New()
	view := newView(liveRows(authz.AccessClause{Kind: "unit", References: []uuid.UUID{unit}}), now)
	view.units[unit] = UnitContext{AgencyID: agency}
	fact := liveFact{kind: kindChange, messageID: message, before: liveBefore{units: []uuid.UUID{unit}}}
	if frames := view.frames(fact); len(frames) != 1 || frames[0].Event != FrameRemove {
		t.Errorf("a Message that moved out of the caller's reach is removed through the before Unit: %v", frames)
	}
	fact.before = liveBefore{units: []uuid.UUID{uuid.New()}}
	if frames := view.frames(fact); frames != nil {
		t.Errorf("a correction between hidden Units yields nothing: %v", frames)
	}
}

func TestBuildFramesStampsLastFrameOfEachFact(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_800_000_000, 0).UTC()
	message, other := uuid.New(), uuid.New()
	view := newView(liveRows(authz.AccessClause{Kind: "all"}), now)
	view.matches[message] = FeedItem{MessageID: message}
	view.eligible[message] = true
	facts := []liveFact{
		{position: events.Position{TxID: 5, Seq: 9}, kind: kindMerge, messageID: message, absorbed: []uuid.UUID{uuid.New()}},
		{position: events.Position{TxID: 6, Seq: 10}, kind: kindAdd, messageID: other},
		{position: events.Position{TxID: 6, Seq: 11}, kind: kindAdd, messageID: message},
	}
	frames, ok := view.buildFrames(facts, now)
	if !ok || len(frames) != 3 {
		t.Fatalf("frames %v ok %v", frames, ok)
	}
	if frames[0].ID != "" || frames[1].ID != "5-9-1800000000" || frames[2].ID != "6-11-1800000000" {
		t.Errorf("IDs %q %q %q", frames[0].ID, frames[1].ID, frames[2].ID)
	}
}

func TestBuildFramesBounds(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	message := uuid.New()
	view := newView(liveRows(authz.AccessClause{Kind: "all"}), now)
	view.matches[message] = FeedItem{MessageID: message}
	view.eligible[message] = true

	absorbed := make([]uuid.UUID, LiveBatchFrames)
	for i := range absorbed {
		absorbed[i] = uuid.New()
	}
	if _, ok := view.buildFrames([]liveFact{{kind: kindMerge, messageID: message, absorbed: absorbed}}, now); ok {
		t.Error("more than the frame bound must fail")
	}
	if _, ok := view.buildFrames([]liveFact{{kind: kindMerge, messageID: message, absorbed: absorbed[:LiveBatchFrames-1]}}, now); !ok {
		t.Error("exactly the frame bound must pass")
	}

	view.matches[message] = FeedItem{MessageID: message, Payload: json.RawMessage(`"` + strings.Repeat("x", LiveBatchBytes) + `"`)}
	if _, ok := view.buildFrames([]liveFact{{kind: kindAdd, messageID: message}}, now); ok {
		t.Error("more than the byte bound must fail")
	}
}

func TestControlFrames(t *testing.T) {
	t.Parallel()
	if frame := ErrorFrame("LOGIN_REQUIRED"); frame.Event != FrameError || string(frame.Data) != `{"code":"LOGIN_REQUIRED"}` {
		t.Errorf("error frame %+v", frame)
	}
	if frame := ResyncFrame(); frame.Event != FrameResync || string(frame.Data) != `{}` {
		t.Errorf("resync frame %+v", frame)
	}
	if frame := HeartbeatFrame("1-2-3"); frame.Event != FrameHeartbeat || frame.ID != "1-2-3" || string(frame.Data) != `{}` {
		t.Errorf("heartbeat frame %+v", frame)
	}
}
