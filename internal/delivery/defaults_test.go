package delivery

import (
	"context"
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/fields"
	"github.com/gopager/gopager/internal/processing"
)

func TestDefaultLayoutsCompileAgainstTheRegistry(t *testing.T) {
	t.Parallel()
	declarations := fields.New(fields.Builtin()).Declarations(fields.ScopeDelivery, true)
	for kind, layout := range DefaultLayouts() {
		program, err := processing.CompileLayout(context.Background(), layout, declarations)
		if err != nil {
			t.Fatalf("%s: %v (%v)", kind, err, errors.Unwrap(err))
		}
		if program.OutputKind() != kind {
			t.Fatalf("%s: output kind %s", kind, program.OutputKind())
		}
	}
}
