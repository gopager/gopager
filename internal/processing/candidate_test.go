package processing

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// candidateFixture supplies private synthetic work without asserting source-owner readiness.
func candidateFixture() Candidate {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	return Candidate{CandidateBinding: CandidateBinding{
		TransmissionID: uuid.New(), WorkflowID: uuid.New(), DefinitionVersionID: uuid.New(),
		SourceRevision: 1, ContextRevision: 1, WorkflowRevision: 2, ConfigurationRevision: 3,
	}, ID: uuid.New(), Revision: 4, State: "superseded", Attempts: 7, AutomaticAttempts: 5,
		FencingToken: 9, CreatedAt: now, UpdatedAt: now, DueAt: now}
}

func TestCandidateReselectionPreservesIdentityHistoryAndInvalidatesOldClaim(t *testing.T) {
	current := candidateFixture()
	binding := current.CandidateBinding
	binding.WorkflowRevision++
	binding.ConfigurationRevision++
	worker, until, selected, failure := uuid.New(), current.UpdatedAt.Add(time.Minute), true, "INVALID_INPUT"
	current.State, current.WorkerID, current.LeaseUntil = "running", &worker, &until
	current.Selected, current.FailureCode, current.ManualRetryPending = &selected, &failure, true
	next, changed, err := current.Reselect(binding, current.UpdatedAt.Add(time.Second))
	if err != nil || !changed {
		t.Fatalf("reselect: changed=%v err=%v", changed, err)
	}
	if next.ID != current.ID || next.Revision != 5 || next.FencingToken != 10 ||
		next.Attempts != 7 || next.AutomaticAttempts != 5 || next.State != "failed" ||
		next.WorkerID != nil || next.LeaseUntil != nil || next.Selected != nil ||
		next.FailureCode == nil || *next.FailureCode != string(platform.CodeResourceLimit) ||
		next.ManualRetryPending || next.CreatedAt != current.CreatedAt {
		t.Fatalf("lost history or retained obsolete claim: %+v", next)
	}
	if current.State != "running" || current.WorkerID == nil || current.Revision != 4 {
		t.Fatal("reselection mutated the captured old work")
	}
	replay, changed, err := next.Reselect(binding, next.UpdatedAt.Add(time.Second))
	if err != nil || changed || replay.Revision != next.Revision || replay.FencingToken != next.FencingToken ||
		replay.DueAt != next.DueAt {
		t.Fatalf("unchanged pins restarted work: %+v changed=%v err=%v", replay, changed, err)
	}
}

func TestCandidateReselectionRejectsIdentitySubstitutionStalePinsAndPublishedWork(t *testing.T) {
	current := candidateFixture()
	for _, state := range []string{"failed", "superseded"} {
		current.State = state
		next, changed, err := current.Reselect(current.CandidateBinding, current.UpdatedAt)
		if err != nil || changed || next.State != state {
			t.Fatalf("unchanged %s work revived: changed=%v err=%v", state, changed, err)
		}
	}
	tests := map[string]func(*Candidate, *CandidateBinding){
		"source":         func(_ *Candidate, b *CandidateBinding) { b.SourceRevision++ },
		"context":        func(_ *Candidate, b *CandidateBinding) { b.ContextRevision++ },
		"workflow":       func(_ *Candidate, b *CandidateBinding) { b.WorkflowID = uuid.New() },
		"definition":     func(_ *Candidate, b *CandidateBinding) { b.DefinitionVersionID = uuid.New() },
		"transmission":   func(_ *Candidate, b *CandidateBinding) { b.TransmissionID = uuid.New() },
		"stale-config":   func(_ *Candidate, b *CandidateBinding) { b.ConfigurationRevision = 1 },
		"stale-workflow": func(_ *Candidate, b *CandidateBinding) { b.WorkflowRevision = 1 },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			value, binding := current, current.CandidateBinding
			change(&value, &binding)
			if _, changed, err := value.Reselect(binding, current.UpdatedAt); changed || !errors.Is(err, authz.ErrRevisionConflict) {
				t.Fatalf("accepted obsolete/substituted/published work: changed=%v err=%v", changed, err)
			}
		})
	}
}

