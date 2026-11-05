package main

import (
	"os"
	"testing"

	"github.com/gopager/gopager/internal/testutil/pluginfixture"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// TestMain shares fixture setup but leaves the actual command subprocess unchanged.
func TestMain(m *testing.M) {
	if os.Getenv("GOPAGER_AP05_PROCESS_HELPER") != "" {
		os.Exit(m.Run())
	}
	code := testpostgres.Run(m)
	pluginfixture.Cleanup()
	os.Exit(code)
}
