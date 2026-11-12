package processing

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

// nestedManifest declares an administrative Secret below an object and a personal Secret below
// another administrative object.
func nestedManifest() InstalledManifest {
	manifest, _ := deliveryManifestFixture()
	manifest.ConfigurationSchema = FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{
		"name": {Kind: StringValue},
		"auth": {Kind: ObjectValue, Properties: map[string]FieldSchema{"token": {Kind: StringValue}}},
		"me":   {Kind: ObjectValue, Optional: true, Properties: map[string]FieldSchema{"key": {Kind: StringValue, Optional: true}, "nick": {Kind: StringValue, Optional: true}}},
	}}
	manifest.ConfigurationPolicy = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false), Properties: map[string]ConfigurationPolicy{
		"name": {Scope: "admin", Secret: boolPtr(false)},
		"auth": {Scope: "admin", Secret: boolPtr(false), Properties: map[string]ConfigurationPolicy{"token": {Scope: "admin", Secret: boolPtr(true)}}},
		"me": {Scope: "admin", Secret: boolPtr(false), Properties: map[string]ConfigurationPolicy{
			"key":  {Scope: "user", Secret: boolPtr(true)},
			"nick": {Scope: "user", Secret: boolPtr(false)},
		}},
	}}
	return manifest
}

func TestSecretPropertiesAndReferences(t *testing.T) {
	t.Parallel()
	manifest := nestedManifest()
	if err := validateDeliveryConfiguration(manifest.ConfigurationSchema, manifest.ConfigurationPolicy); err != nil {
		t.Fatal(err)
	}
	properties := manifest.SecretProperties()
	if len(properties) != 2 || properties[0] != (SecretProperty{Path: "auth.token", Scope: "admin"}) ||
		properties[1] != (SecretProperty{Path: "me.key", Scope: "user", Optional: true}) {
		t.Fatalf("wrong properties: %+v", properties)
	}
	row := uuid.New()
	value := objectOf(t, map[string]Value{"name": stringLeaf(t, "n"),
		"auth": objectOf(t, map[string]Value{"token": stringLeaf(t, "secret:"+row.String())})})
	references, err := manifest.ConfigurationSecretReferences(value)
	if err != nil || len(references) != 1 || references["auth.token"] != (SecretReference{Source: SecretStored, ID: row}) {
		t.Fatalf("wrong references: %v %v", references, err)
	}
	for name, bad := range map[string]Value{
		"plain text": objectOf(t, map[string]Value{"auth": objectOf(t, map[string]Value{"token": stringLeaf(t, "plain")})}),
		"not string": objectOf(t, map[string]Value{"auth": objectOf(t, map[string]Value{"token": {Kind: IntegerValue, Value: []byte(`"1"`)}})}),
		"undeclared": objectOf(t, map[string]Value{"other": stringLeaf(t, "x")}),
		"not object": stringLeaf(t, "x"),
	} {
		if _, err := manifest.ConfigurationSecretReferences(bad); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Errorf("%s accepted: %v", name, err)
		}
	}
}

