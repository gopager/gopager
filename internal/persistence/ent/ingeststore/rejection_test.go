package ingeststore_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/persistence/ent/ingeststore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// failingRejections is the production store with a failing rejection record.
type failingRejections struct{ *ingeststore.Store }

func (failingRejections) RecordRejectedSubmission(context.Context, platform.TransactionContext, uuid.UUID, string,
	string, time.Time) error {
	return platform.NewFailure(platform.CodeStorageUnavailable, errors.New("injected rejection-record failure"))
}

// unusedMatcher fails the test if an invalid Submission ever reaches target matching.
type unusedMatcher struct{ t *testing.T }

func (m unusedMatcher) NetworksWithMatches(context.Context, platform.TransactionContext, []uuid.UUID,
	protocols.Protocol, time.Time, []protocols.Target) (assignments.NetworkCandidates, error) {
	m.t.Error("invalid Submission reached Network matching")
	return assignments.NetworkCandidates{}, errors.New("unexpected matching")
}

func (m unusedMatcher) MatchTargets(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol,
	time.Time, []protocols.Target) (assignments.MatchResult, error) {
	m.t.Error("invalid Submission reached target matching")
	return assignments.MatchResult{}, errors.New("unexpected matching")
}

// TestRejectedSubmissionIsReportedOnlyWithItsRecord covers the C-01 rejection invariant: when
// the rejection record cannot be committed, Submit returns the storage error instead of the
// rejection code and nothing is recorded; when it can, Submit returns INVALID_INPUT and exactly
// one operation.submission-rejected fact exists.
func TestRejectedSubmissionIsReportedOnlyWithItsRecord(t *testing.T) {
	t.Parallel()
	f := newStateFixture(t)
	gateway := f.gateway(t, ingest.StateActive, true, nil)
	secret, hash, err := identity.NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO gateway_credentials(id,gateway_id,secret_hash,created_at) VALUES($1,$2,$3,now())`,
		uuid.New(), gateway, hash[:])
	invalid := ingest.Submission{SubmissionID: "rejected-1", Protocol: protocols.Protocol("pocsag")}
	rejections := func() int {
		t.Helper()
		var count int
		if err := f.primary.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
			executor, err := f.primary.Resolve(tx)
			if err != nil {
				return err
			}
			return executor.QueryRowContext(tx.Context(), `SELECT count(*) FROM domain_events
				WHERE type='operation.submission-rejected' AND envelope->'data'->>'submissionId'='rejected-1'`).Scan(&count)
		}); err != nil {
			t.Fatal(err)
		}
		return count
	}

	failing, err := ingest.NewService(f.primary, f.primary, failingRejections{f.store}, unusedMatcher{t})
	if err != nil {
		t.Fatal(err)
	}
	_, err = failing.Submit(t.Context(), secret, invalid, 64)
	if code := platform.FailureCode(err); code != platform.CodeStorageUnavailable {
		t.Fatalf("Submit with a failing rejection record returned %v (%s), want STORAGE_UNAVAILABLE", err, code)
	}
	if count := rejections(); count != 0 {
		t.Fatalf("rejection facts after a failed record: %d", count)
	}

	service, err := ingest.NewService(f.primary, f.primary, f.store, unusedMatcher{t})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Submit(t.Context(), secret, invalid, 64)
	if code := platform.FailureCode(err); code != platform.CodeInvalidInput {
		t.Fatalf("Submit returned %v (%s), want INVALID_INPUT", err, code)
	}
	if count := rejections(); count != 1 {
		t.Fatalf("rejection facts after a recorded rejection: %d, want 1", count)
	}
}
