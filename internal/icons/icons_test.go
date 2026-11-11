package icons

import (
	"strings"
	"testing"
)

func TestValid(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"a", "fire-truck", "mdi.bell_ring", "A9", strings.Repeat("x", MaxLength)} {
		if !Valid(value) {
			t.Errorf("Valid(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "with space", "slash/no", "ümlaut", "emoji🔥", strings.Repeat("x", MaxLength+1)} {
		if Valid(value) {
			t.Errorf("Valid(%q) = true, want false", value)
		}
	}
}
