// Package ingeststore owns Gateway/Network and immutable intake persistence in the caller's transaction.
package ingeststore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/messages"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// Store persists gateway intake (Transmissions, Messages, targets and contributions),
// Networks, gateways and gateway credentials, and appends the matching intake facts. It
// holds no transaction of its own: every method runs in the caller's active primary
// transaction.
type Store struct {
	primary       *postgres.Store
	facts         events.IntakeAppender
	identityFacts events.Appender
}

// New returns a Store bound to the primary database. All dependencies are required; facts
// receives intake facts and identityFacts receives the credential-revoked configuration
// fact emitted when a pending gateway credential replaces the current one.
func New(primary *postgres.Store, facts events.IntakeAppender, identityFacts events.Appender) (*Store, error) {
	if primary == nil || facts == nil || identityFacts == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing intake persistence dependency"))
	}
	return &Store{primary: primary, facts: facts, identityFacts: identityFacts}, nil
}

// AuthenticateGatewayCredential verifies current credential state and atomically cuts over a pending replacement.
func (s *Store) AuthenticateGatewayCredential(ctx context.Context, tx platform.TransactionContext, hash [32]byte) (identity.GatewayAuthentication, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return identity.GatewayAuthentication{}, err
	}
	var result identity.GatewayAuthentication
	var pending bool
	err = executor.QueryRowContext(ctx, `SELECT c.gateway_id, c.id, c.pending
		FROM gateway_credentials c JOIN gateways g ON g.id=c.gateway_id
		WHERE c.secret_hash=$1 AND c.revoked_at IS NULL
		FOR UPDATE OF c,g`, hash[:]).Scan(&result.GatewayID, &result.CredentialID, &pending)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return identity.GatewayAuthentication{}, identity.ErrLoginRequired
		}
		return identity.GatewayAuthentication{}, err
	}
	if pending {
		now, err := s.primary.Now(ctx, tx)
		if err != nil {
			return identity.GatewayAuthentication{}, err
		}
		var previousCredential uuid.UUID
		err = executor.QueryRowContext(ctx, `SELECT id FROM gateway_credentials
			WHERE gateway_id=$1 AND revoked_at IS NULL AND pending=false FOR UPDATE`,
			result.GatewayID).Scan(&previousCredential)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return identity.GatewayAuthentication{}, err
		}
		if err == nil {
			if _, err := executor.ExecContext(ctx, `UPDATE gateway_credentials
				SET revoked_at=$2 WHERE id=$1 AND revoked_at IS NULL`, previousCredential, now); err != nil {
				return identity.GatewayAuthentication{}, err
			}
			var revision int64
			if err := executor.QueryRowContext(ctx, `UPDATE gateways SET revision=revision+1
				WHERE id=$1 RETURNING revision`, result.GatewayID).Scan(&revision); err != nil {
				return identity.GatewayAuthentication{}, err
			}
			if _, err := s.identityFacts.AppendIdentityFact(ctx, tx, events.IdentityFact{
				ID: uuid.New(), Type: events.ConfigurationChanged,
				// Issuance is credential revision 1; the final revocation is revision 2.
				Aggregate: events.EntityRef{Type: "credential", ID: previousCredential}, Revision: 2,
				OccurredAt: now, ChangeKind: events.CredentialRevoked,
			}); err != nil {
				return identity.GatewayAuthentication{}, err
			}
		}
		if _, err := executor.ExecContext(ctx, `UPDATE gateway_credentials
			SET pending=false, last_used_at=$2 WHERE id=$1 AND pending=true AND revoked_at IS NULL`,
			result.CredentialID, now); err != nil {
			return identity.GatewayAuthentication{}, err
		}
	} else {
		now, err := s.primary.Now(ctx, tx)
		if err != nil {
			return identity.GatewayAuthentication{}, err
		}
		if _, err := executor.ExecContext(ctx, `UPDATE gateway_credentials SET last_used_at=$2
			WHERE id=$1 AND revoked_at IS NULL`, result.CredentialID, now); err != nil {
			return identity.GatewayAuthentication{}, err
		}
	}
	return result, nil
}

