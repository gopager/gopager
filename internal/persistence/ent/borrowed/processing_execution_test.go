package borrowed_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"entgo.io/ent/entql"
	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/ent"
	"github.com/gopager/gopager/internal/persistence/ent/borrowed"
	"github.com/gopager/gopager/internal/persistence/ent/fieldpermission"
	"github.com/gopager/gopager/internal/persistence/ent/grant"
	"github.com/gopager/gopager/internal/persistence/ent/guard"
	"github.com/gopager/gopager/internal/persistence/ent/processingstore"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
	"github.com/gopager/gopager/internal/processing/execution"
	"github.com/gopager/gopager/internal/protocols"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

// installedSourceFixture provides explicit synthetic source scope, not production AP-06 originals.
type installedSourceFixture struct {
	f         matcherFixture
	source    processing.SourceIdentity
	last      platform.TransactionContext
	deny      bool
	principal uuid.UUID
}

func (s *installedSourceFixture) CheckInstalledSource(ctx context.Context, tx platform.TransactionContext,
	binding processing.ExecutionBinding, authority authz.Authority, call processing.InvocationContext,
	dependencies []processing.Dependency, editor bool) error {
	s.last = tx
	if binding.Source != s.source || call.Source != s.source || s.deny || binding.PreviewEditor != nil {
		return authz.ErrRevisionConflict
	}
	if !editor && authority.PrincipalID != s.principal.String() {
		return authz.ErrForbidden
	}
	now, err := s.f.store.Now(ctx, tx)
	if err != nil {
		return err
	}
	target := authz.Target{NetworkID: s.source.NetworkID, WithoutUnit: true, AgeAnchor: now}
	if err := authz.Authorize(authority, "message.read", nil, target, now); err != nil {
		return err
	}
	if err := authz.Authorize(authority, "without-unit.read", nil, target, now); err != nil {
		return err
	}
	if editor {
		return authz.Authorize(authority, "rule.preview", nil, target, now)
	}
	return nil
}

// seedExecutionPrincipal creates an explicit fixture identity without borrowing the editor's grants.
func seedExecutionPrincipal(t *testing.T, f matcherFixture) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := f.owners.identity.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(f.store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Create: map[string]bool{"Account": true, "Grant": true, "FieldPermission": true}})
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		if err := createAccount(scope, client, id, id.String(), now); err != nil {
			return err
		}
		if err := client.Grant.Create().SetSubjectID(id).SetSubjectKind(grant.SubjectKindPrincipal).
			SetActions([]string{"message.read", "without-unit.read"}).SetScope(authz.Scope{Kind: "all"}).Exec(scope); err != nil {
			return err
		}
		return client.FieldPermission.Create().SetSubjectID(id).SetSubjectKind(fieldpermission.SubjectKindPrincipal).
			SetFieldNames([]string{"address"}).Exec(scope)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// setExecutionFields mutates actual current fixture field permissions under the normal shared lock.
func setExecutionFields(t *testing.T, f matcherFixture, fields []string) {
	t.Helper()
	if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
		if _, err := f.owners.identity.LockAccess(tx.Context(), tx); err != nil {
			return err
		}
		driver, err := borrowed.New(f.store, tx)
		if err != nil {
			return err
		}
		client := ent.NewClient(ent.Driver(driver))
		scope := guard.Bind(tx.Context(), guard.Access{Current: driver.Current,
			Create: map[string]bool{"FieldPermission": true},
			Predicates: map[string]entql.P{"FieldPermission": entql.And(
				entql.FieldEQ("subject_id", f.principal), entql.FieldEQ("subject_kind", "principal"))}})
		if _, err := client.FieldPermission.Delete().Exec(scope); err != nil {
			return err
		}
		if len(fields) != 0 {
			if err := client.FieldPermission.Create().SetSubjectID(f.principal).SetSubjectKind(fieldpermission.SubjectKindPrincipal).
				SetFieldNames(fields).Exec(scope); err != nil {
				return err
			}
		}
		now, err := f.store.Now(tx.Context(), tx)
		if err != nil {
			return err
		}
		return f.owners.identity.RecordConfigurationChange(tx.Context(), tx, f.principal, f.principal, "grant.manage", now)
	}); err != nil {
		t.Fatal(err)
	}
}

