// Package wasmcache locates the persistent compiled-guest cache shared by test binaries.
// Production packages must not import test utilities.
package wasmcache

import (
	"os"
	"path/filepath"
)

// Dir returns the per-user cache directory for compiled WASM test guests, or "" when disabled.
// GOPAGER_TEST_WASM_CACHE overrides the location; the value "off" disables persistence so each
// test binary compiles once in memory.
func Dir() string {
	switch value := os.Getenv("GOPAGER_TEST_WASM_CACHE"); value {
	case "off":
		return ""
	case "":
	default:
		return value
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "gopager", "wazero-test")
}
