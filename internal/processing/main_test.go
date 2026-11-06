package processing

import (
	"os"
	"testing"

	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

// TestMain exports a Core-only secret once for the whole process: tests run in parallel, so a
// per-test t.Setenv is not possible. The plugin environment allowlist must keep it out of plugins
// (see TestInstalledPluginWireInvocationRestartAndIsolation); no other test reads the variable.
func TestMain(m *testing.M) {
	if err := os.Setenv("GOPAGER_TEST_CORE_SECRET", "must-not-reach-plugins"); err != nil {
		panic(err)
	}
	code := m.Run()
	pluginfixture.Cleanup()
	os.Exit(code)
}
