// Package rowcheck checks the outcome of the stores' single-row writes, which are guarded by a
// revision or a claim in their WHERE clause.
package rowcheck

import (
	"database/sql"

	"github.com/gopager/gopager/internal/authz"
)

// RequireOne returns err when the statement failed, the failure to count the affected rows, or
// authz.ErrRevisionConflict when the statement did not affect exactly one row: the row changed,
// vanished or was claimed by someone else between the read and the write. It returns nil for
// exactly one affected row.
func RequireOne(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return authz.ErrRevisionConflict
	}
	return nil
}
