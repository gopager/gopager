package postgres

import (
	"database/sql"
	"testing"
	"testing/fstest"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	"github.com/gopager/gopager/internal/platform"
)

// TestMigrationBehavior runs scenario specifications through the canonical Go runner.
func TestMigrationBehavior(t *testing.T) {
	t.Parallel()
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "PostgreSQL Migration Conformance")
}

var _ = ginkgo.Describe("Release migrations", ginkgo.Label("migrations"), func() {
	ginkgo.Context("registration before database access", ginkgo.Label("unit"), func() {
		var db *sql.DB
		ginkgo.BeforeEach(func() {
			var err error
			db, err = sql.Open("pgx", "")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			ginkgo.DeferCleanup(db.Close)
		})

		ginkgo.DescribeTable("rejects unsupported release artifacts", func(files fstest.MapFS) {
			_, err := migrationProvider(db, files)
			gomega.Expect(err).To(gomega.HaveOccurred())
			gomega.Expect(platform.FailureCode(err)).To(gomega.Equal(platform.CodeInvalidInput))
		},
			ginkgo.Entry("version zero", fstest.MapFS{"0_zero.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}),
			ginkgo.Entry("duplicate versions", fstest.MapFS{
				"1_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")},
				"1_b.sql": {Data: []byte("-- +goose Up\nSELECT 2;\n")},
			}),
			ginkgo.Entry("signed integer overflow", fstest.MapFS{"9223372036854775808_a.sql": {Data: []byte("-- +goose Up\nSELECT 1;\n")}}),
			ginkgo.Entry("nontransactional execution", migrationFiles("-- +goose NO TRANSACTION\nSELECT 1")),
			ginkgo.Entry("environment substitution", migrationFiles("-- +goose ENVSUB ON\nSELECT '${SECRET}'")),
			ginkgo.Entry("invalid UTF-8", migrationFiles(string([]byte{0xff}))),
		)
	})

	ginkgo.Context("a real PostgreSQL history", ginkgo.Label("integration"), func() {
		var store *Store
		ginkgo.BeforeEach(func() {
			store = newTestStore(ginkgo.GinkgoTB())
		})

		ginkgo.DescribeTable("refuses a history that is not the exact release prefix",
			func(ctx ginkgo.SpecContext, mutation string) {
				files := migrationFiles("SELECT 1", "SELECT 2")
				gomega.Expect(store.Migrate(ctx, files, platform.MigrationApply)).To(gomega.Succeed())
				_, err := store.db.ExecContext(ctx, mutation)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())

				for _, mode := range []platform.MigrationMode{platform.MigrationCheck, platform.MigrationApply} {
					err := store.Migrate(ctx, files, mode)
					gomega.Expect(err).To(gomega.HaveOccurred(), "mode %s must fail closed", mode)
					gomega.Expect(platform.FailureCode(err)).To(gomega.Equal(platform.CodeIncompatibleVersion))
				}
			},
			ginkgo.Entry("unknown version", ginkgo.SpecTimeout(15*time.Second), "INSERT INTO platform_schema_migrations(version_id,is_applied) VALUES (99,true)"),
			ginkgo.Entry("duplicate applied version", ginkgo.SpecTimeout(15*time.Second), "INSERT INTO platform_schema_migrations(version_id,is_applied) VALUES (1,true)"),
			ginkgo.Entry("applied hole", ginkgo.SpecTimeout(15*time.Second), "DELETE FROM platform_schema_migrations WHERE version_id=1"),
			ginkgo.Entry("negative version", ginkgo.SpecTimeout(15*time.Second), "UPDATE platform_schema_migrations SET version_id=-1 WHERE version_id=2"),
			ginkgo.Entry("unexpected down record", ginkgo.SpecTimeout(15*time.Second), "UPDATE platform_schema_migrations SET is_applied=false WHERE version_id=2"),
			ginkgo.Entry("missing initialization record", ginkgo.SpecTimeout(15*time.Second), "DELETE FROM platform_schema_migrations WHERE version_id=0"),
			ginkgo.Entry("empty history", ginkgo.SpecTimeout(15*time.Second), "DELETE FROM platform_schema_migrations"),
			ginkgo.Entry("wrong application order", ginkgo.SpecTimeout(15*time.Second), "UPDATE platform_schema_migrations SET version_id=3-version_id WHERE version_id>0"),
		)

		ginkgo.It("allows release number gaps and commits a transactional function body's data changes",
			func(ctx ginkgo.SpecContext) {
				files := migrationFiles("CREATE TABLE synthetic_data (value INTEGER); INSERT INTO synthetic_data VALUES (1)")
				files["00009_function.sql"] = &fstest.MapFile{Data: []byte(`-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
	UPDATE synthetic_data SET value = value + 1;
	UPDATE synthetic_data SET value = value + 1;
END $$;
-- +goose StatementEnd
`)}
				gomega.Expect(store.Migrate(ctx, files, platform.MigrationApply)).To(gomega.Succeed())
				gomega.Expect(store.Migrate(ctx, files, platform.MigrationCheck)).To(gomega.Succeed())
				var value int
				gomega.Expect(store.db.QueryRowContext(ctx, "SELECT value FROM synthetic_data").Scan(&value)).To(gomega.Succeed())
				gomega.Expect(value).To(gomega.Equal(3))
			}, ginkgo.SpecTimeout(15*time.Second),
		)
	})
})
