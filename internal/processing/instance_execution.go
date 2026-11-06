package processing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"slices"
	"sync"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

// ExecutionTransactions borrows the platform transaction boundary without importing identity services.
type ExecutionTransactions interface {
	WithinTransaction(context.Context, platform.TxFunc) error
}

// ExecutionPrincipalResolver supplies current configured authority, never a transport impersonation proof.
type ExecutionPrincipalResolver interface {
	ResolvePrincipalAuthority(context.Context, platform.TransactionContext, uuid.UUID) (authz.Authority, error)
}

// PreviewAuthority resolves the editor's actual current proof inside the invocation transaction.
type PreviewAuthority interface {
	ResolvePreviewAuthority(context.Context, platform.TransactionContext) (authz.Authority, error)
}

// ExecutionBinding identifies one server-created run; PreviewEditor is never the execution Principal.
type ExecutionBinding struct {
	WorkflowID          uuid.UUID
	DefinitionVersionID uuid.UUID
	RunID               uuid.UUID
	Source              SourceIdentity
	PreviewEditor       PreviewAuthority
}

// InvocationSourceOwner checks current source/configuration and scoped data authority under primary locks.
// editor=true additionally requires workflow.preview and all stored sample reads for the current editor.
// Synthetic preview must be explicitly identified by this owner, not inferred from arbitrary source IDs.
type InvocationSourceOwner interface {
	CheckInstalledSource(context.Context, platform.TransactionContext, ExecutionBinding,
		authz.Authority, InvocationContext, []Dependency, bool) error
}

// InstanceExecutions owns rooted preparation and cancellation of active, per-run artifact lifetimes.
// Shared root/host lifetime belongs to application startup/shutdown, not individual runs.
type InstanceExecutions struct {
	host         *PluginHost
	root         *os.Root
	transactions ExecutionTransactions
	repository   InstanceRepository
	resolver     ExecutionPrincipalResolver
	mu           sync.Mutex
	closed       bool
	runs         map[*InstanceExecution]map[uuid.UUID]int64
	changed      chan struct{}
	closeErrors  []error
}

// NewInstanceExecutions requires real instance and current-Principal participants.
// A missing artifact root keeps built-in runs available but rejects installed preparation.
func NewInstanceExecutions(host *PluginHost, root *os.Root, transactions ExecutionTransactions,
	repository InstanceRepository, resolver ExecutionPrincipalResolver) (*InstanceExecutions, error) {
	if host == nil || transactions == nil || repository == nil || resolver == nil {
		return nil, invalid(errors.New("missing installed execution dependency"))
	}
	return &InstanceExecutions{host: host, root: root, transactions: transactions, repository: repository,
		resolver: resolver, runs: make(map[*InstanceExecution]map[uuid.UUID]int64), changed: make(chan struct{})}, nil
}

// Begin allocates an isolated run; callers evaluate with Context and close after all dependent
// work joins. The run's context is independent of ctx, which only has to be live at this call:
// a run begun inside a primary transaction outlives that transaction and ends with Close, with
// InstanceChanged for its instance, or with the manager's Close.
func (m *InstanceExecutions) Begin(ctx context.Context, binding ExecutionBinding, source InvocationSourceOwner) (*InstanceExecution, error) {
	if ctx == nil || source == nil || binding.WorkflowID == uuid.Nil || binding.DefinitionVersionID == uuid.Nil ||
		binding.RunID == uuid.Nil || binding.Source.TransmissionID == uuid.Nil || binding.Source.NetworkID == uuid.Nil ||
		binding.Source.SourceRevision <= 0 || binding.Source.ContextRevision <= 0 || binding.Source.OriginalTargetCount > MaxArguments {
		return nil, invalid(errors.New("invalid installed execution binding"))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, capabilityUnavailable(errors.New("installed execution manager closed"))
	}
	runCtx, cancel := context.WithCancel(context.Background())
	run := &InstanceExecution{manager: m, binding: binding, source: source, ctx: runCtx, cancel: cancel,
		prepared: make(map[uuid.UUID]preparedInstance)}
	m.runs[run] = make(map[uuid.UUID]int64)
	return run, nil
}

// InstanceChanged cancels affected old-revision runs only after a successful configuration commit.
// Current primary checks remain mandatory; this best-effort signal is not execution authority.
// It also stops the instance's older plugin processes so none outlives its configuration.
func (m *InstanceExecutions) InstanceChanged(id uuid.UUID, revision int64) {
	m.mu.Lock()
	for run, instances := range m.runs {
		if expected, exists := instances[id]; exists && expected < revision {
			run.cancel()
		}
	}
	m.mu.Unlock()
	m.host.InstanceChanged(id, revision)
}

// Stop prevents new runs and cancels existing ones; callers join workers before closing host/root.
func (m *InstanceExecutions) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	for run := range m.runs {
		run.cancel()
	}
}

