package borrowed_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/commands"
	"github.com/gopager/gopager/internal/events"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/management"
)

type instanceInspectionFixture struct {
	before  func()
	calls   int
	changes []commands.Result
}

func (i *instanceInspectionFixture) InstanceChanged(id uuid.UUID, revision int64) {
	i.changes = append(i.changes, commands.Result{ID: id, Revision: revision})
}

func (i *instanceInspectionFixture) InspectInstance(_ context.Context, value processing.InstanceConfiguration) error {
	i.calls++
	if i.before != nil {
		i.before()
	}
	// Deliberate mutation proves inspection cannot substitute the persisted configuration.
	if len(value.Configuration.Value) > 0 {
		value.Configuration.Value[0] = '['
	}
	if value.Preview != nil {
		value.Preview.Pin.Digest = strings.Repeat("b", 64)
	}
	return nil
}

type invalidInstanceFactFixture struct {
	events.ProcessingInstanceAppender
}

func (f invalidInstanceFactFixture) AppendProcessingInstance(ctx context.Context, tx platform.TransactionContext, value events.ProcessingInstanceFact) (uuid.UUID, error) {
	value.Revision = 0
	return f.ProcessingInstanceAppender.AppendProcessingInstance(ctx, tx, value)
}

func TestProcessingInstanceManagementFreshAuthorityReceiptsAndAtomicFacts(t *testing.T) {
	f := newMatcherFixture(t)
	base, capture := newProcessingManagementFixture(t, f, &processingPreparationFixture{}, f.facts)
	repository, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	inspector := &instanceInspectionFixture{}
	service, err := management.NewInstances(base, repository, inspector, f.facts, inspector)
	if err != nil {
		t.Fatal(err)
	}
	inspector.before = func() {
		if _, err := f.store.Resolve(capture.last); err == nil {
			t.Fatal("module inspection retained an open primary transaction")
		}
	}
	config, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
	if err != nil {
		t.Fatal(err)
	}
	enabled := true
	pin := processing.ArtifactPin{PluginID: "processor", Version: "1",
		CapabilityID: "convert", Digest: strings.Repeat("a", 64)}
	input := management.InstanceInput{Enabled: &enabled, ExecutionPrincipalID: f.principal,
		Main:    processing.ProvisionedArtifact{ManifestPath: "main.json", Pin: pin},
		Preview: &processing.ProvisionedArtifact{ManifestPath: "preview.json", Pin: pin}, Configuration: config}
	command := uuid.New()
	first, err := service.SaveInstance(t.Context(), f.proof, command, uuid.Nil, 0, input, checkedMutation)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.SaveInstance(t.Context(), f.proof, command, uuid.Nil, 0, input, checkedMutation)
	if err != nil || replayed != first || inspector.calls != 1 || len(inspector.changes) != 1 {
		t.Fatalf("replayed creation inspected/duplicated state: %+v calls=%d err=%v", replayed, inspector.calls, err)
	}
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		value, err := repository.ReadInstance(tx.Context(), tx, first.ID, 1)
		if err != nil {
			return err
		}
		if string(value.Configuration.Value) != "{}" || value.Preview.Pin != pin {
			t.Fatal("inspector substituted immutable configuration")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveInstance(t.Context(), f.proof, uuid.New(), first.ID, 1, input, nil); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("missing mutation participant accepted: %v", err)
	}
	inspector.before = func() {
		replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"rule.read"}, Scope: authz.Scope{Kind: "all"}}})
	}
	if _, err := service.SaveInstance(t.Context(), f.proof, uuid.New(), first.ID, 1, input, checkedMutation); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("revoked plugin configuration rights survived inspection: %v", err)
	}
	replaceManagementGrants(t, f, []authz.Grant{{Actions: []string{"plugin.configure"}, Scope: authz.Scope{Kind: "all"}}})
	inspector.before = nil
	failing, err := management.NewInstances(base, repository, inspector, invalidInstanceFactFixture{f.facts}, inspector)
	if err != nil {
		t.Fatal(err)
	}
	updateCommand := uuid.New()
	if _, err := failing.SaveInstance(t.Context(), f.proof, updateCommand, first.ID, 1, input, checkedMutation); err == nil {
		t.Fatal("invalid final fact did not abort instance update")
	}
	if len(inspector.changes) != 1 {
		t.Fatal("rolled-back instance update notified the active runtime")
	}
	updated, err := service.SaveInstance(t.Context(), f.proof, updateCommand, first.ID, 1, input, checkedMutation)
	if err != nil || updated.Revision != 2 || updated.ID != first.ID || len(inspector.changes) != 2 {
		t.Fatalf("failed fact left pointer/receipt behind: %+v err=%v", updated, err)
	}
	input.ExecutionPrincipalID = uuid.New()
	if _, err := service.SaveInstance(t.Context(), f.proof, uuid.New(), first.ID, 2, input, checkedMutation); err == nil {
		t.Fatal("missing execution Principal accepted")
	}
}