// GatewayNetworks locks the owning Gateway so assignment replacement cannot race this operation.
func (s *Store) GatewayNetworks(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID) ([]ingest.GatewayNetwork, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	var exists uuid.UUID
	if err := executor.QueryRowContext(ctx, `SELECT id FROM gateways WHERE id=$1 FOR UPDATE`, gateway).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, authz.ErrNotFound
		}
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT gn.gateway_network_identifier, gn.network_id,
		n.protocol, gn.is_default
		FROM gateway_networks gn JOIN networks n ON n.id=gn.network_id
		WHERE gn.gateway_id=$1 AND n.deleted=false
		ORDER BY gn.gateway_network_identifier`, gateway)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ingest.GatewayNetwork
	for rows.Next() {
		var network ingest.GatewayNetwork
		var protocol string
		if err := rows.Scan(&network.Identifier, &network.NetworkID, &protocol, &network.Default); err != nil {
			return nil, err
		}
		network.Protocol = protocols.Protocol(protocol)
		result = append(result, network)
	}
	return result, rows.Err()
}

// ReadNetworkProtocol implements current Network ownership for matching and identity-scope checks.
func (s *Store) ReadNetworkProtocol(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return "", err
	}
	var protocol string
	if err := executor.QueryRowContext(ctx, `SELECT protocol FROM networks WHERE id=$1 AND deleted=false`, id).Scan(&protocol); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", authz.ErrNotFound
		}
		return "", err
	}
	return protocols.Protocol(protocol), nil
}

// ReportGatewayState retains authenticated contact/report data and emits only effective state changes.
func (s *Store) ReportGatewayState(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	report ingest.GatewayStateReport, now time.Time) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if err := report.Validate(); err != nil {
		return platform.NewFailure(platform.CodeInvalidInput, err)
	}
	var oldState string
	var revision int64
	if err := executor.QueryRowContext(ctx, `SELECT state,revision FROM gateways WHERE id=$1 FOR UPDATE`,
		gateway).Scan(&oldState, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.ErrNotFound
		}
		return err
	}
	newState := oldState
	switch report.Operation {
	case "heartbeat":
		if oldState == "stopped" || oldState == "contact-lost" {
			newState = "active"
		}
	case "degraded":
		newState = "degraded"
	case "recovered":
		newState = "active"
	case "terminated":
		newState = "stopped"
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE gateways SET state=$2,revision=revision+$3,
		last_contact_at=$4,last_heartbeat_at=CASE WHEN $5 THEN $4 ELSE last_heartbeat_at END,last_report=$6
		WHERE id=$1`, gateway, newState, boolInt64(oldState != newState), now,
		report.Operation == "heartbeat", string(encoded)); err != nil {
		return err
	}
	if oldState == newState {
		return nil
	}
	return s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.GatewayStateChanged,
		GatewayID: gateway, Revision: revision + 1, OccurredAt: now, ContactTime: &now,
		OldState: oldState, NewState: newState, ReasonCode: report.ReasonCode, Detail: report.Detail})
}

