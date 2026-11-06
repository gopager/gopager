package ingeststore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/assignments"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/platform/postgres"
	"github.com/gopager/gopager/internal/processing"
	processingmanagement "github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/processing/publication"
	"github.com/gopager/gopager/internal/processing/worker"
	"github.com/gopager/gopager/internal/protocols"
)

type processingSourceRepository interface {
	processing.CandidateRepository
	processing.PublicationRepository
	SupersedeCandidate(context.Context, platform.TransactionContext, uuid.UUID, int64) (processing.Candidate, error)
	LockRevision(context.Context, platform.TransactionContext) (int64, error)
	processing.InstanceRepository
}

// ProcessingSourceOwner binds worker inputs and publication to current intake and assignment rows.
type ProcessingSourceOwner struct {
	store       *Store
	matcher     *assignments.Service
	repository  processingSourceRepository
	facts       events.ProcessingAppender
	clock       platform.Clock
	artifacts   processing.InstalledArtifactOwner
	executions  *processing.InstanceExecutions
	principals  processing.ExecutionPrincipalResolver
	publication *publication.Service
}

type sourceSnapshot struct {
	projection       processing.SourceProjection
	captured         *processing.CapturedSource
	identity         processing.SourceIdentity
	transmission     uuid.UUID
	network          uuid.UUID
	networkAgency    uuid.UUID
	message          uuid.UUID
	messageRevision  int64
	sourceRevision   int64
	processingStatus string
	protocol         protocols.Protocol
	payload          protocols.TransmissionPayload
	targets          []protocols.Target
	observedAt       time.Time
	networkName      string
	matched          assignments.MatchResult
	targetsCurrent   bool
	suppressed       map[int]bool
}

type selectedWorkflow struct {
	id       uuid.UUID
	version  uuid.UUID
	revision int64
	scope    string
}

type sourceCandidate struct {
	id       uuid.UUID
	revision int64
	binding  processing.CandidateBinding
	state    string
}

// NewProcessingSourceOwner wires intake-owned source authority to Processing admission, evidence
// publication and the Transmission's processing status.
func NewProcessingSourceOwner(store *Store, matcher *assignments.Service, repository processingSourceRepository,
	facts events.ProcessingAppender, clock platform.Clock, artifacts processing.InstalledArtifactOwner,
	executions *processing.InstanceExecutions, principals processing.ExecutionPrincipalResolver,
) (*ProcessingSourceOwner, error) {
	if store == nil || matcher == nil || repository == nil || facts == nil || clock == nil ||
		artifacts == nil || executions == nil || principals == nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("missing Processing source owner dependency"))
	}
	owner := &ProcessingSourceOwner{store: store, matcher: matcher, repository: repository,
		facts: facts, clock: clock, artifacts: artifacts, executions: executions, principals: principals}
	publisher, err := publication.New(repository, owner, facts, clock)
	if err != nil {
		return nil, err
	}
	owner.publication = publisher
	return owner, nil
}

var _ worker.WorkSourceOwner = (*ProcessingSourceOwner)(nil)
var _ worker.Publisher = (*ProcessingSourceOwner)(nil)
var _ publication.SourceOwner = (*ProcessingSourceOwner)(nil)
var _ processing.InvocationSourceOwner = (*ProcessingSourceOwner)(nil)
var _ processingmanagement.RetrySource = (*ProcessingSourceOwner)(nil)
var _ assignments.ProcessingReadmission = (*ProcessingSourceOwner)(nil)

// CaptureCandidate returns a detached original source and a run-owned installed invocation boundary.
func (o *ProcessingSourceOwner) CaptureCandidate(ctx context.Context, tx platform.TransactionContext,
	candidate processing.Candidate,
) (worker.Capture, error) {
	snapshot, err := o.readSource(ctx, tx, candidate.TransmissionID)
	if err != nil {
		return worker.Capture{}, err
	}
	if !snapshot.targetsCurrent || snapshot.identity.SourceRevision != candidate.SourceRevision ||
		snapshot.identity.ContextRevision != candidate.ContextRevision {
		return worker.Capture{}, platform.NewFailure(platform.CodeRevisionConflict, errors.New("candidate source context changed"))
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return worker.Capture{}, err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return worker.Capture{}, err
	}
	var selected *selectedWorkflow
	for i := range workflows {
		if workflows[i].id == candidate.WorkflowID {
			selected = &workflows[i]
			break
		}
	}
	if selected == nil || selected.version != candidate.DefinitionVersionID ||
		selected.revision != candidate.WorkflowRevision || configurationRevision != candidate.ConfigurationRevision {
		return worker.Capture{}, platform.NewFailure(platform.CodeRevisionConflict, errors.New("candidate Workflow configuration changed"))
	}
	definition, err := o.readDefinition(ctx, tx, selected.id, selected.version)
	if err != nil {
		return worker.Capture{}, err
	}
	scope, err := o.evaluationScope(ctx, tx, *selected, snapshot)
	if err != nil {
		return worker.Capture{}, err
	}
	run, err := o.executions.Begin(ctx, processing.ExecutionBinding{WorkflowID: candidate.WorkflowID,
		DefinitionVersionID: candidate.DefinitionVersionID, RunID: candidate.ID, Source: snapshot.identity}, o)
	if err != nil {
		return worker.Capture{}, err
	}
	return worker.Capture{Definition: definition, Source: snapshot.captured, Scope: scope,
		Artifacts: o.artifacts, Invocations: run, Release: func() error {
			return run.Close(context.Background())
		}}, nil
}

