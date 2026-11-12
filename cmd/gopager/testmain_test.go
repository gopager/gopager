package main

import (
	"os"
	"testing"

	"github.com/gopager/gopager/internal/identity"
	"github.com/gopager/gopager/internal/testutil/passwords"
	"github.com/gopager/gopager/internal/testutil/pluginfixture"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// TestMain shares fixture setup but leaves the actual command subprocess unchanged.
func TestMain(m *testing.M) {
	// Tests log in constantly; the production password cost would dominate their run time.
	identityOptions = []identity.Option{passwords.Option()}
	if os.Getenv("GOPAGER_TEST_PROCESS_HELPER") != "" {
		os.Exit(m.Run())
	}
	code := testpostgres.Run(m)
	pluginfixture.Cleanup()
	os.Exit(code)
}
