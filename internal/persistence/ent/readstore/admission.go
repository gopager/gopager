package readstore

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/gopager/gopager/internal/platform"
	"github.com/gopager/gopager/internal/readmodel"
)

var _ readmodel.AdmissionStore = (*Store)(nil)

// expiredSweepLimit bounds how many expired windows of other buckets one admitted request
// deletes. Each admitted request adds at most one row, so the sweep removes expired rows at
// least as fast as they arise and the table stays proportional to the current minute's callers.
const expiredSweepLimit = 100

// TakeSearchAttempt implements readmodel.AdmissionStore. The upsert increments only below the
// limit, so concurrent requests of one bucket serialize on its row and a rejected request
// changes nothing. In the same transaction an admitted request deletes the older windows of its
// own bucket and up to expiredSweepLimit expired windows of any bucket, so buckets that never
// return (one-time guest sources, revoked keys) do not stay behind; rows locked by a concurrent
// sweep are skipped instead of waited for.
func (s *Store) TakeSearchAttempt(ctx context.Context, tx platform.TransactionContext, bucket [32]byte,
	windowStart time.Time, limit int) (bool, error) {
	executor, err := s.primary.Resolve(tx)
	if err != nil {
		return false, err
	}
	var count int
	err = executor.QueryRowContext(ctx, `INSERT INTO search_admissions (bucket, window_start, count) VALUES ($1, $2, 1)
		ON CONFLICT (bucket, window_start) DO UPDATE SET count = search_admissions.count + 1
		WHERE search_admissions.count < $3 RETURNING count`, bucket[:], windowStart.UTC(), limit).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM search_admissions WHERE bucket = $1 AND window_start < $2`,
		bucket[:], windowStart.UTC()); err != nil {
		return false, err
	}
	if _, err := executor.ExecContext(ctx, `DELETE FROM search_admissions WHERE (bucket, window_start) IN (
		SELECT bucket, window_start FROM search_admissions WHERE window_start < $1
		ORDER BY window_start LIMIT $2 FOR UPDATE SKIP LOCKED)`, windowStart.UTC(), expiredSweepLimit); err != nil {
		return false, err
	}
	return true, nil
}
