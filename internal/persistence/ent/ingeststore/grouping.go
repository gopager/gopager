package ingeststore

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/messages"
	"github.com/gopager/gopager/internal/platform/postgres"
)

// lockCandidateTransmissions row-locks every Transmission of the Messages that could be merge
// candidates (same payload identity, bounds within the window), in id order. The merge later
// reassigns all Transmissions of the absorbed Messages, and the worker and the correction batch
// lock a Transmission before its Message, so taking the Transmission locks first keeps the one
// documented order (Transmission, then Message) and avoids a lock-order deadlock. The caller holds
// the payload-identity lock, which keeps the Messages' bounds and membership stable for this identity.
func lockCandidateTransmissions(ctx context.Context, executor postgres.Executor, window int64,
	submission ingest.ValidatedSubmission) error {
	rows, err := executor.QueryContext(ctx, `SELECT t.id FROM transmissions t
		WHERE t.message_id IN (SELECT m.id FROM messages m
			WHERE m.identity_form=$1 AND m.identity_subtype=$2 AND m.identity_value=$3
			AND m.last_observed_at >= $4::timestamptz - make_interval(secs => $5::double precision)
			AND m.first_observed_at <= $4::timestamptz + make_interval(secs => $5::double precision))
		ORDER BY t.id FOR UPDATE OF t`,
		submission.Identity.Form, submission.Identity.Subtype, submission.Identity.Value,
		submission.ObservedAt, window)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
	}
	return rows.Err()
}

// groupCandidates loads, and row-locks, the Messages with the submission's payload identity that
// have a member inside the inclusive grouping window, together with those members' observation
// times. The Message bounds pre-filter through messages_identity; the member lookup is a range
// on transmissions.observed_at so no per-row arithmetic defeats the index.
func groupCandidates(ctx context.Context, executor postgres.Executor, window int64,
	submission ingest.ValidatedSubmission) ([]messages.ExistingMessage, error) {
	if err := lockCandidateTransmissions(ctx, executor, window, submission); err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT m.id, t.id, t.observed_at
		FROM messages m JOIN transmissions t ON t.message_id=m.id
		WHERE m.identity_form=$1 AND m.identity_subtype=$2 AND m.identity_value=$3
		AND m.last_observed_at >= $4::timestamptz - make_interval(secs => $5::double precision)
		AND m.first_observed_at <= $4::timestamptz + make_interval(secs => $5::double precision)
		AND t.observed_at BETWEEN $4::timestamptz - make_interval(secs => $5::double precision)
			AND $4::timestamptz + make_interval(secs => $5::double precision)
		ORDER BY m.id, t.id FOR UPDATE OF m`,
		submission.Identity.Form, submission.Identity.Subtype, submission.Identity.Value,
		submission.ObservedAt, window)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []messages.ExistingMessage
	for rows.Next() {
		var message, transmission uuid.UUID
		var observed time.Time
		if err := rows.Scan(&message, &transmission, &observed); err != nil {
			return nil, err
		}
		if len(candidates) == 0 || candidates[len(candidates)-1].ID != message {
			candidates = append(candidates, messages.ExistingMessage{ID: message})
		}
		last := &candidates[len(candidates)-1]
		last.Members = append(last.Members, messages.Member{TransmissionID: transmission,
			ObservedAt: observed.UTC().Truncate(time.Second), Identity: submission.Identity})
	}
	return candidates, rows.Err()
}

// groupMessage decides the submission's Message with the pure messages.Decide model and applies
// the decision: it creates the Message, or joins and merges candidates into the survivor, widening
// the survivor's observation bounds. The caller holds the payload-identity lock.
func (s *Store) groupMessage(ctx context.Context, executor postgres.Executor, revision, window int64,
	submission ingest.ValidatedSubmission) (messages.Decision, error) {
	var candidates []messages.ExistingMessage
	if window > 0 {
		var err error
		if candidates, err = groupCandidates(ctx, executor, window, submission); err != nil {
			return messages.Decision{}, err
		}
	}
	decision, err := messages.Decide(messages.Policy{Revision: revision, WindowSeconds: window},
		messages.Member{TransmissionID: uuid.New(), ObservedAt: submission.ObservedAt.UTC(),
			Identity: submission.Identity}, candidates)
	if err != nil {
		return messages.Decision{}, err
	}
	if !decision.Joined {
		if _, err := executor.ExecContext(ctx, `INSERT INTO messages
			(id,identity_form,identity_subtype,identity_value,first_observed_at,last_observed_at)
			VALUES($1,$2,$3,$4,$5,$5)`, decision.MessageID, submission.Identity.Form, submission.Identity.Subtype,
			submission.Identity.Value, submission.ObservedAt); err != nil {
			return messages.Decision{}, err
		}
		decision.MessageRevision = 1
		return decision, nil
	}
	var first, last time.Time
	for i, id := range append([]uuid.UUID{decision.MessageID}, decision.AbsorbedMessage...) {
		var candidateFirst, candidateLast time.Time
		if err := executor.QueryRowContext(ctx, `SELECT first_observed_at,last_observed_at FROM messages WHERE id=$1`,
			id).Scan(&candidateFirst, &candidateLast); err != nil {
			return messages.Decision{}, err
		}
		if i == 0 || candidateFirst.Before(first) {
			first = candidateFirst
		}
		if i == 0 || candidateLast.After(last) {
			last = candidateLast
		}
	}
	decision.LateJoin = submission.ObservedAt.Before(last)
	if submission.ObservedAt.Before(first) {
		first = submission.ObservedAt
	}
	if submission.ObservedAt.After(last) {
		last = submission.ObservedAt
	}
	if err := executor.QueryRowContext(ctx, `UPDATE messages SET revision=revision+1,
		first_observed_at=$2,last_observed_at=$3 WHERE id=$1
		RETURNING revision`, decision.MessageID, first, last).Scan(&decision.MessageRevision); err != nil {
		return messages.Decision{}, err
	}
	return decision, nil
}