// Close stops admission and waits for every run to close its prepared artifacts.
// The caller remains responsible for closing the shared host and artifact root afterward.
func (m *InstanceExecutions) Close(ctx context.Context) error {
	if m == nil || ctx == nil {
		return invalid(errors.New("missing installed execution manager/context"))
	}
	m.Stop()
	for {
		m.mu.Lock()
		if len(m.runs) == 0 {
			failure := errors.Join(m.closeErrors...)
			m.mu.Unlock()
			return failure
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

type preparedInstance struct {
	revision int64
	main     *InstalledArtifact
	failure  error
}

// InstanceExecution implements preparation and current invocation authorization for one bound run.
type InstanceExecution struct {
	manager  *InstanceExecutions
	binding  ExecutionBinding
	source   InvocationSourceOwner
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	prepared map[uuid.UUID]preparedInstance
}

// Context propagates run invalidation into compilation and all actual guest calls.
func (r *InstanceExecution) Context() context.Context { return r.ctx }

// PrepareInstalled returns the exact current main artifact, never a fallback for missing bytes.
func (r *InstanceExecution) PrepareInstalled(ctx context.Context, id uuid.UUID, revision int64) (*InstalledArtifact, error) {
	prepared, err := r.prepare(ctx, id, revision)
	return prepared.main, err
}

// prepare serializes per-run preparation and keeps every inspected compilation until the run drains.
func (r *InstanceExecution) prepare(ctx context.Context, id uuid.UUID, revision int64) (result preparedInstance, failure error) {
	if id == uuid.Nil || revision <= 0 || ctx == nil {
		return preparedInstance{}, invalid(errors.New("invalid installed preparation reference"))
	}
	if r.manager.root == nil {
		return preparedInstance{}, capabilityUnavailable(errors.New("Processing artifact root unavailable"))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return preparedInstance{}, capabilityUnavailable(errors.New("installed run closed"))
	}
	if err := r.ctx.Err(); err != nil {
		return preparedInstance{}, err
	}
	if err := ctx.Err(); err != nil {
		return preparedInstance{}, err
	}
	if prepared, exists := r.prepared[id]; exists {
		if prepared.revision != revision {
			return preparedInstance{}, capabilityUnavailable(errors.New("mixed instance revisions in one run"))
		}
		if prepared.failure != nil {
			return preparedInstance{}, prepared.failure
		}
		if err := r.manager.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
			_, err := r.current(tx.Context(), tx, id, revision)
			return err
		}); err != nil {
			return preparedInstance{}, err
		}
		return prepared, nil
	}
	r.manager.mu.Lock()
	r.manager.runs[r][id] = revision
	r.manager.mu.Unlock()
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	defer cancel()
	var configuration InstanceConfiguration
	if err := r.manager.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		var err error
		configuration, err = r.current(tx.Context(), tx, id, revision)
		return err
	}); err != nil {
		return preparedInstance{}, err
	}
	main, err := r.manager.host.LoadInstalledFor(ctx, r.manager.root, configuration.Main.ManifestPath,
		configuration.Main.Pin, configuration.ID, configuration.Revision, r.invocationMode())
	if err != nil {
		return preparedInstance{}, err
	}
	// Retain partially prepared modules too, so Close surfaces cleanup errors on every failure path.
	prepared := preparedInstance{revision: revision, main: main}
	r.prepared[id] = prepared
	defer func() {
		if failure != nil {
			prepared.failure = failure
			r.prepared[id] = prepared
		}
	}()
	manifest, err := main.Manifest()
	if err != nil {
		return preparedInstance{}, err
	}
	if hasSecretPolicy(manifest.ConfigurationPolicy) {
		return preparedInstance{}, capabilityUnavailable(errors.New("instance Secret owner unavailable"))
	}
	if err := validateAdministrativeConfiguration(manifest.ConfigurationSchema,
		manifest.ConfigurationPolicy, configuration.Configuration); err != nil {
		return preparedInstance{}, err
	}
	if err := r.manager.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		_, err := r.current(tx.Context(), tx, id, revision)
		return err
	}); err != nil {
		return preparedInstance{}, err
	}
	return prepared, nil
}

// invocationMode is preview for a run bound to a preview editor and live otherwise; it selects
// the pooled plugin process as well as the envelope mode.
func (r *InstanceExecution) invocationMode() string {
	if r.binding.PreviewEditor != nil {
		return InvocationModePreview
	}
	return InvocationModeLive
}

// current reads only the exact current enabled revision; prior immutable snapshots are not permission.
func (r *InstanceExecution) current(ctx context.Context, tx platform.TransactionContext, id uuid.UUID, revision int64) (InstanceConfiguration, error) {
	configuration, err := r.manager.repository.ReadInstance(ctx, tx, id, revision)
	if err != nil {
		return InstanceConfiguration{}, err
	}
	if !configuration.Enabled {
		return InstanceConfiguration{}, capabilityUnavailable(errors.New("installed instance disabled"))
	}
	return configuration, nil
}

