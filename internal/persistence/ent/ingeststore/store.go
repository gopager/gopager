// Package ingeststore owns Gateway/Network and immutable intake persistence in the caller's transaction.
package ingeststore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/protocols"
)

// Store persists gateway intake (Transmissions, Messages, typed targets and the message_units cache),
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

// AuthenticateGatewayCredential verifies current credential state of an enabled, undeleted Gateway and
// atomically cuts over a pending replacement.
func (s *Store) AuthenticateGatewayCredential(ctx context.Context, tx platform.TransactionContext, hash [32]byte) (identity.GatewayAuthentication, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return identity.GatewayAuthentication{}, err
	}
	var result identity.GatewayAuthentication
	var pending bool
	err = executor.QueryRowContext(ctx, `SELECT c.gateway_id, c.id, c.pending
		FROM gateway_credentials c JOIN gateways g ON g.id=c.gateway_id
		WHERE c.secret_hash=$1 AND c.revoked_at IS NULL AND g.enabled AND NOT g.deleted
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
		n.protocol, gn.priority
		FROM gateway_networks gn JOIN networks n ON n.id=gn.network_id
		WHERE gn.gateway_id=$1 AND n.deleted=false
		ORDER BY gn.priority`, gateway)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ingest.GatewayNetwork
	for rows.Next() {
		var network ingest.GatewayNetwork
		var protocol string
		if err := rows.Scan(&network.Identifier, &network.NetworkID, &protocol, &network.Priority); err != nil {
			return nil, err
		}
		network.Protocol = protocols.Protocol(protocol)
		result = append(result, network)
	}
	return result, rows.Err()
}

// ReadNetwork implements current Radio network ownership for matching and authorization: it
// returns the Network's Protocol and home Agency, or authz.ErrNotFound for an unknown or
// deleted Network. The share lock keeps a concurrent Network deletion from passing its
// in-use check while the caller starts referencing the Network.
func (s *Store) ReadNetwork(ctx context.Context, tx platform.TransactionContext, id uuid.UUID) (protocols.Protocol, uuid.UUID, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return "", uuid.Nil, err
	}
	var protocol string
	var agency uuid.UUID
	if err := executor.QueryRowContext(ctx, `SELECT protocol,agency_id FROM networks WHERE id=$1 AND deleted=false FOR SHARE`, id).
		Scan(&protocol, &agency); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", uuid.Nil, authz.ErrNotFound
		}
		return "", uuid.Nil, err
	}
	return protocols.Protocol(protocol), agency, nil
}

// ReportGatewayState retains authenticated contact/report data and records only effective state changes
// (history row and fact).
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
	newState := ingest.NextGatewayState(oldState, report.Operation)
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
	return s.recordTransition(ctx, tx, executor, gateway, ingest.GatewayTransition{Revision: revision + 1,
		OldState: oldState, NewState: newState, Cause: ingest.CauseReport, OccurredAt: now, ContactAt: now,
		ReasonCode: report.ReasonCode, Detail: report.Detail})
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