func TestCandidateReselectionChecksCounterExhaustionAndPrimaryTime(t *testing.T) {
	current := candidateFixture()
	binding := current.CandidateBinding
	binding.WorkflowRevision++
	for _, counter := range []string{"revision", "fence"} {
		value := current
		if counter == "revision" {
			value.Revision = math.MaxInt64
		} else {
			value.FencingToken = math.MaxInt64
		}

		if _, _, err := value.Reselect(binding, current.UpdatedAt); platform.FailureCode(err) != platform.CodeResourceLimit {
			t.Fatalf("%s overflow not rejected: %v", counter, err)
		}
	}
	for _, now := range []time.Time{{}, current.UpdatedAt.Add(-time.Microsecond), current.UpdatedAt.Add(time.Nanosecond)} {
		if _, _, err := current.Reselect(binding, now); err == nil {
			t.Fatalf("accepted invalid primary time: %v", now)
		}
	}
	current.FencingToken = 0
	next, changed, err := current.Reselect(binding, current.UpdatedAt)
	if err != nil || !changed || next.FencingToken != 1 {
		t.Fatalf("first fence invalid: %+v changed=%v err=%v", next, changed, err)
	}
}

func TestCandidateClaimAndRenewUseExactLeaseAndAttemptBoundaries(t *testing.T) {
	current := candidateFixture()
	current.State, current.Attempts, current.AutomaticAttempts = "pending", 0, 0
	worker := uuid.New()
	if _, err := current.Claim(current.Revision, worker, current.DueAt.Add(-time.Microsecond)); err == nil {
		t.Fatal("claimed before due time")
	}
	claimed, err := current.Claim(current.Revision, worker, current.DueAt)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.LeaseUntil.Sub(current.DueAt) != 60*time.Second || CandidateRenewInterval != 20*time.Second ||
		claimed.Attempts != 1 || claimed.AutomaticAttempts != 1 || claimed.FencingToken != 10 ||
		claimed.Revision != current.Revision+1 {
		t.Fatalf("incorrect lease or reserved attempt: %+v", claimed)
	}
	lease, err := claimed.Lease()
	if err != nil {
		t.Fatal(err)
	}
	if err := claimed.CheckLease(lease, claimed.LeaseUntil.Add(-time.Microsecond)); err != nil {
		t.Fatalf("rejected unexpired claim: %v", err)
	}
	for _, now := range []time.Time{*claimed.LeaseUntil, claimed.LeaseUntil.Add(time.Microsecond)} {
		if _, err := claimed.Renew(lease, now); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("renewed expired claim at %v: %v", now, err)
		}
	}
	renewAt := claimed.UpdatedAt.Add(CandidateRenewInterval)
	renewed, err := claimed.Renew(lease, renewAt)
	if err != nil || renewed.LeaseUntil.Sub(renewAt) != CandidateLeaseDuration ||
		renewed.FencingToken != claimed.FencingToken || renewed.Attempts != claimed.Attempts ||
		renewed.Revision != claimed.Revision+1 {
		t.Fatalf("incorrect renewed lease: %+v err=%v", renewed, err)
	}
	if err := renewed.CheckLease(lease, renewAt); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("old revision still authorized: %v", err)
	}
	current.AutomaticAttempts, current.Attempts = 4, 4
	last, err := current.Claim(current.Revision, worker, current.DueAt)
	if err != nil || last.AutomaticAttempts != 5 {
		t.Fatalf("fifth attempt rejected: %+v err=%v", last, err)
	}
	current.AutomaticAttempts, current.Attempts = 5, 5
	if _, err := current.Claim(current.Revision, worker, current.DueAt); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("sixth automatic attempt allowed: %v", err)
	}
	// An already-authorized pending manual retry consumes total history, not a fresh automatic cycle.
	current.ManualRetryPending = true
	manual, err := current.Claim(current.Revision, worker, current.DueAt)
	if err != nil || manual.AutomaticAttempts != 5 || manual.Attempts != 6 || manual.ManualRetryPending {
		t.Fatalf("manual retry reset automatic history: %+v err=%v", manual, err)
	}
}

