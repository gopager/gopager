package plugintest

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BuildPlugin compiles the main package of a plugin into the test's temporary directory and
// returns the path of the executable. moduleDir is the directory of the plugin's Go module and
// mainPackage the package to build, relative to it (for example "."). The build is static and
// path-trimmed, like the one that produces the shipped artifact. A failing build fails the test.
func BuildPlugin(t testing.TB, moduleDir, mainPackage string) string {
	t.Helper()
	directory, err := filepath.Abs(moduleDir)
	if err != nil {
		t.Fatalf("plugintest: resolve module directory: %v", err)
	}
	output := filepath.Join(t.TempDir(), "plugin")
	command := exec.Command("go", "build", "-trimpath", "-o", output, mainPackage)
	command.Dir = directory
	command.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("plugintest: build %s in %s: %v\n%s", mainPackage, directory, err, out)
	}
	return output
}
