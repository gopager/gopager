package processing

import (
	"strings"
	"testing"
)

func boolPointer(value bool) *bool { return &value }

// assignManifest declares an administrative title and nested Secret, and personal nickname and Secret.
func assignManifest() InstalledManifest {
	return InstalledManifest{
		Capability: CapabilityDelivery,
		Delivery: &DeliveryCapability{OutputKinds: []string{LayoutText}, Inputs: []DeliveryInputDeclaration{
			{Name: "sound", Kind: StringValue, MaxBytes: 8}, {Name: "priority", Kind: IntegerValue}}},
		ConfigurationSchema: FieldSchema{Kind: ObjectValue, Properties: map[string]FieldSchema{
			"title": {Kind: StringValue},
			"auth": {Kind: ObjectValue, Properties: map[string]FieldSchema{
				"token": {Kind: StringValue, Optional: true}}, Optional: true},
			"nick":    {Kind: StringValue, Optional: true},
			"userKey": {Kind: StringValue, Optional: true},
		}},
		ConfigurationPolicy: ConfigurationPolicy{Scope: "admin", Secret: boolPointer(false), Properties: map[string]ConfigurationPolicy{
			"title": {Scope: "admin", Secret: boolPointer(false)},
			"auth": {Scope: "admin", Secret: boolPointer(false), Properties: map[string]ConfigurationPolicy{
				"token": {Scope: "admin", Secret: boolPointer(true)}}},
			"nick":    {Scope: "user", Secret: boolPointer(false)},
			"userKey": {Scope: "user", Secret: boolPointer(true)},
		}},
	}
}

