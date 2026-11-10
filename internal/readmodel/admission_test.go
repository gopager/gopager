package readmodel

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/identity"
)

func TestAdmissionBucketPerCallerKind(t *testing.T) {
	t.Parallel()
	credential, principal := uuid.New(), uuid.New()
	key := identity.Authentication{Kind: identity.AccessKeyProof, CredentialID: credential,
		Authority: authz.Authority{PrincipalID: principal.String()}}
	session := identity.Authentication{Kind: identity.SessionProof, CredentialID: uuid.New(),
		Authority: authz.Authority{PrincipalID: principal.String()}}
	guest := identity.Authentication{Kind: identity.AnonymousProof}
	if AdmissionBucket(key, "10.0.0.1") != sha256.Sum256([]byte("key:"+credential.String())) {
		t.Error("an access key is bucketed by its credential")
	}
	if AdmissionBucket(session, "10.0.0.1") != sha256.Sum256([]byte("principal:"+principal.String())) {
		t.Error("a session is bucketed by its Principal")
	}
	if AdmissionBucket(guest, "10.0.0.1") != sha256.Sum256([]byte("source:10.0.0.1")) {
		t.Error("the guest is bucketed by its client source")
	}
	if AdmissionBucket(key, "a") != AdmissionBucket(key, "b") || AdmissionBucket(session, "a") != AdmissionBucket(session, "b") {
		t.Error("the source matters only for the guest")
	}
	if AdmissionBucket(key, "a") == AdmissionBucket(session, "a") {
		t.Error("a key shares no bucket with its owner's session")
	}
	otherSession := session
	otherSession.CredentialID = uuid.New()
	if AdmissionBucket(session, "a") != AdmissionBucket(otherSession, "a") {
		t.Error("two sessions of one Principal share a bucket")
	}
}

func TestRetryAfterRoundsUpToTheWindowEnd(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 11, 8, 10, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		offset time.Duration
		want   int64
	}{
		{0, 60},
		{22*time.Second + 500*time.Millisecond, 38},
		{59 * time.Second, 1},
		{59*time.Second + 999*time.Millisecond, 1},
	} {
		limited := &identity.RateLimited{RetryAfter: RetryAfter(start.Add(test.offset))}
		if got := limited.Seconds(); got != test.want {
			t.Errorf("offset %v: seconds = %d, want %d", test.offset, got, test.want)
		}
	}
}

func TestNewAdmissionRejectsLimitsBelowOne(t *testing.T) {
	t.Parallel()
	if _, err := NewAdmission(nil, nil, nil, nil, 0); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("err = %v", err)
	}
}
