package platform

import "context"

// TransactionContext is an opaque participation capability valid only in its callback.
// Only infrastructure repository adapters may resolve its store-specific transaction.
type TransactionContext interface {
	Context() context.Context
}

// TxFunc is a transaction callback. It must not retain the TransactionContext
// beyond its return; returning a non-nil error rolls the transaction back. The
// store may run the callback a second time in a new transaction after the server
// rolled the first back with a serialization failure or deadlock, so the callback
// must keep its side effects inside the transaction and reset any state it
// captures from outside.
type TxFunc func(TransactionContext) error
