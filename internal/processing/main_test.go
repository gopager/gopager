package processing

import (
	"os"
	"testing"

	"github.com/gopager/gopager/internal/testutil/pluginfixture"
)

func TestMain(m *testing.M) {
	code := m.Run()
	pluginfixture.Cleanup()
	os.Exit(code)
}
