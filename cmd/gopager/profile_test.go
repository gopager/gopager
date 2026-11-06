package main

import (
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

// TestParseConfigRejectsRemovedProfileFlag pins the removal of --profile (D-135):
// an old deployment script must fail loudly instead of silently ignoring it.
func TestParseConfigRejectsRemovedProfileFlag(t *testing.T) {
	_, err := parseConfig([]string{"--profile=full", "--roles=worker"}, func(key string) string {
		if key == "GOPAGER_DATABASE_URL" {
			return "synthetic-secret"
		}
		return ""
	})
	if err == nil {
		t.Fatal("--profile accepted")
	}
	if platform.FailureCode(err) != platform.CodeInvalidInput || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("unsafe or wrong failure: %v", err)
	}
}