// notFoundIfNoRows maps a missing row to authz.ErrNotFound and returns every other error
// unchanged, so a storage failure reaches the Gateway as a retryable failure instead of a 404.
func notFoundIfNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return authz.ErrNotFound
	}
	return err
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
		return ingest.Receipt{}, notFoundIfNoRows(err)
	}
	newState := ingest.NextGatewayState(oldState, "submission")
	if _, err := executor.ExecContext(ctx, `UPDATE gateways SET state=$2, revision=revision+$3,
		last_contact_at=$4, last_transmission_at=$4 WHERE id=$1`,
		gateway, newState, boolInt64(oldState != newState), now); err != nil {
		return ingest.Receipt{}, err
	}
	if oldState != newState {
		if err := s.recordTransition(ctx, tx, executor, gateway, ingest.GatewayTransition{Revision: gatewayRevision + 1,
			OldState: oldState, NewState: newState, Cause: ingest.CauseSubmission, OccurredAt: now,
			ContactAt: now}); err != nil {
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
	decision, times, err := s.groupMessage(ctx, executor, groupingRevision, window, submission)
	if err != nil {
		return ingest.Receipt{}, err
	}
	messageID := decision.MessageID
	transmissionID := uuid.New()
	payload := submission.Payload
	incomplete := submission.IncompleteValue()
	var quality any
	if len(submission.QualityMetrics) != 0 {
		quality = string(submission.QualityMetrics)
	}
	_, observationOffset := submission.ObservedAt.Zone()
	// The declaration and the revision the Network was resolved at are the inputs of a later
	// re-resolution after a Gateway mapping change.
	var declared any
	if submission.GatewayNetworkIdentifier != "" {
		declared = submission.GatewayNetworkIdentifier
	}
	if _, err := executor.ExecContext(ctx, `INSERT INTO transmissions
		(id,gateway_id,network_id,submission_id,submission_fingerprint,network_resolution,protocol,payload,
		 observed_at,observation_offset_seconds,accepted_at,source_revision,incomplete,quality_metrics,message_id,
		 processing_status,gateway_network_identifier,network_revision)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,1,$12,$13,$14,'pending',$15,$16)`,
		transmissionID, gateway, submission.ResolvedNetwork, submission.SubmissionID, submission.Fingerprint[:],
		submission.Resolution, string(submission.Protocol), string(payload),
		submission.ObservedAt, observationOffset, accepted, incomplete, quality, messageID,
		declared, match.Revision); err != nil {
		return ingest.Receipt{}, err
	}
	if submission.Resolution == "address-priority" {
		slog.Warn("Gateway Network inference was ambiguous; highest-priority matching Network applied",
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
		times.Current, submission, accepted, match); err != nil {
		return ingest.Receipt{}, err
	}
	for _, id := range decision.AbsorbedMessage {
		if err := s.mergeMessageUnits(ctx, executor, messageID, id); err != nil {
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
	switch decision.Kind {
	case "created":
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageCreated,
			TransmissionID: transmissionID, MessageID: messageID, Revision: decision.MessageRevision, OccurredAt: now,
			MessageTime: times.Current}); err != nil {
			return ingest.Receipt{}, err
		}
	case "merged":
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageMerged,
			TransmissionID: transmissionID, MessageID: messageID, Absorbed: decision.AbsorbedMessage,
			Revision: decision.MessageRevision, OccurredAt: now, MessageTime: times.Current,
			PreviousMessageTime: times.Previous}); err != nil {
			return ingest.Receipt{}, err
		}
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageTransmissionJoined,
			TransmissionID: transmissionID, MessageID: messageID, LateJoin: decision.LateJoin,
			Revision: decision.MessageRevision, OccurredAt: now, MessageTime: times.Current,
			PreviousMessageTime: times.Previous}); err != nil {
			return ingest.Receipt{}, err
		}
	default:
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.MessageTransmissionJoined,
			TransmissionID: transmissionID, MessageID: messageID, LateJoin: decision.LateJoin,
			Revision: decision.MessageRevision, OccurredAt: now, MessageTime: times.Current,
			PreviousMessageTime: times.Previous}); err != nil {
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
	// The processing context is the assignment revision stamped on the Transmission's own targets;
	// a targetless Transmission has none to re-resolve and keeps the constant context 1.
	contextRevision := match.Revision
	if len(match.Resolutions) == 0 {
		contextRevision = 1
	}
	for _, workflow := range selected {
		if _, err := executor.ExecContext(ctx, `INSERT INTO processing_candidates
			(id,transmission_id,workflow_id,definition_version_id,workflow_revision,configuration_revision,
			 source_revision,context_revision,state,due_at,created_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,1,$7,'pending',$8,$8,$8)`,
			uuid.New(), transmission, workflow.id, workflow.version, workflow.revision,
			configurationRevision, contextRevision, now); err != nil {
			return "", err
		}
	}
	status := "pending"
	if len(selected) == 0 {
		status = "none"
	} else if _, err := executor.ExecContext(ctx, `SELECT pg_notify('gopager_processing_due', '')`); err != nil {
		return "", err
	}
	if _, err := executor.ExecContext(ctx, `UPDATE transmissions SET processing_status=$2 WHERE id=$1`,
		transmission, status); err != nil {
		return "", err
	}
	return status, nil
}

