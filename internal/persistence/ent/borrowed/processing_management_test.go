package borrowed_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/management"
	"github.com/gopager/gopager/internal/units"
)

type processingPreparationFixture struct {
	before      func()
	substitute  bool
	unavailable bool
}

func (f *processingPreparationFixture) PrepareWorkflow(ctx context.Context, definition processing.Definition) (*processing.WorkflowProgram, error) {
	if f.before != nil {
		f.before()
	}
	if f.unavailable {
		return nil, platform.NewFailure(platform.CodeAuthorityUnavailable, errors.New("fixture source declarations unavailable"))
	}
	if f.substitute {
		no, err := processing.Literal(processing.BooleanValue, false)
		if err != nil {
			return nil, err
		}
		definition.Selector.Value = &no
	}
	return processing.CompileWorkflow(ctx, definition, processingDeclarationsFixture{}, processingOutputsFixture{}, nil)
}

type processingTransactionCapture struct {
	processing.ConfigurationRepository
	last platform.TransactionContext
}

func (r *processingTransactionCapture) LockRevision(ctx context.Context, tx platform.TransactionContext) (int64, error) {
	r.last = tx
	return r.ConfigurationRepository.LockRevision(ctx, tx)
}

type invalidProcessingFactFixture struct{ events.ProcessingAppender }

func (f invalidProcessingFactFixture) AppendProcessingConfiguration(ctx context.Context, tx platform.TransactionContext, fact events.ProcessingConfigurationFact) (uuid.UUID, error) {
	fact.Revision = 0
	return f.ProcessingAppender.AppendProcessingConfiguration(ctx, tx, fact)
}

func newProcessingManagementFixture(t *testing.T, f matcherFixture, preparer *processingPreparationFixture,
	facts events.ProcessingAppender) (*management.Service, *processingTransactionCapture) {
	t.Helper()
	store, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	capture := &processingTransactionCapture{ConfigurationRepository: store}
	service, err := management.New(f.store, f.resolver, f.store, capture, f.owners,
		preparer, f.receipts, facts)
	if err != nil {
		t.Fatal(err)
	}
	return service, capture
}

func processingMetadataFixture(name string, scope processing.WorkflowScope) management.MetadataInput {
	enabled := true
	return management.MetadataInput{Name: name, Scope: scope, Enabled: &enabled}
}

