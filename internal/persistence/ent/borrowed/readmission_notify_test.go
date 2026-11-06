package borrowed_test

import (
	"context"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// TestReadmissionNotifiesDispatchers expects ReadmitProcessing to announce newly admitted work
// on the due channel in its transaction, like intake does, so a dispatcher wakes without
// waiting for its poll interval.
func TestReadmissionNotifiesDispatchers(t *testing.T) {
	t.Parallel()
	f := newProcessingSourceFixture(t)
	receipt := f.accept(t, "readmit-notify", f.now, toneOnlyPayload, nil)
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "none" {
		t.Fatalf("status without a Workflow=%s want none", status)
	}
	f.seedWorkflow(t, "readmit-notify", protocolSelectorDefinition(t))

	listenContext, stopListening := context.WithCancel(t.Context())
	defer stopListening()
	signals, err := f.store.Listen(listenContext, processing.DueNotificationChannel)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-signals: // initial signal after the listener connected
	case <-time.After(10 * time.Second):
		t.Fatal("listener did not connect")
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		return f.owner.ReadmitProcessing(tx.Context(), tx, receipt.TransmissionID)
	}); err != nil {
		t.Fatal(err)
	}
	if status := f.transmissionStatus(t, receipt.TransmissionID); status != "pending" {
		t.Fatalf("status after readmission=%s want pending", status)
	}
	select {
	case <-signals:
	case <-time.After(5 * time.Second):
		t.Fatal("readmission did not notify dispatchers about new due work")
	}
}
