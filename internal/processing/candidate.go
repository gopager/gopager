package processing

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/platform"
)

// CandidateBinding identifies source-bound work and the captured current configuration.
// Configuration revisions are pins, not additional members of the deduplication key.
type CandidateBinding struct {
	TransmissionID        uuid.UUID
	WorkflowID            uuid.UUID
	DefinitionVersionID   uuid.UUID
	SourceRevision        int64
	ContextRevision       int64
	WorkflowRevision      int64
	ConfigurationRevision int64
}

// Validate rejects missing identities and nonpositive revision pins.
func (b CandidateBinding) Validate() error {
	if b.TransmissionID == uuid.Nil || b.WorkflowID == uuid.Nil || b.DefinitionVersionID == uuid.Nil ||
		b.SourceRevision <= 0 || b.ContextRevision <= 0 || b.WorkflowRevision <= 0 || b.ConfigurationRevision <= 0 {
		return invalid(errors.New("invalid candidate binding"))
	}
	return nil
}

// Candidate is private durable evaluation state, not proof of source publication.
type Candidate struct {
	CandidateBinding
	ID                 uuid.UUID
	Revision           int64
	State              string
	Selected           *bool
	Attempts           int64
	AutomaticAttempts  int
	ManualRetryPending bool
	FencingToken       int64
	WorkerID           *uuid.UUID
	LeaseUntil         *time.Time
	DueAt              time.Time
	FailureCode        *string
	FailureStep        *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Validate checks the persisted state without interpreting it as publication authority.
func (c Candidate) Validate() error {
	if err := c.CandidateBinding.Validate(); err != nil {
		return err
	}
	if c.ID == uuid.Nil || c.Revision <= 0 || c.Attempts < 0 ||
		c.AutomaticAttempts < 0 || c.AutomaticAttempts > MaxAutomaticAttempts ||
		int64(c.AutomaticAttempts) > c.Attempts || c.FencingToken < 0 ||
		!slices.Contains([]string{"pending", "running", "completed", "failed", "superseded"}, c.State) ||
		(c.WorkerID == nil) != (c.LeaseUntil == nil) ||
		(c.WorkerID != nil && *c.WorkerID == uuid.Nil) ||
		(c.LeaseUntil != nil && (c.LeaseUntil.IsZero() || !c.LeaseUntil.After(c.UpdatedAt))) ||
		(c.State == "running" && (c.WorkerID == nil || c.FencingToken == 0)) ||
		(c.State == "completed" && c.Selected == nil) ||
		c.CreatedAt.IsZero() || c.UpdatedAt.Before(c.CreatedAt) || c.DueAt.IsZero() {
		return invalid(errors.New("invalid candidate state"))
	}
	return nil
}

// Reselect returns unchanged state for identical pins, or new pending state for current changed pins.
// It retains identity, cumulative attempts and fencing history; an already published result stays
// attached to the Message and the new run adds its own result revision.
func (c Candidate) Reselect(binding CandidateBinding, primaryNow time.Time) (Candidate, bool, error) {
	if err := c.Validate(); err != nil {
		return Candidate{}, false, err
	}
	if err := binding.Validate(); err != nil {
		return Candidate{}, false, err
	}
	if primaryNow.IsZero() || primaryNow.Nanosecond()%1000 != 0 || primaryNow.Before(c.UpdatedAt) {
		return Candidate{}, false, invalid(errors.New("invalid candidate primary time"))
	}
	if c.TransmissionID != binding.TransmissionID || c.WorkflowID != binding.WorkflowID ||
		c.DefinitionVersionID != binding.DefinitionVersionID || c.SourceRevision != binding.SourceRevision ||
		c.ContextRevision != binding.ContextRevision || binding.WorkflowRevision < c.WorkflowRevision ||
		binding.ConfigurationRevision < c.ConfigurationRevision {
		return Candidate{}, false, authz.ErrRevisionConflict
	}
	if c.CandidateBinding == binding {
		return c, false, nil
	}
	revision, err := commands.NextRevision(c.Revision, c.Revision)
	if err != nil {
		return Candidate{}, false, err
	}
	fence := int64(1)
	if c.FencingToken > 0 {
		fence, err = commands.NextRevision(c.FencingToken, c.FencingToken)
		if err != nil {
			return Candidate{}, false, err
		}
	}
	c.CandidateBinding, c.Revision, c.FencingToken = binding, revision, fence
	c.State, c.DueAt, c.UpdatedAt = "pending", primaryNow.UTC(), primaryNow.UTC()
	c.Selected, c.WorkerID, c.LeaseUntil = nil, nil, nil
	c.FailureCode, c.FailureStep, c.ManualRetryPending = nil, nil, false
	if c.AutomaticAttempts == MaxAutomaticAttempts {
		code := string(platform.CodeResourceLimit)
		c.State, c.FailureCode = "failed", &code
	}
	return c, true, nil
}

// CandidateRepository persists caller-authorized current source eligibility in the caller's transaction.
// The source owner must exclude already published originals and supply current source/context pins.
type CandidateRepository interface {
	AdmitCandidate(context.Context, platform.TransactionContext, CandidateBinding) (Candidate, error)
	ClaimCandidate(context.Context, platform.TransactionContext, uuid.UUID, int64, uuid.UUID) (Candidate, error)
	RenewCandidate(context.Context, platform.TransactionContext, CandidateLease) (Candidate, error)
}

// CandidateLeaseDuration is how long a claimed or renewed candidate lease stays valid,
// measured from the primary clock.
const CandidateLeaseDuration = 60 * time.Second

// CandidateRenewInterval is how often a worker renews a held candidate lease; it leaves
// room for two missed renewals before CandidateLeaseDuration expires.
const CandidateRenewInterval = 20 * time.Second

// CandidateLease binds a worker write to one candidate revision and ownership fence.
type CandidateLease struct {
	CandidateID uuid.UUID
	Revision    int64
	Fence       int64
	WorkerID    uuid.UUID
}

// Lease returns the running ownership reference, never a claim for nonrunning work.
func (c Candidate) Lease() (CandidateLease, error) {
	if err := c.Validate(); err != nil {
		return CandidateLease{}, err
	}
	if c.State != "running" {
		return CandidateLease{}, authz.ErrRevisionConflict
	}
	return CandidateLease{CandidateID: c.ID, Revision: c.Revision, Fence: c.FencingToken, WorkerID: *c.WorkerID}, nil
}

// CheckLease rejects stale, substituted or expired writes at authoritative primary time.
func (c Candidate) CheckLease(lease CandidateLease, primaryNow time.Time) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if primaryNow.IsZero() || primaryNow.Nanosecond()%1000 != 0 || primaryNow.Before(c.UpdatedAt) {
		return invalid(errors.New("invalid candidate primary time"))
	}
	if c.State != "running" || lease.CandidateID != c.ID || lease.Revision != c.Revision ||
		lease.Fence != c.FencingToken || lease.WorkerID != *c.WorkerID || !primaryNow.Before(*c.LeaseUntil) {
		return authz.ErrRevisionConflict
	}
	return nil
}