// AuthorizeInstalled resolves current execution authority and optional editor proof before each call.
// The primary transaction closes before the detached, pinned envelope can enter guest code.
func (r *InstanceExecution) AuthorizeInstalled(ctx context.Context, id uuid.UUID, revision int64,
	call InvocationContext, input map[string]FieldValue) (InvocationEnvelope, []Dependency, bool, error) {
	preview := r.binding.PreviewEditor != nil
	mode := r.invocationMode()
	if call.Source != r.binding.Source ||
		(call.TargetPosition != nil && *call.TargetPosition >= call.Source.OriginalTargetCount) {
		return InvocationEnvelope{}, nil, preview, invalid(errors.New("installed invocation source differs from bound run"))
	}
	artifact, err := r.PrepareInstalled(ctx, id, revision)
	if err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	manifest, err := artifact.Manifest()
	if err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	var detached map[string]FieldValue
	if err := Decode(encoded, &detached); err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	values := make(map[string]Value)
	var dependencies []Dependency
	for name, field := range detached {
		schema, declared := manifest.InputSchema[name]
		if !declared {
			return InvocationEnvelope{}, nil, preview, invalid(errors.New("undeclared installed input"))
		}
		if err := validateFieldValue(schema, field, false); err != nil {
			return InvocationEnvelope{}, nil, preview, err
		}
		if field.State == FieldPresent {
			values[name] = *field.Value
		}
		dependencies = append(dependencies, field.Dependencies...)
	}
	if err := validateValues(manifest.InputSchema, values); err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	dependencies, err = UnionDependencies(dependencies)
	if err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	var groups []string
	for _, dependency := range dependencies {
		if dependency.TransmissionID != call.Source.TransmissionID || dependency.NetworkID != call.Source.NetworkID ||
			dependency.SourceRevision != call.Source.SourceRevision || dependency.ContextRevision != call.Source.ContextRevision ||
			(dependency.TargetPosition != nil && *dependency.TargetPosition >= call.Source.OriginalTargetCount) {
			return InvocationEnvelope{}, nil, preview, invalid(errors.New("installed input dependency differs from source"))
		}
		groups = append(groups, dependency.Groups...)
	}
	slices.Sort(groups)
	groups = slices.Compact(groups)
	var envelope InvocationEnvelope
	err = r.manager.transactions.WithinTransaction(ctx, func(tx platform.TransactionContext) error {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		configuration, err := r.current(tx.Context(), tx, id, revision)
		if err != nil {
			return err
		}
		authority, err := r.manager.resolver.ResolvePrincipalAuthority(tx.Context(), tx, configuration.ExecutionPrincipalID)
		if err != nil {
			return err
		}
		if err := authz.AuthorizeFields(authority, groups); err != nil {
			return err
		}
		if err := r.authorizeSource(tx.Context(), tx, authority, call, dependencies, false); err != nil {
			return err
		}
		if preview {
			editor, err := r.binding.PreviewEditor.ResolvePreviewAuthority(tx.Context(), tx)
			if err != nil {
				return err
			}
			if err := authz.AuthorizeFields(editor, groups); err != nil {
				return err
			}
			if err := r.authorizeSource(tx.Context(), tx, editor, call, dependencies, true); err != nil {
				return err
			}
		}
		envelope = InvocationEnvelope{SchemaVersion: 2, InvocationID: uuid.New(), InstanceID: configuration.ID, WorkflowID: r.binding.WorkflowID,
			DefinitionVersionID: r.binding.DefinitionVersionID, RunID: r.binding.RunID,
			ExecutionPrincipalID: configuration.ExecutionPrincipalID, ConfigurationRevision: revision,
			ArtifactDigest: manifest.Artifact.Digest, CapabilityID: manifest.CapabilityID,
			Input: values, Configuration: configuration.Configuration, Mode: mode}
		return nil
	})
	if err != nil {
		return InvocationEnvelope{}, nil, preview, err
	}
	return envelope, nil, preview, nil
}

// authorizeSource detaches source metadata/classification without exposing an editor credential.
func (r *InstanceExecution) authorizeSource(ctx context.Context, tx platform.TransactionContext, authority authz.Authority,
	call InvocationContext, dependencies []Dependency, editor bool) error {
	binding := r.binding
	binding.PreviewEditor = nil
	if call.TargetPosition != nil {
		position := *call.TargetPosition
		call.TargetPosition = &position
	}
	check, err := UnionDependencies(dependencies)
	if err != nil {
		return err
	}
	return r.source.CheckInstalledSource(ctx, tx, binding, authority, call, check, editor)
}

// Close cancels the drained run and closes every prepared compilation, including partial preparation.
func (r *InstanceExecution) Close(ctx context.Context) error {
	if r == nil || ctx == nil || r.manager == nil {
		return invalid(errors.New("missing installed run/context"))
	}
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	var failure error
	for _, prepared := range r.prepared {
		failure = errors.Join(failure, prepared.main.Close(context.WithoutCancel(ctx)))
	}
	r.closed = true
	r.manager.mu.Lock()
	delete(r.manager.runs, r)
	if failure != nil && r.manager.closed {
		r.manager.closeErrors = append(r.manager.closeErrors, failure)
	}
	close(r.manager.changed)
	r.manager.changed = make(chan struct{})
	r.manager.mu.Unlock()
	return failure
}
