package postgres

import (
	"context"
	"os"
	"testing"

	testpostgres "github.com/gopager/gopager/internal/testutil/postgres"
)

// TestMain binds this package to the shared optional PostgreSQL fixture lifecycle.
func TestMain(m *testing.M) {
	os.Exit(testpostgres.Run(m))
}

func TestPostgresFixtureKeepsFsyncEnabled(t *testing.T) {
	store := newTestStore(t)
	var enabled bool
	if err := store.db.QueryRowContext(context.Background(), "SELECT current_setting('fsync')::boolean").Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("PostgreSQL test fixture must not disable fsync")
	}
}
