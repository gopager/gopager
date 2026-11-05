package ingest

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/protocols"
)

// Clock supplies the authoritative transaction-scoped time used for acceptance
// timestamps; it matches platform.Clock.
type Clock interface {
	Now(context.Context, platform.TransactionContext) (time.Time, error)
}

// TargetMatcher resolves Transmission targets against the configured address
// rules at the observation time. NetworksWithMatches reports which of the
// candidate Networks have any matching rule (used to disambiguate a Gateway
// with several Networks for one protocol); MatchTargets produces the final
// per-target resolutions for the chosen Network.
type TargetMatcher interface {
	NetworksWithMatches(context.Context, platform.TransactionContext, []uuid.UUID,
		protocols.Protocol, time.Time, []protocols.Target) (assignments.NetworkCandidates, error)
	MatchTargets(context.Context, platform.TransactionContext, uuid.UUID, protocols.Protocol,
		time.Time, []protocols.Target) (assignments.MatchResult, error)
}

// Repository is the intake persistence port; all methods participate in the
// caller's transaction. SubmissionReceipt looks up an earlier acceptance by
// Gateway and Submission ID and returns its stored fingerprint.
// AcceptTransmission stores the Transmission and its derived records and
// returns the receipt. RecordRejectedSubmission durably logs a rejected
// Submission with its public rejection code.
type Repository interface {
	identity.GatewayCredentialRepository
	GatewayNetworks(context.Context, platform.TransactionContext, uuid.UUID) ([]GatewayNetwork, error)
	SubmissionReceipt(context.Context, platform.TransactionContext, uuid.UUID, string) (Receipt, [32]byte, bool, error)
	AcceptTransmission(context.Context, platform.TransactionContext, uuid.UUID,
		ValidatedSubmission, time.Time, assignments.MatchResult) (Receipt, error)
	ReportGatewayState(context.Context, platform.TransactionContext, uuid.UUID, GatewayStateReport, time.Time) error
	RecordRejectedSubmission(context.Context, platform.TransactionContext, uuid.UUID, string, string, time.Time) error
}

// ReportState records an authenticated report separately from any Transmission acceptance.
func (s *Service) ReportState(ctx context.Context, bearer string, report GatewayStateReport) error {
	if err := report.Validate(); err != nil {
		return err
	}
	return s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		gateway, err := s.authenticator.Authenticate(tx.Context(), tx, bearer)
		if err != nil {
			return err
		}
		now, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return s.repository.ReportGatewayState(tx.Context(), tx, gateway.GatewayID, report, now)
	})
}

// Service is the Gateway intake service: it authenticates Gateway bearer
// credentials and accepts Submissions and state reports, each in its own
// transaction. It is safe for concurrent use.
type Service struct {
	transactions  identity.Transactions
	clock         Clock
	repository    Repository
	matcher       TargetMatcher
	authenticator *identity.GatewayAuthenticator
}

// NewService returns an intake Service, failing with CodeInvalidInput if any
// dependency is nil.
func NewService(transactions identity.Transactions, clock Clock, repository Repository, matcher TargetMatcher) (*Service, error) {
	if transactions == nil || clock == nil || repository == nil || matcher == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing intake dependency"))
	}
	authenticator, err := identity.NewGatewayAuthenticator(repository)
	if err != nil {
		return nil, err
	}
	return &Service{transactions: transactions, clock: clock, repository: repository,
		matcher: matcher, authenticator: authenticator}, nil
}

// Submit authenticates, resolves current Network/target context and accepts one immutable Submission atomically.
func (s *Service) Submit(ctx context.Context, bearer string, request Submission, encodedSize int) (Receipt, error) {
	var receipt Receipt
	var authenticatedGateway uuid.UUID
	err := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		gateway, err := s.authenticator.Authenticate(tx.Context(), tx, bearer)
		if err != nil {
			return err
		}
		authenticatedGateway = gateway.GatewayID
		acceptedAt, err := s.clock.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		submission, err := request.Validate(encodedSize, acceptedAt)
		if err != nil {
			if errors.Is(err, ErrSubmissionTooLarge) {
				return platform.NewFailure(platform.CodeResourceLimit, err)
			}
			return platform.NewFailure(platform.CodeInvalidInput, err)
		}
		existing, fingerprint, found, err := s.repository.SubmissionReceipt(tx.Context(), tx,
			gateway.GatewayID, submission.SubmissionID)
		if err != nil {
			return err
		}
		if found {
			if fingerprint != submission.Fingerprint {
				return ErrSubmissionConflict
			}
			receipt = existing
			return nil
		}
		assigned, err := s.repository.GatewayNetworks(tx.Context(), tx, gateway.GatewayID)
		if err != nil {
			return err
		}
		var addressMatches []uuid.UUID
		ambiguousNetwork := false
		if submission.GatewayNetworkIdentifier == "" {
			allowed := make([]uuid.UUID, 0, len(assigned))
			protocolNetworks := 0
			for _, network := range assigned {
				if network.Protocol == submission.Protocol {
					protocolNetworks++
					allowed = append(allowed, network.NetworkID)
				}
			}
			if protocolNetworks > 1 {
				candidates, err := s.matcher.NetworksWithMatches(tx.Context(), tx, allowed,
					submission.Protocol, submission.ObservedAt, submission.TransmissionTargets)
				if err != nil {
					return err
				}
				addressMatches = candidates.Networks
				ambiguousNetwork = len(addressMatches) > 1
			}
		}
		networkID, resolution, err := ResolveNetwork(submission, assigned, addressMatches)
		if err != nil {
			return platform.NewFailure(platform.CodeInvalidInput, err)
		}
		submission.ResolvedNetwork, submission.Resolution = networkID, resolution
		submission.AmbiguousNetwork = ambiguousNetwork
		matched, err := s.matcher.MatchTargets(tx.Context(), tx, networkID, submission.Protocol,
			submission.ObservedAt, submission.TransmissionTargets)
		if err != nil {
			return err
		}
		submission.TargetResolutions = matched.Resolutions
		receipt, err = s.repository.AcceptTransmission(tx.Context(), tx, gateway.GatewayID,
			submission, acceptedAt, matched)
		return err
	})
	if err != nil {
		if code := rejectedSubmissionCode(err); code != "" && authenticatedGateway != uuid.Nil &&
			request.SubmissionID != "" && utf8.ValidString(request.SubmissionID) &&
			len([]byte(request.SubmissionID)) <= MaxSubmissionIDSize &&
			strings.TrimSpace(request.SubmissionID) == request.SubmissionID {
			recordErr := s.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
				now, clockErr := s.clock.Now(tx.Context(), tx)
				if clockErr != nil {
					return clockErr
				}
				return s.repository.RecordRejectedSubmission(tx.Context(), tx, authenticatedGateway,
					request.SubmissionID, code, now)
			})
			if recordErr != nil {
				slog.Error("rejected Gateway Submission could not be durably recorded",
					"code", platform.FailureCode(recordErr))
				return Receipt{}, recordErr
			}
		}
	}
	return receipt, err
}

func rejectedSubmissionCode(err error) string {
	switch {
	case errors.Is(err, ErrSubmissionConflict):
		return "IDEMPOTENCY_CONFLICT"
	case platform.FailureCode(err) == platform.CodeInvalidInput:
		return "INVALID_INPUT"
	case platform.FailureCode(err) == platform.CodeResourceLimit:
		return "RESOURCE_LIMIT"
	default:
		return ""
	}
}