func TestCandidateManualRetryRequiresFailedExpectedStateWithoutResettingHistory(t *testing.T) {
	current := candidateFixture()
	current.State = "failed"
	failure := "INVALID_INPUT"
	current.FailureCode = &failure
	next, err := current.RetryFailed(current.Revision, current.UpdatedAt)
	if err != nil || next.ID != current.ID || next.Revision != current.Revision+1 ||
		next.State != "pending" || !next.ManualRetryPending || next.Attempts != current.Attempts ||
		next.AutomaticAttempts != 5 || next.FencingToken != current.FencingToken {
		t.Fatalf("manual retry lost history: %+v err=%v", next, err)
	}
	if next.FailureCode != nil || next.FailureStep != nil {
		t.Fatalf("manual retry kept the previous failure: %+v", next)
	}
	if err := next.Validate(); err != nil {
		t.Fatalf("retried candidate is not a valid pending candidate: %v", err)
	}
	if _, err := next.RetryFailed(next.Revision, next.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("pending manual attempt duplicated: %v", err)
	}
	for _, state := range []string{"pending", "running", "completed", "superseded"} {
		value := current
		value.State = state
		if state == "running" {
			worker, until := uuid.New(), current.UpdatedAt.Add(CandidateLeaseDuration)
			value.WorkerID, value.LeaseUntil = &worker, &until
		}
		if state == "completed" {
			selected := true
			value.Selected = &selected
		}
		if _, err := value.RetryFailed(value.Revision, value.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("retried %s work: %v", state, err)
		}
	}
	if _, err := current.RetryFailed(current.Revision-1, current.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale revision retried: %v", err)
	}
}

func TestCandidateFailureSchedulesRemainingAttemptsAndKeepsExhaustionVisible(t *testing.T) {
	pending := candidateFixture()
	pending.State, pending.Attempts, pending.AutomaticAttempts = "pending", 0, 0
	running, err := pending.Claim(pending.Revision, uuid.New(), pending.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := running.Lease()
	if err != nil {
		t.Fatal(err)
	}
	next, err := running.Fail(lease, platform.CodeAuthorityUnavailable, true, running.UpdatedAt)
	if err != nil || next.State != "pending" || next.AutomaticAttempts != 1 ||
		next.Attempts != 1 || next.WorkerID != nil || next.LeaseUntil != nil ||
		next.FailureCode == nil || *next.FailureCode != string(platform.CodeAuthorityUnavailable) ||
		next.DueAt.Sub(next.UpdatedAt) < 24*time.Second || next.DueAt.Sub(next.UpdatedAt) > 36*time.Second {
		t.Fatalf("invalid first retry schedule/history: %+v err=%v", next, err)
	}
	permanent, err := running.Fail(lease, platform.CodeInvalidInput, false, running.UpdatedAt)
	if err != nil || permanent.State != "failed" || permanent.AutomaticAttempts != 1 {
		t.Fatalf("permanent failure was retried: %+v err=%v", permanent, err)
	}
	running.Attempts, running.AutomaticAttempts = 5, 5
	exhausted, err := running.Fail(lease, platform.CodeAuthorityUnavailable, true, running.UpdatedAt)
	if err != nil || exhausted.State != "failed" || exhausted.AutomaticAttempts != 5 || exhausted.Attempts != 5 {
		t.Fatalf("exhausted failure hidden/reset: %+v err=%v", exhausted, err)
	}
	if _, err := running.Fail(lease, platform.ErrorCode("private guest stderr"), true, running.UpdatedAt); err == nil {
		t.Fatal("arbitrary private failure escaped safe vocabulary")
	}
	if _, err := running.Fail(lease, platform.CodeInternalError, false, *running.LeaseUntil); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("expired worker recorded failure: %v", err)
	}
}

func TestCandidateRecoveryFencesExpiredClaimsWithoutResettingAttempts(t *testing.T) {
	pending := candidateFixture()
	pending.State, pending.Attempts, pending.AutomaticAttempts = "pending", 0, 0
	running, err := pending.Claim(pending.Revision, uuid.New(), pending.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := running.Lease()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := running.RecoverExpired(lease, running.LeaseUntil.Add(-time.Microsecond)); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("unexpired claim recovered: %v", err)
	}
	recovered, err := running.RecoverExpired(lease, *running.LeaseUntil)
	if err != nil || recovered.State != "pending" || recovered.Revision != running.Revision+1 ||
		recovered.FencingToken != running.FencingToken+1 || recovered.Attempts != running.Attempts ||
		recovered.AutomaticAttempts != running.AutomaticAttempts || recovered.LeaseUntil != nil ||
		recovered.WorkerID != nil || recovered.FailureCode == nil ||
		*recovered.FailureCode != string(platform.CodeInternalError) ||
		recovered.DueAt.Sub(recovered.UpdatedAt) < 24*time.Second ||
		recovered.DueAt.Sub(recovered.UpdatedAt) > 36*time.Second {
		t.Fatalf("incorrect recovered state: %+v err=%v", recovered, err)
	}
	successor, err := recovered.Claim(recovered.Revision, uuid.New(), recovered.DueAt)
	if err != nil || successor.Attempts != 2 || successor.AutomaticAttempts != 2 ||
		successor.FencingToken <= recovered.FencingToken {
		t.Fatalf("successor lost actual attempts/fence: %+v err=%v", successor, err)
	}
	if _, err := successor.Complete(lease, true, successor.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("lost worker completed over successor: %v", err)
	}
	for _, state := range []Candidate{recovered, successor} {
		if _, err := state.RecoverExpired(lease, state.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatalf("duplicate recovery accepted: %v", err)
		}
	}
	running.Attempts, running.AutomaticAttempts = 5, 5
	exhausted, err := running.RecoverExpired(lease, *running.LeaseUntil)
	if err != nil || exhausted.State != "failed" || exhausted.Attempts != 5 || exhausted.AutomaticAttempts != 5 {
		t.Fatalf("expired fifth attempt restarted: %+v err=%v", exhausted, err)
	}
	running.FencingToken, lease.Fence = math.MaxInt64, math.MaxInt64
	if _, err := running.RecoverExpired(lease, *running.LeaseUntil); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("recovery fence overflow accepted: %v", err)
	}
}

