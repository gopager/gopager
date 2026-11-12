package main

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

// reprocessWindow returns the Message-time range of the scenarios and the times of three
// Messages: two inside it and one before it.
func reprocessWindow() (from, to, first, second, outside time.Time) {
	now := time.Now().UTC().Truncate(time.Second)
	return now.Add(-4 * time.Hour), now.Add(-time.Hour), now.Add(-3 * time.Hour), now.Add(-2 * time.Hour), now.Add(-10 * time.Hour)
}

// TestReprocessReplacesResultsAtTheNewVersion expects v1 results to stay visible after v2 is
// activated, an explicit Reprocess over a time range to re-run only the Transmissions inside the
// range and the cutoff, and to replace their results atomically with a reprocess fact, leaving
// Payload, Message and time untouched.
func TestReprocessReplacesResultsAtTheNewVersion(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, secondTime, outsideTime := reprocessWindow()
	f.activate(f.saveVersion("3"))
	first, second, outside := f.submit("first", firstTime), f.submit("second", secondTime), f.submit("outside", outsideTime)
	f.runWorker()
	for _, id := range []uuid.UUID{first, second, outside} {
		if got := f.levels(id); !slices.Equal(got, []string{"3"}) {
			t.Fatalf("v1 levels = %v, want [3]", got)
		}
	}
	type snapshot struct{ payload, message, observed string }
	read := func(id uuid.UUID) snapshot {
		values := f.queryStrings(`SELECT payload::text || '|' || message_id::text || '|' || observed_at::text
			FROM transmissions WHERE id=$1`, id)
		parts := strings.Split(values[0], "|")
		return snapshot{parts[0], parts[1], parts[2]}
	}
	before := read(first)

	v2 := f.saveVersion("4")
	f.activate(v2)
	f.runWorker()
	for _, id := range []uuid.UUID{first, second, outside} {
		if got := f.levels(id); !slices.Equal(got, []string{"3"}) {
			t.Fatalf("levels after activating v2 without Reprocess = %v, want v1 results kept", got)
		}
	}

	request := f.requestReprocess(v2, from, to)
	// A Transmission accepted after the request lies behind the cutoff: it is admitted at v2 by
	// intake itself and the runner must not touch it.
	late := f.submit("late", firstTime.Add(time.Minute))
	f.runReprocess()
	if view := f.request(request); view.State != processing.ReprocessCompleted || view.Scanned != 2 || view.Readmitted != 2 {
		t.Fatalf("request = %+v, want completed with two Transmissions scanned and readmitted", view)
	}
	if f.marked(first) != 1 || f.marked(second) != 1 || f.marked(outside) != 0 || f.marked(late) != 0 {
		t.Fatal("only the Transmissions inside the range and before the cutoff may carry the mark")
	}
	// Until the new run publishes, the old result stays visible.
	if got := f.levels(first); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("levels before the replacement ran = %v, want v1 kept", got)
	}
	f.runWorker()

	for _, id := range []uuid.UUID{first, second} {
		if got := f.levels(id); !slices.Equal(got, []string{"4"}) {
			t.Fatalf("levels after Reprocess = %v, want [4]", got)
		}
		if published, retracted := f.results(id); published != 1 || retracted != 1 {
			t.Fatalf("results published=%d retracted=%d, want 1 and 1", published, retracted)
		}
		if modes := f.replayModes(id); !slices.Equal(modes, []string{"normal", "reprocess"}) {
			t.Fatalf("replay modes = %v, want the original normal fact then a reprocess fact", modes)
		}
		if f.marked(id) != 0 {
			t.Fatal("the mark must clear once the replacement published")
		}
	}
	if got := f.levels(outside); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("outside levels = %v, want v1 kept", got)
	}
	if modes := f.replayModes(outside); !slices.Equal(modes, []string{"normal"}) {
		t.Fatalf("outside replay modes = %v, want the original fact only", modes)
	}
	if got := f.levels(late); !slices.Equal(got, []string{"4"}) {
		t.Fatalf("late levels = %v, want its own v2 result", got)
	}
	if modes := f.replayModes(late); !slices.Equal(modes, []string{"normal"}) {
		t.Fatalf("late replay modes = %v, want a normal fact", modes)
	}
	if after := read(first); after != before {
		t.Fatalf("Payload, Message or time changed: %+v -> %+v", before, after)
	}
}

