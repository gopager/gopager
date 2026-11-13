package reprocess_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
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
	// retracted lists the results a step withdraws, by Transmission.
	retracted     map[uuid.UUID][]processing.RetractedResult
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

func (w *fakeWork) SnapshotTransmission(context.Context, platform.TransactionContext, processing.ReprocessRequest, uuid.UUID) (processing.ReprocessSnapshot, error) {
	return processing.ReprocessSnapshot{}, nil
}

func (w *fakeWork) ApplyReprocessStep(_ context.Context, _ platform.TransactionContext, _ processing.ReprocessRequest, id uuid.UUID, _ processing.ReprocessSnapshot) ([]processing.RetractedResult, error) {
	w.applied = append(w.applied, id)
	return w.retracted[id], nil
}

func (w *fakeWork) ReadTransmissionMessage(context.Context, platform.TransactionContext, uuid.UUID) (uuid.UUID, time.Time, error) {
	return uuid.New(), time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), nil
}

func (w *fakeWork) FinishReprocess(_ context.Context, _ platform.TransactionContext, _ processing.ReprocessRequest, state string) error {
	w.state = state
	return nil
}

func (w *fakeWork) FailReprocess(_ context.Context, _ platform.TransactionContext, _ processing.ReprocessRequest, code string) error {
	w.state, w.failureCode = processing.ReprocessFailed, code
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

func newRunner(t *testing.T, work *fakeWork, readmission *fakeReadmission, facts ...*fakeFacts) *reprocess.Runner {
	t.Helper()
	recorder := &fakeFacts{}
	if len(facts) > 0 {
		recorder = facts[0]
	}
	runner, err := reprocess.New(fakeTransactions{}, fakeClock{}, work, readmission, recorder)
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
	if _, err := reprocess.New(nil, fakeClock{}, &fakeWork{}, &fakeReadmission{}, &fakeFacts{}); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("err = %v, want invalid input", err)
	}
}

// fakeFacts records the retraction facts the runner appends.
type fakeFacts struct {
	retractions []events.ProcessingRetractionFact
}

func (*fakeFacts) AppendProcessingConfiguration(context.Context, platform.TransactionContext, events.ProcessingConfigurationFact) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (*fakeFacts) AppendProcessingCompletion(context.Context, platform.TransactionContext, events.ProcessingCompletionFact) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeFacts) AppendProcessingRetraction(_ context.Context, _ platform.TransactionContext, fact events.ProcessingRetractionFact) (uuid.UUID, error) {
	f.retractions = append(f.retractions, fact)
	return uuid.New(), nil
}

func TestRunnerAppendsOneRetractionFactPerStepThatWithdrewResults(t *testing.T) {
	t.Parallel()
	work, facts := newWork(3), &fakeFacts{}
	work.retracted = map[uuid.UUID][]processing.RetractedResult{work.transmissions[1]: {{ID: uuid.New(), Revision: 2}, {ID: uuid.New(), Revision: 2}}}
	if _, err := newRunner(t, work, &fakeReadmission{}, facts).RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(facts.retractions) != 1 || facts.retractions[0].TransmissionID != work.transmissions[1] ||
		len(facts.retractions[0].ResultIDs) != 2 || facts.retractions[0].WorkflowID != work.request.WorkflowID {
		t.Fatalf("retractions = %+v, want one fact for the second Transmission naming both results", facts.retractions)
	}
}

// stepFailure runs a request whose first step fails with cause, after the given number of earlier
// consecutive failures, and returns what the runner did with it.
func stepFailure(t *testing.T, cause error, earlierAttempts int) *fakeWork {
	t.Helper()
	work := newWork(2)
	work.request.Attempts = earlierAttempts
	readmission := &fakeReadmission{failOn: work.transmissions[0], err: cause}
	if _, err := newRunner(t, work, readmission).RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	return work
}

