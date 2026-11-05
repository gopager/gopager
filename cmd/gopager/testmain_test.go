package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/gopager/gopager/internal/processing"
	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
	"github.com/gopager/gopager/internal/testutil/wasmcache"
)

// TestMain shares fixture setup but leaves the actual command subprocess unchanged.
func TestMain(m *testing.M) {
	useWASMTestCache()
	if os.Getenv("GOPAGER_AP05_PROCESS_HELPER") != "" {
		os.Exit(m.Run())
	}
	os.Exit(testpostgres.Run(m))
}

// useWASMTestCache reuses compiled test guests across runs; compilation dominates otherwise.
func useWASMTestCache() {
	if dir := wasmcache.Dir(); dir != "" {
		if err := processing.UseCompilationCacheDir(dir); err != nil {
			fmt.Fprintln(os.Stderr, "WASM test cache unavailable; compiling in memory:", err)
		}
	}
}
