package processing

import (
	"fmt"
	"os"
	"testing"

	"github.com/gopager/gopager/internal/testutil/wasmcache"
)

// TestMain reuses compiled test guests across runs; compilation dominates otherwise.
func TestMain(m *testing.M) {
	if dir := wasmcache.Dir(); dir != "" {
		if err := UseCompilationCacheDir(dir); err != nil {
			fmt.Fprintln(os.Stderr, "WASM test cache unavailable; compiling in memory:", err)
		}
	}
	os.Exit(m.Run())
}
