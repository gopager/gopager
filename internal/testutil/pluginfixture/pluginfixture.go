// Package pluginfixture builds the supplied test plugin executables once per test process.
// Production packages must not import test utilities.
package pluginfixture

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	mu    sync.Mutex
	dir   string
	built = map[string][]byte{}
)

// Binary returns the bytes of internal/processing/testdata/<name> compiled for the host platform.
func Binary(t testing.TB, name string) []byte {
	t.Helper()
	mu.Lock()
	defer mu.Unlock()
	if data, ok := built[name]; ok {
		return data
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("pluginfixture: cannot locate repository")
	}
	repository := filepath.Join(filepath.Dir(source), "..", "..", "..")
	if dir == "" {
		var err error
		if dir, err = os.MkdirTemp("", "gopager-plugin-fixtures-"); err != nil {
			t.Fatal(err)
		}
	}
	output := filepath.Join(dir, name)
	command := exec.Command("go", "build", "-trimpath", "-o", output, "./internal/processing/testdata/"+name)
	command.Dir = repository
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("pluginfixture: build %s: %v\n%s", name, err, out)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	built[name] = data
	return data
}

// Cleanup removes built fixtures; TestMain calls it after m.Run.
func Cleanup() {
	mu.Lock()
	defer mu.Unlock()
	if dir != "" {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintln(os.Stderr, "pluginfixture: cleanup:", err)
		}
		dir = ""
	}
}
