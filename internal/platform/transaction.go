package platform

import "context"

// TransactionContext is an opaque participation capability valid only in its callback.
// Only infrastructure repository adapters may resolve its store-specific transaction.
type TransactionContext interface {
	Context() context.Context
}

// TxFunc is a transaction callback. It must not retain the TransactionContext
// beyond its return; returning a non-nil error rolls the transaction back.
type TxFunc func(TransactionContext) error
