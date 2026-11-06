package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// TestMain binds this package to the fixture lifecycle it provides.
func TestMain(m *testing.M) {
	os.Exit(Run(m))
}

// seededTemplate holds one table with one row, to prove clones inherit the template content.
var seededTemplate = NewTemplate("seeded", func(ctx context.Context, dsn string) error {
	db, err := openDB(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.ExecContext(ctx, "CREATE TABLE seed (id int PRIMARY KEY); INSERT INTO seed VALUES (1)")
	return err
})

// openDB opens a pool for a test DSN.
func openDB(dsn string) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	return stdlib.OpenDB(*config), nil
}

// TestConcurrentClonesAreIsolated clones one template from parallel tests and writes to each
// clone, which must neither fail on the template lock nor leak rows between clones.
func TestConcurrentClonesAreIsolated(t *testing.T) {
	t.Parallel()
	if os.Getenv("GOPAGER_TEST_DATABASE_URL") == "" {
		t.Skip(skipMessage)
	}
	t.Run("group", func(t *testing.T) {
		t.Parallel()
		for range 12 {
			t.Run("clone", func(t *testing.T) {
				t.Parallel()
				db, err := openDB(seededTemplate.Database(t))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				if _, err := db.Exec("INSERT INTO seed VALUES (2)"); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := db.QueryRow("SELECT count(*) FROM seed").Scan(&count); err != nil || count != 2 {
					t.Fatalf("seed rows = %d, err = %v, want 2", count, err)
				}
			})
		}
	})
}

// TestEmptyDatabaseIsEmpty checks that EmptyDatabase does not inherit template content.
func TestEmptyDatabaseIsEmpty(t *testing.T) {
	t.Parallel()
	db, err := openDB(EmptyDatabase(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM pg_tables WHERE schemaname = 'public'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("public tables = %d, err = %v, want 0", count, err)
	}
}

// TestWithDatabaseSupportsBothDSNForms checks URL and keyword/value rewriting.
func TestWithDatabaseSupportsBothDSNForms(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{
		"postgres://u:p@h:5432/a?sslmode=disable",
		"host=h port=5432 user=u password=p dbname=a sslmode=disable",
	} {
		scoped, err := withDatabase(dsn, "b")
		if err != nil {
			t.Fatal(err)
		}
		config, err := pgx.ParseConfig(scoped)
		if err != nil || config.Database != "b" || config.Host != "h" {
			t.Fatalf("%q -> %q: database=%q err=%v", dsn, scoped, config.Database, err)
		}
	}
}