func (s *Store) storeTargets(ctx context.Context, tx platform.TransactionContext, executor postgres.Executor,
	transmission, message uuid.UUID, messageRevision int64, messageTime time.Time, submission ingest.ValidatedSubmission,
	accepted time.Time, match assignments.MatchResult) error {
	resolved := make([]events.IntakeTarget, 0, len(submission.TransmissionTargets))
	unassigned := false
	for position, target := range submission.TransmissionTargets {
		original, err := json.Marshal(target)
		if err != nil {
			return err
		}
		resolution := match.Resolutions[position]
		if resolution.TargetPosition != position || resolution.Status != "assigned" && resolution.Status != "unassigned" {
			return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("matcher returned invalid target resolution"))
		}
		addressNumber, addressText, function, err := typedTarget(target)
		if err != nil {
			return err
		}
		if _, err := executor.ExecContext(ctx, `INSERT INTO transmission_targets
			(transmission_id,position,original,resolution_status,unit_id,matched_rule_id,assignment_revision,
			 network_id,protocol,observed_at,accepted_at,address_number,address_text,function)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, transmission, position, string(original),
			resolution.Status, resolution.UnitID, resolution.AddressRuleID, match.Revision,
			submission.ResolvedNetwork, string(submission.Protocol), submission.ObservedAt, accepted,
			addressNumber, addressText, function); err != nil {
			return err
		}
		resolved = append(resolved, events.IntakeTarget{Position: position, IntakeResolution: events.IntakeResolution{
			Status: resolution.Status, UnitID: resolution.UnitID, AddressRuleID: resolution.AddressRuleID}})
		if resolution.UnitID == nil {
			unassigned = true
			continue
		}
		// The cache row carries the Unit's Agency; an earlier observation moves both times back.
		if _, err := executor.ExecContext(ctx, `INSERT INTO message_units
			(message_id,unit_id,agency_id,first_observed_at,age_anchor)
			SELECT $1::uuid,$2::uuid,u.agency_id,$3::timestamptz,LEAST($3::timestamptz,$4::timestamptz) FROM units u WHERE u.id=$2
			ON CONFLICT (message_id,unit_id) WHERE unit_id IS NOT NULL DO UPDATE SET
				first_observed_at=LEAST(message_units.first_observed_at,EXCLUDED.first_observed_at),
				age_anchor=LEAST(message_units.age_anchor,EXCLUDED.age_anchor)`,
			message, *resolution.UnitID, submission.ObservedAt, accepted); err != nil {
			return err
		}
	}
	if len(resolved) > 0 {
		// One fact per Transmission keeps the log proportional to submissions, not to targets.
		if err := s.appendFact(ctx, tx, events.IntakeFact{ID: uuid.New(), Type: events.AssignmentChanged,
			TransmissionID: transmission, MessageID: message, Revision: messageRevision, OccurredAt: accepted,
			Reason: "created", Targets: resolved, MessageTime: messageTime}); err != nil {
			return err
		}
	}
	if unassigned || len(submission.TransmissionTargets) == 0 {
		// Targetless traffic and targets no rule matches are cached per Message and Network Agency
		// without a Unit, so the Network's Agency sees them as unassigned.
		if _, err := executor.ExecContext(ctx, `INSERT INTO message_units
			(message_id,unit_id,agency_id,first_observed_at,age_anchor)
			SELECT $1::uuid,NULL,n.agency_id,$3::timestamptz,LEAST($3::timestamptz,$4::timestamptz) FROM networks n WHERE n.id=$2
			ON CONFLICT (message_id,agency_id) WHERE unit_id IS NULL DO UPDATE SET
				first_observed_at=LEAST(message_units.first_observed_at,EXCLUDED.first_observed_at),
				age_anchor=LEAST(message_units.age_anchor,EXCLUDED.age_anchor)`,
			message, submission.ResolvedNetwork, submission.ObservedAt, accepted); err != nil {
			return err
		}
	}
	return nil
}

// typedTarget returns the indexed address columns of a target: exactly one of the numeric and
// the text address is set, and the function is carried as a small integer.
func typedTarget(target protocols.Target) (*int64, *string, *int16, error) {
	var addressNumber *int64
	var addressText *string
	if number, ok := target.Address.Number(); ok {
		if number > math.MaxInt64 {
			return nil, nil, nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("numeric address exceeds the stored range"))
		}
		value := int64(number)
		addressNumber = &value
	} else if text, ok := target.Address.Text(); ok {
		value := text
		addressText = &value
	} else {
		return nil, nil, nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("target without an address"))
	}
	var function *int16
	if target.Function != nil {
		value := int16(*target.Function)
		function = &value
	}
	return addressNumber, addressText, function, nil
}

// mergeMessageUnits folds the cache rows of an absorbed Message into the survivor: equal rows
// keep the earliest times and stay correcting if either side was; the absorbed rows vanish with
// their Message (ON DELETE CASCADE).
func (s *Store) mergeMessageUnits(ctx context.Context, executor postgres.Executor, survivor, absorbed uuid.UUID) error {
	const merged = `first_observed_at=LEAST(message_units.first_observed_at,EXCLUDED.first_observed_at),
			age_anchor=LEAST(message_units.age_anchor,EXCLUDED.age_anchor),
			correction_status=CASE WHEN 'correcting' IN (message_units.correction_status,EXCLUDED.correction_status)
				THEN 'correcting' ELSE 'current' END`
	for _, statement := range []string{
		`INSERT INTO message_units (message_id,unit_id,agency_id,first_observed_at,age_anchor,correction_status)
		SELECT $1::uuid,unit_id,agency_id,first_observed_at,age_anchor,correction_status FROM message_units
		WHERE message_id=$2 AND unit_id IS NOT NULL
		ON CONFLICT (message_id,unit_id) WHERE unit_id IS NOT NULL DO UPDATE SET ` + merged,
		`INSERT INTO message_units (message_id,unit_id,agency_id,first_observed_at,age_anchor,correction_status)
		SELECT $1::uuid,NULL,agency_id,first_observed_at,age_anchor,correction_status FROM message_units
		WHERE message_id=$2 AND unit_id IS NULL
		ON CONFLICT (message_id,agency_id) WHERE unit_id IS NULL DO UPDATE SET ` + merged,
	} {
		if _, err := executor.ExecContext(ctx, statement, survivor, absorbed); err != nil {
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