// CheckCandidate revalidates the original, current assignments and saved Workflow before a fenced write.
func (o *ProcessingSourceOwner) CheckCandidate(ctx context.Context, tx platform.TransactionContext,
	candidate processing.Candidate, capture worker.Capture,
) error {
	snapshot, err := o.readSource(ctx, tx, candidate.TransmissionID)
	if err != nil {
		return err
	}
	if !snapshot.targetsCurrent || snapshot.identity.SourceRevision != candidate.SourceRevision ||
		snapshot.identity.ContextRevision != candidate.ContextRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("candidate source context changed"))
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	selected := findSelectedWorkflow(workflows, candidate.WorkflowID)
	if selected == nil || selected.version != candidate.DefinitionVersionID ||
		selected.revision != candidate.WorkflowRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("candidate Workflow is no longer selected"))
	}
	revision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	if revision != candidate.ConfigurationRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("Processing configuration changed"))
	}
	identity, err := capture.Source.Identity()
	if err != nil {
		return err
	}
	if identity != snapshot.identity {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("captured source differs from current source"))
	}
	return o.checkDefinitionInstances(ctx, tx, capture.Definition)
}

// CheckRetrySource verifies, in the retrying transaction, that a failed candidate may be
// manually retried: its Transmission is still unpublished with current targets and the
// same source and context revisions, its Workflow is still selected at the same version
// and Workflow revision, the Processing configuration revision is unchanged, and every
// installed plugin instance its definition references is still valid. Any staleness is
// a revision conflict; a missing Transmission is authz.ErrNotFound.
func (o *ProcessingSourceOwner) CheckRetrySource(ctx context.Context, tx platform.TransactionContext,
	candidate processing.Candidate,
) error {
	snapshot, err := o.readSource(ctx, tx, candidate.TransmissionID)
	if err != nil {
		return err
	}
	if !snapshot.targetsCurrent || snapshot.identity.SourceRevision != candidate.SourceRevision ||
		snapshot.identity.ContextRevision != candidate.ContextRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("candidate source context changed"))
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	selected := findSelectedWorkflow(workflows, candidate.WorkflowID)
	if selected == nil || selected.version != candidate.DefinitionVersionID ||
		selected.revision != candidate.WorkflowRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("candidate Workflow is no longer selected"))
	}
	revision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	if revision != candidate.ConfigurationRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("Processing configuration changed"))
	}
	definition, err := o.readDefinition(ctx, tx, selected.id, selected.version)
	if err != nil {
		return err
	}
	return o.checkDefinitionInstances(ctx, tx, definition)
}

// expectedBindings returns the current candidate binding per selected Workflow for a source snapshot.
func expectedBindings(snapshot sourceSnapshot, configurationRevision int64, workflows []selectedWorkflow) map[uuid.UUID]processing.CandidateBinding {
	expected := make(map[uuid.UUID]processing.CandidateBinding, len(workflows))
	for _, workflow := range workflows {
		expected[workflow.id] = processing.CandidateBinding{
			TransmissionID: snapshot.transmission, WorkflowID: workflow.id,
			DefinitionVersionID: workflow.version, SourceRevision: snapshot.identity.SourceRevision,
			ContextRevision: snapshot.identity.ContextRevision, WorkflowRevision: workflow.revision,
			ConfigurationRevision: configurationRevision,
		}
	}
	return expected
}

// sameKey reports whether a stored candidate binding identifies the same durable candidate as the
// expected one: same Workflow version and same source and Assignment context revisions.
func sameKey(stored, expected processing.CandidateBinding) bool {
	return stored.DefinitionVersionID == expected.DefinitionVersionID &&
		stored.SourceRevision == expected.SourceRevision && stored.ContextRevision == expected.ContextRevision
}

// reconcileBindings supersedes every candidate whose binding is no longer current and admits a
// candidate for every selected Workflow, reselecting an existing one whose Workflow or configuration
// revision moved. It returns the resulting candidate per Workflow and the superseded candidates.
func (o *ProcessingSourceOwner) reconcileBindings(ctx context.Context, tx platform.TransactionContext,
	snapshot sourceSnapshot, configurationRevision int64, workflows []selectedWorkflow, candidates []sourceCandidate,
) (map[uuid.UUID]processing.Candidate, []processing.Candidate, error) {
	expected := expectedBindings(snapshot, configurationRevision, workflows)
	current := make(map[uuid.UUID]processing.Candidate, len(expected))
	var superseded []processing.Candidate
	for _, row := range candidates {
		if row.state == "superseded" {
			continue
		}
		binding, active := expected[row.binding.WorkflowID]
		if !active || !sameKey(row.binding, binding) {
			candidate, err := o.repository.SupersedeCandidate(ctx, tx, row.id, row.revision)
			if err != nil {
				return nil, nil, err
			}
			superseded = append(superseded, candidate)
			continue
		}
		candidate, err := o.repository.AdmitCandidate(ctx, tx, binding)
		if err != nil {
			return nil, nil, err
		}
		current[row.binding.WorkflowID] = candidate
	}
	for _, workflow := range workflows {
		if _, admitted := current[workflow.id]; admitted {
			continue
		}
		candidate, err := o.repository.AdmitCandidate(ctx, tx, expected[workflow.id])
		if err != nil {
			return nil, nil, err
		}
		current[workflow.id] = candidate
	}
	return current, superseded, nil
}

