package processingstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/rowcheck"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

var _ processing.ReprocessRepository = (*Store)(nil)
var _ processing.ReprocessWorkRepository = (*Store)(nil)

// reprocessColumns lists the columns scanReprocess reads, in order.
const reprocessColumns = `id, revision, workflow_id, definition_version_id, workflow_revision,
	message_time_from, message_time_to, cutoff_at, cursor, state, scanned, readmitted, retracted,
	requested_by, due_at, worker_id, lease_until, failure_code, attempts, created_at, updated_at`

// scanReprocess reads one request row in the order of reprocessColumns from row and returns it.
func scanReprocess(row interface{ Scan(...any) error }) (processing.ReprocessRequest, error) {
	var r processing.ReprocessRequest
	err := row.Scan(&r.ID, &r.Revision, &r.WorkflowID, &r.DefinitionVersionID, &r.WorkflowRevision,
		&r.MessageTimeFrom, &r.MessageTimeTo, &r.CutoffAt, &r.Cursor, &r.State, &r.Scanned, &r.Readmitted,
		&r.Retracted, &r.RequestedBy, &r.DueAt, &r.WorkerID, &r.LeaseUntil, &r.FailureCode,
		&r.Attempts, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

// CreateReprocess inserts the pending request.
func (s *Store) CreateReprocess(ctx context.Context, tx platform.TransactionContext, request processing.ReprocessRequest) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	_, err = executor.ExecContext(ctx, `INSERT INTO processing_reprocess_requests
		(id, workflow_id, definition_version_id, workflow_revision, message_time_from, message_time_to,
		 cutoff_at, state, requested_by, due_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',$8,$9,$10,$10)`,
		request.ID, request.WorkflowID, request.DefinitionVersionID, request.WorkflowRevision,
		request.MessageTimeFrom.UTC(), request.MessageTimeTo.UTC(), request.CutoffAt.UTC(),
		request.RequestedBy, request.DueAt.UTC(), request.CreatedAt.UTC())
	if err != nil {
		return err
	}
	// Wake idle runners; a wake-up without work is harmless.
	_, err = executor.ExecContext(ctx, `SELECT pg_notify($1, '')`, processing.ReprocessNotificationChannel)
	return err
}

// ReadReprocess returns one request of the Workflow, or authz.ErrNotFound.
func (s *Store) ReadReprocess(ctx context.Context, tx platform.TransactionContext, workflow, id uuid.UUID) (processing.ReprocessRequest, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.ReprocessRequest{}, err
	}
	request, err := scanReprocess(executor.QueryRowContext(ctx, `SELECT `+reprocessColumns+
		` FROM processing_reprocess_requests WHERE workflow_id=$1 AND id=$2`, workflow, id))
	if errors.Is(err, sql.ErrNoRows) {
		return processing.ReprocessRequest{}, authz.ErrNotFound
	}
	return request, err
}

