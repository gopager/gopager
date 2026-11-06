package processing

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gopager/gopager/internal/platform"
)

func TestCoreRegexSemanticsAndBoundaries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	regex := CoreRegex{}
	pattern := `(?P<unit>A)(?P<number>[0-9]+)?`
	result, err := regex.Regex(ctx, pattern, nil)
	if err != nil || result.Matched || strings.Join(result.Names, ",") != ",unit,number" {
		t.Fatalf("inspection: %+v, %v", result, err)
	}
	text := "A"
	result, err = regex.Regex(ctx, pattern, &text)
	if err != nil || !result.Matched || len(result.Captures) != 2 ||
		result.Captures[0].Text == nil || *result.Captures[0].Text != "A" || result.Captures[1].Text != nil {
		t.Fatalf("unmatched capture lost absence: %+v, %v", result, err)
	}
	text = "no match"
	result, err = regex.Regex(ctx, pattern, &text)
	if err != nil || result.Matched || len(result.Captures) != 0 {
		t.Fatalf("no-match became a failure: %+v, %v", result, err)
	}
	text = "xA123"
	result, err = regex.Regex(ctx, pattern, &text)
	if err != nil || !result.Matched || *result.Captures[1].Text != "123" {
		t.Fatalf("capture: %+v, %v", result, err)
	}
	text = "äöü A7"
	result, err = regex.Regex(ctx, pattern, &text)
	if err != nil || !result.Matched || *result.Captures[0].Text != "A" || *result.Captures[1].Text != "7" {
		t.Fatalf("multi-byte input shifted byte indexes: %+v, %v", result, err)
	}
	for _, unsupported := range []string{`(?<=secret)a`, `(a)\1`} {
		if _, err := regex.Regex(ctx, unsupported, nil); platform.FailureCode(err) != platform.CodeInvalidInput ||
			strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsupported pattern was not safely rejected: %v", err)
		}
	}
	if _, err := regex.Regex(ctx, strings.Repeat("a", maxRegexPatternBytes), nil); err != nil {
		t.Fatalf("exact pattern boundary failed: %v", err)
	}
	if _, err := regex.Regex(ctx, strings.Repeat("a", maxRegexPatternBytes+1), nil); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("pattern +1 accepted: %v", err)
	}
	// The anchored pattern matches at once, so this checks text-size admission at the exact
	// boundary independently of matching speed (an unanchored scan of 1 MiB under the race
	// detector can exceed the per-call time budget).
	text = strings.Repeat("x", maxRegexTextBytes)
	if result, err := regex.Regex(ctx, `^(?P<head>x)`, &text); err != nil || !result.Matched {
		t.Fatalf("exact text boundary failed: %+v, %v", result, err)
	}
	text += "x"
	if _, err := regex.Regex(ctx, `x`, &text); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("text +1 accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := regex.Regex(cancelled, pattern, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled invocation started: %v", err)
	}
}

// captureExpansionCounterexample is the capture-expansion counterexample that OOM-killed a 512 MiB native process.
func captureExpansionCounterexample() string {
	var pattern strings.Builder
	for n := 0; pattern.Len() < 4070; n++ {
		fmt.Fprintf(&pattern, "(?P<c%d>a?){1000}", n)
	}
	pattern.WriteString("b")
	return pattern.String()
}

func TestCoreRegexRejectsCaptureExpansionBeforeCompilation(t *testing.T) {
	t.Parallel()
	pattern := captureExpansionCounterexample()
	if len(pattern) != 4085 {
		t.Fatalf("counterexample drifted: %d bytes", len(pattern))
	}
	text := strings.Repeat("a", 1024)
	start := time.Now()
	if _, err := (CoreRegex{}).Regex(context.Background(), pattern, &text); platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatalf("capture expansion was admitted: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("rejection compiled the oversized program first: %s", elapsed)
	}
	for _, oversized := range []string{`(a{1000}){1000}`, `((((a{100}){100}){100}){100})`, `(?:x{1000}){30}`} {
		// Go's parser already rejects some nested repeats; either rejection is a safe non-admission.
		code := platform.FailureCode(func() error { _, err := (CoreRegex{}).Regex(context.Background(), oversized, nil); return err }())
		if code != platform.CodeResourceLimit && code != platform.CodeInvalidInput {
			err := errors.New(string(code))
			t.Fatalf("%s was admitted: %v", oversized, err)
		}
	}
}

func TestCoreRegexAdmittedWorstCaseStaysBoundedAndCancellable(t *testing.T) { //nolint:paralleltest // asserts elapsed wall-clock time, which CPU contention from parallel tests inflates
	// Close to the instruction bound with captures: admitted, and the full 1 MiB scan still terminates.
	pattern := `(?P<a>x?){1000}(?P<b>x?){1000}y`
	text := strings.Repeat("x", maxRegexTextBytes)
	result, err := (CoreRegex{}).Regex(context.Background(), pattern, nil)
	if err != nil || len(result.Names) != 3 {
		t.Fatalf("near-bound pattern rejected: %+v %v", result, err)
	}
	// The scan without a caller deadline runs until the call's own budget ends it, which is the
	// cost of running the scan to completion on this machine and under this load. The cancelled
	// scan is compared with that measurement instead of a fixed bound, because the race detector
	// and a loaded machine slow both by the same factor.
	start := time.Now()
	if _, err := (CoreRegex{}).Regex(context.Background(), pattern, &text); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("uncancelled scan did not run into the call budget: %v", err)
	}
	uncancelled := time.Since(start)
	const deadline = 20 * time.Millisecond
	if uncancelled < 10*deadline {
		t.Fatalf("scan finished in %s, too fast to show cancellation", uncancelled)
	}
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	start = time.Now()
	if _, err := (CoreRegex{}).Regex(ctx, pattern, &text); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired scan was not reported as failure: %v", err)
	}
	if elapsed := time.Since(start); elapsed > uncancelled/2 {
		t.Fatalf("cancellation was not cooperative: cancelled scan took %s, uncancelled %s", elapsed, uncancelled)
	}
}

// TestCancellableReaderStopsWithinOneInterval pins cooperative cancellation by counting runes, which
// does not depend on how fast the machine is.
func TestCancellableReaderStopsWithinOneInterval(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancellableReader{ctx: ctx, source: strings.NewReader(strings.Repeat("x", 10*regexCancelInterval))}
	cancel()
	reads := 0
	for {
		if _, _, err := reader.ReadRune(); err != nil {
			break
		}
		reads++
	}
	if reads >= regexCancelInterval {
		t.Fatalf("read %d runes after cancellation, want fewer than %d", reads, regexCancelInterval)
	}
}
