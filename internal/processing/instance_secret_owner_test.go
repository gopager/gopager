package processing

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/gopager/gopager/internal/platform"
)

func TestSecretReferenceBelongsToPlugin(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		reference string
		plugin    string
		want      bool
	}{
		"own env":                   {"env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN", "pushover", true},
		"own env with hyphen":       {"env:GOPAGER_PLUGIN_SECRET_MY_HOOK__KEY", "my-hook", true},
		"other plugin env":          {"env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN", "webhook", false},
		"prefix collision env":      {"env:GOPAGER_PLUGIN_SECRET_PUSHOVER_X__TOKEN", "pushover", false},
		"prefix collision reverse":  {"env:GOPAGER_PLUGIN_SECRET_PUSHOVER__TOKEN", "pushover-x", false},
		"missing double underscore": {"env:GOPAGER_PLUGIN_SECRET_PUSHOVER_TOKEN", "pushover", false},
		"nothing after prefix":      {"env:GOPAGER_PLUGIN_SECRET_PUSHOVER__", "pushover", false},
		"own file":                  {"file:pushover/token", "pushover", true},
		"own nested file":           {"file:pushover/a/token", "pushover", true},
		"other plugin file":         {"file:webhook/token", "pushover", false},
		"file prefix collision":     {"file:pushover-x/token", "pushover", false},
		"file at root":              {"file:token", "pushover", false},
		"file named like plugin":    {"file:pushover", "pushover", false},
		"stored":                    {"secret:" + uuid.NewString(), "pushover", true},
		"underscore plugin ID":      {"env:GOPAGER_PLUGIN_SECRET_MY_HOOK__KEY", "my_hook", false},
		"upper-case plugin ID":      {"file:Pushover/token", "Pushover", false},
		"nested plugin ID":          {"file:a/b/token", "a/b", false},
		"double hyphen plugin ID":   {"env:GOPAGER_PLUGIN_SECRET_A__B__KEY", "a--b", false},
	}
	for name, tc := range cases {
		reference, err := ParseSecretReference(tc.reference)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := reference.BelongsTo(tc.plugin); got != tc.want {
			t.Errorf("%s: BelongsTo = %v, want %v", name, got, tc.want)
		}
	}
	if got := SecretEnvPluginPrefix("my-hook"); got != "GOPAGER_PLUGIN_SECRET_MY_HOOK__" {
		t.Errorf("prefix = %q", got)
	}
}

// TestDeliveryPluginIDsMapToDisjointNamespaces checks that canonical Delivery plugin IDs give
// environment prefixes of which none equals or starts with another, and that non-canonical IDs
// are refused.
func TestDeliveryPluginIDsMapToDisjointNamespaces(t *testing.T) {
	t.Parallel()
	ids := []string{"a", "a-b", "ab", "a-b-c", "a-bc", "ab-c", "pushover", "pushover-x", "x1", "1x"}
	for _, id := range ids {
		if !ValidDeliveryPluginID(id) {
			t.Fatalf("%s must be canonical", id)
		}
	}
	for _, first := range ids {
		for _, second := range ids {
			if first != second && strings.HasPrefix(SecretEnvPluginPrefix(second), SecretEnvPluginPrefix(first)) {
				t.Errorf("namespace of %s contains namespace of %s", first, second)
			}
		}
	}
	for _, id := range []string{"", "A", "a_b", "a--b", "-a", "a-", "a/b", "a.b", "..", "a b"} {
		if ValidDeliveryPluginID(id) {
			t.Errorf("%q must not be canonical", id)
		}
	}
}

func TestSecretResolverRejectsAnotherPluginsSecret(t *testing.T) {
	t.Parallel()
	resolver, _ := resolverFixture(t)
	for name, reference := range map[string]SecretReference{
		"env":  {Source: SecretEnv, Name: "GOPAGER_PLUGIN_SECRET_A"},
		"file": {Source: SecretFile, Path: "pushover/token"},
	} {
		_, err := resolver.Resolve("webhook", uuid.New(), []SecretRequest{{Path: "a", Reference: reference}}, nil)
		if platform.FailureCode(err) != platform.CodeAuthorityUnavailable {
			t.Errorf("%s: %v", name, err)
		}
	}
}