// Claim reserves one due attempt with a fresh fence and the shared primary-clock lease.
// Exhausted automatic history is not reset by a reselected configuration.
func (c Candidate) Claim(expected int64, worker uuid.UUID, primaryNow time.Time) (Candidate, error) {
	if err := c.Validate(); err != nil {
		return Candidate{}, err
	}
	if worker == uuid.Nil || primaryNow.IsZero() || primaryNow.Nanosecond()%1000 != 0 ||
		primaryNow.Before(c.UpdatedAt) {
		return Candidate{}, invalid(errors.New("invalid candidate claim"))
	}
	if c.State != "pending" || c.Revision != expected || primaryNow.Before(c.DueAt) {
		return Candidate{}, authz.ErrRevisionConflict
	}
	if !c.ManualRetryPending && c.AutomaticAttempts == MaxAutomaticAttempts {
		return Candidate{}, platform.NewFailure(platform.CodeResourceLimit, errors.New("automatic attempts exhausted"))
	}
	revision, err := commands.NextRevision(c.Revision, expected)
	if err != nil {
		return Candidate{}, err
	}
	fence := int64(1)
	if c.FencingToken > 0 {
		fence, err = commands.NextRevision(c.FencingToken, c.FencingToken)
		if err != nil {
			return Candidate{}, err
		}
	}
	attempts := int64(1)
	if c.Attempts > 0 {
		attempts, err = commands.NextRevision(c.Attempts, c.Attempts)
		if err != nil {
			return Candidate{}, err
		}
	}
	until := primaryNow.UTC().Add(CandidateLeaseDuration)
	c.Revision, c.FencingToken, c.Attempts = revision, fence, attempts
	if !c.ManualRetryPending {
		c.AutomaticAttempts++
	}
	c.State, c.WorkerID, c.LeaseUntil = "running", &worker, &until
	c.ManualRetryPending, c.UpdatedAt = false, primaryNow.UTC()
	return c, nil
}

