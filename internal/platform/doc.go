// Package platform is the process foundation shared by every GoPager role:
// configuration, error codes, transaction boundaries, health endpoints and
// the lifecycle of a running process.
//
// Start/StartWithAPI validate a Config, verify the Store (connectivity, write
// authority on a primary, and an exactly current migration schema, or apply
// migrations in MigrationApply mode) and then compose the requested roles into
// a RunningHandle: the API role serves health routes plus optional application
// routes on one listener, the worker role runs the registered Runners. A
// background monitor re-checks the store periodically and toggles readiness.
// RunningHandle.Shutdown closes admission, cancels and drains in-flight work
// and finally closes the store; it is idempotent.
//
// All domain work runs through WithinTransaction with a TxFunc that receives
// an opaque TransactionContext. Only infrastructure adapters may resolve that
// capability to a concrete database transaction, and it is invalid after the
// callback returns. Nested transactions are rejected.
//
// Every error crossing the platform boundary is a *Failure carrying an
// ErrorCode. Failure messages are generic fallback texts; private causes are
// never rendered or logged. CodeCommitUncertain is never retried automatically.
package platform