// ListReprocesses returns a page of the Workflow's requests, newest first, with their total.
func (s *Store) ListReprocesses(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID,
	page processing.PageInput) (processing.Page[processing.ReprocessRequest], error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.Page[processing.ReprocessRequest]{}, err
	}
	result := processing.Page[processing.ReprocessRequest]{Items: []processing.ReprocessRequest{}}
	if err := executor.QueryRowContext(ctx, `SELECT count(*) FROM processing_reprocess_requests WHERE workflow_id=$1`,
		workflow).Scan(&result.Total); err != nil {
		return processing.Page[processing.ReprocessRequest]{}, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT `+reprocessColumns+` FROM processing_reprocess_requests
		WHERE workflow_id=$1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3`, workflow, page.Limit, page.Offset)
	if err != nil {
		return processing.Page[processing.ReprocessRequest]{}, err
	}
	defer rows.Close()
	for rows.Next() {
		request, err := scanReprocess(rows)
		if err != nil {
			return processing.Page[processing.ReprocessRequest]{}, err
		}
		result.Items = append(result.Items, request)
	}
	return result, rows.Err()
}

// ClaimReprocess leases the oldest due request. Rows another open transaction holds are skipped,
// so concurrent runners take different requests.
func (s *Store) ClaimReprocess(ctx context.Context, tx platform.TransactionContext, worker uuid.UUID) (processing.ReprocessRequest, bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.ReprocessRequest{}, false, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return processing.ReprocessRequest{}, false, err
	}
	var id uuid.UUID
	err = executor.QueryRowContext(ctx, `SELECT id FROM processing_reprocess_requests
		WHERE (state='pending' AND due_at <= $1) OR (state='running' AND lease_until <= $1)
		ORDER BY due_at, id LIMIT 1 FOR UPDATE SKIP LOCKED`, now).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return processing.ReprocessRequest{}, false, nil
	}
	if err != nil {
		return processing.ReprocessRequest{}, false, err
	}
	request, err := scanReprocess(executor.QueryRowContext(ctx, `UPDATE processing_reprocess_requests
		SET state='running', worker_id=$2, lease_until=$3, revision=revision+1, updated_at=$4
		WHERE id=$1 RETURNING `+reprocessColumns, id, worker, now.Add(processing.ReprocessLeaseDuration), now))
	return request, err == nil, err
}

// ReadWorkflowPins locks the Processing configuration against change for the rest of the
// transaction and returns the Workflow's current pins, so a step that checks them cannot race a
// Workflow change.
func (s *Store) ReadWorkflowPins(ctx context.Context, tx platform.TransactionContext, workflow uuid.UUID) (processing.WorkflowPins, error) {
	if _, err := s.LockRevision(ctx, tx); err != nil {
		return processing.WorkflowPins{}, err
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.WorkflowPins{}, err
	}
	var pins processing.WorkflowPins
	var enabled, deleted bool
	err = executor.QueryRowContext(ctx, `SELECT revision, active_version_id, enabled, deleted
		FROM processing_workflows WHERE id=$1`, workflow).Scan(&pins.Revision, &pins.ActiveVersionID, &enabled, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return processing.WorkflowPins{}, nil
	}
	pins.Usable = enabled && !deleted
	return pins, err
}

// NextReprocessTransmission returns the first covered Transmission after the request's cursor. A
// Transmission is covered when it was accepted by the cutoff, its Message time lies in the range
// and it either arrives on a Network of an Agency assigned to the Workflow or holds a published
// result of the Workflow, which must be re-judged against the current selection.
func (s *Store) NextReprocessTransmission(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest) (uuid.UUID, bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return uuid.Nil, false, err
	}
	var id uuid.UUID
	err = executor.QueryRowContext(ctx, `SELECT t.id FROM transmissions t JOIN messages m ON m.id = t.message_id
		WHERE ($1::uuid IS NULL OR t.id > $1) AND t.accepted_at <= $2
			AND m.first_observed_at >= $3 AND m.first_observed_at < $4
			AND (EXISTS (SELECT 1 FROM networks n JOIN processing_workflow_agencies a ON a.agency_id = n.agency_id
					WHERE n.id = t.network_id AND a.workflow_id = $5)
				OR EXISTS (SELECT 1 FROM processing_results r JOIN processing_candidates c ON c.id = r.candidate_id
					WHERE c.transmission_id = t.id AND c.workflow_id = $5 AND r.published_at IS NOT NULL))
		ORDER BY t.id LIMIT 1`, request.Cursor, request.CutoffAt, request.MessageTimeFrom, request.MessageTimeTo,
		request.WorkflowID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	return id, err == nil, err
}

// SnapshotTransmission reads the Transmission's candidates before readmission. It is fresh when
// a non-superseded candidate of the request's Workflow carries the pinned revision, was created
// at or after the request and is the Workflow's first work on the Transmission: a candidate that
// replaced earlier work is a historical re-run, however recently it was admitted. Candidates that existed before carry an older revision, and
// ones the runner's own readmission creates do not exist yet when this is asked.
func (s *Store) SnapshotTransmission(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, transmission uuid.UUID) (processing.ReprocessSnapshot, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return processing.ReprocessSnapshot{}, err
	}
	var snapshot processing.ReprocessSnapshot
	err = executor.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM processing_candidates c
		WHERE c.transmission_id=$1 AND c.workflow_id=$2 AND c.state <> 'superseded'
			AND c.workflow_revision=$3 AND c.created_at >= $4 AND NOT `+workflowHistory+`)`,
		transmission, request.WorkflowID, request.WorkflowRevision, request.CreatedAt).Scan(&snapshot.Fresh)
	if err != nil {
		return processing.ReprocessSnapshot{}, err
	}
	snapshot.Others, err = s.otherCandidates(ctx, tx, transmission, request.WorkflowID)
	return snapshot, err
}

