package processing

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gopager/gopager/internal/platform"
)

func boolPtr(b bool) *bool { return &b }

// deliveryManifestFixture is a valid delivery manifest whose digest equals the returned pin.
func deliveryManifestFixture() (InstalledManifest, ArtifactPin) {
	digest := "4f69af662a1c0009e8220304c26f548058f0b60815835a95ca0e3c6872361b12"
	manifest := InstalledManifest{SchemaVersion: 2, SDKMajor: 2, SDKMinor: 1, Capability: CapabilityDelivery,
		PluginID: "test-delivery", Version: "1", CapabilityID: "send",
		Artifact: ArtifactReference{Path: "plugin", Digest: digest},
		Delivery: &DeliveryCapability{OutputKinds: []string{"text", "json"},
			Inputs:       []DeliveryInputDeclaration{{Name: "priority", Kind: IntegerValue}},
			Dependencies: []DeliveryDependency{{Kind: "network", Description: "provider API"}},
			Idempotent:   true},
		ConfigurationSchema: FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{
			"title":    {Kind: StringValue},
			"apiToken": {Kind: StringValue},
			"userKey":  {Kind: StringValue, Optional: true},
		}},
		ConfigurationPolicy: ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false),
			Properties: map[string]ConfigurationPolicy{
				"title":    {Scope: "admin", Secret: boolPtr(false)},
				"apiToken": {Scope: "admin", Secret: boolPtr(true)},
				"userKey":  {Scope: "user", Secret: boolPtr(true)},
			}},
	}
	return manifest, ArtifactPin{PluginID: "test-delivery", Version: "1", CapabilityID: "send", Digest: digest}
}

func TestDeliveryManifestValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*InstalledManifest)
		code   platform.ErrorCode
		undecl bool
	}{
		{name: "happy path", mutate: func(*InstalledManifest) {}},
		{name: "required core fields", undecl: true, mutate: func(m *InstalledManifest) { m.RequiredCoreFields = []FieldRef{{}} }},
		{name: "required operations", undecl: true, mutate: func(m *InstalledManifest) { m.RequiredOperations = []string{"x"} }},
		{name: "unknown sdk minor", code: platform.CodeIncompatibleVersion, mutate: func(m *InstalledManifest) { m.SDKMinor = 2 }},
		{name: "delivery needs minor 1", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.SDKMinor = 0 }},
		{name: "unknown capability", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Capability = "other" }},
		{name: "missing delivery object", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Delivery = nil }},
		{name: "pure", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Pure = true }},
		{name: "preview", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.SupportsPreview = true }},
		{name: "input schema", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.InputSchema = map[string]FieldSchema{"a": {Kind: StringValue}}
		}},
		{name: "host capabilities", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.HostCapabilities = []string{"x"} }},
		{name: "no output kinds", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Delivery.OutputKinds = nil }},
		{name: "unknown output kind", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Delivery.OutputKinds = []string{"html"} }},
		{name: "composite input", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Delivery.Inputs[0].Kind = ObjectValue }},
		{name: "duplicate input", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.Delivery.Inputs = append(m.Delivery.Inputs, m.Delivery.Inputs[0])
		}},
		{name: "unknown dependency", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Delivery.Dependencies[0].Kind = "magic" }},
		{name: "sends per minute", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.Delivery.SendsPerMinute = 100001 }},
		{name: "secret on integer", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties["title"] = FieldSchema{Kind: IntegerValue}
			m.ConfigurationPolicy.Properties["title"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(true)}
		}},
		{name: "secret inside list", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			element := FieldSchema{Kind: StringValue}
			m.ConfigurationSchema.Properties["tokens"] = FieldSchema{Kind: ListValue, Optional: true, Element: &element}
			m.ConfigurationPolicy.Properties["tokens"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false),
				Element: &ConfigurationPolicy{Scope: "admin", Secret: boolPtr(true)}}
		}},
		{name: "secret container", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties["auth"] = FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{}}
			m.ConfigurationPolicy.Properties["auth"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(true)}
		}},
		{name: "user property required", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties["userKey"] = FieldSchema{Kind: StringValue}
		}},
		{name: "user root", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) { m.ConfigurationPolicy.Scope = "user" }},
		{name: "personal property below an administrative list", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			element := FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{"nick": {Kind: StringValue, Optional: true}}}
			m.ConfigurationSchema.Properties["routes"] = FieldSchema{Kind: ListValue, Optional: true, Element: &element}
			m.ConfigurationPolicy.Properties["routes"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false),
				Element: &ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false), Properties: map[string]ConfigurationPolicy{
					"nick": {Scope: "user", Secret: boolPtr(false)}}}}
		}},
		{name: "nullable administrative container with personal descendants", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties["me"] = FieldSchema{Kind: ObjectValue, Optional: true, Nullable: true,
				Properties: map[string]FieldSchema{"nick": {Kind: StringValue, Optional: true}}}
			m.ConfigurationPolicy.Properties["me"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false),
				Properties: map[string]ConfigurationPolicy{"nick": {Scope: "user", Secret: boolPtr(false)}}}
		}},
		{name: "non-nullable administrative container with personal descendants", mutate: func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties["me"] = FieldSchema{Kind: ObjectValue, Optional: true,
				Properties: map[string]FieldSchema{"nick": {Kind: StringValue, Optional: true}}}
			m.ConfigurationPolicy.Properties["me"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false),
				Properties: map[string]ConfigurationPolicy{"nick": {Scope: "user", Secret: boolPtr(false)}}}
		}},
		{name: "dotted property name", code: platform.CodeInvalidInput, mutate: func(m *InstalledManifest) {
			m.ConfigurationSchema.Properties["a.b"] = FieldSchema{Kind: StringValue, Optional: true}
			m.ConfigurationPolicy.Properties["a.b"] = ConfigurationPolicy{Scope: "admin", Secret: boolPtr(false)}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			manifest, pin := deliveryManifestFixture()
			tc.mutate(&manifest)
			err := validateManifest(manifest, pin)
			if tc.undecl {
				var undeclared *CapabilityUndeclaredError
				if !errors.Is(err, ErrCapabilityUndeclared) || !errors.As(err, &undeclared) || undeclared.Capability != "delivery" {
					t.Fatalf("want capability undeclared, got %v", err)
				}
				return
			}
			if tc.code == "" {
				if err != nil {
					t.Fatalf("valid manifest rejected: %v", err)
				}
				return
			}
			if platform.FailureCode(err) != tc.code {
				t.Fatalf("want %s, got %v", tc.code, err)
			}
		})
	}
}

