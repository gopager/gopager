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
		{"combined", []string{"--profile=full", "--roles=api,worker", "--listen-address=127.0.0.1:8080", "--migration-mode=apply"}, "synthetic-secret", true},
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
				if key != "GOPAGER_DATABASE_URL" {
					t.Fatalf("unexpected environment input %q", key)
				}
				return test.dsn
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
		})
	}
}