// ReconcileCandidate supersedes obsolete bindings, including the selected candidate's own when
// its source or Assignment context moved, and admits the current selected Workflow set. It returns
// the selected candidate's persisted state after that reconciliation.
func (o *ProcessingSourceOwner) ReconcileCandidate(ctx context.Context, tx platform.TransactionContext,
	selected processing.Candidate,
) (processing.Candidate, error) {
	snapshot, err := o.readSource(ctx, tx, selected.TransmissionID)
	if err != nil {
		return processing.Candidate{}, err
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return processing.Candidate{}, err
	}
	candidates, err := o.readCandidates(ctx, tx, selected.TransmissionID)
	if err != nil {
		return processing.Candidate{}, err
	}
	if !snapshot.targetsCurrent {
		return o.supersedeCandidates(ctx, tx, candidates, selected.ID)
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return processing.Candidate{}, err
	}
	current, superseded, err := o.reconcileBindings(ctx, tx, snapshot, configurationRevision, workflows, candidates)
	if err != nil {
		return processing.Candidate{}, err
	}
	for _, candidate := range superseded {
		if candidate.ID == selected.ID {
			return candidate, nil
		}
	}
	for _, candidate := range current {
		if candidate.ID == selected.ID {
			return candidate, nil
		}
	}
	return processing.Candidate{}, platform.NewFailure(platform.CodeRevisionConflict, errors.New("selected candidate was replaced"))
}

// ReadmitProcessing supersedes the Transmission's candidates whose binding is no longer current,
// admits a candidate for every currently selected Workflow without one, notifies the
// dispatchers when any Workflow is selected and refreshes the processing status, in the
// caller's transaction. The correction runner calls it for every
// Transmission whose target resolution it changed. It fails with a revision conflict while the
// stored targets differ from the current matching. It also returns the Transmission's processing
// context: the highest assignment revision stamped on its targets, or 1 without targets.
func (o *ProcessingSourceOwner) ReadmitProcessing(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID,
) error {
	snapshot, err := o.readSource(ctx, tx, transmission)
	if err != nil {
		return err
	}
	if !snapshot.targetsCurrent {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("stored targets differ from current matching"))
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	candidates, err := o.readCandidates(ctx, tx, transmission)
	if err != nil {
		return err
	}
	if _, _, err := o.reconcileBindings(ctx, tx, snapshot, configurationRevision, workflows, candidates); err != nil {
		return err
	}
	candidates, err = o.readCandidates(ctx, tx, transmission)
	if err != nil {
		return err
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if len(workflows) > 0 {
		// Wake the dispatchers like intake does; a wake-up without new work is harmless.
		if _, err := executor.ExecContext(ctx, `SELECT pg_notify('gopager_processing_due', '')`); err != nil {
			return err
		}
	}
	return o.refreshProcessingStatus(ctx, executor, snapshot, configurationRevision, workflows, candidates)
}

// Publish attaches the completed evidence of the Transmission's current candidates to its Message
// and refreshes its processing status through the publication service.
func (o *ProcessingSourceOwner) Publish(ctx context.Context, tx platform.TransactionContext, transmission uuid.UUID) error {
	return o.publication.Publish(ctx, tx, transmission)
}

// CheckInstalledSource validates current source identity and per-dependency read authority.
func (o *ProcessingSourceOwner) CheckInstalledSource(ctx context.Context, tx platform.TransactionContext,
	binding processing.ExecutionBinding, authority authz.Authority, call processing.InvocationContext,
	dependencies []processing.Dependency, editor bool,
) error {
	if editor || binding.Source.Synthetic || call.Source != binding.Source {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("installed source identity is not current"))
	}
	snapshot, err := o.readSource(ctx, tx, binding.Source.TransmissionID)
	if err != nil {
		return err
	}
	if !snapshot.targetsCurrent || snapshot.identity != binding.Source {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("installed source context changed"))
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var candidate processing.CandidateBinding
	var candidateState string
	err = executor.QueryRowContext(ctx, `SELECT transmission_id,workflow_id,definition_version_id,
		workflow_revision,configuration_revision,source_revision,context_revision,state
		FROM processing_candidates WHERE id=$1 FOR SHARE`, binding.RunID).Scan(
		&candidate.TransmissionID, &candidate.WorkflowID, &candidate.DefinitionVersionID,
		&candidate.WorkflowRevision, &candidate.ConfigurationRevision, &candidate.SourceRevision,
		&candidate.ContextRevision, &candidateState)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return platform.NewFailure(platform.CodeRevisionConflict, errors.New("installed invocation candidate is missing"))
		}
		return err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	selected := findSelectedWorkflow(workflows, binding.WorkflowID)
	if candidate.TransmissionID != binding.Source.TransmissionID ||
		candidate.WorkflowID != binding.WorkflowID ||
		candidate.DefinitionVersionID != binding.DefinitionVersionID ||
		candidate.SourceRevision != binding.Source.SourceRevision ||
		candidate.ContextRevision != binding.Source.ContextRevision ||
		candidate.ConfigurationRevision != configurationRevision || candidateState != "running" ||
		selected == nil || selected.version != candidate.DefinitionVersionID ||
		selected.revision != candidate.WorkflowRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("installed Workflow configuration changed"))
	}
	now, err := o.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	for _, dependency := range dependencies {
		if dependency.TransmissionID != binding.Source.TransmissionID ||
			dependency.NetworkID != binding.Source.NetworkID ||
			dependency.SourceRevision != binding.Source.SourceRevision ||
			dependency.ContextRevision != binding.Source.ContextRevision {
			return platform.NewFailure(platform.CodeRevisionConflict, errors.New("installed dependency differs from source"))
		}
		target, err := o.authorizationTarget(snapshot, dependency.TargetPosition)
		if err != nil {
			return err
		}
		target.AgeAnchor = snapshot.observedAt
		if err := authz.Authorize(authority, "message.read", dependency.Groups, target, now); err != nil {
			return err
		}
	}
	return nil
}

