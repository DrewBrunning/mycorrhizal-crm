// Package dbtest builds the real migrated schema once per test binary and hands
// each test an isolated byte copy of it, instead of re-running the full embedded
// migration set (database.InitDB) once per test.
//
// The migrated schema is identical for every persistence test in this repo --
// only the data differs -- so re-deriving it hundreds of times was the dominant
// cost of the services/ and controllers/ packages under `-race
// -covermode=atomic` (issue #632). A copy of a ~1-2 MB SQLite file is
// sub-millisecond; a full migration run is not.
//
// This still honours CLAUDE.md backend trap #1 ("test against the real migrated
// schema, not AutoMigrate"): the template is produced by database.InitDB itself
// -- the hand-written migration SQL -- and then copied verbatim. It is not a
// GORM AutoMigrate schema.
//
// It lives in its own package (not database) so that importing `testing` stays
// out of the production database package, mirroring internal/rfctest. That also
// means the database package's own tests cannot use it (import cycle) -- which
// is correct: those tests exercise the migrator and the DSN directly and must
// keep calling database.InitDB.
package dbtest

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/fireandforget"

	"gorm.io/gorm"
)

var (
	tmplOnce sync.Once
	tmplPath string
	tmplErr  error
)

// template builds the migrated template database once per test binary (guarded
// by sync.Once, so once per `go test` package process) and returns its path.
//
// The file lives under os.MkdirTemp, not t.TempDir(), so it outlives the first
// test that triggers the build. No teardown is registered: the directory is
// process-scoped, CI runners are ephemeral, and a local /tmp is swept by the OS
// -- issue #632 explicitly accepts this over threading a TestMain into every
// package. It is a single small file per package run.
func template(tb testing.TB) string {
	tb.Helper()
	tmplOnce.Do(func() {
		dir, err := os.MkdirTemp("", "dbtest-template-")
		if err != nil { // # pragma: no cover — os.MkdirTemp under the OS temp root does not fail in practice; no seam exists to inject that failure without breaking every other test in the binary (tmplOnce runs exactly once per process).
			tmplErr = err
			return
		}
		tmplPath, tmplErr = buildTemplate(dir)
	})
	if tmplErr != nil {
		tb.Fatalf("dbtest: building migrated template database: %v", tmplErr)
	}
	return tmplPath
}

// buildTemplate runs the actual migration + WAL-checkpoint sequence that
// produces the copyable template file at dir/template.db. It is factored out
// of template's sync.Once body so a test can drive it directly against a
// throwaway directory, exercising its failure branches without touching the
// process-wide cached template every other dbtest-backed test in this binary
// depends on.
func buildTemplate(dir string) (string, error) {
	p := filepath.Join(dir, "template.db")

	db, err := database.InitDB(p)
	if err != nil {
		return "", err
	}
	return finalizeTemplate(db, p)
}

// finalizeTemplate folds the WAL back into the main file and closes the
// connection so a plain file copy of p is a complete, consistent database.
// Split out of buildTemplate so a test can drive db.DB()'s and the WAL
// checkpoint's error branches directly (a zero-value *gorm.DB for the
// former, a pre-closed *sql.DB for the latter) without needing to make the
// real migration itself fail.
func finalizeTemplate(db *gorm.DB, p string) (string, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return "", err
	}
	if _, err := sqlDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = sqlDB.Close()
		return "", err
	}
	if err := sqlDB.Close(); err != nil { // # pragma: no cover — closing a connection immediately after a successful checkpoint, with no outstanding transaction or checked-out borrow, does not fail in practice.
		return "", err
	}
	return p, nil
}

// New returns an isolated, fully-migrated *gorm.DB backed by a fresh copy of the
// per-binary template, under the test's own t.TempDir(). The underlying sql.DB
// is closed via t.Cleanup.
//
// Drop-in replacement for the common
//
//	db, err := database.InitDB(filepath.Join(t.TempDir(), "x.db"))
//	require.NoError(t, err)
func New(tb testing.TB) *gorm.DB {
	tb.Helper()
	return NewAt(tb, filepath.Join(tb.TempDir(), "x.db"))
}

// NewAt is New but writes the database copy to a caller-chosen path, for tests
// that also need the database file on disk (backup, restore-drill and
// VACUUM INTO style tests that pass the path to code under test).
func NewAt(tb testing.TB, dbPath string) *gorm.DB {
	tb.Helper()

	if err := copyFile(template(tb), dbPath); err != nil {
		tb.Fatalf("dbtest: copying migrated template to %s: %v", dbPath, err)
	}

	db, err := database.OpenMigratedFile(dbPath)
	if err != nil {
		tb.Fatalf("dbtest: opening copied database at %s: %v", dbPath, err)
	}
	tb.Cleanup(func() {
		// Drain the fire-and-forget goroutines (webhook deliveries, audit
		// writes) that hold this *gorm.DB before closing it. Handlers spawn
		// them asynchronously and they can outlive the test body; if the
		// database file is deleted by t.TempDir() while such a connection is
		// still open, modernc/sqlite recreates a transient "001" temp
		// directory and TempDir's cleanup fails with "directory not empty" —
		// the TestDeleteContact_NullsSelfContactPointer flake (issue #703).
		// DB.Close() alone does not wait for a connection a straggler has
		// already checked out, so the drain must come first.
		fireandforget.Wait()
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func copyFile(src, dst string) error {
	// #nosec G304 -- src is the package's own os.MkdirTemp template path; dst
	// is a test-supplied t.TempDir() path. Neither is request input, and this
	// file is test-only infrastructure.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close() //nolint:errcheck // read-only copy source; the destination Close is checked

	out, err := os.Create(dst) // #nosec G304 -- see the note on os.Open above.
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// HideTable makes table unreachable under its own name, so any query against
// it fails with "no such table" -- the error-path seam for tests that used to
// build an AutoMigrate schema deliberately missing a table. It renames the
// table instead of dropping it: SQLite's ALTER TABLE ... RENAME rewrites the
// foreign keys and triggers on every other table to follow, so the rest of the
// real migrated schema (and rows already seeded into it) keep working. Call it
// after seeding whatever rows the test needs.
func HideTable(tb testing.TB, db *gorm.DB, table string) {
	tb.Helper()
	// Identifiers cannot be bound parameters; table is a test-supplied
	// constant, never request input.
	stmt := fmt.Sprintf(`ALTER TABLE %q RENAME TO %q`, table, table+"__hidden_by_test")
	if err := db.Exec(stmt).Error; err != nil {
		tb.Fatalf("dbtest: hiding table %s: %v", table, err)
	}
}