func TestRunnerEndsTheRequestAtOnceOnAPermanentFailure(t *testing.T) {
	t.Parallel()
	for name, cause := range map[string]error{
		"invalid input":      platform.NewFailure(platform.CodeInvalidInput, errors.New("bad")),
		"forbidden":          platform.NewFailure(platform.CodeForbidden, errors.New("no")),
		"not found":          platform.NewFailure(platform.CodeNotFound, errors.New("gone")),
		"login required":     platform.NewFailure(platform.CodeLoginRequired, errors.New("no session")),
		"idempotency":        platform.NewFailure(platform.CodeIdempotencyConflict, errors.New("again")),
		"permanent conflict": platform.NewPermanentFailure(platform.CodeRevisionConflict, errors.New("unique")),
		"permanent storage":  platform.NewPermanentFailure(platform.CodeStorageUnavailable, errors.New("constraint")),
	} {
		work := stepFailure(t, cause, 0)
		if work.state != processing.ReprocessFailed || work.failureCode == "" || !work.rescheduledAt.IsZero() {
			t.Errorf("%s: state=%q code=%q rescheduled=%v, want a terminal failed request that keeps its code", name, work.state, work.failureCode, work.rescheduledAt)
		}
	}
}

func TestRunnerRetriesEveryOtherFailureUntilTheAttemptLimit(t *testing.T) {
	t.Parallel()
	for name, cause := range map[string]error{
		"storage":      platform.NewFailure(platform.CodeStorageUnavailable, errors.New("down")),
		"authority":    platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("down")),
		"conflict":     platform.NewFailure(platform.CodeRevisionConflict, errors.New("correction")),
		"resource":     platform.NewFailure(platform.CodeResourceLimit, errors.New("connections")),
		"incompatible": platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("rolling upgrade")),
		"uncertain":    platform.NewFailure(platform.CodeCommitUncertain, errors.New("commit")),
		"internal":     platform.NewFailure(platform.CodeInternalError, errors.New("driver error wrapped by a store")),
		"unclassified": errors.New("plain"),
	} {
		retried := stepFailure(t, cause, processing.MaxReprocessAttempts-2)
		if retried.state == processing.ReprocessFailed || retried.rescheduledAt.IsZero() || retried.failureCode == "" {
			t.Errorf("%s: state=%q rescheduled=%v, want a backoff reschedule before the limit", name, retried.state, retried.rescheduledAt)
		}
		last := stepFailure(t, cause, processing.MaxReprocessAttempts-1)
		if last.state != processing.ReprocessFailed || last.failureCode == "" || !last.rescheduledAt.IsZero() {
			t.Errorf("%s: state=%q code=%q, want failed on the %dth consecutive failure keeping the last code", name, last.state, last.failureCode, processing.MaxReprocessAttempts)
		}
	}
}

func TestRunnerLogsEveryStepFailureWithCodesOnly(t *testing.T) { //nolint:paralleltest // replaces the process-wide slog default logger
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	defer slog.SetDefault(previous)
	cause := platform.NewFailure(platform.CodeStorageUnavailable, errors.New("secret-cause-text"))

	retried := stepFailure(t, cause, 0)
	rescheduled := buffer.String()
	buffer.Reset()
	failed := stepFailure(t, cause, processing.MaxReprocessAttempts-1)
	ended := buffer.String()

	for name, check := range map[string]struct {
		line, level string
		work        *fakeWork
		attempts    string
	}{
		"rescheduled": {rescheduled, "level=WARN", retried, "attempts=1"},
		"failed":      {ended, "level=ERROR", failed, "attempts=" + strconv.Itoa(processing.MaxReprocessAttempts)},
	} {
		for _, want := range []string{check.level, "request=" + check.work.request.ID.String(),
			"workflow=" + check.work.request.WorkflowID.String(), check.attempts, "code=" + string(platform.CodeStorageUnavailable)} {
			if !strings.Contains(check.line, want) {
				t.Errorf("%s: log %q lacks %q", name, check.line, want)
			}
		}
		if strings.Contains(check.line, "secret-cause-text") {
			t.Errorf("%s: log %q leaks the cause text", name, check.line)
		}
	}
}
