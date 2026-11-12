package rowcheck_test

import (
	"errors"
	"testing"

	"github.com/gopager/gopager/internal/authz"
	"github.com/gopager/gopager/internal/persistence/rowcheck"
)

// result is an sql.Result with a fixed row count or counting failure.
type result struct {
	rows int64
	err  error
}

func (r result) LastInsertId() (int64, error) { return 0, errors.New("unsupported") }
func (r result) RowsAffected() (int64, error) { return r.rows, r.err }

func TestRequireOne(t *testing.T) {
	t.Parallel()
	statement, counting := errors.New("statement"), errors.New("counting")
	for _, tc := range []struct {
		name   string
		result result
		err    error
		want   error
	}{
		{"one row", result{rows: 1}, nil, nil},
		{"no row", result{rows: 0}, nil, authz.ErrRevisionConflict},
		{"two rows", result{rows: 2}, nil, authz.ErrRevisionConflict},
		{"statement failed", result{}, statement, statement},
		{"count failed", result{err: counting}, nil, counting},
	} {
		if got := rowcheck.RequireOne(tc.result, tc.err); !errors.Is(got, tc.want) || (tc.want == nil) != (got == nil) {
			t.Errorf("%s: RequireOne = %v, want %v", tc.name, got, tc.want)
		}
	}
}
