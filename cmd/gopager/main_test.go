package main

import (
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func TestParseConfigBindsExplicitInputs(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		dsn  string
		ok   bool
	}{
		{"worker", []string{"--profile=full", "--roles=worker"}, "synthetic-secret", true},
		{"combined", []string{"--profile=full", "--roles=api,worker", "--listen-address=127.0.0.1:8080", "--public-origin=https://pager.example", "--migration-mode=apply"}, "synthetic-secret", true},
		{"HTTP origin", []string{"--profile=full", "--roles=api", "--listen-address=127.0.0.1:8080", "--public-origin=http://pager.example:8080"}, "synthetic-secret", true},
		{"missing public origin", []string{"--profile=full", "--roles=api", "--listen-address=127.0.0.1:8080"}, "synthetic-secret", false},
		{"bad public origin", []string{"--profile=full", "--roles=api", "--listen-address=127.0.0.1:8080", "--public-origin=https://pager.example/path"}, "synthetic-secret", false},
		{"configured login limits", []string{"--profile=full", "--roles=worker", "--login-attempts-per-account=2", "--login-attempts-per-instance=20"}, "synthetic-secret", true},
		{"configured artifact root", []string{"--profile=full", "--roles=worker", "--processing-artifact-root=/"}, "synthetic-secret", true},
		{"empty artifact root", []string{"--profile=full", "--roles=worker", "--processing-artifact-root="}, "synthetic-secret", false},
		{"invalid artifact root", []string{"--profile=full", "--roles=worker", "--processing-artifact-root=/dev/null"}, "synthetic-secret", false},
		{"configured source limit", []string{"--profile=full", "--roles=worker", "--login-attempts-per-source=5"}, "synthetic-secret", true},
		{"invalid source limit", []string{"--profile=full", "--roles=worker", "--login-attempts-per-source=0"}, "synthetic-secret", false},
		{"invalid login limit", []string{"--profile=full", "--roles=worker", "--login-attempts-per-account=0"}, "synthetic-secret", false},
		{"unsupported profile", []string{"--profile=lite", "--roles=worker"}, "synthetic-secret", false},
		{"unknown role", []string{"--profile=full", "--roles=other"}, "synthetic-secret", false},
		{"empty role", []string{"--profile=full", "--roles=worker,"}, "synthetic-secret", false},
		{"duplicate role", []string{"--profile=full", "--roles=worker,worker"}, "synthetic-secret", false},
		{"missing listen", []string{"--profile=full", "--roles=api"}, "synthetic-secret", false},
		{"missing dsn", []string{"--profile=full", "--roles=worker"}, "", false},
		{"unknown flag", []string{"--unknown=synthetic-secret"}, "", false},
		{"positional", []string{"synthetic-secret"}, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := parseConfig(test.args, func(key string) string {
				switch key {
				case "GOPAGER_DATABASE_URL":
					return test.dsn
				case "GOPAGER_PROCESSING_ARTIFACT_ROOT":
					return ""
				}
				t.Fatalf("unexpected environment input %q", key)
				return ""
			})
			if (err == nil) != test.ok {
				t.Fatalf("parse result: %v", err)
			}
			if err != nil && (platform.FailureCode(err) != platform.CodeInvalidInput || strings.Contains(err.Error(), "synthetic-secret")) {
				t.Fatalf("unsafe config failure: %v", err)
			}
			if test.ok && config.DatabaseURL != test.dsn {
				t.Fatal("secret environment input was not bound")
			}
			if test.name == "configured login limits" && (config.LoginLimits.AccountAttempts != 2 || config.LoginLimits.InstanceAttempts != 20) {
				t.Fatal("configured login limits were not bound")
			}
			if test.name == "configured source limit" && config.LoginLimits.SourceAttempts != 5 {
				t.Fatal("configured source login limit was not bound")
			}
			if test.ok && test.name != "configured source limit" && config.LoginLimits.SourceAttempts != 30 {
				t.Fatalf("default source login limit = %d, want 30", config.LoginLimits.SourceAttempts)
			}
			if test.name == "configured artifact root" && config.ProcessingArtifactRoot != "/" {
				t.Fatal("Processing artifact root was not bound")
			}
		})
	}
}

// TestParseConfigReadsPluginRootFromEnvironment covers plugin-directory provisioning through the environment, as used by container deployments.
func TestParseConfigReadsPluginRootFromEnvironment(t *testing.T) {
	fromEnv, fromFlag := t.TempDir(), t.TempDir()
	environment := func(root string) func(string) string {
		return func(key string) string {
			switch key {
			case "GOPAGER_DATABASE_URL":
				return "synthetic-secret"
			case "GOPAGER_PROCESSING_ARTIFACT_ROOT":
				return root
			}
			return ""
		}
	}
	args := []string{"--profile=full", "--roles=worker"}
	config, err := parseConfig(args, environment(fromEnv))
	if err != nil || config.ProcessingArtifactRoot != fromEnv {
		t.Fatalf("environment plugin root not used: %q, %v", config.ProcessingArtifactRoot, err)
	}
	config, err = parseConfig(append(args, "--processing-artifact-root="+fromFlag), environment(fromEnv))
	if err != nil || config.ProcessingArtifactRoot != fromFlag {
		t.Fatalf("flag did not take precedence over environment: %q, %v", config.ProcessingArtifactRoot, err)
	}
	if _, err := parseConfig(args, environment("/dev/null")); err == nil {
		t.Fatal("invalid environment plugin root accepted")
	}
	config, err = parseConfig(args, environment(""))
	if err != nil || config.ProcessingArtifactRoot != "" {
		t.Fatalf("empty environment plugin root was not treated as unset: %q, %v", config.ProcessingArtifactRoot, err)
	}
}
