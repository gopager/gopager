package main

import (
	"strings"
	"testing"
	"time"

	ingestdomain "github.com/gopager/gopager/internal/ingest"
	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/processing/worker"
)

func TestParseConfigBindsExplicitInputs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		args []string
		dsn  string
		ok   bool
	}{
		{"worker", []string{"--roles=worker"}, "synthetic-secret", true},
		{"combined", []string{"--roles=api,worker", "--listen-address=127.0.0.1:8080", "--public-origin=https://pager.example", "--migration-mode=apply"}, "synthetic-secret", true},
		{"HTTP origin", []string{"--roles=api", "--listen-address=127.0.0.1:8080", "--public-origin=http://pager.example:8080"}, "synthetic-secret", true},
		{"missing public origin", []string{"--roles=api", "--listen-address=127.0.0.1:8080"}, "synthetic-secret", false},
		{"bad public origin", []string{"--roles=api", "--listen-address=127.0.0.1:8080", "--public-origin=https://pager.example/path"}, "synthetic-secret", false},
		{"configured login limits", []string{"--roles=worker", "--login-attempts-per-account=2", "--login-attempts-per-instance=20"}, "synthetic-secret", true},
		{"configured artifact root", []string{"--roles=worker", "--processing-artifact-root=/"}, "synthetic-secret", true},
		{"empty artifact root", []string{"--roles=worker", "--processing-artifact-root="}, "synthetic-secret", false},
		{"invalid artifact root", []string{"--roles=worker", "--processing-artifact-root=/dev/null"}, "synthetic-secret", false},
		{"configured source limit", []string{"--roles=worker", "--login-attempts-per-source=5"}, "synthetic-secret", true},
		{"invalid source limit", []string{"--roles=worker", "--login-attempts-per-source=0"}, "synthetic-secret", false},
		{"invalid login limit", []string{"--roles=worker", "--login-attempts-per-account=0"}, "synthetic-secret", false},
		{"configured search limit", []string{"--roles=worker", "--search-attempts-per-minute=3"}, "synthetic-secret", true},
		{"zero search limit", []string{"--roles=worker", "--search-attempts-per-minute=0"}, "synthetic-secret", false},
		{"configured dispatch", []string{"--roles=worker", "--processing-concurrency=8", "--processing-poll-interval=750ms"}, "synthetic-secret", true},
		{"zero concurrency", []string{"--roles=worker", "--processing-concurrency=0"}, "synthetic-secret", false},
		{"negative concurrency", []string{"--roles=worker", "--processing-concurrency=-1"}, "synthetic-secret", false},
		{"zero poll interval", []string{"--roles=worker", "--processing-poll-interval=0s"}, "synthetic-secret", false},
		{"negative poll interval", []string{"--roles=worker", "--processing-poll-interval=-1s"}, "synthetic-secret", false},
		{"malformed poll interval", []string{"--roles=worker", "--processing-poll-interval=soon"}, "synthetic-secret", false},
		{"configured event poll", []string{"--roles=worker", "--event-poll-interval=750ms"}, "synthetic-secret", true},
		{"zero event poll", []string{"--roles=worker", "--event-poll-interval=0s"}, "synthetic-secret", false},
		{"negative event poll", []string{"--roles=worker", "--event-poll-interval=-1s"}, "synthetic-secret", false},
		{"malformed event poll", []string{"--roles=worker", "--event-poll-interval=soon"}, "synthetic-secret", false},
		{"configured live replay window", []string{"--roles=worker", "--live-replay-window=48h"}, "synthetic-secret", true},
		{"zero live replay window", []string{"--roles=worker", "--live-replay-window=0s"}, "synthetic-secret", false},
		{"negative live replay window", []string{"--roles=worker", "--live-replay-window=-1h"}, "synthetic-secret", false},
		{"malformed live replay window", []string{"--roles=worker", "--live-replay-window=long"}, "synthetic-secret", false},
		{"unknown role", []string{"--roles=other"}, "synthetic-secret", false},
		{"empty role", []string{"--roles=worker,"}, "synthetic-secret", false},
		{"duplicate role", []string{"--roles=worker,worker"}, "synthetic-secret", false},
		{"configured contact monitor", []string{"--roles=worker", "--gateway-heartbeat-interval=30s", "--gateway-contact-timeout=90s", "--gateway-contact-scan-interval=5s"}, "synthetic-secret", true},
		{"contact timeout equals heartbeat", []string{"--roles=worker", "--gateway-heartbeat-interval=60s", "--gateway-contact-timeout=60s"}, "synthetic-secret", true},
		{"contact timeout below heartbeat", []string{"--roles=worker", "--gateway-contact-timeout=59s"}, "synthetic-secret", false},
		{"zero contact timeout", []string{"--roles=worker", "--gateway-heartbeat-interval=0s", "--gateway-contact-timeout=0s"}, "synthetic-secret", false},
		{"contact scan above timeout", []string{"--roles=worker", "--gateway-contact-scan-interval=181s"}, "synthetic-secret", false},
		{"zero contact scan", []string{"--roles=worker", "--gateway-contact-scan-interval=0s"}, "synthetic-secret", false},
		{"missing listen", []string{"--roles=api"}, "synthetic-secret", false},
		{"missing dsn", []string{"--roles=worker"}, "", false},
		{"unknown flag", []string{"--unknown=synthetic-secret"}, "", false},
		{"positional", []string{"synthetic-secret"}, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
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
			if test.name == "configured search limit" && config.SearchAttempts != 3 {
				t.Fatal("configured search limit was not bound")
			}
			if test.ok && test.name != "configured search limit" && config.SearchAttempts != 60 {
				t.Fatalf("default search limit = %d, want 60", config.SearchAttempts)
			}
			if test.name == "configured dispatch" && (config.ProcessingDispatch.Concurrency != 8 ||
				config.ProcessingDispatch.PollInterval != 750*time.Millisecond) {
				t.Fatal("configured Processing dispatch was not bound")
			}
			if test.ok && test.name != "configured dispatch" && (config.ProcessingDispatch.Concurrency != worker.DefaultDispatchConcurrency ||
				config.ProcessingDispatch.PollInterval != worker.DefaultDispatchPollInterval) {
				t.Fatalf("Processing dispatch defaults not applied: %+v", config.ProcessingDispatch)
			}
			if test.name == "configured source limit" && config.LoginLimits.SourceAttempts != 5 {
				t.Fatal("configured source login limit was not bound")
			}
			if test.ok && test.name != "configured source limit" && config.LoginLimits.SourceAttempts != 30 {
				t.Fatalf("default source login limit = %d, want 30", config.LoginLimits.SourceAttempts)
			}
			if test.name == "configured contact monitor" && config.GatewayContact != (ingestdomain.ContactConfig{
				HeartbeatInterval: 30 * time.Second, Timeout: 90 * time.Second, ScanInterval: 5 * time.Second}) {
				t.Fatalf("contact monitor configuration was not bound: %+v", config.GatewayContact)
			}
			if test.ok && !strings.Contains(test.name, "contact") && config.GatewayContact != (ingestdomain.ContactConfig{
				HeartbeatInterval: ingestdomain.DefaultHeartbeatInterval, Timeout: ingestdomain.DefaultContactTimeout,
				ScanInterval: ingestdomain.DefaultContactScan}) {
				t.Fatalf("contact monitor defaults not applied: %+v", config.GatewayContact)
			}
			if test.name == "configured artifact root" && config.ProcessingArtifactRoot != "/" {
				t.Fatal("Processing artifact root was not bound")
			}
		})
	}
}

// TestParseConfigReadsPluginRootFromEnvironment covers plugin-directory provisioning through the environment, as used by container deployments.
func TestParseConfigReadsPluginRootFromEnvironment(t *testing.T) {
	t.Parallel()
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
	args := []string{"--roles=worker"}
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