// ScopeExists checks all referenced current Network IDs without inventing scope support elsewhere.
func (s *Store) ScopeExists(ctx context.Context, tx platform.TransactionContext, scope authz.Scope) (bool, error) {
	if scope.Kind != "network" || !authz.ValidScope(scope) {
		return false, nil
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	for _, reference := range scope.References {
		id, err := uuid.Parse(reference)
		if err != nil {
			return false, nil
		}
		var exists bool
		if err := executor.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM networks WHERE id=$1 AND deleted=false)`, id).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

// SubmissionReceipt serializes all retries for a Gateway while the operation's fingerprint is checked.
func (s *Store) SubmissionReceipt(ctx context.Context, tx platform.TransactionContext,
	gateway uuid.UUID, submissionID string) (ingest.Receipt, [32]byte, bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return ingest.Receipt{}, [32]byte{}, false, err
	}
	var foundGateway uuid.UUID
	if err := executor.QueryRowContext(ctx, `SELECT id FROM gateways WHERE id=$1 FOR UPDATE`, gateway).Scan(&foundGateway); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ingest.Receipt{}, [32]byte{}, false, authz.ErrNotFound
		}
		return ingest.Receipt{}, [32]byte{}, false, err
	}
	var receipt ingest.Receipt
	var fingerprint []byte
	err = executor.QueryRowContext(ctx, `SELECT submission_id, id, accepted_at, processing_status, message_id,
		submission_fingerprint FROM transmissions WHERE gateway_id=$1 AND submission_id=$2`,
		gateway, submissionID).Scan(&receipt.SubmissionID, &receipt.TransmissionID, &receipt.AcceptanceTime,
		&receipt.ProcessingStatus, &receipt.MessageID, &fingerprint)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ingest.Receipt{}, [32]byte{}, false, nil
		}
		return ingest.Receipt{}, [32]byte{}, false, err
	}
	var result [32]byte
	if len(fingerprint) != len(result) {
		return ingest.Receipt{}, [32]byte{}, false, platform.NewFailure(platform.CodeAuthorityUnavailable,
			errors.New("stored Submission fingerprint has invalid size"))
	}
	copy(result[:], fingerprint)
	return receipt, result, true, nil
}

// AcceptTransmission persists original evidence, current target admission, Grouping and facts atomically.
func (s *Store) AcceptTransmission(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	submission ingest.ValidatedSubmission, accepted time.Time, match assignments.MatchResult) (ingest.Receipt, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return ingest.Receipt{}, err
	}
	if submission.ResolvedNetwork == uuid.Nil || match.Revision <= 0 ||
		len(match.Resolutions) != len(submission.TransmissionTargets) || accepted.IsZero() {
		return ingest.Receipt{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("incomplete intake resolution"))
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return ingest.Receipt{}, err
	}
	var oldState string
	var gatewayRevision int64
	if err := executor.QueryRowContext(ctx, `SELECT state, revision FROM gateways WHERE id=$1 FOR UPDATE`,
		gateway).Scan(&oldState, &gatewayRevision); err != nil {
		return ingest.Receipt{}, authz.ErrNotFound
	}
	newState := oldState
	if oldState == "stopped" || oldState == "contact-lost" {
		newState = "active"
	}
	if _, err := executor.ExecContext(ctx, `UPDATE gateways SET state=$2, revision=revision+$3,
		last_contact_at=$4, last_transmission_at=$4 WHERE id=$1`,
		gateway, newState, boolInt64(oldState != newState), now); err != nil {
		return ingest.Receipt{}, err
	}
	if oldState != newState {
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.GatewayStateChanged,
			GatewayID: gateway, Revision: gatewayRevision + 1, OccurredAt: now, ContactTime: &now,
			OldState: oldState, NewState: newState}); err != nil {
			return ingest.Receipt{}, err
		}
	}
	// Exact Grouping serializes per Payload identity; the control row is only read.
	if _, err := executor.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || E'\x1f' || $2 || E'\x1f' || encode($3, 'hex'), 0))`,
		submission.Identity.Form, submission.Identity.Subtype, submission.Identity.Value); err != nil {
		return ingest.Receipt{}, err
	}
	var groupingRevision, window int64
	if err := executor.QueryRowContext(ctx, `SELECT revision, window_seconds FROM grouping_control WHERE id=1 FOR SHARE`).
		Scan(&groupingRevision, &window); err != nil {
		return ingest.Receipt{}, err
	}
	decision, err := s.groupMessage(ctx, executor, groupingRevision, window, submission)
	if err != nil {
		return ingest.Receipt{}, err
	}
	messageID := decision.MessageID
	transmissionID := uuid.New()
	payload := submission.Payload
	targets, err := json.Marshal(submission.TransmissionTargets)
	if err != nil {
		return ingest.Receipt{}, err
	}
	incomplete := submission.IncompleteValue()
	var quality any
	if len(submission.QualityMetrics) != 0 {
		quality = string(submission.QualityMetrics)
	}
	_, observationOffset := submission.ObservedAt.Zone()
	if _, err := executor.ExecContext(ctx, `INSERT INTO transmissions
		(id,gateway_id,network_id,submission_id,submission_fingerprint,network_resolution,protocol,payload,targets,
		 observed_at,observation_offset_seconds,accepted_at,source_revision,incomplete,quality_metrics,message_id,
		 processing_status)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,1,$13,$14,$15,'pending')`,
		transmissionID, gateway, submission.ResolvedNetwork, submission.SubmissionID, submission.Fingerprint[:],
		submission.Resolution, string(submission.Protocol), string(payload), string(targets),
		submission.ObservedAt, observationOffset, accepted, incomplete, quality, messageID); err != nil {
		return ingest.Receipt{}, err
	}
	if submission.AmbiguousNetwork {
		slog.Warn("Gateway Network inference was ambiguous; default applied",
			"transmissionId", transmissionID)
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.NetworkAmbiguous,
			GatewayID: gateway, TransmissionID: transmissionID, NetworkID: submission.ResolvedNetwork,
			Resolution: submission.Resolution, Revision: 1, OccurredAt: accepted}); err != nil {
			return ingest.Receipt{}, err
		}
	}
	if submission.ObservedAt.After(accepted.Add(15 * time.Minute)) {
		slog.Warn("Gateway Observation time exceeds the configured clock-skew tolerance",
			"transmissionId", transmissionID)
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.ObservationTimeImplausible,
			GatewayID: gateway, TransmissionID: transmissionID, Revision: 1, OccurredAt: accepted}); err != nil {
			return ingest.Receipt{}, err
		}
	}
	if err := s.storeTargets(ctx, tx, executor, transmissionID, messageID, decision.MessageRevision,
		submission, accepted, match); err != nil {
		return ingest.Receipt{}, err
	}
	for _, id := range decision.AbsorbedMessage {
		if err := s.mergeContributions(ctx, executor, messageID, id); err != nil {
			return ingest.Receipt{}, err
		}
	}
	if len(decision.AbsorbedMessage) > 0 {
		if _, err := executor.ExecContext(ctx, `UPDATE transmissions SET message_id=$1
			WHERE message_id=ANY($2::uuid[])`, messageID, decision.AbsorbedMessage); err != nil {
			return ingest.Receipt{}, err
		}
		// Published results follow their Transmissions into the surviving Message.
		if _, err := executor.ExecContext(ctx, `UPDATE processing_results SET message_id=$1
			WHERE message_id=ANY($2::uuid[])`, messageID, decision.AbsorbedMessage); err != nil {
			return ingest.Receipt{}, err
		}
		if _, err := executor.ExecContext(ctx, `DELETE FROM messages WHERE id=ANY($1::uuid[])`, decision.AbsorbedMessage); err != nil {
			return ingest.Receipt{}, err
		}
	}
	if decision.Kind == "created" {
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageCreated,
			TransmissionID: transmissionID, MessageID: messageID, Revision: decision.MessageRevision, OccurredAt: now}); err != nil {
			return ingest.Receipt{}, err
		}
	} else if decision.Kind == "merged" {
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageMerged,
			TransmissionID: transmissionID, MessageID: messageID, Absorbed: decision.AbsorbedMessage,
			Revision: decision.MessageRevision, OccurredAt: now}); err != nil {
			return ingest.Receipt{}, err
		}
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageTransmissionJoined,
			TransmissionID: transmissionID, MessageID: messageID, LateJoin: decision.LateJoin,
			Revision: decision.MessageRevision, OccurredAt: now}); err != nil {
			return ingest.Receipt{}, err
		}
	} else {
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageTransmissionJoined,
			TransmissionID: transmissionID, MessageID: messageID, LateJoin: decision.LateJoin,
			Revision: decision.MessageRevision, OccurredAt: now}); err != nil {
			return ingest.Receipt{}, err
		}
	}
	status, err := s.admitProcessing(ctx, executor, transmissionID, submission.ResolvedNetwork, match, now)
	if err != nil {
		return ingest.Receipt{}, err
	}
	receipt := ingest.Receipt{SubmissionID: submission.SubmissionID, TransmissionID: transmissionID,
		AcceptanceTime: accepted, ProcessingStatus: status, MessageID: messageID}
	return receipt, nil
}