func TestCandidateSupersessionPreservesHistoryAndRejectsPublishedOrStaleWork(t *testing.T) {
	pending := candidateFixture()
	pending.State, pending.Attempts, pending.AutomaticAttempts = "pending", 0, 0
	running, err := pending.Claim(pending.Revision, uuid.New(), pending.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	lease, err := running.Lease()
	if err != nil {
		t.Fatal(err)
	}
	superseded, err := running.Supersede(running.Revision, running.UpdatedAt)
	if err != nil || superseded.State != "superseded" || superseded.Revision != running.Revision+1 ||
		superseded.FencingToken != running.FencingToken+1 || superseded.Attempts != running.Attempts ||
		superseded.AutomaticAttempts != running.AutomaticAttempts || superseded.WorkerID != nil ||
		superseded.LeaseUntil != nil || superseded.Selected != nil {
		t.Fatalf("supersession lost history or ownership: %+v err=%v", superseded, err)
	}
	if _, err := superseded.Complete(lease, true, superseded.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("superseded worker completed: %v", err)
	}
	if _, err := superseded.Supersede(running.Revision, superseded.UpdatedAt); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale supersession accepted: %v", err)
	}
	replayed, err := superseded.Supersede(superseded.Revision, superseded.UpdatedAt)
	if err != nil || replayed.Revision != superseded.Revision || replayed.FencingToken != superseded.FencingToken {
		t.Fatalf("idempotent supersession advanced history: %+v err=%v", replayed, err)
	}
	completed, err := running.Complete(lease, true, running.UpdatedAt)
	if err != nil {
		t.Fatal(err)
	}
	// Completed work with a published result stays supersedable: a newer selection replaces it.
	if superseded, err := completed.Supersede(completed.Revision, completed.UpdatedAt); err != nil || superseded.State != "superseded" {
		t.Fatalf("completed work was not supersedable: %+v err=%v", superseded, err)
	}
}

func TestCandidateExhaustedReselectionRemainsManuallyRetryableWithoutAutomaticReset(t *testing.T) {
	for _, attempts := range []int{4, 5} {
		current := candidateFixture()
		current.AutomaticAttempts = attempts
		binding := current.CandidateBinding
		binding.ConfigurationRevision++
		next, changed, err := current.Reselect(binding, current.UpdatedAt)
		if err != nil || !changed || next.AutomaticAttempts != attempts || next.Attempts != current.Attempts {
			t.Fatalf("reselection history: %+v changed=%v err=%v", next, changed, err)
		}
		if attempts == 4 {
			if next.State != "pending" || next.FailureCode != nil {
				t.Fatalf("remaining automatic attempt blocked: %+v", next)
			}
			continue
		}
		if next.State != "failed" || next.FailureCode == nil || *next.FailureCode != string(platform.CodeResourceLimit) {
			t.Fatalf("exhausted reselection not visibly failed: %+v", next)
		}
		retried, err := next.RetryFailed(next.Revision, next.UpdatedAt)
		if err != nil {
			t.Fatal(err)
		}
		manual, err := retried.Claim(retried.Revision, uuid.New(), retried.UpdatedAt)
		if err != nil || manual.AutomaticAttempts != 5 || manual.Attempts != current.Attempts+1 {
			t.Fatalf("manual attempt reset automatic history: %+v err=%v", manual, err)
		}
	}
}