func findSelectedWorkflow(workflows []selectedWorkflow, id uuid.UUID) *selectedWorkflow {
	for i := range workflows {
		if workflows[i].id == id {
			return &workflows[i]
		}
	}
	return nil
}

func (o *ProcessingSourceOwner) readSource(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID,
) (sourceSnapshot, error) {
	if transmission == uuid.Nil {
		return sourceSnapshot{}, authz.ErrInvalidInput
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return sourceSnapshot{}, err
	}
	// Control rows are locked before the Transmission row so that the worker and the
	// correction runner acquire locks in the same order.
	if _, err := o.matcher.HoldRevision(ctx, tx); err != nil {
		return sourceSnapshot{}, err
	}
	var snapshot sourceSnapshot
	var protocolValue, networkProtocol string
	var payloadJSON, targetsJSON []byte
	err = executor.QueryRowContext(ctx, `SELECT t.network_id,n.agency_id,t.message_id,t.protocol,t.payload,
		(SELECT COALESCE(json_agg(tt.original ORDER BY tt.position),'[]'::json) FROM transmission_targets tt WHERE tt.transmission_id=t.id),
		t.observed_at,t.source_revision,t.processing_status,n.name,n.protocol
		FROM transmissions t JOIN networks n ON n.id=t.network_id AND n.deleted=false
		WHERE t.id=$1 FOR UPDATE OF t`, transmission).Scan(
		&snapshot.network, &snapshot.networkAgency, &snapshot.message, &protocolValue, &payloadJSON, &targetsJSON,
		&snapshot.observedAt, &snapshot.sourceRevision, &snapshot.processingStatus,
		&snapshot.networkName, &networkProtocol)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return sourceSnapshot{}, authz.ErrNotFound
		}
		return sourceSnapshot{}, err
	}
	if snapshot.sourceRevision <= 0 {
		return sourceSnapshot{}, platform.NewFailure(platform.CodeRevisionConflict, errors.New("Transmission has no source revision"))
	}
	snapshot.transmission = transmission
	snapshot.protocol = protocols.Protocol(protocolValue)
	if snapshot.protocol != protocols.Protocol(networkProtocol) {
		return sourceSnapshot{}, platform.NewFailure(platform.CodeRevisionConflict, errors.New("Transmission Network Protocol changed"))
	}
	if err := json.Unmarshal(payloadJSON, &snapshot.payload); err != nil {
		return sourceSnapshot{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	if err := json.Unmarshal(targetsJSON, &snapshot.targets); err != nil {
		return sourceSnapshot{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	snapshot.matched, err = o.matcher.MatchTargets(ctx, tx, snapshot.network, snapshot.protocol,
		snapshot.observedAt, snapshot.targets)
	if err != nil {
		return sourceSnapshot{}, err
	}
	snapshot.suppressed, err = o.suppressedTargets(ctx, tx, transmission)
	if err != nil {
		return sourceSnapshot{}, err
	}
	var contextRevision int64
	snapshot.targetsCurrent, contextRevision, err = o.targetsMatch(ctx, tx, snapshot)
	if err != nil {
		return sourceSnapshot{}, err
	}
	projectedMatch := snapshot.matched
	// The processing context follows the Transmission's own targets, not the global revision,
	// so a rule change elsewhere does not invalidate this Transmission's candidates.
	projectedMatch.Revision = contextRevision
	projectedMatch.Resolutions = slices.Clone(snapshot.matched.Resolutions)
	for position := range projectedMatch.Resolutions {
		if snapshot.suppressed[position] {
			projectedMatch.Resolutions[position].Status = "unassigned"
			projectedMatch.Resolutions[position].UnitID = nil
			projectedMatch.Resolutions[position].AddressRuleID = nil
		}
	}
	snapshot.projection, err = projectProcessingSource(ctx, transmission, snapshot.network, snapshot.networkName,
		snapshot.protocol, snapshot.payload, snapshot.observedAt, snapshot.sourceRevision, projectedMatch, snapshot.targets)
	if err != nil {
		return sourceSnapshot{}, err
	}
	snapshot.captured, err = processing.CaptureSource(ctx, snapshot.projection)
	if err != nil {
		return sourceSnapshot{}, err
	}
	snapshot.identity, err = snapshot.captured.Identity()
	return snapshot, err
}

func (o *ProcessingSourceOwner) suppressedTargets(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID,
) (map[int]bool, error) {
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT target_position FROM target_suppressions
		WHERE transmission_id=$1 AND removed_at IS NULL`, transmission)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int]bool)
	for rows.Next() {
		var position int
		if err := rows.Scan(&position); err != nil {
			return nil, err
		}
		result[position] = true
	}
	return result, rows.Err()
}

func (o *ProcessingSourceOwner) targetsMatch(ctx context.Context, tx platform.TransactionContext,
	snapshot sourceSnapshot,
) (bool, int64, error) {
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return false, 0, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT position,resolution_status,unit_id::text,matched_rule_id::text,
		assignment_revision FROM transmission_targets WHERE transmission_id=$1 ORDER BY position FOR UPDATE`,
		snapshot.transmission)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()
	position := 0
	contextRevision := int64(1)
	for rows.Next() {
		var actualPosition int
		var status string
		var unitID, ruleID sql.NullString
		var revision int64
		if err := rows.Scan(&actualPosition, &status, &unitID, &ruleID, &revision); err != nil {
			return false, 0, err
		}
		if position >= len(snapshot.matched.Resolutions) {
			return false, 0, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("stored target count exceeds original"))
		}
		expected := snapshot.matched.Resolutions[position]
		actualUnit, err := parseNullableUUID(unitID)
		if err != nil {
			return false, 0, err
		}
		actualRule, err := parseNullableUUID(ruleID)
		if err != nil {
			return false, 0, err
		}
		if actualPosition != position || expected.TargetPosition != position ||
			status != expected.Status || !equalUUIDPointer(actualUnit, expected.UnitID) ||
			!equalUUIDPointer(actualRule, expected.AddressRuleID) || revision <= 0 {
			return false, 0, nil
		}
		contextRevision = max(contextRevision, revision)
		position++
	}
	if err := rows.Err(); err != nil {
		return false, 0, err
	}
	return position == len(snapshot.matched.Resolutions), contextRevision, nil
}

func parseNullableUUID(value sql.NullString) (*uuid.UUID, error) {
	if !value.Valid {
		return nil, nil
	}
	id, err := uuid.Parse(value.String)
	if err != nil {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, err)
	}
	return &id, nil
}