// Renew extends only the matching unexpired claim and returns its new revision.
func (c Candidate) Renew(lease CandidateLease, primaryNow time.Time) (Candidate, error) {
	if err := c.CheckLease(lease, primaryNow); err != nil {
		return Candidate{}, err
	}
	revision, err := commands.NextRevision(c.Revision, c.Revision)
	if err != nil {
		return Candidate{}, err
	}
	until := primaryNow.UTC().Add(CandidateLeaseDuration)
	c.Revision, c.LeaseUntil, c.UpdatedAt = revision, &until, primaryNow.UTC()
	return c, nil
}

// RetryFailed queues one already-authorized manual attempt at current unchanged source/configuration.
// It never revives published/nonfailed work or resets the automatic attempt ceiling.
func (c Candidate) RetryFailed(expected int64, primaryNow time.Time) (Candidate, error) {
	if err := c.Validate(); err != nil {
		return Candidate{}, err
	}

	if primaryNow.IsZero() || primaryNow.Nanosecond()%1000 != 0 || primaryNow.Before(c.UpdatedAt) {
		return Candidate{}, invalid(errors.New("invalid retry primary time"))
	}
	if c.State != "failed" || c.Revision != expected {
		return Candidate{}, authz.ErrRevisionConflict
	}
	revision, err := commands.NextRevision(c.Revision, expected)
	if err != nil {
		return Candidate{}, err
	}
	c.Revision, c.State, c.ManualRetryPending = revision, "pending", true
	c.DueAt, c.UpdatedAt = primaryNow.UTC(), primaryNow.UTC()
	c.Selected, c.WorkerID, c.LeaseUntil = nil, nil, nil
	c.FailureCode, c.FailureStep = nil, nil
	return c, nil
}

// Complete records a fenced private evaluation outcome, never source/publication readiness.
func (c Candidate) Complete(lease CandidateLease, selected bool, primaryNow time.Time) (Candidate, error) {
	if err := c.CheckLease(lease, primaryNow); err != nil {
		return Candidate{}, err
	}
	revision, err := commands.NextRevision(c.Revision, c.Revision)
	if err != nil {
		return Candidate{}, err
	}
	c.Revision, c.State, c.Selected, c.UpdatedAt = revision, "completed", &selected, primaryNow.UTC()
	c.WorkerID, c.LeaseUntil, c.FailureCode, c.FailureStep = nil, nil, nil, nil
	return c, nil
}

// Fail records a bounded safe error and schedules only remaining automatic attempts.
// The caller derives retryability from the actual failure, never from a public request.
func (c Candidate) Fail(lease CandidateLease, code platform.ErrorCode, retryable bool, primaryNow time.Time) (Candidate, error) {
	if err := c.CheckLease(lease, primaryNow); err != nil {
		return Candidate{}, err
	}
	return c.recordFailure(code, retryable, primaryNow)
}

// RecoverExpired fences a lost pure-compute claim and schedules only remaining automatic attempts.
func (c Candidate) RecoverExpired(lease CandidateLease, primaryNow time.Time) (Candidate, error) {
	if err := c.Validate(); err != nil {
		return Candidate{}, err
	}
	if primaryNow.IsZero() || primaryNow.Nanosecond()%1000 != 0 || primaryNow.Before(c.UpdatedAt) {
		return Candidate{}, invalid(errors.New("invalid recovery primary time"))
	}
	if c.State != "running" || lease.CandidateID != c.ID || lease.Revision != c.Revision ||
		lease.Fence != c.FencingToken || lease.WorkerID != *c.WorkerID ||
		primaryNow.Before(*c.LeaseUntil) {
		return Candidate{}, authz.ErrRevisionConflict
	}
	fence, err := commands.NextRevision(c.FencingToken, c.FencingToken)
	if err != nil {
		return Candidate{}, err
	}
	next, err := c.recordFailure(platform.CodeInternalError, true, primaryNow)
	if err != nil {
		return Candidate{}, err
	}
	next.FencingToken = fence
	return next, nil
}

