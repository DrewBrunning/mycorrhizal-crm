package schemafixture

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/internal/canonicalfixture"

	"gorm.io/gorm"
)

// Fixture is a loaded, populated historical-schema database for one supported
// release: the release's schema dump applied to a fresh temp file, populated
// with the canonical TEST-02 manifest dataset. DB is open and closed on test
// cleanup; Path is the database file (for callers that migrate it and reopen,
// e.g. the upgrade tests); Dataset mirrors what Populate created (IDs
// preserved across the schema copy, so the two are consistent).
type Fixture struct {
	DB      *gorm.DB
	Path    string
	Dataset *canonicalfixture.Dataset
	Release Release
}

// Load builds the populated fixture database for a supported release (MIG-01,
// issue #436).
//
// The manifest loader (canonicalfixture.Populate) drives GORM models whose
// hooks and column set assume the CURRENT schema, so it cannot insert directly
// into a historical schema — a pre-000044 database has no `revision` column
// for contacts/notes/activities/life_events, and the models' AfterCreate hooks
// would stamp it on every row. The loader therefore populates a scratch
// database at the CURRENT schema (full hook fidelity, the same code path every
// other canonicalfixture consumer uses) and then copies the resulting data
// into a fresh database built from the release's committed schema dump,
// intersecting each table's columns against the columns that actually exist at
// that version. That intersection is the "version-appropriate subsetting" the
// MIG-01 ticket says belongs in the loader, and it is discovered from the live
// schema rather than hardcoded, so a future migration that adds a column to a
// fixture table needs no change here.
//
// FTS content is deliberately not copied: the historical schema's FTS triggers
// rebuild the indexes from the copied contacts/notes/activities rows.
func Load(tb testing.TB, release Release) *Fixture {
	tb.Helper()

	data, dataset, err := populateCurrentSchema(tb, release)
	if err != nil {
		tb.Fatalf("schemafixture: populating %s dataset: %v", release.Tag, err) // # pragma: no cover — a committed manifest against a migrated scratch DB always populates
	}

	fixturePath := filepath.Join(tb.TempDir(), "fixture.db")

	conn, err := openHistoricalSchema(fixturePath, release)
	if err != nil {
		tb.Fatalf("schemafixture: building %s schema: %v", release.Tag, err) // # pragma: no cover — committed dumps always apply to a fresh file
	}
	if err := copyData(conn, data); err != nil {
		_ = conn.Close()
		tb.Fatalf("schemafixture: populating %s schema with manifest data: %v", release.Tag, err) // # pragma: no cover — see copyData's own pragmas
	}
	if err := conn.Close(); err != nil {
		tb.Fatalf("schemafixture: closing %s schema connection: %v", release.Tag, err) // # pragma: no cover — Close on an open handle does not fail
	}

	db, err := database.OpenMigratedFile(fixturePath)
	if err != nil {
		tb.Fatalf("schemafixture: opening %s fixture: %v", release.Tag, err) // # pragma: no cover — the file was just built
	}
	tb.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return &Fixture{DB: db, Path: fixturePath, Dataset: dataset, Release: release}
}

// populateCurrentSchema builds a scratch database at the current schema,
// populates it with the canonical manifest, and extracts its data as
// per-table column lists plus rows (in SQLite's own serialized form, so the
// copy is lossless). The scratch database is checkpointed and closed before
// anything reads it, so the returned data is a stable snapshot.
func populateCurrentSchema(tb testing.TB, release Release) (map[string]tableData, *canonicalfixture.Dataset, error) {
	srcPath := filepath.Join(tb.TempDir(), "source.db")

	srcDB, err := database.InitDB(srcPath)
	if err != nil {
		return nil, nil, fmt.Errorf("building current-schema scratch database: %w", err) // # pragma: no cover — a fresh temp file always migrates
	}
	defer func() {
		if sqlDB, err := srcDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	m, err := canonicalfixture.Read()
	if err != nil {
		return nil, nil, err // # pragma: no cover — the committed manifest is always present and valid under the repo root
	}
	ds, err := canonicalfixture.Populate(srcDB, m)
	if err != nil {
		return nil, nil, err // # pragma: no cover — the manifest is validated to fit a migrated current schema
	}

	data, err := extractData(srcDB)
	if err != nil {
		return nil, nil, err // # pragma: no cover — see extractData's own pragmas
	}
	return data, ds, nil
}

// openHistoricalSchema creates a fresh database at the release's schema: it
// applies the committed schema dump (DDL plus the schema_migrations row, so
// the fixture presents the right version and a clean flag) and returns a raw
// connection with foreign-key enforcement turned off for the subsequent data
// copy (the dump's own PRAGMA foreign_keys=OFF keeps it off for this
// connection; OpenMigratedFile re-enables it for the returned handle).
func openHistoricalSchema(path string, release Release) (*sql.DB, error) {
	dump, err := readDump(release)
	if err != nil {
		return nil, err // # pragma: no cover — readDump's own error path is unit-tested; Load only passes committed releases
	}

	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("opening fixture database: %w", err) // # pragma: no cover — a driver already registered cannot fail to open a file DSN
	}
	if _, err := conn.Exec(string(dump)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("applying %s schema dump: %w", release.Tag, err) // # pragma: no cover — committed dumps are generated to apply cleanly
	}
	return conn, nil
}

// readDump loads a release's committed schema dump, locating the repo root the
// way canonicalfixture does (walking up from the working directory).
func readDump(release Release) ([]byte, error) {
	dir, err := findSchemaDir()
	if err != nil {
		return nil, err // # pragma: no cover — findSchemaDir's own error path is unit-tested; Load runs inside the repo
	}
	return os.ReadFile(filepath.Join(dir, DumpFile(release)))
}

// findSchemaDir locates backend/database/testdata/schemas by walking up from
// the working directory to the repo root.
func findSchemaDir() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, SchemaDumpsDirRel)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("schemafixture: schemas dir %s not found (run tests from the backend/ dir or the repo root)", SchemaDumpsDirRel) // # pragma: no cover — the schemas dir is committed; a real repo root always has it
	}
	return dir, nil
}

// findRepoRoot locates the repository root by walking up from the working
// directory until a directory containing backend/database/migrations is found,
// mirroring canonicalfixture.FindManifest.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "database", "migrations")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("schemafixture: repo root not found above %s (looked for backend/database/migrations)", dir)
		}
		dir = parent
	}
}