// ApplyReprocessStep marks the Transmission's current candidates of the Workflow, or, when the
// readmission left none, retracts the Workflow's published results of the Transmission, marks the
// re-runs of other Workflows as historical and advances the fenced request's cursor and counts.
func (s *Store) ApplyReprocessStep(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, transmission uuid.UUID, snapshot processing.ReprocessSnapshot) ([]processing.RetractedResult, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	var readmitted int64
	var withdrawn []processing.RetractedResult
	if !snapshot.Fresh {
		readmitted, withdrawn, err = s.markOrRetract(ctx, tx, request, transmission)
		if err != nil {
			return nil, err
		}
		if err := s.markCompanionReruns(ctx, tx, request, transmission, snapshot.Others); err != nil {
			return nil, err
		}
	}
	var markedTransmissions int64
	if readmitted > 0 {
		markedTransmissions = 1
	}
	updated, err := executor.ExecContext(ctx, `UPDATE processing_reprocess_requests
		SET cursor=$3, scanned=scanned+1, readmitted=readmitted+$4, retracted=retracted+$5,
			lease_until=$6, updated_at=$7, attempts=0, failure_code=NULL
		WHERE id=$1 AND worker_id=$2 AND state='running'`, request.ID, request.WorkerID, transmission,
		markedTransmissions, int64(len(withdrawn)), now.Add(processing.ReprocessLeaseDuration), now)
	return withdrawn, rowcheck.RequireOne(updated, err)
}

// markOrRetract marks the Transmission's current candidates of the Workflow with the request,
// keeping the attribution of one that an earlier request already marked, and returns how many
// current candidates there are, or, when there are none, retracts the Workflow's published results
// and returns how many.
func (s *Store) markOrRetract(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, transmission uuid.UUID) (readmitted int64, retracted []processing.RetractedResult, err error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return 0, nil, err
	}
	marked, err := executor.ExecContext(ctx, `UPDATE processing_candidates SET reprocess_id=COALESCE(reprocess_id, $3)
		WHERE transmission_id=$1 AND workflow_id=$2 AND state <> 'superseded'`, transmission, request.WorkflowID, request.ID)
	if err != nil {
		return 0, nil, err
	}
	readmitted, err = marked.RowsAffected()
	if err != nil || readmitted > 0 {
		return readmitted, nil, err
	}
	ids, err := s.RetractPublished(ctx, tx, transmission, request.WorkflowID, uuid.Nil)
	return 0, ids, err
}

// ReadTransmissionMessage returns the Message of the Transmission and its Message time.
func (s *Store) ReadTransmissionMessage(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID) (uuid.UUID, time.Time, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return uuid.Nil, time.Time{}, err
	}
	var message uuid.UUID
	var messageTime time.Time
	err = executor.QueryRowContext(ctx, `SELECT m.id, m.first_observed_at FROM transmissions t
		JOIN messages m ON m.id = t.message_id WHERE t.id=$1`, transmission).Scan(&message, &messageTime)
	return message, messageTime.UTC(), err
}

// FinishReprocess ends the claimed request in the given terminal state.
func (s *Store) FinishReprocess(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, state string) error {
	if state != processing.ReprocessCompleted && state != processing.ReprocessSuperseded {
		return authz.ErrInvalidInput
	}
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	updated, err := executor.ExecContext(ctx, `UPDATE processing_reprocess_requests
		SET state=$3, worker_id=NULL, lease_until=NULL, failure_code=NULL, attempts=0, revision=revision+1, updated_at=$4
		WHERE id=$1 AND worker_id=$2 AND state='running'`, request.ID, request.WorkerID, state, now)
	return rowcheck.RequireOne(updated, err)
}

// FailReprocess ends the claimed request as failed and keeps the failure code.
func (s *Store) FailReprocess(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, failureCode string) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	updated, err := executor.ExecContext(ctx, `UPDATE processing_reprocess_requests
		SET state='failed', worker_id=NULL, lease_until=NULL, failure_code=$3, revision=revision+1, updated_at=$4
		WHERE id=$1 AND worker_id=$2 AND state='running'`, request.ID, request.WorkerID, failureCode, now)
	return rowcheck.RequireOne(updated, err)
}

// RescheduleReprocess releases the claim and makes the request due again at due.
func (s *Store) RescheduleReprocess(ctx context.Context, tx platform.TransactionContext,
	request processing.ReprocessRequest, due time.Time, failureCode string) error {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return err
	}
	now, err := s.primary.Now(ctx, tx)
	if err != nil {
		return err
	}
	updated, err := executor.ExecContext(ctx, `UPDATE processing_reprocess_requests
		SET state='pending', worker_id=NULL, lease_until=NULL, due_at=$3, failure_code=$4,
			attempts=attempts+1, revision=revision+1, updated_at=$5
		WHERE id=$1 AND worker_id=$2 AND state='running'`, request.ID, request.WorkerID, due.UTC(), failureCode, now)
	return rowcheck.RequireOne(updated, err)
}
