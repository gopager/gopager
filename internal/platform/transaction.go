package platform

import "context"

// TransactionContext is an opaque participation capability valid only in its callback.
// Only infrastructure repository adapters may resolve its store-specific transaction.
type TransactionContext interface {
	Context() context.Context
}

type TxFunc func(TransactionContext) error
