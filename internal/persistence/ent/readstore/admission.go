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

// TakeSearchAttempt implements readmodel.AdmissionStore. The upsert increments only below the
// limit, so concurrent requests of one bucket serialize on its row and a rejected request
// changes nothing. Older windows of the bucket are deleted in the same transaction, which keeps
// one row per bucket.
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
	return true, nil
}