func TestProcessingManifestRejectsDeliveryObject(t *testing.T) {
	t.Parallel()
	manifest, pin := deliveryManifestFixture()
	manifest.Capability, manifest.Pure, manifest.SDKMinor = "", true, 0
	if err := validateManifest(manifest, pin); platform.FailureCode(err) != platform.CodeInvalidInput {
		t.Fatalf("processing manifest with a delivery object accepted: %v", err)
	}
	manifest.Delivery = nil
	manifest.Capability = CapabilityProcessing
	if err := validateManifest(manifest, pin); err != nil {
		t.Fatalf("explicit processing capability rejected: %v", err)
	}
}

func TestTargetUse(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	instance := InstanceConfiguration{TargetUse: &TargetUse{PrincipalIDs: []uuid.UUID{a}}}
	if !instance.AllowsTargetUse(a) || instance.AllowsTargetUse(b) {
		t.Fatal("listed principal handling wrong")
	}
	if !(InstanceConfiguration{TargetUse: &TargetUse{Everyone: true}}).AllowsTargetUse(b) {
		t.Fatal("everyone must admit any principal")
	}
	if (InstanceConfiguration{}).AllowsTargetUse(a) {
		t.Fatal("absent target use must admit nobody")
	}
	many := make([]uuid.UUID, MaxTargetUsePrincipals+1)
	for i := range many {
		many[i] = uuid.New()
	}
	for name, use := range map[string]TargetUse{
		"too many":  {PrincipalIDs: many},
		"nil id":    {PrincipalIDs: []uuid.UUID{uuid.Nil}},
		"duplicate": {PrincipalIDs: []uuid.UUID{a, a}},
	} {
		if err := use.Validate(); platform.FailureCode(err) != platform.CodeInvalidInput {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	if err := (TargetUse{PrincipalIDs: many[:MaxTargetUsePrincipals]}).Validate(); err != nil {
		t.Fatal(err)
	}
}