// TestReprocessAtTheSameVersionRerunsTheWorkflow expects a revision bump alone to re-run the
// active version and replace its earlier result.
func TestReprocessAtTheSameVersionRerunsTheWorkflow(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	f.activate(v1)
	transmission := f.submit("same", firstTime)
	f.runWorker()
	oldResult := f.queryStrings(`SELECT r.id::text FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
		WHERE c.transmission_id=$1`, transmission)

	f.requestReprocess(v1, from, to)
	f.runReprocess()
	f.runWorker()

	published, retracted := f.results(transmission)
	if published != 1 || retracted != 1 {
		t.Fatalf("results published=%d retracted=%d, want a new published result and the old one retracted", published, retracted)
	}
	if now := f.queryStrings(`SELECT r.id::text FROM processing_results r JOIN processing_candidates c ON c.id=r.candidate_id
		WHERE c.transmission_id=$1 AND r.published_at IS NOT NULL`, transmission); now[0] == oldResult[0] {
		t.Fatal("the published result is still the old one")
	}
	if modes := f.replayModes(transmission); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("replay modes = %v", modes)
	}
}

// TestReprocessRetractsATransmissionTheWorkflowNoLongerSelects expects the published result of a
// Transmission to be withdrawn when its Agency left the Workflow, without a new run.
func TestReprocessRetractsATransmissionTheWorkflowNoLongerSelects(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	f.activate(v1)
	transmission := f.submit("gone", firstTime)
	f.runWorker()
	other := uuid.New()
	f.exec(`INSERT INTO agencies(id,name) VALUES($1,'Other agency')`, other)
	f.updateWorkflow("Reprocess", other)
	if got := f.levels(transmission); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("levels after the scope change = %v, want kept until Reprocess", got)
	}

	request := f.requestReprocess(v1, from, to)
	f.runReprocess()

	if view := f.request(request); view.State != processing.ReprocessCompleted || view.Retracted != 1 || view.Readmitted != 0 {
		t.Fatalf("request = %+v, want completed with one retraction", view)
	}
	if published, retracted := f.results(transmission); published != 0 || retracted != 1 {
		t.Fatalf("results published=%d retracted=%d, want none published and one retracted", published, retracted)
	}
	if got := f.levels(transmission); len(got) != 0 {
		t.Fatalf("levels = %v, want no attribute values of a retracted result", got)
	}
}

// TestFailedReprocessRunKeepsTheOldResultUntilARetrySucceeds expects a failed marked candidate to
// replace nothing and a later successful retry to apply the replacement as a reprocess fact.
func TestFailedReprocessRunKeepsTheOldResultUntilARetrySucceeds(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	f.activate(f.saveVersion("3"))
	transmission := f.submit("flaky", firstTime)
	f.runWorker()
	v2 := f.saveVersion("4")
	f.activate(v2)
	f.requestReprocess(v2, from, to)
	f.runReprocess()
	candidate := uuid.MustParse(f.queryStrings(`SELECT id::text FROM processing_candidates
		WHERE transmission_id=$1 AND reprocess_id IS NOT NULL`, transmission)[0])
	failProcessingCandidateTerminally(t, f.store, candidate)
	f.runWorker()

	if got := f.levels(transmission); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("levels after the failed run = %v, want the old result kept", got)
	}
	if f.marked(transmission) != 1 {
		t.Fatal("a failed run must keep the mark for its retry")
	}
	path := "/api/v1/workflows/" + f.workflow.String() + "/candidates/" + candidate.String()
	revision := f.queryStrings(`SELECT revision::text FROM processing_candidates WHERE id=$1`, candidate)[0]
	f.post(path+"/retry", `{"schemaVersion":1,"expectedRevision":"`+revision+`"}`, http.StatusOK)
	f.runWorker()

	if got := f.levels(transmission); !slices.Equal(got, []string{"4"}) {
		t.Fatalf("levels after the retry = %v, want the replacement", got)
	}
	if modes := f.replayModes(transmission); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("replay modes = %v, want the retried replacement to stay a reprocess fact", modes)
	}
}