func equalUUIDPointer(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (o *ProcessingSourceOwner) selectWorkflows(ctx context.Context, tx platform.TransactionContext,
	snapshot sourceSnapshot,
) ([]selectedWorkflow, error) {
	unitIDs := make([]uuid.UUID, 0, len(snapshot.matched.UnitContext))
	for id := range snapshot.matched.UnitContext {
		unitIDs = append(unitIDs, id)
	}
	sort.Slice(unitIDs, func(i, j int) bool { return unitIDs[i].String() < unitIDs[j].String() })
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT w.id,w.active_version_id::text,w.revision,w.scope_kind
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
		ORDER BY w.id FOR SHARE OF w`, snapshot.network, unitIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workflows []selectedWorkflow
	for rows.Next() {
		var workflow selectedWorkflow
		var version string
		if err := rows.Scan(&workflow.id, &version, &workflow.revision, &workflow.scope); err != nil {
			return nil, err
		}
		workflow.version, err = uuid.Parse(version)
		if err != nil || workflow.version == uuid.Nil || workflow.revision <= 0 {
			return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("invalid active Workflow reference"))
		}
		workflows = append(workflows, workflow)
	}
	return workflows, rows.Err()
}

func (o *ProcessingSourceOwner) readDefinition(ctx context.Context, tx platform.TransactionContext,
	workflow, version uuid.UUID,
) (processing.Definition, error) {
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return processing.Definition{}, err
	}
	var encoded []byte
	if err := executor.QueryRowContext(ctx, `SELECT definition FROM processing_workflow_versions
		WHERE id=$1 AND workflow_id=$2`, version, workflow).Scan(&encoded); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return processing.Definition{}, authz.ErrNotFound
		}
		return processing.Definition{}, err
	}
	var definition processing.Definition
	if err := json.Unmarshal(encoded, &definition); err != nil {
		return processing.Definition{}, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	return definition, nil
}

func (o *ProcessingSourceOwner) evaluationScope(ctx context.Context, tx platform.TransactionContext,
	workflow selectedWorkflow, snapshot sourceSnapshot,
) (processing.EvaluationScope, error) {
	if workflow.scope != "unit" {
		positions := make([]uint32, 0, len(snapshot.targets))
		for i := range snapshot.targets {
			if !snapshot.suppressed[i] {
				positions = append(positions, uint32(i))
			}
		}
		return processing.EvaluationScope{SourceEligible: true, TargetPositions: positions}, nil
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return processing.EvaluationScope{}, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT unit_id FROM processing_workflow_scope_references
		WHERE workflow_id=$1 AND unit_id IS NOT NULL ORDER BY unit_id`, workflow.id)
	if err != nil {
		return processing.EvaluationScope{}, err
	}
	defer rows.Close()
	allowed := make(map[uuid.UUID]bool)
	for rows.Next() {
		var unit uuid.UUID
		if err := rows.Scan(&unit); err != nil {
			return processing.EvaluationScope{}, err
		}
		allowed[unit] = true
	}
	if err := rows.Err(); err != nil {
		return processing.EvaluationScope{}, err
	}
	var positions []uint32
	for _, resolution := range snapshot.matched.Resolutions {
		if !snapshot.suppressed[resolution.TargetPosition] &&
			resolution.UnitID != nil && allowed[*resolution.UnitID] {
			positions = append(positions, uint32(resolution.TargetPosition))
		}
	}
	return processing.EvaluationScope{SourceEligible: len(positions) > 0, TargetPositions: positions}, nil
}