// admitProcessing inserts one pending candidate per Workflow currently selected for the Transmission
// and returns the resulting processing status: pending when work was admitted, none otherwise.
func (s *Store) admitProcessing(ctx context.Context, executor postgres.Executor, transmission,
	network uuid.UUID, match assignments.MatchResult, now time.Time) (string, error) {
	var configurationRevision int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM processing_control WHERE id=1 FOR SHARE`).
		Scan(&configurationRevision); err != nil {
		return "", err
	}
	unitIDs := make([]uuid.UUID, 0, len(match.UnitContext))
	for id := range match.UnitContext {
		unitIDs = append(unitIDs, id)
	}
	sort.Slice(unitIDs, func(i, j int) bool { return unitIDs[i].String() < unitIDs[j].String() })
	rows, err := executor.QueryContext(ctx, `SELECT w.id,w.active_version_id,w.revision
		FROM processing_workflows w
		WHERE w.enabled=true AND w.deleted=false AND w.active_version_id IS NOT NULL AND (
			w.scope_kind='all' OR
			(w.scope_kind='network' AND EXISTS (
				SELECT 1 FROM processing_workflow_scope_references r
				WHERE r.workflow_id=w.id AND r.network_id=$1
			)) OR
			(w.scope_kind='unit' AND EXISTS (
				SELECT 1 FROM processing_workflow_scope_references r
				WHERE r.workflow_id=w.id AND r.unit_id=ANY($2::uuid[])
			))
		)
		ORDER BY w.id FOR SHARE OF w`, network, unitIDs)
	if err != nil {
		return "", err
	}
	type selectedWorkflow struct {
		id       uuid.UUID
		version  uuid.UUID
		revision int64
	}
	var selected []selectedWorkflow
	for rows.Next() {
		var workflow selectedWorkflow
		if err := rows.Scan(&workflow.id, &workflow.version, &workflow.revision); err != nil {
			rows.Close()
			return "", err
		}
		selected = append(selected, workflow)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	for _, workflow := range selected {
		if _, err := executor.ExecContext(ctx, `INSERT INTO processing_candidates
			(id,transmission_id,workflow_id,definition_version_id,workflow_revision,configuration_revision,
			 source_revision,context_revision,state,due_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,1,$7,'pending',$8,$8,$8)`,
			uuid.New(), transmission, workflow.id, workflow.version, workflow.revision,
			configurationRevision, match.Revision, now); err != nil {
			return "", err
		}
	}
	status := "pending"
	if len(selected) == 0 {
		status = "none"
	}
	if _, err := executor.ExecContext(ctx, `UPDATE transmissions SET processing_status=$2 WHERE id=$1`,
		transmission, status); err != nil {
		return "", err
	}
	return status, nil
}

func (s *Store) groupMessage(ctx context.Context, executor postgres.Executor, revision, window int64,
	submission ingest.ValidatedSubmission) (messages.Decision, error) {
	ids := []uuid.UUID{}
	if window > 0 {
		rows, err := executor.QueryContext(ctx, `SELECT m.id FROM messages m
			WHERE m.identity_form=$1 AND m.identity_subtype=$2 AND m.identity_value=$3
			AND EXISTS (
				SELECT 1 FROM transmissions t WHERE t.message_id=m.id
				AND ABS(EXTRACT(EPOCH FROM (t.observed_at-$4::timestamptz))) <= $5::numeric
			)
			ORDER BY m.id FOR UPDATE OF m`,
			submission.Identity.Form, submission.Identity.Subtype, submission.Identity.Value,
			submission.ObservedAt, window)
		if err != nil {
			return messages.Decision{}, err
		}
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return messages.Decision{}, err
			}
			ids = append(ids, id)
		}
		if err := rows.Close(); err != nil {
			return messages.Decision{}, err
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return messages.Decision{}, err
		}
	}
	if len(ids) == 0 {
		id := uuid.New()
		if _, err := executor.ExecContext(ctx, `INSERT INTO messages
			(id,identity_form,identity_subtype,identity_value,first_observed_at,last_observed_at)
			VALUES($1,$2,$3,$4,$5,$5)`, id, submission.Identity.Form, submission.Identity.Subtype,
			submission.Identity.Value, submission.ObservedAt); err != nil {
			return messages.Decision{}, err
		}
		return messages.Decision{Kind: "created", MessageID: id, PolicyRevision: revision,
			MessageRevision: 1}, nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	survivor := ids[0]
	absorbed := append([]uuid.UUID(nil), ids[1:]...)
	var first, last time.Time
	var messageRevision int64
	if err := executor.QueryRowContext(ctx, `SELECT first_observed_at,last_observed_at,revision
		FROM messages WHERE id=$1`, survivor).Scan(&first, &last, &messageRevision); err != nil {
		return messages.Decision{}, err
	}
	for _, id := range absorbed {
		var candidateFirst, candidateLast time.Time
		if err := executor.QueryRowContext(ctx, `SELECT first_observed_at,last_observed_at FROM messages WHERE id=$1`,
			id).Scan(&candidateFirst, &candidateLast); err != nil {
			return messages.Decision{}, err
		}
		if candidateFirst.Before(first) {
			first = candidateFirst
		}
		if candidateLast.After(last) {
			last = candidateLast
		}
	}
	late := submission.ObservedAt.Before(last)
	if submission.ObservedAt.Before(first) {
		first = submission.ObservedAt
	}
	if submission.ObservedAt.After(last) {
		last = submission.ObservedAt
	}
	if err := executor.QueryRowContext(ctx, `UPDATE messages SET revision=revision+1,
		first_observed_at=$2,last_observed_at=$3 WHERE id=$1
		RETURNING revision`, survivor, first, last).Scan(&messageRevision); err != nil {
		return messages.Decision{}, err
	}
	kind := "joined"
	if len(absorbed) > 0 {
		kind = "merged"
	}
	return messages.Decision{Kind: kind, MessageID: survivor, AbsorbedMessage: absorbed, Joined: true,
		PolicyRevision: revision, MessageRevision: messageRevision, LateJoin: late}, nil
}

func (s *Store) storeTargets(ctx context.Context, tx platform.TransactionContext, executor postgres.Executor,
	transmission, message uuid.UUID, messageRevision int64, submission ingest.ValidatedSubmission,
	accepted time.Time, match assignments.MatchResult) error {
	if err := s.captureCorrectionDirty(ctx, executor, transmission, submission.ResolvedNetwork, accepted, match.Revision); err != nil {
		return err
	}
	for position, target := range submission.TransmissionTargets {
		original, err := json.Marshal(target)
		if err != nil {
			return err
		}
		resolution := match.Resolutions[position]
		if resolution.TargetPosition != position || resolution.Status != "assigned" && resolution.Status != "unassigned" {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("matcher returned invalid target resolution"))
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO transmission_targets
			(transmission_id,position,original,resolution_status,unit_id,matched_rule_id,assignment_revision)
			VALUES($1,$2,$3,$4,$5,$6,$7)`, transmission, position, string(original),
			resolution.Status, resolution.UnitID, resolution.AddressRuleID, match.Revision); err != nil {
			return err
		}
		after := &events.IntakeResolution{Status: resolution.Status, UnitID: resolution.UnitID, AddressRuleID: resolution.AddressRuleID}
		targetPosition := position
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.AssignmentChanged,
			TransmissionID: transmission, MessageID: message, Revision: messageRevision, OccurredAt: accepted,
			Reason: "created", TargetPosition: &targetPosition, After: after}); err != nil {
			return err
		}
		if resolution.UnitID == nil {
			continue
		}
		var contributionID uuid.UUID
		err = executor.QueryRowContext(ctx, `SELECT id FROM message_contributions
			WHERE message_id=$1 AND unit_id=$2`, message, *resolution.UnitID).Scan(&contributionID)
		if errors.Is(err, sql.ErrNoRows) {
			contributionID = uuid.New()
			ageAnchor := submission.ObservedAt
			if accepted.Before(ageAnchor) {
				ageAnchor = accepted
			}
			if _, err := executor.ExecContext(ctx, `INSERT INTO message_contributions
				(id,message_id,unit_id,contribution_time,age_anchor)
				VALUES($1,$2,$3,$4,$5)`, contributionID, message, *resolution.UnitID,
				submission.ObservedAt, ageAnchor); err != nil {
				return err
			}
		} else if err == nil {
			if _, err := executor.ExecContext(ctx, `UPDATE message_contributions SET
				contribution_time=LEAST(contribution_time,$2), age_anchor=LEAST(age_anchor,LEAST($2,$3))
				WHERE id=$1`, contributionID, submission.ObservedAt, accepted); err != nil {
				return err
			}
		} else {
			return err
		}
		if _, err = executor.ExecContext(ctx, `INSERT INTO contribution_supports
			(id,contribution_id,transmission_id,target_position,observed_at,accepted_at)
			VALUES($1,$2,$3,$4,$5,$6)`, uuid.New(), contributionID, transmission, position,
			submission.ObservedAt, accepted); err != nil {
			return err
		}
	}
	if len(submission.TransmissionTargets) == 0 {
		var contributionID uuid.UUID
		err := executor.QueryRowContext(ctx, `SELECT id FROM message_contributions
			WHERE message_id=$1 AND unit_id IS NULL`, message).Scan(&contributionID)
		if errors.Is(err, sql.ErrNoRows) {
			contributionID = uuid.New()
			ageAnchor := submission.ObservedAt
			if accepted.Before(ageAnchor) {
				ageAnchor = accepted
			}
			if _, err := executor.ExecContext(ctx, `INSERT INTO message_contributions
				(id,message_id,unit_id,contribution_time,age_anchor)
				VALUES($1,$2,NULL,$3,$4)`, contributionID, message, submission.ObservedAt, ageAnchor); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if _, err := executor.ExecContext(ctx, `UPDATE message_contributions SET
			contribution_time=LEAST(contribution_time,$2),age_anchor=LEAST(age_anchor,LEAST($2,$3))
			WHERE id=$1`, contributionID, submission.ObservedAt, accepted); err != nil {
			return err
		}
		_, err = executor.ExecContext(ctx, `INSERT INTO contribution_supports
			(id,contribution_id,transmission_id,target_position,observed_at,accepted_at)
			VALUES($1,$2,$3,NULL,$4,$5)`, uuid.New(), contributionID, transmission, submission.ObservedAt, accepted)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) captureCorrectionDirty(ctx context.Context, executor postgres.Executor,
	transmission, network uuid.UUID, accepted time.Time, assignmentRevision int64,
) error {
	_, err := executor.ExecContext(ctx, `INSERT INTO correction_dirty_transmissions
		(scope_id,transmission_id,accepted_at,recorded_at)
		SELECT scope.id,$1,$2,$2 FROM assignment_pending_scopes scope
		WHERE scope.assignment_revision=$3 AND scope.state IN ('queued','running')
		AND (jsonb_array_length(scope.network_ids)=0 OR scope.network_ids @> jsonb_build_array($4::text))
		ON CONFLICT (scope_id,accepted_at,transmission_id) DO NOTHING`,
		transmission, accepted, assignmentRevision, network)
	return err
}

func (s *Store) mergeContributions(ctx context.Context, executor postgres.Executor, survivor, absorbed uuid.UUID) error {
	rows, err := executor.QueryContext(ctx, `SELECT id,unit_id,contribution_time,age_anchor
		FROM message_contributions WHERE message_id=$1 ORDER BY id FOR UPDATE`, absorbed)
	if err != nil {
		return err
	}
	type contribution struct {
		id     uuid.UUID
		unit   *uuid.UUID
		time   time.Time
		anchor time.Time
	}
	var rowsToMove []contribution
	for rows.Next() {
		var item contribution
		if err := rows.Scan(&item.id, &item.unit, &item.time, &item.anchor); err != nil {
			rows.Close()
			return err
		}
		rowsToMove = append(rowsToMove, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range rowsToMove {
		var target uuid.UUID
		var err error
		if item.unit != nil {
			err = executor.QueryRowContext(ctx, `SELECT id FROM message_contributions
				WHERE message_id=$1 AND unit_id=$2 FOR UPDATE`, survivor, *item.unit).Scan(&target)
		} else {
			err = executor.QueryRowContext(ctx, `SELECT id FROM message_contributions
				WHERE message_id=$1 AND unit_id IS NULL FOR UPDATE`, survivor).Scan(&target)
		}
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := executor.ExecContext(ctx, `UPDATE message_contributions SET message_id=$2
				WHERE id=$1`, item.id, survivor); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `UPDATE contribution_supports SET contribution_id=$2
			WHERE contribution_id=$1`, item.id, target); err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `UPDATE message_contributions SET
			contribution_time=LEAST(contribution_time,$2),age_anchor=LEAST(age_anchor,$3) WHERE id=$1`,
			target, item.time, item.anchor); err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `DELETE FROM message_contributions WHERE id=$1`, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) appendFact(ctx context.Context, tx platform.TransactionContext, fact events.IntakeFact) error {
	if tx == nil {
		return fmt.Errorf("intake fact requires a live transaction")
	}
	_, err := s.facts.AppendIntakeFact(ctx, tx, fact)
	return err
}

// RecordRejectedSubmission appends a SubmissionRejected intake fact for the gateway,
// stamped with the gateway's current revision; nothing else is stored. It returns
// authz.ErrNotFound for an unknown gateway.
func (s *Store) RecordRejectedSubmission(ctx context.Context, tx platform.TransactionContext, gateway uuid.UUID,
	submissionID, reasonCode string, now time.Time) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var revision int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM gateways WHERE id=$1`, gateway).Scan(&revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.ErrNotFound
		}
		return err
	}
	return s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.SubmissionRejected,
		GatewayID: gateway, SubmissionID: submissionID, Revision: revision, OccurredAt: now, ReasonCode: reasonCode})
}

func boolInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

var _ ingest.Repository = (*Store)(nil)
var _ assignments.NetworkOwner = (*Store)(nil)
var _ identity.ScopeReferences = (*Store)(nil)