// provisionExecutionFixture writes the supplied command/preview plugin executables built for this test process.
func provisionExecutionFixture(t *testing.T, root *os.Root, preview bool) processing.ProvisionedArtifact {
	t.Helper()
	name := "command"
	if preview {
		name = "preview"
	}
	data := pluginfixture.Binary(t, name)
	if err := root.WriteFile(name, data, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	noSecret := false
	manifest := processing.InstalledManifest{SchemaVersion: 2, SDKMajor: 2, PluginID: "test-command",
		Version: "1", CapabilityID: "test", Pure: true, PurePreview: true,
		Artifact: processing.ArtifactReference{Path: name, Digest: hex.EncodeToString(digest[:])},
		InputSchema: map[string]processing.FieldSchema{"mode": {Kind: processing.StringValue},
			"text": {Kind: processing.StringValue, Optional: true}},
		ConfigurationSchema: processing.FieldSchema{Kind: processing.ObjectValue, Properties: map[string]processing.FieldSchema{}},
		ConfigurationPolicy: processing.ConfigurationPolicy{Scope: "admin", Secret: &noSecret}}
	manifest.ResultSchema = manifest.InputSchema
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile(name+".json", encoded, 0600); err != nil {
		t.Fatal(err)
	}
	return processing.ProvisionedArtifact{ManifestPath: name + ".json", Pin: processing.ArtifactPin{
		PluginID: manifest.PluginID, Version: manifest.Version, CapabilityID: manifest.CapabilityID, Digest: manifest.Artifact.Digest}}
}

func TestProcessingExecutionCurrentAuthorityAndArtifactLifetime(t *testing.T) {
	f := newMatcherFixture(t)
	repository, err := processingstore.New(f.store)
	if err != nil {
		t.Fatal(err)
	}
	host := processing.NewPluginHost()
	t.Cleanup(func() {
		if err := host.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	for _, name := range []string{"main", "preview", "missing-preview", "invalid-preview", "source-denied",
		"global-hidden", "changed-revision", "principal-revoked", "editor-revoked", "stop"} {
		t.Run(name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := root.Close(); err != nil {
					t.Error(err)
				}
			})
			main := provisionExecutionFixture(t, root, false)
			preview := provisionExecutionFixture(t, root, true)
			manager, err := processing.NewInstanceExecutions(host, root, f.store, repository, f.resolver)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(manager.Stop)
			configuration, err := processing.Literal(processing.ObjectValue, map[string]processing.Value{})
			if err != nil {
				t.Fatal(err)
			}
			instance := processing.InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true,
				ExecutionPrincipalID: f.principal, CreatedBy: f.principal, Main: main, Preview: &preview,
				Configuration: configuration}
			if name == "preview" {
				instance.ExecutionPrincipalID = seedExecutionPrincipal(t, f)
			}
			if name == "missing-preview" {
				instance.Preview = nil
			}
			save := func(expected int64) {
				t.Helper()
				if err := f.store.WithinTransaction(t.Context(), func(tx platform.TransactionContext) error {
					authentication, err := f.resolver.Resolve(tx.Context(), tx, f.proof)
					if err != nil {
						return err
					}
					instance.CreatedAt, err = f.store.Now(tx.Context(), tx)
					if err != nil {
						return err
					}
					return repository.SaveInstance(tx.Context(), tx, authentication.Authority, instance, expected)
				}); err != nil {
					t.Fatal(err)
				}
			}
			save(0)
			sourceIdentity := processing.SourceIdentity{TransmissionID: uuid.New(), NetworkID: f.networks[0],
				SourceRevision: 1, ContextRevision: 1}
			sourceOwner := &installedSourceFixture{f: f, source: sourceIdentity, deny: name == "source-denied",
				principal: instance.ExecutionPrincipalID}
			binding := processing.ExecutionBinding{WorkflowID: uuid.New(), DefinitionVersionID: uuid.New(),
				RunID: uuid.New(), Source: sourceIdentity}
			if name == "preview" || name == "missing-preview" || name == "editor-revoked" {
				binding.PreviewEditor, err = execution.NewPreviewAuthority(f.resolver, f.proof)
				if err != nil {
					t.Fatal(err)
				}
			}
			run, err := manager.Begin(t.Context(), binding, sourceOwner)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := run.Close(context.Background()); err != nil {
					t.Error(err)
				}
			})
			if name == "invalid-preview" {
				if err := root.WriteFile("preview.json", []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
				for range 2 {
					if _, err := run.PrepareInstalled(run.Context(), instance.ID, 1); err == nil {
						t.Fatal("invalid configured preview or cached failed preparation accepted")
					}
				}
				return
			}
			flag, err := processing.Literal(processing.BooleanValue, true)
			if err != nil {
				t.Fatal(err)
			}
			text, err := processing.Literal(processing.StringValue, "CURRENT")
			if err != nil {
				t.Fatal(err)
			}
			group := "address"
			if name == "global-hidden" {
				group = "payload"
			}
			dependency := processing.Dependency{TransmissionID: sourceIdentity.TransmissionID,
				NetworkID: sourceIdentity.NetworkID, SourceRevision: 1, ContextRevision: 1, Groups: []string{group}}
			source, err := processing.CaptureSource(t.Context(), processing.SourceProjection{
				TransmissionID: sourceIdentity.TransmissionID, NetworkID: sourceIdentity.NetworkID,
				SourceRevision: 1, ContextRevision: 1, Protocol: protocols.POCSAG, Form: "text", ObservedAt: instance.CreatedAt,
				Fields: []processing.CapturedField{
					{Reference: processing.FieldRef{Namespace: "source", Path: []string{"flag"}},
						Schema: processing.FieldSchema{Kind: processing.BooleanValue},
						Field:  processing.FieldValue{State: processing.FieldPresent, Value: &flag, Dependencies: []processing.Dependency{dependency}}},
					{Reference: processing.FieldRef{Namespace: "source", Path: []string{"text"}},
						Schema: processing.FieldSchema{Kind: processing.StringValue},
						Field:  processing.FieldValue{State: processing.FieldPresent, Value: &text, Dependencies: []processing.Dependency{dependency}}},
				}})
			if err != nil {
				t.Fatal(err)
			}
			mode, err := processing.Literal(processing.StringValue, "ok")
			if err != nil {
				t.Fatal(err)
			}
			definition := preparedProcessingDefinition(t)
			schema := map[string]processing.FieldSchema{"mode": {Kind: processing.StringValue}, "text": {Kind: processing.StringValue, Optional: true}}
			input := processing.FieldRef{Namespace: "source", Path: []string{"text"}}
			output := processing.FieldRef{Namespace: "step", Path: []string{"module", "text"}}
			definition.Steps = []processing.Step{{ID: "module", Kind: "installed", OutputSchema: schema,
				Installed: &processing.InstalledStep{InstanceID: instance.ID, ConfigurationRevision: 1,
					Input: map[string]processing.Assignment{"mode": {Literal: &mode}, "text": {Field: &input}}}}}
			definition.OutputSchema["text"] = schema["text"]
			definition.Outputs["text"] = processing.Assignment{Field: &output}
			program, err := processing.CompileWorkflowInstalled(run.Context(), definition, source, nil, run)
			if err != nil {
				t.Fatal(err)
			}
			if name == "source-denied" || name == "global-hidden" || name == "missing-preview" {
				if result, err := program.EvaluateInstalled(run.Context(), source, run); err == nil || result.Selected {
					t.Fatalf("unavailable authority/preview returned result: %+v err=%v", result, err)
				}
				return
			}
			result, err := program.EvaluateInstalled(run.Context(), source, run)
			want := `"CURRENT"`
			if name == "preview" || name == "editor-revoked" {
				want = `"PREVIEW:CURRENT"`
			}
			if err != nil || result.Fields["text"].Value == nil || string(result.Fields["text"].Value.Value) != want {
				t.Fatalf("current actual guest result=%+v err=%v", result, err)
			}
			if _, err := f.store.Resolve(sourceOwner.last); err == nil {
				t.Fatal("guest invocation retained the primary authorization transaction")
			}
			switch name {
			case "main":
				setExecutionFields(t, f, nil)
				defer setExecutionFields(t, f, []string{"unit", "address"})
				if _, err := program.EvaluateInstalled(run.Context(), source, run); !errors.Is(err, authz.ErrForbidden) {
					t.Fatalf("revoked global field rights survived current call: %v", err)
				}
			case "changed-revision":
				instance.Revision = 2
				save(1)
				if _, err := program.EvaluateInstalled(run.Context(), source, run); err == nil {
					t.Fatal("old current instance revision remained executable without a notification")
				}
				manager.InstanceChanged(instance.ID, 2)
				if !errors.Is(run.Context().Err(), context.Canceled) {
					t.Fatal("committed revision did not cancel its old run")
				}
				newRun, err := manager.Begin(t.Context(), binding, sourceOwner)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := newRun.Close(context.Background()); err != nil {
						t.Error(err)
					}
				}()
				if _, err := newRun.PrepareInstalled(newRun.Context(), instance.ID, 2); err != nil {
					t.Fatal(err)
				}
				manager.InstanceChanged(instance.ID, 1)
				if newRun.Context().Err() != nil {
					t.Fatal("late old commit notification cancelled a newer run")
				}
			case "principal-revoked":
				replaceManagementGrants(t, f, nil)
				defer replaceManagementGrants(t, f, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})
				if _, err := program.EvaluateInstalled(run.Context(), source, run); !errors.Is(err, authz.ErrForbidden) {
					t.Fatalf("revoked execution grants survived current call: %v", err)
				}
			case "editor-revoked":
				actions := authz.Actions()
				for i, action := range actions {
					if action == "rule.preview" {
						actions = append(actions[:i], actions[i+1:]...)
						break
					}
				}
				replaceManagementGrants(t, f, []authz.Grant{{Actions: actions, Scope: authz.Scope{Kind: "all"}}})
				defer replaceManagementGrants(t, f, []authz.Grant{{Actions: authz.Actions(), Scope: authz.Scope{Kind: "all"}}})
				if _, err := program.EvaluateInstalled(run.Context(), source, run); !errors.Is(err, authz.ErrForbidden) {
					t.Fatalf("preview used execution grants in place of editor preview authority: %v", err)
				}
			case "stop":
				manager.Stop()
				if !errors.Is(run.Context().Err(), context.Canceled) {
					t.Fatal("shutdown did not cancel actual run context")
				}
				if _, err := manager.Begin(t.Context(), binding, sourceOwner); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
					t.Fatalf("new run accepted during shutdown: %v", err)
				}
			}
			if err := run.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			if _, err := run.PrepareInstalled(t.Context(), instance.ID, 1); err == nil {
				t.Fatal("closed run reused a drained compilation")
			}
		})
	}
}
