package ingest

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
)

// Contact monitoring defaults: a Gateway is expected to send a heartbeat every
// minute and is considered to have lost contact after three missed heartbeats.
const (
	DefaultHeartbeatInterval = 60 * time.Second
	DefaultContactTimeout    = 180 * time.Second
	DefaultContactScan       = 15 * time.Second
	// contactScanBatch bounds the Gateways changed in one scan transaction.
	contactScanBatch = 100
	// contactMaxRetry bounds the backoff after consecutive storage failures.
	contactMaxRetry = 30 * time.Second
)

// ContactConfig is the Operator's contact-loss configuration. Timeout is the
// silence after which an active or degraded Gateway becomes contact-lost; it
// must be at least HeartbeatInterval, the interval in which Gateways are
// expected to report. ScanInterval is the pause between scans and must not
// exceed Timeout.
type ContactConfig struct {
	HeartbeatInterval time.Duration
	Timeout           time.Duration
	ScanInterval      time.Duration
}

// Validate rejects non-positive durations, a Timeout shorter than the
// HeartbeatInterval and a ScanInterval longer than the Timeout with
// CodeInvalidInput.
func (c ContactConfig) Validate() error {
	if c.HeartbeatInterval <= 0 || c.Timeout < c.HeartbeatInterval || c.ScanInterval <= 0 || c.ScanInterval > c.Timeout {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid Gateway contact configuration"))
	}
	return nil
}

// ContactRepository marks contact loss in the caller's transaction.
// MarkContactLost sets at most limit enabled, undeleted Gateways in state
// active or degraded whose last contact is at or before cutoff to
// contact-lost, skipping Gateways locked by other transactions. For each it
// advances the revision and records the transition (history row and
// gateway.state-changed fact, cause timeout, contact time = last contact,
// occurred at now) in the same transaction, and it returns the number changed.
type ContactRepository interface {
	MarkContactLost(context.Context, platform.TransactionContext, time.Time, time.Time, int) (int, error)
}

// ContactMonitor is the worker-role runner that detects Gateway contact loss
// against the database primary clock. Several monitors may run concurrently:
// each Gateway changes at most once per expiry because changed rows no longer
// qualify and locked rows are skipped.
type ContactMonitor struct {
	transactions identity.Transactions
	clock        platform.Clock
	repository   ContactRepository
	config       ContactConfig
}

// NewContactMonitor returns a ContactMonitor, failing with CodeInvalidInput for
// a nil dependency or an invalid configuration.
func NewContactMonitor(transactions identity.Transactions, clock platform.Clock, repository ContactRepository,
	config ContactConfig) (*ContactMonitor, error) {
	if transactions == nil || clock == nil || repository == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Gateway contact monitor dependency"))
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &ContactMonitor{transactions: transactions, clock: clock, repository: repository, config: config}, nil
}

// RunOnce performs one scan transaction and returns the number of Gateways set
// to contact-lost; a full batch means more may be due.
func (m *ContactMonitor) RunOnce(ctx context.Context) (int, error) {
	var changed int
	err := m.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		now, err := m.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		changed, err = m.repository.MarkContactLost(tx.Context(), tx, now, now.Add(-m.config.Timeout), contactScanBatch)
		return err
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}

// Run scans until ctx is cancelled and then returns ctx.Err(). A full batch is
// followed immediately by the next scan, otherwise it waits ScanInterval. A
// transient storage failure (STORAGE_UNAVAILABLE, AUTHORITY_UNAVAILABLE,
// COMMIT_UNCERTAIN, REVISION_CONFLICT) is logged and retried after a backoff
// that starts at ScanInterval and doubles up to 30 s (or up to ScanInterval when
// that is longer); any other failure is returned.
func (m *ContactMonitor) Run(ctx context.Context) error {
	failures := 0
	for {
		wait := m.config.ScanInterval
		changed, err := m.RunOnce(ctx)
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil && !transientContactFailure(err):
			return err
		case err != nil:
			failures++
			wait = min(m.config.ScanInterval<<min(failures-1, 16), max(contactMaxRetry, m.config.ScanInterval))
			slog.Warn("Gateway contact monitor storage failure; retrying", "code", platform.FailureCode(err),
				"failures", failures, "retryIn", wait)
		default:
			if failures > 0 {
				slog.Info("Gateway contact monitor recovered", "failures", failures)
				failures = 0
			}
			if changed == contactScanBatch {
				wait = 0
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// transientContactFailure reports storage failures that a later scan can overcome.
func transientContactFailure(err error) bool {
	switch platform.FailureCode(err) {
	case platform.CodeStorageUnavailable, platform.CodeAuthorityUnavailable,
		platform.CodeCommitUncertain, platform.CodeRevisionConflict:
		return true
	}
	return false
}
