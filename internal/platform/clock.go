package platform

import (
	"context"
	"time"
)

type Clock interface {
	Now(context.Context, TransactionContext) (time.Time, error)
}
