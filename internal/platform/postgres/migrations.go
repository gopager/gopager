package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/database"
	"github.com/pressly/goose/v3/lock"

	"github.com/gopager/gopager/internal/platform"
)

const migrationLockKey int64 = 0x47504D4947520001
const ledgerName = "platform_schema_migrations"

var errIncompatibleLedger = errors.New("incompatible migration ledger")

// Migrate checks or applies a release's immutable SQL filesystem under a session lock
// and records the resulting applied schema version for WithinTransaction. A nil
// filesystem registers no domain migrations; only apply may initialize history.
func (s *Store) Migrate(ctx context.Context, migrations fs.FS, mode platform.MigrationMode) (result error) {
	if mode != platform.MigrationCheck && mode != platform.MigrationApply {
		return platform.NewFailure(platform.CodeInvalidInput, errors.New("unknown migration mode"))
	}
	provider, err := migrationProvider(s.db, migrations)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, platform.DefaultCallTimeout)
	defer cancel()
	// Reserve at most one lock session per pool; provider execution needs another connection.
	s.migrationOnce.Do(func() { s.migrationGate = make(chan struct{}, 1) })
	select {
	case s.migrationGate <- struct{}{}:
		defer func() { <-s.migrationGate }()
	case <-ctx.Done():
		return classifyFailure(ctx.Err())
	}
	locker, err := lock.NewPostgresSessionLocker(
		lock.WithLockID(migrationLockKey),
		lock.WithLockTimeout(1, 30),
		lock.WithUnlockTimeout(1, 2),
	)
	if err != nil {
		return platform.NewFailure(platform.CodeInternalError, err)
	}
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return classifyFailure(err)
	}
	unlocked := false
	defer func() {
		if !unlocked {
			// Never return a possibly still locked physical session to the pool.
			err := conn.Raw(func(any) error { return driver.ErrBadConn })
			if err != nil && !errors.Is(err, driver.ErrBadConn) && !errors.Is(err, sql.ErrConnDone) {
				result = errors.Join(result, classifyFailure(err))
			}
		}
		if err := conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			result = errors.Join(result, classifyFailure(err))
		}
	}()
	if err := locker.SessionLock(ctx, conn); err != nil {
		return classifyFailure(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := locker.SessionUnlock(cleanup, conn); err != nil {
			result = errors.Join(result, classifyFailure(err))
			return
		}
		unlocked = true
	}()
	version, err := s.migrateLocked(ctx, conn, provider, mode)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.migrations, s.registered, s.schemaVersion = migrations, true, version
	s.mu.Unlock()
	return nil
}

// migrationProvider delegates discovery, ordering and SQL parsing/execution to Goose.
// Only its two opt-out directives are restricted by the foundation contract.
func migrationProvider(db *sql.DB, migrations fs.FS) (*goose.Provider, error) {
	if migrations != nil {
		entries, err := fs.ReadDir(migrations, ".")
		if err != nil {
			return nil, platform.NewFailure(platform.CodeInvalidInput, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
				return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("migration collection must contain SQL files only"))
			}
			if _, err := goose.NumericComponent(entry.Name()); err != nil {
				return nil, platform.NewFailure(platform.CodeInvalidInput, err)
			}
		}
	}
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations,
		goose.WithTableName(ledgerName), goose.WithDisableGlobalRegistry(true), goose.WithSlog(slog.Default()))
	if errors.Is(err, goose.ErrNoMigrations) {
		return nil, nil
	}
	if err != nil {
		return nil, platform.NewFailure(platform.CodeInvalidInput, err)
	}
	for _, source := range p.ListSources() {
		contents, err := fs.ReadFile(migrations, source.Path)
		if err != nil {
			return nil, platform.NewFailure(platform.CodeInvalidInput, err)
		}
		if source.Version <= 0 || !utf8.Valid(contents) || len(strings.TrimSpace(string(contents))) == 0 {
			return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("invalid migration source"))
		}
		for _, line := range strings.Split(string(contents), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 3 && fields[0] == "--" && fields[1] == "+goose" {
				annotation := strings.Join(fields[2:], " ")
				if strings.HasPrefix(annotation, "NO TRANSACTION") || strings.HasPrefix(annotation, "ENVSUB") {
					return nil, platform.NewFailure(platform.CodeInvalidInput, errors.New("unsupported migration directive"))
				}
			}
		}
	}
	return p, nil
}