// TestReprocessIsSupersededByALaterWorkflowChange expects a Workflow change after the request to
// end the request without readmitting anything.
func TestReprocessIsSupersededByALaterWorkflowChange(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	f.activate(v1)
	transmission := f.submit("fenced", firstTime)
	f.runWorker()
	request := f.requestReprocess(v1, from, to)
	f.updateWorkflow("Renamed", f.agency)

	f.runReprocess()

	if view := f.request(request); view.State != processing.ReprocessSuperseded || view.Scanned != 0 {
		t.Fatalf("request = %+v, want superseded before any step", view)
	}
	if f.marked(transmission) != 0 {
		t.Fatal("a superseded request must not mark candidates")
	}
	if got := f.levels(transmission); !slices.Equal(got, []string{"3"}) {
		t.Fatalf("levels = %v, want the old result kept", got)
	}
}

// failingReadmission runs the real readmission and then fails once for one Transmission, so the
// step has written state that must roll back.
type failingReadmission struct {
	inner interface {
		ReadmitProcessing(context.Context, platform.TransactionContext, uuid.UUID) error
	}
	failOn uuid.UUID
	failed bool
	seen   []uuid.UUID
}

func (r *failingReadmission) ReadmitProcessing(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) error {
	r.seen = append(r.seen, id)
	if err := r.inner.ReadmitProcessing(ctx, tx, id); err != nil {
		return err
	}
	if id == r.failOn && !r.failed {
		r.failed = true
		return errors.New("injected failure after readmission")
	}
	return nil
}

// TestReprocessRunnerRollsBackAFailedStepAndResumesAtTheCursor expects a failed step to leave no
// partial state, the request to keep its cursor and a later runner to continue after it.
func TestReprocessRunnerRollsBackAFailedStepAndResumesAtTheCursor(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, _, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	f.activate(v1)
	ids := []uuid.UUID{f.submit("a", firstTime), f.submit("b", firstTime.Add(time.Minute)), f.submit("c", firstTime.Add(2*time.Minute))}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	f.runWorker()
	request := f.requestReprocess(v1, from, to)

	failing := &failingReadmission{inner: f.runtime.readmission(), failOn: ids[1]}
	if _, err := f.reprocessRunner(failing).RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if view := f.request(request); view.State != processing.ReprocessPending || view.Scanned != 1 {
		t.Fatalf("request after the failed step = %+v, want pending after one committed step", view)
	}
	cursor := f.queryStrings(`SELECT cursor::text FROM processing_reprocess_requests WHERE id=$1`, request)[0]
	if cursor != ids[0].String() {
		t.Fatalf("cursor = %s, want the first Transmission %s", cursor, ids[0])
	}
	if f.marked(ids[0]) != 1 || f.marked(ids[1]) != 0 {
		t.Fatal("the committed step must keep its mark and the rolled-back step must leave none")
	}
	if got := f.queryStrings(`SELECT count(*)::text FROM processing_candidates WHERE transmission_id=$1 AND state='superseded'`, ids[1]); got[0] != "0" {
		t.Fatalf("the rolled-back step superseded %s candidates", got[0])
	}

	// A new runner (a restart) picks the request up once it is due again and continues after the cursor.
	f.exec(`UPDATE processing_reprocess_requests SET due_at = now() WHERE id=$1`, request)
	resumed := &failingReadmission{inner: f.runtime.readmission(), failOn: uuid.Nil}
	if _, err := f.reprocessRunner(resumed).RunOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resumed.seen, ids[1:]) {
		t.Fatalf("resumed runner handled %v, want %v", resumed.seen, ids[1:])
	}
	if view := f.request(request); view.State != processing.ReprocessCompleted || view.Scanned != 3 || view.Readmitted != 3 {
		t.Fatalf("request = %+v, want completed with three Transmissions", view)
	}

	// A runner that died holding the lease is replaced once the lease expired.
	again := f.requestReprocess(v1, from, to)
	f.exec(`UPDATE processing_reprocess_requests SET state='running', worker_id=$2, lease_until=now() - interval '1 second'
		WHERE id=$1`, again, uuid.New())
	f.runReprocess()
	if view := f.request(again); view.State != processing.ReprocessCompleted || view.Scanned != 3 {
		t.Fatalf("taken-over request = %+v, want completed", view)
	}
}

