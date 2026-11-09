package readmodel

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// DefaultLiveReplayWindow is how long a position stays usable to resume a stream.
const DefaultLiveReplayWindow = 24 * time.Hour

// UnitContext is the current eligibility context of a Unit: its Agency and Unit collections.
type UnitContext struct {
	AgencyID    uuid.UUID
	Collections []uuid.UUID
}

// LiveStore is the persistence port of live updates beyond the feed query. Every method runs in
// the batch's transaction.
type LiveStore interface {
	// CurrentTxID returns the highest transaction ID a position handed out so far can carry.
	CurrentTxID(ctx context.Context, tx platform.TransactionContext) (int64, error)
	// UnitContexts returns the current Agency and collections of the listed Units; unknown
	// Units are absent.
	UnitContexts(ctx context.Context, tx platform.TransactionContext, ids []uuid.UUID) (map[uuid.UUID]UnitContext, error)
	// NetworkAgencies returns the Agency of each listed Network; unknown Networks are absent.
	NetworkAgencies(ctx context.Context, tx platform.TransactionContext, ids []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
}

// Live produces the frames of Message live streams. It is not a durable consumer: every stream
// keeps its own position and reads the fact log directly. Create it with NewLive; it is safe
// for concurrent use.
type Live struct {
	service *Service
	log     events.Log
	store   LiveStore
	window  time.Duration
}

// NewLive composes live updates on the read service. window is how old a position may be when a
// stream resumes from it; it fails with authz.ErrInvalidInput on a missing dependency or a
// window that is not positive.
func NewLive(service *Service, log events.Log, store LiveStore, window time.Duration) (*Live, error) {
	if service == nil || log == nil || store == nil || window <= 0 {
		return nil, authz.ErrInvalidInput
	}
	return &Live{service: service, log: log, store: store, window: window}, nil
}

// LiveBatch is the outcome of one batch.
type LiveBatch struct {
	// Frames are the frames to write in order. It holds only the resync frame when Resync is set.
	Frames []LiveFrame
	// Resync is true when the stream must send resync-required and close.
	Resync bool
	// Position is the stream's current position, issued now; it is the ID of a heartbeat.
	Position string
	// More is true when the batch read the maximum number of facts, so another may be waiting.
	More bool
}

// LiveSession is the state of one stream. It is used by a single goroutine.
type LiveSession struct {
	live    *Live
	filters Filters
	// position is the last fact the session has processed; invalid marks a start position that
	// could not be parsed.
	position Position
	invalid  bool
	// revision is the access revision of the first batch; zero until then.
	revision int64
}

// Open starts a session over the client's loaded range and start position. The filters need a
// From; a malformed position is not an error here but ends the stream with a resync on the first
// batch, after the caller was authenticated. Failures are authz.ErrInvalidInput.
func (l *Live) Open(filters Filters, position string) (*LiveSession, error) {
	if filters.From == nil || position == "" {
		return nil, authz.ErrInvalidInput
	}
	filters, err := filters.Validate()
	if err != nil {
		return nil, err
	}
	session := &LiveSession{live: l, filters: filters}
	if session.position, err = ParsePosition(position); err != nil {
		session.invalid = true
	}
	return session, nil
}

// Batch runs one batch for the caller in a single transaction: it resolves the caller's
// authority, checks that it is unchanged, reads up to LiveBatchFacts facts after the session's
// position, evaluates them and returns the frames. The session advances only when the batch
// succeeds. A failure to resolve the authority is returned as is, so the caller can end the
// stream with the matching public code.
func (s *LiveSession) Batch(ctx context.Context, proof identity.Proof) (LiveBatch, error) {
	service := s.live.service
	if err := service.warmClassifications(ctx); err != nil {
		return LiveBatch{}, err
	}
	var batch LiveBatch
	var next Position
	var revision int64
	err := service.run(ctx, proof, func(tx platform.TransactionContext, current access, now time.Time) error {
		var err error
		batch, next, err = s.evaluate(ctx, tx, current, now)
		revision = current.revision
		return err
	})
	if err != nil {
		return LiveBatch{}, err
	}
	s.revision = revision
	if !batch.Resync {
		s.position = next
	}
	return batch, nil
}

// evaluate is the body of one batch. It returns the batch and the position to continue from.
func (s *LiveSession) evaluate(ctx context.Context, tx platform.TransactionContext, current access,
	now time.Time) (LiveBatch, Position, error) {
	l := s.live
	resync := LiveBatch{Resync: true, Frames: []LiveFrame{ResyncFrame()}}
	if err := current.requireGroups(s.filters.RequiredFieldGroups()); err != nil {
		return LiveBatch{}, Position{}, err
	}
	if (s.revision != 0 && s.revision != current.revision) || s.invalid || s.position.Issued.Before(now.Add(-l.window)) {
		return resync, s.position, nil
	}
	head, err := l.store.CurrentTxID(tx.Context(), tx)
	if err != nil {
		return LiveBatch{}, Position{}, err
	}
	if s.position.TxID > head {
		return resync, s.position, nil
	}
	records, err := l.log.ReadAfter(tx.Context(), tx, events.Position{TxID: uint64(s.position.TxID), Seq: s.position.Seq}, LiveBatchFacts)
	if err != nil {
		return LiveBatch{}, Position{}, err
	}
	next := s.position
	var facts []liveFact
	for _, record := range records {
		next = Position{TxID: int64(record.TxID), Seq: record.Seq}
		fact, outcome := decodeLiveFact(record, s.filters.From, s.filters.Until)
		switch outcome {
		case outcomeResync:
			return resync, s.position, nil
		case outcomeFact:
			facts = append(facts, fact)
		}
	}
	next.Issued = now
	batch := LiveBatch{Position: next.String(), More: len(records) == LiveBatchFacts}
	view, err := s.view(ctx, tx, current, now, facts)
	if err != nil {
		return LiveBatch{}, Position{}, err
	}
	frames, ok := view.buildFrames(facts, now)
	if !ok {
		return resync, s.position, nil
	}
	batch.Frames = frames
	return batch, next, nil
}

// view reads what the facts' evaluation needs. The affected Messages are evaluated with the
// feed query minus its time filter, so a repositioned Message is upserted and the client decides
// whether it falls into its range. Eligibility and the before contexts are only read for the
// Messages that no longer match, which is where a remove might be due.
func (s *LiveSession) view(ctx context.Context, tx platform.TransactionContext, current access, now time.Time,
	facts []liveFact) (liveView, error) {
	service := s.live.service
	view := liveView{matches: map[uuid.UUID]FeedItem{}, eligible: map[uuid.UUID]bool{},
		units: map[uuid.UUID]UnitContext{}, networks: map[uuid.UUID]uuid.UUID{}, rows: current.rows, now: now}
	if len(facts) == 0 {
		return view, nil
	}
	var ids []uuid.UUID
	for _, fact := range facts {
		ids = append(ids, fact.messageID)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	ids = slices.Compact(ids)
	items, err := service.evaluate(ctx, tx, current, s.filters, ids)
	if err != nil {
		return liveView{}, err
	}
	for _, item := range items {
		view.matches[item.MessageID] = item
		view.eligible[item.MessageID] = true
	}
	var unmatched []uuid.UUID
	for _, id := range ids {
		if _, matched := view.matches[id]; !matched {
			unmatched = append(unmatched, id)
		}
	}
	if len(unmatched) == 0 {
		return view, nil
	}
	eligible, err := service.store.Eligible(tx.Context(), tx, current.rows, unmatched)
	if err != nil {
		return liveView{}, err
	}
	for _, id := range eligible {
		view.eligible[id] = true
	}
	var units, networks []uuid.UUID
	for _, fact := range facts {
		if fact.kind == kindChange && !view.eligible[fact.messageID] {
			units = append(units, fact.before.units...)
			networks = append(networks, fact.before.networks...)
		}
	}
	if len(units) > 0 {
		if view.units, err = s.live.store.UnitContexts(tx.Context(), tx, units); err != nil {
			return liveView{}, err
		}
	}
	if len(networks) > 0 {
		if view.networks, err = s.live.store.NetworkAgencies(tx.Context(), tx, networks); err != nil {
			return liveView{}, err
		}
	}
	return view, nil
}