func TestProcessingManagementVersionsActivationOnlyAndRetainedHistory(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	preparer := &processingPreparationFixture{}
	service, capture := newProcessingManagementFixture(t, f, preparer, f.facts)
	preparer.before = func() {
		if _, err := f.store.Resolve(capture.last); err == nil {
			t.Fatal("Workflow compiler retained a live primary transaction")
		}
	}
	input := processingMetadataFixture(strings.Repeat("n", 300), f.agencyScope())
	command := uuid.New()
	created, err := service.CreateWorkflow(t.Context(), f.proof, command, input, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.CreateWorkflow(t.Context(), f.proof, command, input, checkedMutation)
	if err != nil || !reflect.DeepEqual(replayed, created) {
		t.Fatalf("create replay changed identity: %+v %v", replayed, err)
	}
	definition := preparedProcessingDefinition(t)
	saveCommand := uuid.New()
	first, err := service.SaveVersion(t.Context(), f.proof, saveCommand, created.ID, 1, definition, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err = service.SaveVersion(t.Context(), f.proof, saveCommand, created.ID, 1, definition, checkedMutation)
	if err != nil || !reflect.DeepEqual(replayed, first) {
		t.Fatalf("save replay changed version/revision: %+v %v", replayed, err)
	}
	metadata, err := service.ReadWorkflow(t.Context(), f.proof, created.ID)
	if err != nil || metadata.Revision != 2 || metadata.ActiveVersionID != nil || !metadata.Enabled {
		t.Fatalf("save changed activation/enabled: %+v %v", metadata, err)
	}
	second, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 2, definition, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Scope: authz.Scope{Kind: "all"}, Actions: []string{"workflow.activate"}}})
	if _, err := service.ReadVersion(t.Context(), f.proof, created.ID, first.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("activate authority became public definition read: %v", err)
	}
	selected, err := service.ActivateVersion(t.Context(), f.proof, created.ID, second.ID, 3, checkedMutation)
	if err != nil || selected.Revision != 4 {
		t.Fatalf("activate alone could not select saved version: %+v %v", selected, err)
	}
	selected, err = service.ActivateVersion(t.Context(), f.proof, created.ID, first.ID, 4, checkedMutation)
	if err != nil || selected.Revision != 5 {
		t.Fatalf("select previous failed: %+v %v", selected, err)
	}
	if _, err := service.ActivateVersion(t.Context(), f.proof, created.ID, second.ID, 4, checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("stale activation succeeded: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Scope: authz.Scope{Kind: "all"}, Actions: authz.Actions()}})
	metadata, err = service.ReadWorkflow(t.Context(), f.proof, created.ID)
	if err != nil || metadata.ActiveVersionID == nil || *metadata.ActiveVersionID != first.ID {
		t.Fatalf("previous selection did not persist: %+v %v", metadata, err)
	}
	disabled := false
	input.Enabled = &disabled
	if _, err := service.UpdateWorkflow(t.Context(), f.proof, created.ID, 5, input, checkedMutation); err != nil {
		t.Fatal(err)
	}
	metadata, err = service.ReadWorkflow(t.Context(), f.proof, created.ID)
	if err != nil || metadata.Enabled || metadata.ActiveVersionID == nil || *metadata.ActiveVersionID != first.ID {
		t.Fatalf("disable silently changed chosen version: %+v %v", metadata, err)
	}
	if _, err := service.DeleteWorkflow(t.Context(), f.proof, created.ID, 6, checkedMutation); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReadWorkflow(t.Context(), f.proof, created.ID); !errors.Is(err, authz.ErrNotFound) {
		t.Fatalf("deleted Workflow remained selectable: %v", err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var versions, receipts, facts int
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM processing_workflow_versions WHERE workflow_id=$1),
			(SELECT count(*) FROM command_receipts WHERE operation IN ('rule.create','rule.version.save')),
			(SELECT count(*) FROM domain_events WHERE envelope->'aggregateRef'->>'id'=$1::text)`, created.ID).
			Scan(&versions, &receipts, &facts); err != nil {
			return err
		}
		if versions != 2 || receipts != 3 || facts != 7 {
			t.Fatalf("history/replays/atomic changes wrong: versions=%d receipts=%d facts=%d", versions, receipts, facts)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessingManagementRechecksAuthorityAfterPreparationAndRejectsInvalidFact(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	preparer := &processingPreparationFixture{}
	service, _ := newProcessingManagementFixture(t, f, preparer, f.facts)
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
		processingMetadataFixture("protected", f.agencyScope()), checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	preparer.before = func() { replaceManagementGrants(t, f, nil) }
	if _, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1,
		preparedProcessingDefinition(t), checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("revoked editor saved a prepared definition: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Scope: authz.Scope{Kind: "all"}, Actions: authz.Actions()}})
	preparer.before = nil
	failing, _ := newProcessingManagementFixture(t, f, preparer, invalidProcessingFactFixture{f.facts})
	if _, err := failing.UpdateWorkflow(t.Context(), f.proof, created.ID, 1,
		processingMetadataFixture("must roll back", f.agencyScope()), checkedMutation); err == nil {
		t.Fatal("invalid final append was accepted")
	}
	metadata, err := service.ReadWorkflow(t.Context(), f.proof, created.ID)
	if err != nil || metadata.Revision != 1 || metadata.Name != "protected" {
		t.Fatalf("invalid append leaked metadata/revision: %+v %v", metadata, err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		executor, err := f.store.Resolve(tx)
		if err != nil {
			return err
		}
		var versions, audit, facts int
		var revision int64
		if err := executor.QueryRowContext(tx.Context(), `SELECT
			(SELECT count(*) FROM processing_workflow_versions WHERE workflow_id=$1),
			(SELECT count(*) FROM identity_audit WHERE target_id=$1),
			(SELECT count(*) FROM domain_events WHERE envelope->'aggregateRef'->>'id'=$1::text),
			(SELECT revision FROM processing_control WHERE id=1)`, created.ID).
			Scan(&versions, &audit, &facts, &revision); err != nil {
			return err
		}
		if versions != 0 || audit != 1 || facts != 1 || revision != 2 {
			t.Fatalf("failed preparation/append leaked participants: versions=%d audit=%d facts=%d revision=%d",
				versions, audit, facts, revision)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProcessingManagementPreparationRejectsChangedConfigurationSubstitutionAndMissingOwner(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	preparer := &processingPreparationFixture{}
	service, _ := newProcessingManagementFixture(t, f, preparer, f.facts)
	input := processingMetadataFixture("checked", f.agencyScope())
	created, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	definition := preparedProcessingDefinition(t)
	preparer.before = func() {
		if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(),
			processingMetadataFixture("concurrent configuration", f.agencyScope()), checkedMutation); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1, definition,
		checkedMutation); !errors.Is(err, authz.ErrRevisionConflict) {
		t.Fatalf("changed shared configuration accepted stale preparation: %v", err)
	}
	preparer.before, preparer.substitute = nil, true
	if _, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1, definition,
		checkedMutation); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("preparation substitution accepted: %v", err)
	}
	preparer.substitute, preparer.unavailable = false, true
	if _, err := service.SaveVersion(t.Context(), f.proof, uuid.New(), created.ID, 1, definition,
		checkedMutation); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("missing declaration owner became successful: %v", err)
	}
	page, err := service.ListVersions(t.Context(), f.proof, created.ID, processing.PageInput{})
	if err != nil || page.Total != 0 || len(page.Items) != 0 {
		t.Fatalf("failed preparations persisted a version: %+v %v", page, err)
	}
}

// TestProcessingManagementUnitScopeFollowsTheUnitAgencyAndExplicitInput expects a Unit-scoped
// Workflow to be authorized through the Unit's own Agency, not through the Agencies of the
// Networks whose rules alert the Unit, and required metadata to have no implicit defaults.
func TestProcessingManagementUnitScopeFollowsTheUnitAgencyAndExplicitInput(t *testing.T) {
	t.Parallel()
	f := newMatcherFixture(t)
	preparer := &processingPreparationFixture{}
	service, _ := newProcessingManagementFixture(t, f, preparer, f.facts)
	unit, err := f.units.CreateUnit(t.Context(), f.proof, uuid.New(), units.UnitInput{AgencyID: f.agencies[1], Name: "shared"}, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	for _, network := range f.networks[:2] {
		if _, err := f.rules.CreateRule(t.Context(), f.proof, uuid.New(), ruleInput(network, unit.ID, 1, 1, 0), checkedMutation); err != nil {
			t.Fatal(err)
		}
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"workflow.create"},
		Scope: authz.Scope{Kind: "agency", References: []string{f.agencies[0].String()}}}})
	input := processingMetadataFixture("shared unit workflow", processing.WorkflowScope{
		Agencies:  []uuid.UUID{f.agencies[1]},
		Narrowing: &processing.WorkflowNarrowing{Kind: "unit", References: []uuid.UUID{unit.ID}}})
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("a rule on an own-Agency Network authorized a foreign-Agency Unit scope: %v", err)
	}
	input.Scope = workflowNetworkScope(f, f.networks[0])
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input, checkedMutation); err != nil {
		t.Fatal(err)
	}
	input.Enabled = nil
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input, checkedMutation); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("missing enabled received an implicit default: %v", err)
	}
	input = processingMetadataFixture("missing network", workflowNetworkScope(f, uuid.New()))
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input, checkedMutation); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("missing scope became installation scope: %v", err)
	}
	input = processingMetadataFixture("transport", workflowNetworkScope(f, f.networks[0]))
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input, nil); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("mutation omitted Origin/CSRF participant: %v", err)
	}
	denied := errors.New("fixture transport denied")
	if _, err := service.CreateWorkflow(t.Context(), f.proof, uuid.New(), input,
		func(identity.Authentication) error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("transport failure was swallowed: %v", err)
	}
}
