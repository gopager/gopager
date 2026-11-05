package commands

import (
	"errors"
	"math"
	"testing"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/platform"
)

func TestNextRevisionChecksStalenessAndExhaustionWithoutWrapping(t *testing.T) {
	if next, err := NextRevision(math.MaxInt64-1, math.MaxInt64-1); err != nil || next != math.MaxInt64 {
		t.Fatal("last representable revision was rejected")
	}
	if next, err := NextRevision(math.MaxInt64, math.MaxInt64); next != 0 || platform.FailureCode(err) != platform.CodeResourceLimit {
		t.Fatal("revision exhaustion did not return RESOURCE_LIMIT")
	}
	for _, pair := range [][2]int64{{1, math.MaxInt64}, {0, 0}, {2, 1}} {
		if next, err := NextRevision(pair[0], pair[1]); next != 0 || !errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatal("stale revision was mistaken for exhaustion")
		}
	}
}

func TestReceiptCheckReplaysIdenticalInputAndRejectsConflictingReuse(t *testing.T) {
	first, err := Fingerprint(map[string]int{"a": 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Fingerprint(map[string]int{"a": 2})
	if err != nil {
		t.Fatal(err)
	}
	receipt := Receipt{Operation: "unit.create", Fingerprint: first, Result: Result{Revision: 1}}
	if result, err := receipt.Check("unit.create", first); err != nil || result.Revision != 1 {
		t.Fatal("identical replay did not return the stored result")
	}
	for _, check := range []struct {
		operation   string
		fingerprint [32]byte
	}{{"unit.create", second}, {"unit.delete", first}} {
		_, err := receipt.Check(check.operation, check.fingerprint)
		if !errors.Is(err, ErrIdempotencyConflict) || errors.Is(err, authz.ErrRevisionConflict) {
			t.Fatal("conflicting command reuse was not an idempotency conflict")
		}
	}
}