func (o *ProcessingSourceOwner) readCandidates(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID,
) ([]sourceCandidate, error) {
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return nil, err
	}
	rows, err := executor.QueryContext(ctx, `SELECT id,revision,transmission_id,workflow_id,
		definition_version_id,workflow_revision,configuration_revision,source_revision,context_revision,
		state FROM processing_candidates WHERE transmission_id=$1
		ORDER BY workflow_id,definition_version_id,id FOR UPDATE`, transmission)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var candidates []sourceCandidate
	for rows.Next() {
		var candidate sourceCandidate
		if err := rows.Scan(&candidate.id, &candidate.revision, &candidate.binding.TransmissionID,
			&candidate.binding.WorkflowID, &candidate.binding.DefinitionVersionID,
			&candidate.binding.WorkflowRevision, &candidate.binding.ConfigurationRevision,
			&candidate.binding.SourceRevision, &candidate.binding.ContextRevision,
			&candidate.state); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (o *ProcessingSourceOwner) supersedeCandidates(ctx context.Context, tx platform.TransactionContext,
	candidates []sourceCandidate, selected uuid.UUID,
) (processing.Candidate, error) {
	var result processing.Candidate
	var found bool
	for _, row := range candidates {
		if row.state == "superseded" && row.id != selected {
			continue
		}
		candidate, err := o.repository.SupersedeCandidate(ctx, tx, row.id, row.revision)
		if err != nil {
			return processing.Candidate{}, err
		}
		if row.id == selected {
			result, found = candidate, true
		}
	}
	if !found {
		return processing.Candidate{}, platform.NewFailure(platform.CodeRevisionConflict, errors.New("selected candidate is no longer current"))
	}
	return result, nil
}

// CapturePublication locks the Transmission's Message and the processing configuration
// revision and returns the publication binding (fenced by the Message revision) plus one
// candidate binding per Workflow currently selected for the source. It fails with a
// revision conflict when the current target context differs from the accepted one, and
// with authz.ErrNotFound when the Message is missing.
func (o *ProcessingSourceOwner) CapturePublication(ctx context.Context, tx platform.TransactionContext,
	transmission uuid.UUID,
) (processing.PublicationBinding, []processing.CandidateBinding, error) {
	snapshot, err := o.readSource(ctx, tx, transmission)
	if err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	if !snapshot.targetsCurrent {
		return processing.PublicationBinding{}, nil, platform.NewFailure(platform.CodeRevisionConflict,
			errors.New("current target context differs from accepted context"))
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM messages WHERE id=$1 FOR UPDATE`,
		snapshot.message).Scan(&snapshot.messageRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return processing.PublicationBinding{}, nil, authz.ErrNotFound
		}
		return processing.PublicationBinding{}, nil, err
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return processing.PublicationBinding{}, nil, err
	}
	bindings := make([]processing.CandidateBinding, 0, len(workflows))
	for _, workflow := range workflows {
		bindings = append(bindings, processing.CandidateBinding{
			TransmissionID: transmission, WorkflowID: workflow.id, DefinitionVersionID: workflow.version,
			SourceRevision: snapshot.identity.SourceRevision, ContextRevision: snapshot.identity.ContextRevision,
			WorkflowRevision: workflow.revision, ConfigurationRevision: configurationRevision,
		})
	}
	return processing.PublicationBinding{Source: snapshot.identity, MessageID: snapshot.message,
		SourceFence: snapshot.messageRevision}, bindings, nil
}

// CheckItem rechecks, in the publishing transaction, that one publication item is still
// current: same source identity and Message revision, the candidate's Workflow still
// selected at the same version, Workflow and configuration revisions, and every
// referenced plugin instance still valid. For items with a result it also checks every
// published target lies in the current Workflow scope and every dependency is readable
// by each installed step's execution Principal. Staleness is a revision conflict; a
// missing Message is authz.ErrNotFound.
func (o *ProcessingSourceOwner) CheckItem(ctx context.Context, tx platform.TransactionContext,
	binding processing.PublicationBinding, item processing.PublicationItem,
) error {
	snapshot, err := o.readSource(ctx, tx, binding.Source.TransmissionID)
	if err != nil {
		return err
	}
	if !snapshot.targetsCurrent || snapshot.identity != binding.Source || snapshot.message != binding.MessageID {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("publication source changed"))
	}
	var messageRevision int64
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return err
	}
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM messages WHERE id=$1 FOR UPDATE`,
		binding.MessageID).Scan(&messageRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.ErrNotFound
		}
		return err
	}
	if messageRevision != binding.SourceFence {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("publication Message revision changed"))
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	selected := findSelectedWorkflow(workflows, item.Candidate.WorkflowID)
	if selected == nil || selected.version != item.Candidate.DefinitionVersionID ||
		selected.revision != item.Candidate.WorkflowRevision ||
		item.Candidate.ConfigurationRevision != configurationRevision ||
		item.Candidate.TransmissionID != binding.Source.TransmissionID ||
		item.Candidate.SourceRevision != binding.Source.SourceRevision ||
		item.Candidate.ContextRevision != binding.Source.ContextRevision {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("publication candidate is no longer selected"))
	}
	if err := o.checkDefinitionInstances(ctx, tx, item.Definition); err != nil {
		return err
	}
	if item.Result != nil {
		scope, err := o.evaluationScope(ctx, tx, *selected, snapshot)
		if err != nil {
			return err
		}
		return o.checkPublicationAuthority(ctx, tx, snapshot, item, scope)
	}
	return nil
}

