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

// TestOwnedPostgresFixtureIsNonDurable pins the speed settings of the container started by
// Run. An external DSN may legitimately be durable, so it is not asserted.
func TestOwnedPostgresFixtureIsNonDurable(t *testing.T) {
	t.Parallel()
	store := newTestStore(t)
	if !testpostgres.OwnsContainer() {
		t.Skip("fixture is not an owned container; durability settings are not controlled")
	}
	for _, setting := range []string{"fsync", "full_page_writes"} {
		var value string
		if err := store.db.QueryRowContext(context.Background(), "SELECT current_setting($1)", setting).Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value != "off" {
			t.Fatalf("%s = %q, want off on the disposable fixture", setting, value)
		}
	}
	var value string
	if err := store.db.QueryRowContext(context.Background(), "SELECT current_setting('synchronous_commit')").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "off" {
		t.Fatalf("synchronous_commit = %q, want off on the disposable fixture", value)
	}
}