// recordFailure retains actual attempt history and stores only a closed platform failure code.
func (c Candidate) recordFailure(code platform.ErrorCode, retryable bool, primaryNow time.Time) (Candidate, error) {
	if !slices.Contains([]platform.ErrorCode{platform.CodeInvalidInput, platform.CodeIncompatibleVersion,
		platform.CodeResourceLimit, platform.CodeStorageUnavailable, platform.CodeAuthorityUnavailable,
		platform.CodeInternalError}, code) {
		return Candidate{}, invalid(errors.New("invalid safe candidate failure"))
	}
	revision, err := commands.NextRevision(c.Revision, c.Revision)
	if err != nil {
		return Candidate{}, err
	}
	due, schedule := primaryNow.UTC(), retryable
	if retryable && c.AutomaticAttempts > 0 {
		due, schedule, err = NextAutomaticRetry(c.AutomaticAttempts, primaryNow)
		if err != nil {
			return Candidate{}, err
		}
	}
	c.Revision, c.State, c.UpdatedAt = revision, "failed", primaryNow.UTC()
	if schedule {
		c.State, c.DueAt = "pending", due
	}
	safeCode := string(code)
	c.FailureCode, c.FailureStep, c.Selected = &safeCode, nil, nil
	c.WorkerID, c.LeaseUntil, c.ManualRetryPending = nil, nil, false
	return c, nil
}

// CandidateFailureRepository records failures only under a current unexpired claim.
type CandidateFailureRepository interface {
	FailCandidate(context.Context, platform.TransactionContext, CandidateLease, platform.ErrorCode, bool) (Candidate, error)
}

// CandidateRecoveryRepository recovers expired pure computation after current source/role checks.
// It does not reclaim completed private results or external side effects.
type CandidateRecoveryRepository interface {
	RecoverCandidate(context.Context, platform.TransactionContext, CandidateLease) (Candidate, error)
}

// CandidateWorkRepository exposes bounded private dispatch metadata and owner-decided supersession.
// Neither operation grants source access or permission to publish.
type CandidateWorkRepository interface {
	ListCandidateWork(context.Context, platform.TransactionContext, PageInput) (Page[Candidate], error)
	SupersedeCandidate(context.Context, platform.TransactionContext, uuid.UUID, int64) (Candidate, error)
}

// Supersede permanently ends this execution revision without deleting private evidence or attempts.
// Only an unpublished candidate may be superseded; a changed current selection uses Reselect.
func (c Candidate) Supersede(expected int64, primaryNow time.Time) (Candidate, error) {
	if err := c.Validate(); err != nil {
		return Candidate{}, err
	}
	if primaryNow.IsZero() || primaryNow.Nanosecond()%1000 != 0 || primaryNow.Before(c.UpdatedAt) {
		return Candidate{}, invalid(errors.New("invalid supersession primary time"))
	}
	if c.Revision != expected {
		return Candidate{}, authz.ErrRevisionConflict
	}
	if c.State == "superseded" {
		return c, nil
	}
	revision, err := commands.NextRevision(c.Revision, expected)
	if err != nil {
		return Candidate{}, err
	}
	fence := int64(1)
	if c.FencingToken > 0 {
		fence, err = commands.NextRevision(c.FencingToken, c.FencingToken)
		if err != nil {
			return Candidate{}, err
		}
	}
	c.Revision, c.FencingToken, c.State, c.UpdatedAt = revision, fence, "superseded", primaryNow.UTC()
	c.WorkerID, c.LeaseUntil, c.Selected, c.ManualRetryPending = nil, nil, nil, false
	return c, nil
}

// CandidateRetryRepository supplies action-filtered private state and protected expected-revision retry.
// The management/source owners must authorize the current source before requesting the mutation.
type CandidateRetryRepository interface {
	ReadRetryCandidate(context.Context, platform.TransactionContext, authz.Authority, uuid.UUID, uuid.UUID, time.Time) (Candidate, error)
	RetryCandidate(context.Context, platform.TransactionContext, authz.Authority, uuid.UUID, uuid.UUID, int64) (Candidate, error)
}
