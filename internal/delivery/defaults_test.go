package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/processing"
)

func TestDefaultLayoutsCompileAgainstTheCatalog(t *testing.T) {
	t.Parallel()
	fields := newFieldSet(context.Background(), nil, nil)
	for kind, layout := range DefaultLayouts() {
		program, err := processing.CompileLayout(context.Background(), layout, fields)
		if err != nil {
			t.Fatalf("%s: %v (%v)", kind, err, errors.Unwrap(err))
		}
		if program.OutputKind() != kind {
			t.Fatalf("%s: output kind %s", kind, program.OutputKind())
		}
	}
}
