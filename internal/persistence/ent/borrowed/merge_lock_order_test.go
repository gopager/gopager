package borrowed_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// tryAccept is accept without t.Fatal, so it can run on another goroutine.
func (f *processingSourceFixture) tryAccept(ctx context.Context, id string, observed time.Time) error {
	submission := ingest.Submission{SchemaVersion: 1, SubmissionID: id, Protocol: protocols.POCSAG,
		ObservationTime: observed.Format(time.RFC3339), GatewayNetworkIdentifier: "default",
		Payload: json.RawMessage(toneOnlyPayload), TransmissionTargets: []protocols.Target{}}
	return f.store.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		accepted, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		validated, err := submission.Validate(4096, accepted)
		if err != nil {
			return err
		}
		validated.ResolvedNetwork, validated.Resolution = f.network, "declared"
		matched, err := f.matcher.MatchTargets(tx.Context(), tx, f.network, protocols.POCSAG,
			validated.ObservedAt, validated.TransmissionTargets)
		if err != nil {
			return err
		}
		_, err = f.intake.AcceptTransmission(tx.Context(), tx, f.gateway, validated, accepted, matched)
		return err
	})
}

// TestMergeLocksTransmissionsBeforeMessages expects the intake merge to take the documented
// lock order (Transmission, then Message), the order of the worker and the correction batch.
// A worker-like transaction holds the absorbed Message's Transmission while the merge runs
// and then locks that Message; with the merge holding the Message first the two would wait
// on each other until the deadlock detector aborts one of them.
func TestMergeLocksTransmissionsBeforeMessages(t *testing.T) {
	f := newProcessingSourceFixture(t)
	f.exec(t, `UPDATE grouping_control SET window_seconds=30 WHERE id=1`)
	first := f.accept(t, "order-first", f.now, toneOnlyPayload, nil)
	second := f.accept(t, "order-second", f.now.Add(60*time.Second), toneOnlyPayload, nil)
	absorbedMessage, absorbedTransmission := first.MessageID, first.TransmissionID
	if second.MessageID.String() > first.MessageID.String() {
		absorbedMessage, absorbedTransmission = second.MessageID, second.TransmissionID
	}

	merged := make(chan error, 1)
	workerErr := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(tx.Context(), `SELECT 1 FROM transmissions WHERE id=$1 FOR UPDATE`,
			absorbedTransmission); err != nil {
			return err
		}
		go func() { merged <- f.tryAccept(t.Context(), "order-bridge", f.now.Add(30*time.Second)) }()
		// Give the merge time to take its locks; lock_timeout is below the deadlock detector's
		// one second, so a merge holding the Message fails this transaction instead of waiting.
		time.Sleep(300 * time.Millisecond)
		if _, err := executor.ExecContext(tx.Context(), `SET LOCAL lock_timeout='500ms'`); err != nil {
			return err
		}
		var found uuid.UUID
		return executor.QueryRowContext(tx.Context(), `SELECT id FROM messages WHERE id=$1 FOR UPDATE`,
			absorbedMessage).Scan(&found)
	})
	if workerErr != nil {
		t.Fatalf("worker-like transaction could not lock the Message after its Transmission: %v", workerErr)
	}
	if err := <-merged; err != nil {
		t.Fatalf("merge failed: %v", err)
	}
}