func stringsObject(t *testing.T, members map[string]string) Value {
	t.Helper()
	typed := map[string]Value{}
	for name, text := range members {
		value, err := Literal(StringValue, text)
		if err != nil {
			t.Fatal(err)
		}
		typed[name] = value
	}
	value, err := Literal(ObjectValue, typed)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSecretReferencesRoundTripAtDottedPaths(t *testing.T) {
	t.Parallel()
	manifest := assignManifest()
	configured, err := manifest.WithSecretReferences(stringsObject(t, map[string]string{"title": "T"}),
		map[string]string{"auth.token": "file:token"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configured.Native(manifest.ConfigurationSchema); err != nil {
		t.Fatalf("configuration with a created container is invalid: %v", err)
	}
	found, err := manifest.SecretReferenceStrings(configured)
	if err != nil || len(found) != 1 || found["auth.token"] != "file:token" {
		t.Fatalf("references = %v, %v", found, err)
	}
	if _, err := manifest.WithSecretReferences(Value{Kind: StringValue, Value: []byte(`"x"`)},
		map[string]string{"auth.token": "file:token"}); err == nil {
		t.Fatal("reference set into a non-object")
	}
}

func TestRejectSecretValuesRefusesAnySecretInsideConfiguration(t *testing.T) {
	t.Parallel()
	manifest := assignManifest()
	if err := manifest.RejectSecretValues(stringsObject(t, map[string]string{"title": "T"})); err != nil {
		t.Fatalf("configuration without Secrets rejected: %v", err)
	}
	if err := manifest.RejectSecretValues(stringsObject(t, map[string]string{"userKey": "file:x"})); err == nil {
		t.Fatal("a Secret inside the configuration was accepted")
	}
	nested := Value{Kind: ObjectValue, Value: []byte(`{"auth":{"kind":"object","value":{"token":{"kind":"string","value":"plain"}}}}`)}
	if err := manifest.RejectSecretValues(nested); err == nil {
		t.Fatal("a nested Secret inside the configuration was accepted")
	}
}

func TestValidatePersonalConfigurationAdmitsOnlyUserProperties(t *testing.T) {
	t.Parallel()
	manifest := assignManifest()
	for name, test := range map[string]struct {
		value Value
		ok    bool
	}{
		"empty":         {EmptyConfiguration(), true},
		"nickname":      {stringsObject(t, map[string]string{"nick": "n"}), true},
		"secret ref":    {stringsObject(t, map[string]string{"userKey": "secret:" + strings.Repeat("0", 8) + "-0000-0000-0000-000000000001"}), true},
		"admin":         {stringsObject(t, map[string]string{"title": "T"}), false},
		"unknown":       {stringsObject(t, map[string]string{"other": "x"}), false},
		"wrong type":    {Value{Kind: ObjectValue, Value: []byte(`{"nick":{"kind":"boolean","value":true}}`)}, false},
		"not an object": {Value{Kind: StringValue, Value: []byte(`"x"`)}, false},
	} {
		if err := manifest.ValidatePersonalConfiguration(test.value); (err == nil) != test.ok {
			t.Errorf("%s: err = %v, want ok=%v", name, err, test.ok)
		}
	}
}

func TestValidateUnitParametersChecksKeysKindsAndSize(t *testing.T) {
	t.Parallel()
	manifest := assignManifest()
	text := func(value string) Value {
		typed, err := Literal(StringValue, value)
		if err != nil {
			t.Fatal(err)
		}
		return typed
	}
	number, err := Literal(IntegerValue, "2")
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.ValidateUnitParameters(map[string]Value{"sound": text("bell"), "priority": number}); err != nil {
		t.Fatalf("valid values rejected: %v", err)
	}
	for name, values := range map[string]map[string]Value{
		"unknown key": {"volume": number},
		"wrong kind":  {"priority": text("high")},
		"too large":   {"sound": text("bells12")},
	} {
		if err := manifest.ValidateUnitParameters(values); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := (InstalledManifest{}).ValidateUnitParameters(nil); err == nil {
		t.Error("a processing manifest accepted unit parameters")
	}
}

func TestSecretCipherFingerprintIsKeyedAndStable(t *testing.T) {
	t.Parallel()
	first, second := testCipher(t, 1), testCipher(t, 2)
	data := []byte("token")
	fingerprint := first.Fingerprint(data)
	if again := first.Fingerprint(data); fingerprint != again {
		t.Fatal("fingerprint is not stable")
	}
	if fingerprint == second.Fingerprint(data) {
		t.Fatal("fingerprint does not depend on the key")
	}
	if strings.Contains(fingerprint, "token") || len(fingerprint) != 64 {
		t.Fatal("unexpected fingerprint shape")
	}
}

// TestRetainableSecretReferencesNeedPresentParents checks that a previous Secret reference is
// carried over only while every parent object of its path is present in the submitted
// configuration, so omitting an optional branch removes the branch instead of recreating it.
func TestRetainableSecretReferencesNeedPresentParents(t *testing.T) {
	t.Parallel()
	manifest := assignManifest()
	manifest.ConfigurationSchema.Properties["auth"].Properties["token"] = FieldSchema{Kind: StringValue}
	previous := map[string]string{"auth.token": "file:token", "userKey": "file:key"}
	title := stringsObject(t, map[string]string{"title": "T"})

	omitted := manifest.RetainableSecretReferences(previous, title)
	if _, carried := omitted["auth.token"]; carried || omitted["userKey"] != "file:key" {
		t.Fatalf("omitted branch: %v", omitted)
	}
	configuration, err := manifest.WithSecretReferences(title, omitted)
	if err != nil || strings.Contains(string(configuration.Value), "auth") {
		t.Fatalf("the omitted branch came back: %s, %v", configuration.Value, err)
	}

	empty := objectOf(t, map[string]Value{"title": stringLeaf(t, "T"), "auth": objectOf(t, nil)})
	present := manifest.RetainableSecretReferences(previous, empty)
	if present["auth.token"] != "file:token" {
		t.Fatalf("present branch lost its Secret: %v", present)
	}
	null := objectOf(t, map[string]Value{"title": stringLeaf(t, "T"), "auth": {Kind: NullValue, Value: []byte("null")}})
	if _, carried := manifest.RetainableSecretReferences(previous, null)["auth.token"]; carried {
		t.Fatal("a null branch kept its Secret")
	}
}
