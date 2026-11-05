// Package postgres implements platform.Store and platform.Clock on PostgreSQL
// via pgx's database/sql driver.
//
// NewStore refuses remote connections without certificate-verified TLS unless
// explicitly allowed. Store.Migrate checks or applies Goose migrations under a
// PostgreSQL session advisory lock and, on success, registers the migration
// set so that every later WithinTransaction re-verifies schema compatibility
// before beginning.
//
// WithinTransaction runs one READ COMMITTED transaction on a verified writable
// primary and hands callbacks an opaque platform.TransactionContext;
// repositories obtain the underlying Executor through Store.Resolve, which
// rejects capabilities from another store or from a finished callback.
// Nested transactions are rejected with CodeInvalidInput. Database errors are
// mapped to platform codes: serialization failures, deadlocks and unique
// violations become CodeRevisionConflict, read-only transactions
// CodeAuthorityUnavailable, numeric overflow and resource exhaustion
// CodeResourceLimit, and everything else CodeStorageUnavailable. A commit
// whose outcome is unknown (transport failure, connection-class errors)
// yields CodeCommitUncertain and is never retried.
package postgres
