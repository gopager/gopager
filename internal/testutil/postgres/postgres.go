// Package postgres supplies disposable PostgreSQL infrastructure to test entry points.
// Production packages must not import test utilities.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

const image = "postgres:18.0-alpine@sha256:48c8ad3a7284b82be4482a52076d47d879fd6fb084a1cbfccbd551f9331b0e40"

// Run executes a package's tests, starting an owned fixture only when requested.
// An explicit disposable DSN overrides container creation; setup/cleanup errors fail the run.
func Run(m *testing.M) (code int) {
	mode := os.Getenv("GOPAGER_INTEGRATION")
	if mode != "" && mode != "1" {
		fmt.Fprintln(os.Stderr, "test PostgreSQL: GOPAGER_INTEGRATION must be unset or 1")
		return 1
	}
	dsn := os.Getenv("GOPAGER_TEST_DATABASE_URL")
	if mode == "" && dsn == "" {
		return m.Run()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if dsn == "" {
		container, err := tcpostgres.Run(ctx, image,
			tcpostgres.WithDatabase("gopager_test"),
			tcpostgres.WithUsername("gopager_test"),
			tcpostgres.WithPassword("disposable-test-only"),
			tcpostgres.WithSQLDriver("pgx"),
			// The module defaults to fsync=off; keep normal database durability settings.
			testcontainers.WithCmd("postgres", "-c", "fsync=on"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
				wait.ForListeningPort("5432/tcp"),
			),
		)
		if container != nil {
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if err := container.Terminate(cleanup); err != nil {
					fmt.Fprintln(os.Stderr, "test PostgreSQL: container cleanup failed")
					code = 1
				}
			}()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "test PostgreSQL: container startup failed; check Docker availability and image access")
			return 1
		}
		dsn, err = container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			fmt.Fprintln(os.Stderr, "test PostgreSQL: could not resolve fixture address")
			return 1
		}
		if err := os.Setenv("GOPAGER_TEST_DATABASE_URL", dsn); err != nil {
			fmt.Fprintln(os.Stderr, "test PostgreSQL: could not configure fixture environment")
			return 1
		}
		defer func() {
			if err := os.Unsetenv("GOPAGER_TEST_DATABASE_URL"); err != nil {
				fmt.Fprintln(os.Stderr, "test PostgreSQL: could not clear fixture environment")
				code = 1
			}
		}()
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test PostgreSQL: invalid fixture connection configuration")
		return 1
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "test PostgreSQL: connection cleanup failed")
			code = 1
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "test PostgreSQL: fixture connection failed")
		return 1
	}
	fmt.Fprintln(os.Stderr, "test PostgreSQL: fixture ready")
	return m.Run()
}
