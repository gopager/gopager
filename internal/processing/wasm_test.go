package processing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gopager/gopager/internal/platform"
)

func TestWASIOutputBoundaryRemainsFailed(t *testing.T) {
	output := &boundedOutput{limit: MaxOutputBytes}
	if n, err := output.Write([]byte(strings.Repeat(" ", MaxOutputBytes))); err != nil || n != MaxOutputBytes {
		t.Fatalf("exact output boundary failed: %d, %v", n, err)
	}
	if _, err := output.Write([]byte("x")); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("output +1 accepted: %v", err)
	}
	if _, err := output.Write(nil); platform.FailureCode(err) != platform.CodeResourceLimit ||
		platform.FailureCode(output.err) != platform.CodeResourceLimit {
		t.Fatalf("overflow became successful: %v", err)
	}
}

func TestSuppliedWASIRegexGuest(t *testing.T) {
	ctx := context.Background()
	host, err := NewWASMHost(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := host.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	pattern := `(?P<unit>A)(?P<number>[0-9]+)?`
	result, err := host.Regex(ctx, pattern, nil)
	if err != nil || result.Matched || strings.Join(result.Names, ",") != ",unit,number" {
		t.Fatalf("inspection: %+v, %v", result, err)
	}
	text := "A"
	result, err = host.Regex(ctx, pattern, &text)
	if err != nil || !result.Matched || len(result.Captures) != 2 ||
		result.Captures[0].Text == nil || *result.Captures[0].Text != "A" || result.Captures[1].Text != nil {
		t.Fatalf("unmatched capture lost absence: %+v, %v", result, err)
	}
	text = "no match"
	result, err = host.Regex(ctx, pattern, &text)
	if err != nil || result.Matched || len(result.Captures) != 0 {
		t.Fatalf("no-match became a failure: %+v, %v", result, err)
	}
	text = "A123"
	result, err = host.Regex(ctx, pattern, &text)
	if err != nil || !result.Matched || *result.Captures[1].Text != "123" {
		t.Fatalf("capture: %+v, %v", result, err)
	}
	for _, pattern := range []string{`(?<=secret)a`, `(a)\1`} {
		if _, err := host.Regex(ctx, pattern, nil); platform.FailureCode(err) != platform.CodeInvalidInput ||
			strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsupported pattern was not safely rejected: %v", err)
		}
	}
	if _, err := host.Regex(ctx, strings.Repeat("a", 4096), nil); err != nil {
		t.Fatalf("exact pattern boundary failed: %v", err)
	}
	if _, err := host.Regex(ctx, strings.Repeat("a", 4097), nil); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("pattern +1 accepted: %v", err)
	}
	// JSON escaping exceeds the output bound without a long regex scan.
	text = strings.Repeat("\x00", MaxOutputBytes/6+1)
	if _, err := host.Regex(ctx, `(?P<large>.*)`, &text); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("oversized capture was accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := host.Regex(cancelled, pattern, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled invocation started: %v", err)
	}
	text = "A"
	if result, err := host.Regex(ctx, pattern, &text); err != nil || !result.Matched {
		t.Fatalf("host did not survive rejected invocation: %+v, %v", result, err)
	}
	if err := host.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Regex(ctx, pattern, nil); err == nil {
		t.Fatal("closed host accepted invocation")
	}
}