// TestReprocessRequestAuthorizationAndValidation expects 403 without workflow.reprocess, a
// revision conflict for a version that is not active or a stale revision and invalid input for a
// bad range.
func TestReprocessRequestAuthorizationAndValidation(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, _, _, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	v2 := f.saveVersion("4")
	f.activate(v1)
	path := "/api/v1/workflows/" + f.workflow.String() + "/reprocess"

	if conflict := f.post(path, f.reprocessBody(v2, f.revision, from, to), http.StatusConflict); conflict.Code != string(platform.CodeRevisionConflict) {
		t.Fatalf("inactive version code = %q", conflict.Code)
	}
	if conflict := f.post(path, f.reprocessBody(v1, f.revision-1, from, to), http.StatusConflict); conflict.Code != string(platform.CodeRevisionConflict) {
		t.Fatalf("stale revision code = %q", conflict.Code)
	}
	for name, body := range map[string]string{
		"reversed range": f.reprocessBody(v1, f.revision, to, from),
		"empty range":    f.reprocessBody(v1, f.revision, from, from),
		"bad time":       strings.Replace(f.reprocessBody(v1, f.revision, from, to), from.UTC().Format(time.RFC3339), "yesterday", 1),
		"wrong schema":   strings.Replace(f.reprocessBody(v1, f.revision, from, to), `"schemaVersion":1`, `"schemaVersion":2`, 1),
	} {
		if invalid := f.post(path, body, http.StatusBadRequest); invalid.Code != string(platform.CodeInvalidInput) {
			t.Fatalf("%s code = %q", name, invalid.Code)
		}
	}
	if got := f.count(`SELECT count(*) FROM processing_reprocess_requests`); got != 0 {
		t.Fatalf("%d requests stored by rejected calls", got)
	}

	id := f.requestReprocess(v1, from, to)
	list := f.get("/api/v1/workflows/"+f.workflow.String()+"/reprocesses?limit=10", http.StatusOK)
	if !strings.Contains(string(list.Data), id.String()) {
		t.Fatalf("list = %s, want the request", list.Data)
	}

	f.exec(`UPDATE identity_grants SET actions = (SELECT COALESCE(jsonb_agg(a), '[]'::jsonb)
		FROM jsonb_array_elements_text(actions) a WHERE a <> 'workflow.reprocess')`)
	f.exec(`UPDATE identity_control SET access_revision = access_revision + 1`)
	if denied := f.post(path, f.reprocessBody(v1, f.revision, from, to), http.StatusForbidden); denied.Code != string(platform.CodeForbidden) {
		t.Fatalf("missing action code = %q", denied.Code)
	}
}

// TestReprocessSkipsFreshIntakeThatRacedTheRequest expects a Transmission that intake admitted at
// the bumped Workflow revision after the request, but whose accepted_at fell before the cutoff,
// to count as scanned without being marked, so its fact stays a normal one.
func TestReprocessSkipsFreshIntakeThatRacedTheRequest(t *testing.T) {
	t.Parallel()
	f := newReprocessFixture(t)
	from, to, firstTime, secondTime, _ := reprocessWindow()
	v1 := f.saveVersion("3")
	f.activate(v1)
	old := f.submit("old", firstTime)
	f.runWorker()
	request := f.requestReprocess(v1, from, to)
	fresh := f.submit("fresh", secondTime)
	// Intake took its acceptance time before the request's exclusive lock and committed after it.
	f.exec(`UPDATE transmissions SET accepted_at = (SELECT cutoff_at FROM processing_reprocess_requests WHERE id=$2) - interval '1 second'
		WHERE id=$1`, fresh, request)

	f.runReprocess()
	f.runWorker()

	if view := f.request(request); view.State != processing.ReprocessCompleted || view.Scanned != 2 || view.Readmitted != 1 {
		t.Fatalf("request = %+v, want two scanned and only the historical one readmitted", view)
	}
	if f.marked(fresh) != 0 || f.marked(old) != 0 {
		t.Fatal("the fresh Transmission must not carry a mark, and the old one's mark must be cleared")
	}
	if modes := f.replayModes(fresh); !slices.Equal(modes, []string{"normal"}) {
		t.Fatalf("fresh replay modes = %v, want a normal fact", modes)
	}
	if modes := f.replayModes(old); !slices.Equal(modes, []string{"normal", "reprocess"}) {
		t.Fatalf("old replay modes = %v", modes)
	}
}
