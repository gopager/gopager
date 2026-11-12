package reprocess_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/reprocess"
)

type fakeTx struct{}

func (fakeTx) Context() context.Context { return context.Background() }

type fakeTransactions struct{}

func (fakeTransactions) WithinTransaction(_ context.Context, fn platform.TxFunc) error {
	return fn(fakeTx{})
}

type fakeClock struct{}

func (fakeClock) Now(context.Context, platform.TransactionContext) (time.Time, error) {
	return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC), nil
}

// fakeWork is an in-memory request with a fixed list of covered Transmissions.
type fakeWork struct {
	request       processing.ReprocessRequest
	pins          processing.WorkflowPins
	transmissions []uuid.UUID
	applied       []uuid.UUID
	state         string
	rescheduledAt time.Time
	failureCode   string
}

func (w *fakeWork) ClaimReprocess(context.Context, platform.TransactionContext, uuid.UUID) (processing.ReprocessRequest, bool, error) {
	return w.request, true, nil
}

func (w *fakeWork) ReadWorkflowPins(context.Context, platform.TransactionContext, uuid.UUID) (processing.WorkflowPins, error) {
	return w.pins, nil
}

func (w *fakeWork) NextReprocessTransmission(_ context.Context, _ platform.TransactionContext, request processing.ReprocessRequest) (uuid.UUID, bool, error) {
	for _, id := range w.transmissions {
		if request.Cursor == nil || id.String() > request.Cursor.String() {
			return id, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (w *fakeWork) IsFreshIntake(context.Context, platform.TransactionContext, processing.ReprocessRequest, uuid.UUID) (bool, error) {
	return false, nil
}

func (w *fakeWork) ApplyReprocessStep(_ context.Context, _ platform.TransactionContext, _ processing.ReprocessRequest, id uuid.UUID, _ bool) error {
	w.applied = append(w.applied, id)
	return nil
}

func (w *fakeWork) FinishReprocess(_ context.Context, _ platform.TransactionContext, _ processing.ReprocessRequest, state string) error {
	w.state = state
	return nil
}

func (w *fakeWork) RescheduleReprocess(_ context.Context, _ platform.TransactionContext, _ processing.ReprocessRequest, due time.Time, code string) error {
	w.rescheduledAt, w.failureCode = due, code
	return nil
}

// fakeReadmission fails for the Transmission listed in failOn.
type fakeReadmission struct {
	failOn uuid.UUID
	err    error
	seen   []uuid.UUID
}

func (r *fakeReadmission) ReadmitProcessing(_ context.Context, _ platform.TransactionContext, id uuid.UUID) error {
	if id == r.failOn {
		return r.err
	}
	r.seen = append(r.seen, id)
	return nil
}

func newWork(count int) *fakeWork {
	version := uuid.New()
	work := &fakeWork{request: processing.ReprocessRequest{ID: uuid.New(), WorkflowID: uuid.New(),
		DefinitionVersionID: version, WorkflowRevision: 4},
		pins: processing.WorkflowPins{Revision: 4, ActiveVersionID: &version, Usable: true}}
	for range count {
		work.transmissions = append(work.transmissions, uuid.New())
	}
	for i := range work.transmissions {
		for j := i + 1; j < len(work.transmissions); j++ {
			if work.transmissions[j].String() < work.transmissions[i].String() {
				work.transmissions[i], work.transmissions[j] = work.transmissions[j], work.transmissions[i]
			}
		}
	}
	return work
}

func newRunner(t *testing.T, work *fakeWork, readmission *fakeReadmission) *reprocess.Runner {
	t.Helper()
	runner, err := reprocess.New(fakeTransactions{}, fakeClock{}, work, readmission)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestRunnerScansEveryTransmissionInOrderAndCompletes(t *testing.T) {
	t.Parallel()
	work, readmission := newWork(3), &fakeReadmission{}
	if worked, err := newRunner(t, work, readmission).RunOnce(t.Context()); err != nil || !worked {
		t.Fatalf("RunOnce = %v, %v", worked, err)
	}
	if len(work.applied) != 3 || work.state != processing.ReprocessCompleted {
		t.Fatalf("applied=%d state=%q, want 3 and completed", len(work.applied), work.state)
	}
	for i, id := range work.applied {
		if id != work.transmissions[i] || readmission.seen[i] != id {
			t.Fatalf("step %d handled out of order", i)
		}
	}
}

func TestRunnerSupersedesAfterAWorkflowChange(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*processing.WorkflowPins){
		"revision":          func(p *processing.WorkflowPins) { p.Revision++ },
		"version":           func(p *processing.WorkflowPins) { other := uuid.New(); p.ActiveVersionID = &other },
		"disabled":          func(p *processing.WorkflowPins) { p.Usable = false },
		"no active version": func(p *processing.WorkflowPins) { p.ActiveVersionID = nil },
	} {
		work := newWork(2)
		change(&work.pins)
		if _, err := newRunner(t, work, &fakeReadmission{}).RunOnce(t.Context()); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if work.state != processing.ReprocessSuperseded || len(work.applied) != 0 {
			t.Errorf("%s: state=%q applied=%d, want superseded and none", name, work.state, len(work.applied))
		}
	}
}

func TestRunnerReschedulesAfterAStepFailureAndKeepsEarlierSteps(t *testing.T) {
	t.Parallel()
	work := newWork(3)
	readmission := &fakeReadmission{failOn: work.transmissions[1], err: platform.NewFailure(platform.CodeRevisionConflict, errors.New("open correction"))}
	if _, err := newRunner(t, work, readmission).RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(work.applied) != 1 || work.state != "" {
		t.Fatalf("applied=%d state=%q, want the first step only and no end state", len(work.applied), work.state)
	}
	if work.failureCode != string(platform.CodeRevisionConflict) || !work.rescheduledAt.After(time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("rescheduled at %v with %q, want a later due time and the conflict code", work.rescheduledAt, work.failureCode)
	}
}

func TestRunnerRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if _, err := reprocess.New(nil, fakeClock{}, &fakeWork{}, &fakeReadmission{}); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("err = %v, want invalid input", err)
	}
}