// migrateLocked keeps strict compatibility checks and library execution in one lock
// interval and returns the applied schema version (the last applied version_id).
func (s *Store) migrateLocked(ctx context.Context, conn *sql.Conn, p *goose.Provider, mode platform.MigrationMode) (int64, error) {
	var ledger sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT to_regclass('platform_schema_migrations')").Scan(&ledger); err != nil {
		return 0, classifyFailure(err)
	}
	if !ledger.Valid {
		if mode == platform.MigrationCheck {
			return 0, platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("migration ledger is pending"))
		}
		versionStore, err := database.NewStore(database.DialectPostgres, ledgerName)
		if err != nil {
			return 0, platform.NewFailure(platform.CodeInternalError, err)
		}
		tx, err := conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return 0, classifyFailure(err)
		}
		if err := versionStore.CreateVersionTable(ctx, tx); err != nil {
			return 0, classifyFailure(errors.Join(err, rollback(tx)))
		}
		if err := versionStore.Insert(ctx, tx, database.InsertRequest{Version: 0}); err != nil {
			return 0, classifyFailure(errors.Join(err, rollback(tx)))
		}
		if err := tx.Commit(); err != nil {
			if isUnknownCommit(err) {
				return 0, platform.NewFailure(platform.CodeCommitUncertain, err)
			}
			return 0, classifyFailure(err)
		}
	}
	if err := validateLedger(ctx, conn); err != nil {
		if errors.Is(err, errIncompatibleLedger) {
			return 0, platform.NewFailure(platform.CodeIncompatibleVersion, err)
		}
		return 0, classifyFailure(err)
	}
	var sources []*goose.Source
	if p != nil {
		sources = p.ListSources()
	}
	pending, version, err := validateHistory(ctx, conn, sources)
	if err != nil {
		return 0, err
	}
	if !pending {
		return version, nil
	}
	if mode == platform.MigrationCheck {
		return 0, platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("migration suffix is pending"))
	}
	// The dedicated session above holds the lock throughout this provider call.
	// Goose uses a second pooled connection and owns each SQL+history transaction.
	if _, err := p.Up(ctx); err != nil {
		var partial *goose.PartialError
		if errors.As(err, &partial) && isUnknownCommit(err) {
			return 0, platform.NewFailure(platform.CodeCommitUncertain, err)
		}
		return 0, classifyFailure(err)
	}
	pending, version, err = validateHistory(ctx, conn, sources)
	if err != nil {
		return 0, err
	}
	if pending {
		return 0, platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("migration suffix remains pending"))
	}
	return version, nil
}

// validateLedger rejects the previous custom layout rather than converting it silently.
func validateLedger(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'platform_schema_migrations'
		ORDER BY ordinal_position
	`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type column struct{ name, dataType, nullable string }
	var found []column
	for rows.Next() {
		var c column
		if err := rows.Scan(&c.name, &c.dataType, &c.nullable); err != nil {
			return err
		}
		found = append(found, c)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !slices.Equal(found, []column{
		{"id", "integer", "NO"}, {"version_id", "bigint", "NO"},
		{"is_applied", "boolean", "NO"}, {"tstamp", "timestamp without time zone", "NO"},
	}) {
		return errIncompatibleLedger
	}
	var valid bool
	if err := conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conrelid = 'platform_schema_migrations'::regclass
			  AND contype = 'p' AND pg_get_constraintdef(oid) = 'PRIMARY KEY (id)'
		) AND EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = 'platform_schema_migrations'
			  AND column_name = 'id' AND is_identity = 'YES'
		)
	`).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errIncompatibleLedger
	}
	return nil
}

// validateHistory compares ordered identities, never historical content bytes. It
// reports whether registered migrations are still pending and the last applied version.
func validateHistory(ctx context.Context, conn *sql.Conn, sources []*goose.Source) (bool, int64, error) {
	rows, err := conn.QueryContext(ctx, "SELECT version_id, is_applied FROM platform_schema_migrations ORDER BY id")
	if err != nil {
		return false, 0, classifyFailure(err)
	}
	defer rows.Close()
	index := -1
	var last int64
	for rows.Next() {
		var version int64
		var applied bool
		if err := rows.Scan(&version, &applied); err != nil {
			return false, 0, classifyFailure(err)
		}
		expected := int64(0)
		if index >= 0 && index < len(sources) {
			expected = sources[index].Version
		}
		if !applied || index >= len(sources) || version != expected {
			return false, 0, platform.NewFailure(platform.CodeIncompatibleVersion, errors.New("migration history is not the registered prefix"))
		}
		last = version
		index++
	}
	if err := rows.Err(); err != nil {
		return false, 0, classifyFailure(err)
	}
	if index < 0 {
		return false, 0, platform.NewFailure(platform.CodeIncompatibleVersion, fmt.Errorf("missing initial migration history"))
	}
	return index < len(sources), last, nil
}
