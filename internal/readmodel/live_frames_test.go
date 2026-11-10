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
		networks: map[uuid.UUID]uuid.UUID{}, rows: rows, now: now, key: []byte("live-frames-test-key")}
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
	merge := liveFact{kind: kindMerge, messageID: message, absorbed: []uuid.UUID{uuid.New(), uuid.New()}}
	if frames := view.frames(merge); frames != nil {
		t.Errorf("a merge never yields Message frames, which could disclose absorbed IDs: %v", frames)
	}
}

func TestBeforeBasisSeparatesUnlimitedAndAgeLimitedGrants(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	agency, unit, collection, network := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	cutoff := now.Add(-time.Hour)
	limited := authz.AccessClause{Kind: "unitCollection", References: []uuid.UUID{collection}, Cutoff: &cutoff}
	view := newView(liveRows(limited), now)
	view.units[unit] = UnitContext{AgencyID: agency, Collections: []uuid.UUID{collection}}
	if got := view.beforeBasis(liveBefore{units: []uuid.UUID{unit}}); got != basisResync {
		t.Errorf("a before Unit reached only by an age-limited grant needs a resync, got %v", got)
	}
	if got := view.beforeBasis(liveBefore{units: []uuid.UUID{uuid.New()}}); got != basisNone {
		t.Errorf("an unknown Unit is no basis, got %v", got)
	}
	view.units[unit] = UnitContext{AgencyID: agency}
	if got := view.beforeBasis(liveBefore{units: []uuid.UUID{unit}}); got != basisNone {
		t.Errorf("a Unit that left the readable collection is no basis, got %v", got)
	}
	both := newView(liveRows(limited, authz.AccessClause{Kind: "unit", References: []uuid.UUID{unit}}), now)
	both.units[unit] = UnitContext{AgencyID: agency, Collections: []uuid.UUID{collection}}
	if got := both.beforeBasis(liveBefore{units: []uuid.UUID{unit}}); got != basisRemove {
		t.Errorf("a grant without a maximum age makes the remove safe, got %v", got)
	}

	agencyRows := liveRows(authz.AccessClause{Kind: "agency", References: []uuid.UUID{agency}})
	less := newView(agencyRows, now)
	less.networks[network] = agency
	if got := less.beforeBasis(liveBefore{networks: []uuid.UUID{network}}); got != basisRemove {
		t.Errorf("the Unit-less context of a readable Agency's Network is a remove basis, got %v", got)
	}
	less.networks[network] = uuid.New()
	if got := less.beforeBasis(liveBefore{networks: []uuid.UUID{network}}); got != basisNone {
		t.Errorf("the Unit-less context of another Agency is no basis, got %v", got)
	}
	noWithoutUnit := agencyRows
	noWithoutUnit.WithoutUnit = authz.ActionAccess{}
	denied := newView(noWithoutUnit, now)
	denied.networks[network] = agency
	if got := denied.beforeBasis(liveBefore{networks: []uuid.UUID{network}}); got != basisNone {
		t.Errorf("a Unit-less context needs without-unit.read, got %v", got)
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

func TestAgeLimitedBeforeBasisResynchronizesWithoutAnID(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	message, unit, agency := uuid.New(), uuid.New(), uuid.New()
	cutoff := now.Add(-24 * time.Hour)
	view := newView(liveRows(authz.AccessClause{Kind: "unit", References: []uuid.UUID{unit}, Cutoff: &cutoff}), now)
	view.units[unit] = UnitContext{AgencyID: agency}
	fact := liveFact{kind: kindChange, messageID: message, before: liveBefore{units: []uuid.UUID{unit}}}
	if frames := view.frames(fact); frames != nil {
		t.Errorf("an age-limited caller gets no remove naming the Message: %v", frames)
	}
	if frames, ok := view.buildFrames([]liveFact{fact}, now); ok || frames != nil {
		t.Errorf("the batch resynchronizes instead: %v %v", frames, ok)
	}
	view.eligible[message] = true
	if frames, ok := view.buildFrames([]liveFact{fact}, now); !ok || len(frames) != 1 || frames[0].Event != FrameRemove {
		t.Errorf("a Message the caller still reads but that no longer matches is removed: %v %v", frames, ok)
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
		{position: events.Position{TxID: 5, Seq: 9}, kind: kindAdd, messageID: other},
		{position: events.Position{TxID: 6, Seq: 10}, kind: kindAdd, messageID: message},
		{position: events.Position{TxID: 6, Seq: 11}, kind: kindChange, messageID: message},
	}
	frames, ok := view.buildFrames(facts, now)
	if !ok || len(frames) != 2 {
		t.Fatalf("frames %v ok %v", frames, ok)
	}
	for i, want := range []Position{{TxID: 6, Seq: 10, Issued: now}, {TxID: 6, Seq: 11, Issued: now}} {
		if got, err := ParseSignedPosition(view.key, frames[i].ID); err != nil || got != want {
			t.Errorf("frame %d ID %q = %+v, %v; want %+v", i, frames[i].ID, got, err, want)
		}
	}
}

func TestBuildFramesBounds(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	message := uuid.New()
	view := newView(liveRows(authz.AccessClause{Kind: "all"}), now)
	view.matches[message] = FeedItem{MessageID: message}
	view.eligible[message] = true

	facts := make([]liveFact, LiveBatchFrames+1)
	for i := range facts {
		facts[i] = liveFact{kind: kindAdd, messageID: message}
	}
	if _, ok := view.buildFrames(facts, now); ok {
		t.Error("more than the frame bound must fail")
	}
	if _, ok := view.buildFrames(facts[:LiveBatchFrames], now); !ok {
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
