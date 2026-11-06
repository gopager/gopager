package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// skipMessage is the shared reason for skipping database tests in the quick tier.
const skipMessage = "PostgreSQL integration disabled; run with GOPAGER_INTEGRATION=1"

// templatePrefix marks the template databases owned by a test process, so Run can drop them.
const templatePrefix = "tmpl_"

// ownedTemplates records the template databases created by this process for cleanup.
var ownedTemplates struct {
	sync.Mutex
	names []string
}

// Template is a lazily built, migrated database from which tests clone isolated databases.
// A Template is safe for concurrent use.
type Template struct {
	name  string
	build func(ctx context.Context, dsn string) error

	once   sync.Once
	err    error
	dbName string
	// clone serializes CREATE DATABASE ... TEMPLATE: PostgreSQL refuses to copy a template that
	// has other sessions, and concurrent clones would otherwise race on its lock.
	clone sync.Mutex
}

// NewTemplate declares a template database. build receives the DSN of an empty database and
// must bring it to the state clones should start from (for example by applying migrations).
// It runs at most once per process, on the first call to Database.
func NewTemplate(name string, build func(ctx context.Context, dsn string) error) *Template {
	return &Template{name: sanitize(name), build: build}
}

// Database returns the DSN of a fresh database cloned from the template. The database is
// dropped, with connections forced closed, when the test ends. The test is skipped when no
// PostgreSQL fixture is configured; a failed template build fails every caller.
func (tp *Template) Database(t testing.TB) string {
	t.Helper()
	dsn := baseDSN(t)
	tp.once.Do(func() { tp.err = tp.prepare(dsn) })
	if tp.err != nil {
		t.Fatalf("build template database %q: %v", tp.name, tp.err)
	}
	tp.clone.Lock()
	defer tp.clone.Unlock()
	return createDatabase(t, dsn, "test_", tp.dbName)
}

// prepare creates the template database, runs the builder and freezes the result.
// The template is marked IS_TEMPLATE (so CREATE DATABASE may copy it without superuser rights
// being needed for it to stay unmodified) and refuses connections, so no stray session can
// block cloning.
func (tp *Template) prepare(dsn string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name := templatePrefix + tp.name + "_" + randomSuffix()
	if err := adminExec(ctx, dsn, "CREATE DATABASE "+quote(name)); err != nil {
		return err
	}
	ownedTemplates.Lock()
	ownedTemplates.names = append(ownedTemplates.names, name)
	ownedTemplates.Unlock()
	scoped, err := withDatabase(dsn, name)
	if err != nil {
		return err
	}
	if err := tp.build(ctx, scoped); err != nil {
		return err
	}
	if err := adminExec(ctx, dsn, "ALTER DATABASE "+quote(name)+" WITH ALLOW_CONNECTIONS false IS_TEMPLATE true"); err != nil {
		return err
	}
	tp.dbName = name
	return nil
}

// EmptyDatabase returns the DSN of a fresh, empty database cloned from template0, dropped when
// the test ends. The test is skipped when no PostgreSQL fixture is configured.
func EmptyDatabase(t testing.TB) string {
	t.Helper()
	return createDatabase(t, baseDSN(t), "test_", "template0")
}

// baseDSN returns the fixture DSN or skips the test.
func baseDSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv("GOPAGER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip(skipMessage)
	}
	return dsn
}

// createDatabase clones source into a new randomly named database and registers its removal.
// It returns the DSN of the new database.
func createDatabase(t testing.TB, dsn, prefix, source string) string {
	t.Helper()
	name := prefix + randomSuffix()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := adminExec(ctx, dsn, "CREATE DATABASE "+quote(name)+" TEMPLATE "+quote(source)); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := adminExec(ctx, dsn, "DROP DATABASE IF EXISTS "+quote(name)+" WITH (FORCE)"); err != nil {
			t.Errorf("drop test database: %v", err)
		}
	})
	scoped, err := withDatabase(dsn, name)
	if err != nil {
		t.Fatalf("address test database: %v", err)
	}
	return scoped
}

// dropTemplates removes the template databases created by this process.
func dropTemplates(dsn string) error {
	ownedTemplates.Lock()
	defer ownedTemplates.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var first error
	for _, name := range ownedTemplates.names {
		// A template cannot be dropped while IS_TEMPLATE is set.
		_ = adminExec(ctx, dsn, "ALTER DATABASE "+quote(name)+" WITH IS_TEMPLATE false")
		if err := adminExec(ctx, dsn, "DROP DATABASE IF EXISTS "+quote(name)+" WITH (FORCE)"); err != nil && first == nil {
			first = err
		}
	}
	ownedTemplates.names = nil
	return first
}

// adminExec runs one statement on the fixture's maintenance connection (the DSN's own database).
func adminExec(ctx context.Context, dsn, statement string) error {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("parse fixture DSN: %w", err)
	}
	db := stdlib.OpenDB(*config)
	defer db.Close()
	if _, err := db.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("%s: %w", firstWords(statement), err)
	}
	return nil
}

// withDatabase returns dsn addressing another database on the same server. It accepts the URL
// and keyword/value DSN forms.
func withDatabase(dsn, name string) (string, error) {
	if _, err := pgx.ParseConfig(dsn); err != nil {
		return "", fmt.Errorf("parse fixture DSN: %w", err)
	}
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		address, err := url.Parse(dsn)
		if err != nil {
			return "", err
		}
		address.Path = "/" + name
		return address.String(), nil
	}
	// Keyword/value form: later keywords override earlier ones.
	return dsn + " dbname=" + quoteValue(name), nil
}

// quote returns a double-quoted SQL identifier.
func quote(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

// quoteValue returns a single-quoted keyword/value DSN value.
func quoteValue(value string) string {
	return `'` + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(value) + `'`
}

// sanitize reduces a template name to lowercase identifier characters.
func sanitize(name string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 'a' - 'A'
		}
		return '_'
	}, name)
}

// randomSuffix returns 12 random hexadecimal characters.
func randomSuffix() string {
	var random [6]byte
	if _, err := rand.Read(random[:]); err != nil {
		panic(fmt.Sprintf("generate database name: %v", err))
	}
	return hex.EncodeToString(random[:])
}

// firstWords names a statement in errors without echoing long SQL.
func firstWords(statement string) string {
	fields := strings.Fields(statement)
	if len(fields) > 3 {
		fields = fields[:3]
	}
	return strings.Join(fields, " ")
}
