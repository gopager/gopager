package platform

import (
	"context"
	"time"
)

// Clock supplies the authoritative current time for domain decisions. Now is
// evaluated through the caller's open transaction so every participant in one
// transaction reads the same database-side clock rather than a worker's wall
// clock; it fails with the store's failure code if the transaction capability
// is invalid or expired.
type Clock interface {
	Now(context.Context, TransactionContext) (time.Time, error)
}