func (o *ProcessingSourceOwner) checkPublicationAuthority(ctx context.Context, tx platform.TransactionContext,
	snapshot sourceSnapshot, item processing.PublicationItem, scope processing.EvaluationScope,
) error {
	allowed := make(map[uint32]bool, len(scope.TargetPositions))
	for _, position := range scope.TargetPositions {
		allowed[position] = true
	}
	for _, target := range item.Result.Evaluation.TargetResults {
		if !allowed[target.TargetPosition] {
			return platform.NewFailure(platform.CodeRevisionConflict, errors.New("published target is outside current Workflow scope"))
		}
	}
	dependencies, err := item.Result.Evaluation.Dependencies()
	if err != nil {
		return err
	}
	now, err := o.clock.Now(ctx, tx)
	if err != nil {
		return err
	}
	var authorizeAll func([]processing.Step) error
	authorizeAll = func(steps []processing.Step) error {
		for _, step := range steps {
			if step.Installed != nil {
				configuration, err := o.repository.ReadInstance(ctx, tx, step.Installed.InstanceID,
					step.Installed.ConfigurationRevision)
				if err != nil {
					return err
				}
				authority, err := o.principals.ResolvePrincipalAuthority(ctx, tx, configuration.ExecutionPrincipalID)
				if err != nil {
					return err
				}
				for _, dependency := range dependencies {
					if dependency.TransmissionID != snapshot.transmission || dependency.NetworkID != snapshot.network ||
						dependency.SourceRevision != snapshot.identity.SourceRevision ||
						dependency.ContextRevision != snapshot.identity.ContextRevision {
						return platform.NewFailure(platform.CodeRevisionConflict, errors.New("published dependency differs from source"))
					}
					target, err := o.authorizationTarget(snapshot, dependency.TargetPosition)
					if err != nil {
						return err
					}
					target.AgeAnchor = snapshot.observedAt
					if err := authz.Authorize(authority, "message.read", dependency.Groups, target, now); err != nil {
						return err
					}
				}
			}
			if step.Branch != nil {
				if err := authorizeAll(step.Branch.Then); err != nil {
					return err
				}
				if err := authorizeAll(step.Branch.Else); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return authorizeAll(item.Definition.Steps)
}

// ApplyProcessingStatus rechecks the source identity, Message revision and configuration revision
// of binding and derives the Transmission's processing status from its current candidates (see
// refreshProcessingStatus). Staleness is a revision conflict; a missing Message is authz.ErrNotFound.
func (o *ProcessingSourceOwner) ApplyProcessingStatus(ctx context.Context, tx platform.TransactionContext,
	binding processing.PublicationBinding,
) error {
	snapshot, err := o.readSource(ctx, tx, binding.Source.TransmissionID)
	if err != nil {
		return err
	}
	if !snapshot.targetsCurrent || snapshot.identity != binding.Source || snapshot.message != binding.MessageID {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("publication source changed before the status refresh"))
	}
	executor, err := o.store.primary.Resolve(tx)
	if err != nil {
		return err
	}
	var messageRevision int64
	if err := executor.QueryRowContext(ctx, `SELECT revision FROM messages WHERE id=$1 FOR UPDATE`,
		binding.MessageID).Scan(&messageRevision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return authz.ErrNotFound
		}
		return err
	}
	if messageRevision != binding.SourceFence {
		return platform.NewFailure(platform.CodeRevisionConflict, errors.New("publication Message revision changed"))
	}
	configurationRevision, err := o.repository.LockRevision(ctx, tx)
	if err != nil {
		return err
	}
	workflows, err := o.selectWorkflows(ctx, tx, snapshot)
	if err != nil {
		return err
	}
	candidates, err := o.readCandidates(ctx, tx, binding.Source.TransmissionID)
	if err != nil {
		return err
	}
	return o.refreshProcessingStatus(ctx, executor, snapshot, configurationRevision, workflows, candidates)
}

// refreshProcessingStatus derives the Transmission's processing status from the candidates of its
// currently selected Workflows: pending while any is pending or running, failed when none is
// pending and at least one failed, completed when all have completed. Candidates of bindings that
// are no longer current are ignored, and a Transmission without a current candidate keeps its status.
func (o *ProcessingSourceOwner) refreshProcessingStatus(ctx context.Context, executor postgres.Executor,
	snapshot sourceSnapshot, configurationRevision int64, workflows []selectedWorkflow, candidates []sourceCandidate,
) error {
	status, decided := processingStatus(expectedBindings(snapshot, configurationRevision, workflows), candidates)
	if !decided {
		return nil
	}
	_, err := executor.ExecContext(ctx, `UPDATE transmissions SET processing_status=$2
		WHERE id=$1 AND processing_status<>$2`, snapshot.transmission, status)
	return err
}

// processingStatus folds the states of the current candidates into one Transmission status; the
// bool is false when no current candidate exists.
func processingStatus(expected map[uuid.UUID]processing.CandidateBinding, candidates []sourceCandidate) (string, bool) {
	var current, pending, failed bool
	for _, candidate := range candidates {
		binding, active := expected[candidate.binding.WorkflowID]
		if !active || !sameKey(candidate.binding, binding) || candidate.state == "superseded" {
			continue
		}
		current = true
		switch candidate.state {
		case "pending", "running":
			pending = true
		case "failed":
			failed = true
		}
	}
	switch {
	case !current:
		return "", false
	case pending:
		return "pending", true
	case failed:
		return "failed", true
	default:
		return "completed", true
	}
}

// authorizationTarget derives the authorization target of one dependency: unassigned or targetless
// evidence belongs to the Radio network's Agency; an assigned target to its Unit's Agency.
func (o *ProcessingSourceOwner) authorizationTarget(snapshot sourceSnapshot, position *uint32) (authz.Target, error) {
	target := authz.Target{AgencyID: snapshot.networkAgency, WithoutUnit: true}
	if position == nil {
		return target, nil
	}
	if *position >= uint32(len(snapshot.matched.Resolutions)) {
		return authz.Target{}, platform.NewFailure(platform.CodeInvalidInput, errors.New("dependency target position is outside source"))
	}
	target.WithoutUnit = false
	resolution := snapshot.matched.Resolutions[*position]
	if resolution.UnitID == nil {
		target.WithoutUnit = true
		return target, nil
	}
	unit, ok := snapshot.matched.UnitContext[*resolution.UnitID]
	if !ok || unit.ID != *resolution.UnitID || unit.Deleted {
		return authz.Target{}, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("current Unit context unavailable"))
	}
	target.AgencyID = unit.AgencyID
	target.UnitID = unit.ID
	target.UnitCollections = slices.Clone(unit.CollectionIDs)
	return target, nil
}

func (o *ProcessingSourceOwner) checkDefinitionInstances(ctx context.Context, tx platform.TransactionContext,
	definition processing.Definition,
) error {
	var visit func([]processing.Step) error
	visit = func(steps []processing.Step) error {
		for _, step := range steps {
			if step.Installed != nil {
				configuration, err := o.repository.ReadInstance(ctx, tx, step.Installed.InstanceID,
					step.Installed.ConfigurationRevision)
				if err != nil {
					return err
				}
				if !configuration.Enabled {
					return platform.NewFailure(platform.CodeRevisionConflict, errors.New("Processing instance is disabled"))
				}
				if _, err := o.principals.ResolvePrincipalAuthority(ctx, tx, configuration.ExecutionPrincipalID); err != nil {
					return err
				}
				artifact, err := o.artifacts.LoadInstalled(ctx, configuration)
				if err != nil {
					return err
				}
				if artifact == nil {
					return platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("Processing artifact owner returned no artifact"))
				}
				if err := artifact.Close(context.WithoutCancel(ctx)); err != nil {
					return err
				}
			}
			if step.Branch != nil {
				if err := visit(step.Branch.Then); err != nil {
					return err
				}
				if err := visit(step.Branch.Else); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return visit(definition.Steps)
}
