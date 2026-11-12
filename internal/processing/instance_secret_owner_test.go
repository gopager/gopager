package processing

import (
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
