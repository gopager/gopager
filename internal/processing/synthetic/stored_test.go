package synthetic

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/protocols"
)

type storedReader struct {
	record StoredTransmission
	err    error
}

func (s storedReader) ReadStoredTransmission(context.Context, platform.TransactionContext, uuid.UUID) (StoredTransmission, error) {
	return s.record, s.err
}

type storedAccess struct {
	err    error
	called int
}

func (s *storedAccess) CheckTransmissionReadable(context.Context, platform.TransactionContext, authz.Authority,
	time.Time, uuid.UUID, uuid.UUID) error {
	s.called++
	return s.err
}

func storedOwner(t *testing.T, network Network, record StoredTransmission, access *storedAccess) *Owner {
	t.Helper()
	owner, err := NewOwner(testMatcher{}, testNetworks{network: network},
		testClock{now: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}, &testScope{},
		WithStoredTransmissions(storedReader{record: record}, access))
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func storedRecord(network Network) StoredTransmission {
	function := uint8(1)
	return StoredTransmission{MessageID: uuid.New(), Protocol: protocols.POCSAG, NetworkID: network.ID,
		Payload:    json.RawMessage(`{"form":"text","value":"stored alarm"}`),
		ObservedAt: time.Date(2026, 11, 2, 9, 30, 15, 500, time.UTC),
		Targets:    []protocols.Target{{Address: protocols.NumericAddress(123), Function: &function}}}
}

func TestStoredPreviewProjectsTheStoredTransmission(t *testing.T) {
	t.Parallel()
	network := Network{ID: uuid.New(), Name: "Dispatch", Protocol: protocols.POCSAG, AgencyID: testAgency}
	access := &storedAccess{}
	owner := storedOwner(t, network, storedRecord(network), access)
	capture, err := owner.CaptureStoredPreview(context.Background(), testTx{ctx: context.Background()},
		testAuthority("payload", "gateway", "address", "unit"), testWorkflow(nil), processing.Definition{}, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if access.called != 1 {
		t.Fatalf("readability checks = %d, want 1", access.called)
	}
	text, ok := findField(capture.Projection.Fields, "payload", "text", "value")
	if !ok || text.Field.State != processing.FieldPresent || string(text.Field.Value.Value) != `"stored alarm"` {
		t.Fatalf("stored text not projected: %+v", text)
	}
	if capture.Projection.NetworkID != network.ID || capture.Projection.Protocol != protocols.POCSAG ||
		!capture.Projection.ObservedAt.Equal(time.Date(2026, 11, 2, 9, 30, 15, 0, time.UTC)) {
		t.Fatalf("stored identity not projected: %+v", capture.Projection)
	}
	if len(capture.Projection.Targets) != 1 {
		t.Fatalf("targets = %d, want the stored target", len(capture.Projection.Targets))
	}
	if capture.Projection.TransmissionID == uuid.Nil {
		t.Fatal("the preview source needs its own identity")
	}
	owner.ReleaseSyntheticPreview(capture.Projection.TransmissionID)
}

func TestStoredPreviewDeniesAnUnreadableTransmissionAsNotFound(t *testing.T) {
	t.Parallel()
	network := Network{ID: uuid.New(), Name: "Dispatch", Protocol: protocols.POCSAG, AgencyID: testAgency}
	owner := storedOwner(t, network, storedRecord(network), &storedAccess{err: authz.ErrNotFound})
	_, err := owner.CaptureStoredPreview(context.Background(), testTx{ctx: context.Background()},
		testAuthority("payload", "gateway", "address", "unit"), testWorkflow(nil), processing.Definition{}, uuid.New())
	if !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
	missing, err := NewOwner(testMatcher{}, testNetworks{network: network}, testClock{now: time.Now()}, &testScope{},
		WithStoredTransmissions(storedReader{err: authz.ErrNotFound}, &storedAccess{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = missing.CaptureStoredPreview(context.Background(), testTx{ctx: context.Background()},
		testAuthority("payload", "address", "unit"), testWorkflow(nil), processing.Definition{}, uuid.New()); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("unknown Transmission err = %v, want not found", err)
	}
}

func TestStoredPreviewNeedsEveryProjectedFieldGroup(t *testing.T) {
	t.Parallel()
	network := Network{ID: uuid.New(), Name: "Dispatch", Protocol: protocols.POCSAG, AgencyID: testAgency}
	for _, missing := range []string{"payload", "address", "unit"} {
		groups := []string{"payload", "gateway", "address", "unit"}
		for i, group := range groups {
			if group == missing {
				groups = append(groups[:i], groups[i+1:]...)
				break
			}
		}
		owner := storedOwner(t, network, storedRecord(network), &storedAccess{})
		_, err := owner.CaptureStoredPreview(context.Background(), testTx{ctx: context.Background()},
			testAuthority(groups...), testWorkflow(nil), processing.Definition{}, uuid.New())
		if !errors.Is(err, authz.ErrForbidden) {
			t.Errorf("without %s: err = %v, want forbidden", missing, err)
		}
	}
}

func TestStoredPreviewIsUnavailableWithoutAStoredSource(t *testing.T) {
	t.Parallel()
	owner, _, tx := testOwner(protocols.POCSAG, nil)
	_, err := owner.CaptureStoredPreview(context.Background(), tx, testAuthority("payload"), testWorkflow(nil), processing.Definition{}, uuid.New())
	if platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("err = %v, want unavailable", err)
	}
}
