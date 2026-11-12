package management

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing"
)

func newCipher(t *testing.T, fill byte) *processing.SecretCipher {
	t.Helper()
	cipher, err := processing.NewSecretCipher(bytes.Repeat([]byte{fill}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return cipher
}

func truth(value bool) *bool { return &value }

func text(value string) *string { return &value }

// secretManifest declares an administrative required Secret, an optional administrative Secret and
// a personal Secret.
func secretManifest() processing.InstalledManifest {
	return processing.InstalledManifest{
		PluginID: "pushover", Capability: processing.CapabilityDelivery,
		ConfigurationSchema: processing.FieldSchema{Kind: processing.ObjectValue, Properties: map[string]processing.FieldSchema{
			"token": {Kind: processing.StringValue}, "extra": {Kind: processing.StringValue, Optional: true},
			"mine": {Kind: processing.StringValue, Optional: true}}},
		ConfigurationPolicy: processing.ConfigurationPolicy{Scope: "admin", Secret: truth(false),
			Properties: map[string]processing.ConfigurationPolicy{
				"token": {Scope: "admin", Secret: truth(true)}, "extra": {Scope: "admin", Secret: truth(true)},
				"mine": {Scope: "user", Secret: truth(true)}}},
	}
}

func TestSecretFingerprintsHideThePlaintext(t *testing.T) {
	t.Parallel()
	inputs := map[string]*SecretInput{"token": {Value: text("plaintext-token")}}
	first, err := secretFingerprints(newCipher(t, 1), inputs)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := secretFingerprints(newCipher(t, 1), inputs)
	other, _ := secretFingerprints(newCipher(t, 2), inputs)
	if first["token"] != again["token"] || first["token"] == other["token"] {
		t.Fatal("fingerprint must be stable and keyed")
	}
	plain := sha256.Sum256([]byte("token\x00plaintext-token"))
	if strings.Contains(first["token"], "plaintext-token") || strings.Contains(first["token"], hex.EncodeToString(plain[:])) {
		t.Fatal("fingerprint reveals or unkeyed-hashes the plaintext")
	}
	if _, err := secretFingerprints(nil, inputs); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("no key: %v", err)
	}
	references, err := secretFingerprints(nil, map[string]*SecretInput{"token": {Reference: text("file:pushover/a")}, "extra": nil})
	if err != nil || references["token"] != "reference:file:pushover/a" || references["extra"] != "remove" {
		t.Fatalf("reference fingerprints = %v, %v", references, err)
	}
}

func TestPlanSecrets(t *testing.T) {
	t.Parallel()
	settings := secretSettings{cipher: newCipher(t, 1)}
	instance, owner := uuid.New(), uuid.New()
	administrative := secretScope{scope: "admin", instanceID: instance, allowReference: true}
	previous := map[string]string{"token": "file:pushover/old", "extra": "env:GOPAGER_PLUGIN_SECRET_PUSHOVER__X", "mine": "secret:" + uuid.NewString()}

	plan, err := planSecrets(settings, secretManifest(), administrative, previous, nil)
	if err != nil || len(plan.rows) != 0 || plan.references["token"] != "file:pushover/old" || len(plan.references) != 2 {
		t.Fatalf("keeping previous references: %+v, %v", plan, err)
	}
	plan, err = planSecrets(settings, secretManifest(), administrative, previous,
		map[string]*SecretInput{"token": {Value: text("new")}, "extra": nil})
	if err != nil || len(plan.rows) != 1 || plan.rows[0].OwnerID != nil || plan.rows[0].InstanceID != instance ||
		plan.references["token"] != "secret:"+plan.rows[0].ID.String() {
		t.Fatalf("new value: %+v, %v", plan, err)
	}
	if _, kept := plan.references["extra"]; kept {
		t.Fatal("null did not remove the optional Secret")
	}
	personal := secretScope{scope: "user", instanceID: instance, owner: &owner}
	plan, err = planSecrets(settings, secretManifest(), personal, previous, map[string]*SecretInput{"mine": {Value: text("v")}})
	if err != nil || plan.rows[0].OwnerID == nil || *plan.rows[0].OwnerID != owner || len(plan.references) != 1 {
		t.Fatalf("personal value: %+v, %v", plan, err)
	}
	for name, test := range map[string]struct {
		target secretScope
		inputs map[string]*SecretInput
	}{
		"remove required":            {administrative, map[string]*SecretInput{"token": nil}},
		"personal path as admin":     {administrative, map[string]*SecretInput{"mine": {Value: text("v")}}},
		"admin path as user":         {personal, map[string]*SecretInput{"token": {Value: text("v")}}},
		"personal reference":         {personal, map[string]*SecretInput{"mine": {Reference: text("file:pushover/a")}}},
		"stored reference":           {administrative, map[string]*SecretInput{"token": {Reference: text("secret:" + uuid.NewString())}}},
		"empty value":                {administrative, map[string]*SecretInput{"token": {Value: text("")}}},
		"both set":                   {administrative, map[string]*SecretInput{"token": {Value: text("a"), Reference: text("file:pushover/a")}}},
		"neither set":                {administrative, map[string]*SecretInput{"token": {}}},
		"unknown path":               {administrative, map[string]*SecretInput{"nothing": {Value: text("v")}}},
		"environment outside prefix": {administrative, map[string]*SecretInput{"token": {Reference: text("env:HOME")}}},
		"another plugin's variable":  {administrative, map[string]*SecretInput{"token": {Reference: text("env:GOPAGER_PLUGIN_SECRET_WEBHOOK__TOKEN")}}},
		"prefix collision variable":  {administrative, map[string]*SecretInput{"token": {Reference: text("env:GOPAGER_PLUGIN_SECRET_PUSHOVER_X__TOKEN")}}},
		"another plugin's file":      {administrative, map[string]*SecretInput{"token": {Reference: text("file:webhook/token")}}},
		"file outside any plugin":    {administrative, map[string]*SecretInput{"token": {Reference: text("file:token")}}},
	} {
		if _, err := planSecrets(settings, secretManifest(), test.target, nil, test.inputs); !errors.Is(err, authz.ErrInvalidInput) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := planSecrets(secretSettings{}, secretManifest(), administrative, nil,
		map[string]*SecretInput{"token": {Value: text("v")}}); platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
		t.Fatalf("no key: %v", err)
	}
}

func TestPlanSecretsAcceptsOwnNamespace(t *testing.T) {
	t.Parallel()
	administrative := secretScope{scope: "admin", instanceID: uuid.New(), allowReference: true}
	for _, reference := range []string{"env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN", "file:pushover/token"} {
		plan, err := planSecrets(secretSettings{}, secretManifest(), administrative, nil,
			map[string]*SecretInput{"token": {Reference: text(reference)}})
		if err != nil || plan.references["token"] != reference {
			t.Errorf("%s: %+v, %v", reference, plan, err)
		}
	}
}

func TestPlanSecretsRejectsRetainedForeignReference(t *testing.T) {
	t.Parallel()
	administrative := secretScope{scope: "admin", instanceID: uuid.New(), allowReference: true}
	previous := map[string]string{"token": "env:GOPAGER_PLUGIN_SECRET_WEBHOOK__TOKEN"}
	if _, err := planSecrets(secretSettings{}, secretManifest(), administrative, previous, nil); !errors.Is(err, authz.ErrInvalidInput) {
		t.Fatalf("retained foreign reference: %v", err)
	}
}