func TestMergeConfiguration(t *testing.T) {
	t.Parallel()
	manifest := nestedManifest()
	admin := objectOf(t, map[string]Value{"name": stringLeaf(t, "admin"),
		"auth": objectOf(t, map[string]Value{"token": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__A")})})
	personal := objectOf(t, map[string]Value{
		"name": stringLeaf(t, "shadow"),
		"auth": objectOf(t, map[string]Value{"token": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__B")}),
		"me":   objectOf(t, map[string]Value{"key": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__C"), "nick": stringLeaf(t, "nick")}),
		"junk": stringLeaf(t, "x"),
	})
	merged, err := manifest.mergeConfiguration(admin, &personal)
	if err != nil {
		t.Fatal(err)
	}
	native, err := merged.Native(manifest.ConfigurationSchema)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(native)
	want := `{"auth":{"token":"env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__A"},"me":{"key":"env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__C","nick":"nick"},"name":"admin"}`
	if string(got) != want {
		t.Fatalf("merged %s, want %s", got, want)
	}
	alone, err := manifest.mergeConfiguration(admin, nil)
	if err != nil {
		t.Fatal(err)
	}
	if native, err = alone.Native(manifest.ConfigurationSchema); err != nil {
		t.Fatal(err)
	}
	if got, _ = json.Marshal(native); strings.Contains(string(got), `"me"`) {
		t.Fatalf("absent personal values appeared: %s", got)
	}
}

func TestInspectDeliveryInstanceAndProjection(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	inspector, err := NewLocalInstanceInspector(env.host, env.root)
	if err != nil {
		t.Fatal(err)
	}
	instance := env.instance("Title", "")
	if err := inspector.InspectInstance(t.Context(), instance); err != nil {
		t.Fatalf("valid delivery instance rejected: %v", err)
	}
	for name, mutate := range map[string]func(*InstanceConfiguration){
		"no target use": func(i *InstanceConfiguration) { i.TargetUse = nil },
		"plain secret": func(i *InstanceConfiguration) {
			i.Configuration = objectOf(t, map[string]Value{"title": stringLeaf(t, "T"), "apiToken": stringLeaf(t, "plaintext")})
		},
		"user value in admin": func(i *InstanceConfiguration) {
			i.Configuration = objectOf(t, map[string]Value{"title": stringLeaf(t, "T"), "apiToken": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__A"), "userKey": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__B")})
		},
		"too many principals": func(i *InstanceConfiguration) {
			i.TargetUse = &TargetUse{PrincipalIDs: make([]uuid.UUID, MaxTargetUsePrincipals+1)}
		},
	} {
		changed := instance
		mutate(&changed)
		if err := inspector.InspectInstance(t.Context(), changed); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	view, err := inspector.ProjectInstanceConfiguration(t.Context(), instance)
	if err != nil {
		t.Fatal(err)
	}
	token := view.Fields["apiToken"]
	if !token.Secret || !token.Configured || !token.Redacted || token.Source != "stored" || token.Value != nil {
		t.Fatalf("Secret view %+v", token)
	}
	if view.Fields["title"].Value == nil || view.Fields["userKey"].Configured {
		t.Fatalf("view %+v", view.Fields)
	}
	encoded, _ := json.Marshal(view)
	if strings.Contains(string(encoded), "secret:") || strings.Contains(string(encoded), "GOPAGER_PLUGIN") {
		t.Fatalf("view reveals a reference: %s", encoded)
	}

	for reference, source := range map[string]string{"env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__A": "env", "file:test-delivery/token": "file"} {
		other := env.instance("T", reference)
		view, err := inspector.ProjectInstanceConfiguration(t.Context(), other)
		if err != nil || view.Fields["apiToken"].Source != source {
			t.Fatalf("%s: %+v %v", reference, view.Fields["apiToken"], err)
		}
		encoded, _ := json.Marshal(view)
		if strings.Contains(string(encoded), "GOPAGER_PLUGIN") || strings.Contains(string(encoded), "test-delivery/") {
			t.Fatalf("view reveals location: %s", encoded)
		}
	}
}

func TestProjectPersonalConfiguration(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	inspector, _ := NewLocalInstanceInspector(env.host, env.root)
	instance := env.instance("T", "")
	owner := uuid.New()
	personal := PersonalConfiguration{InstanceID: instance.ID, OwnerID: owner, Revision: 1,
		Configuration: objectOf(t, map[string]Value{"userKey": stringLeaf(t, "file:keys/me")})}
	view, err := inspector.ProjectPersonalConfiguration(t.Context(), instance, personal)
	if err != nil {
		t.Fatal(err)
	}
	key := view.Fields["userKey"]
	if len(view.Fields) != 1 || !key.Secret || !key.Configured || !key.Redacted || key.Source != "file" || key.Value != nil || key.Scope != "user" {
		t.Fatalf("personal view %+v", view)
	}
	personal.Configuration = objectOf(t, map[string]Value{})
	if view, err = inspector.ProjectPersonalConfiguration(t.Context(), instance, personal); err != nil || view.Fields["userKey"].Configured {
		t.Fatalf("empty personal view %+v %v", view, err)
	}
	personal.InstanceID = uuid.New()
	if _, err := inspector.ProjectPersonalConfiguration(t.Context(), instance, personal); err == nil {
		t.Fatal("another instance's personal values projected")
	}

	nested := nestedManifest()
	nestedView, err := nested.ProjectPersonalConfiguration(objectOf(t, map[string]Value{
		"me": objectOf(t, map[string]Value{"key": stringLeaf(t, "env:GOPAGER_PLUGIN_SECRET_TEST_DELIVERY__C"), "nick": stringLeaf(t, "nick")})}))
	if err != nil {
		t.Fatal(err)
	}
	me := nestedView.Fields["me"].Properties
	if _, adminShown := nestedView.Fields["name"]; adminShown || me["key"].Source != "env" || string(me["nick"].Value) != `"nick"` {
		t.Fatalf("nested personal view %+v", nestedView)
	}
}

// TestProjectPersonalObjectRedactsOnlySecretLeaves checks that a personal object holding a Secret
// and a readable property shows the readable value and only redacts the Secret.
func TestProjectPersonalObjectRedactsOnlySecretLeaves(t *testing.T) {
	t.Parallel()
	manifest := nestedManifest()
	manifest.ConfigurationSchema.Properties["mine"] = FieldSchema{Kind: ObjectValue, Optional: true, Properties: map[string]FieldSchema{
		"token": {Kind: StringValue, Optional: true}, "device": {Kind: StringValue, Optional: true}}}
	manifest.ConfigurationPolicy.Properties["mine"] = ConfigurationPolicy{Scope: "user", Secret: boolPtr(false), Properties: map[string]ConfigurationPolicy{
		"token":  {Scope: "user", Secret: boolPtr(true)},
		"device": {Scope: "user", Secret: boolPtr(false)},
	}}
	view, err := manifest.ProjectPersonalConfiguration(objectOf(t, map[string]Value{
		"mine": objectOf(t, map[string]Value{"token": stringLeaf(t, "secret:"+uuid.NewString()), "device": stringLeaf(t, "phone")})}))
	if err != nil {
		t.Fatal(err)
	}
	mine := view.Fields["mine"]
	token, device := mine.Properties["token"], mine.Properties["device"]
	if mine.Redacted || !token.Redacted || token.Source != "stored" || token.Value != nil ||
		device.Redacted || string(device.Value) != `"phone"` {
		t.Fatalf("personal object view %+v", mine)
	}
}

func TestInspectProcessingInstanceKeepsRules(t *testing.T) {
	t.Parallel()
	_, root, manifest, pin := artifactFixture(t, markerScript)
	writeManifest(t, root, manifest)
	host := NewPluginHost()
	t.Cleanup(func() { _ = host.Close(t.Context()) })
	inspector, _ := NewLocalInstanceInspector(host, root)
	instance := InstanceConfiguration{ID: uuid.New(), Revision: 1, Enabled: true, ExecutionPrincipalID: uuid.New(),
		Main: ProvisionedArtifact{ManifestPath: "manifest.json", Pin: pin}, CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), CreatedBy: uuid.New(),
		Configuration: objectOf(t, map[string]Value{})}
	if err := inspector.InspectInstance(t.Context(), instance); err != nil {
		t.Fatal(err)
	}
	instance.TargetUse = &TargetUse{Everyone: true}
	if err := inspector.InspectInstance(t.Context(), instance); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("processing instance with target use accepted: %v", err)
	}
}

func TestDeliveryArtifactIsRejectedForWorkflowSteps(t *testing.T) {
	t.Parallel()
	env := newDeliveryEnv(t)
	artifact, err := env.host.LoadInstalledFor(t.Context(), env.root, "manifest.json", env.pin, uuid.New(), 1, InvocationModeLive)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = artifact.Close(t.Context()) }()
	envelope := installedEnvelope(t, env.pin, map[string]Value{})
	if _, err := artifact.invokeProjected(t.Context(), envelope); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("delivery artifact invoked as a workflow step: %v", err)
	}
}
