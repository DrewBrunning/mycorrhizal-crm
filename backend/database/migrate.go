package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"time"

	"mycorrhizal/logger"
	"strconv"
	"strings"

	"github.com/glebarez/sqlite"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
	gormLogger "gorm.io/gorm/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// openDSN appends this app's standard connection pragmas to a plain file
// path:
//   - journal_mode(WAL): the SQLite concurrency mode this app runs in. It is
//     what allows readers and a writer to coexist without blocking, but it is
//     also why a plain `cp` of the .db file is NOT a valid online backup --
//     committed writes can sit in a -wal sidecar that the copy never sees.
//     The documented online backup uses VACUUM INTO (see database/backup.go
//     and docs/deployment.md's Backups section), which snapshots the live
//     database consistently. Persisted in the database file itself once set,
//     so this only needs to run against a real file, never ":memory:".
//   - foreign_keys(1): turns on real FK enforcement  --
//     unlike journal_mode this is a per-connection setting, not persisted in
//     the file, so it must be supplied via the DSN (applied by the driver on
//     every new physical connection it opens) rather than a one-time
//     PRAGMA statement, which would only affect whichever single connection
//     ran it.
//   - busy_timeout(5000): wait up to 5s for a write lock instead of
//     immediately returning SQLITE_BUSY. The scheduled jobs fire concurrently
//     at startup and the first one to INSERT into job_executions fails the
//     others; a timeout lets them queue instead.
//   - _txlock=immediate: begin every transaction with BEGIN IMMEDIATE rather
//     than SQLite's default deferred BEGIN.
//
// The last one is not cosmetic, and busy_timeout alone does not cover it. A
// deferred transaction takes a read lock first and only tries to upgrade to a
// write lock at its first write; SQLite's busy handler is NOT invoked for that
// upgrade (it cannot safely wait — another writer may already hold a snapshot),
// so the upgrade fails instantly with SQLITE_BUSY no matter how long the
// timeout is. GORM wraps even a single Create in an implicit transaction, so
// two concurrent POSTs to a write endpoint could produce an immediate 500:
//
//	POST /api/v1/contacts -> 500 "database is locked (5) (SQLITE_BUSY)" in 4.8ms
//
// observed intermittently under the e2e suite's parallel workers, and equally
// reachable by a real client issuing concurrent writes. BEGIN IMMEDIATE takes
// the write lock up front, which IS a case the busy handler retries, so
// concurrent writers queue for up to busy_timeout instead of erroring.
// Readers are unaffected: WAL keeps them lock-free.
func openDSN(dbPath string) string {
	return dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_txlock=immediate"
}

// newGormLogger returns the GORM logger every connection opened through this
// package uses.
//
// GORM's default logger (logger.Default) writes the full SQL statement with
// literal values interpolated on every query that errors, is slow, or returns
// ErrRecordNotFound, and it is not gated by LOG_LEVEL — it writes straight to
// os.Stdout via its own log.New, independently of zerolog. Because this is an
// instance-wide log with no redaction layer, a missing row or a constraint
// failure echoed its WHERE/VALUES clause verbatim — a password-reset miss
// logged `SELECT ... WHERE email = "<address>"` (issue #510, filed as #621).
// Two flags close it:
//   - ParameterizedQueries: log `?` placeholders instead of interpolated
//     values, so the email/vcard_uid/column value never reaches the log.
//   - IgnoreRecordNotFoundError: stop logging the common, benign not-found
//     SELECTs entirely.
//
// The writer is injectable so a test can point it at a *bytes.Buffer and
// assert on exactly what GORM would emit.
func newGormLogger(w io.Writer) gormLogger.Interface {
	return gormLogger.New(
		log.New(w, "", log.LstdFlags),
		gormLogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		},
	)
}

// InitDB initializes the database connection and runs migrations
func InitDB(dbPath string) (*gorm.DB, error) {
	// Open database connection for migrations
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations
	if err := RunMigrations(sqlDB); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	// Open GORM connection
	db, err := gorm.Open(sqlite.Open(openDSN(dbPath)), &gorm.Config{Logger: newGormLogger(os.Stdout)})
	if err != nil {
		return nil, fmt.Errorf("failed to connect with GORM: %w", err)
	}

	return db, nil
}

// OpenMigratedFile opens dbPath with this app's standard connection pragmas
// (journal_mode(WAL), foreign_keys(1), busy_timeout(5000), _txlock=immediate --
// see openDSN) but WITHOUT running migrations. dbPath must already be at the
// current schema: this is for callers that copied a pre-migrated database file
// (internal/dbtest builds the migrated schema once per test binary and hands
// each test a byte copy) or are reopening a database the same process already
// migrated. Use InitDB for a fresh or possibly-stale database.
func OpenMigratedFile(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(openDSN(dbPath)), &gorm.Config{Logger: newGormLogger(os.Stdout)})
	if err != nil {
		return nil, fmt.Errorf("failed to connect with GORM: %w", err)
	}
	return db, nil
}

// newMigrator builds a golang-migrate instance over the EMBEDDED migrations FS
// for an already-open database handle. Every migration entry point in this
// package goes through it, so the source of migration SQL can never differ
// between them — cmd/migrate previously built its own instance from
// `file://database/migrations`, which meant it only worked from one working
// directory and could disagree with the binary's embedded copy.
func newMigrator(db *sql.DB) (*migrate.Migrate, error) {
	driver, err := withInstance(db, &sqliteConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to create migration driver: %w", err)
	}

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("failed to create migration source: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", driver)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration instance: %w", err)
	}
	return m, nil
}

// closeMigrator releases the resources newMigrator opened. golang-migrate's
// source driver leaves a goroutine parked forever waiting to hand off the
// next migration until Close() runs, and Close()'s database half closes the
// *sql.DB passed into newMigrator (sqliteDriver.Close(), see
// sqlite_driver.go) -- both are silent leaks otherwise. Every real-DB test
// (CLAUDE.md's "test against the real migrated schema" trap) calls InitDB,
// so skipping this piles up one leaked goroutine + one open sqlite handle
// per test; across the full suite that was enough to exhaust file
// descriptors and blow the CI job's 20-minute timeout (issue #415 PR
// review). *sql.DB.Close is idempotent, so this is safe even for callers
// that also defer-close their own sqlDB.
func closeMigrator(m *migrate.Migrate) {
	if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
		logger.Warn().AnErr("source", srcErr).AnErr("database", dbErr).Msg("failed to close migrator")
	}
}

// RunMigrations runs all pending database migrations
func RunMigrations(db *sql.DB) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	// Get current version
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if dirty {
		logger.Warn().Uint("version", version).Msg("Database is in dirty state, forcing version")
		if err := m.Force(int(version)); err != nil {
			return fmt.Errorf("failed to force version: %w", err)
		}
	}

	startVersion := version
	start := time.Now()

	// Run migrations
	upErr := m.Up()
	if upErr != nil && upErr != migrate.ErrNoChange {
		// Milestone v0.6.2 gate (issue #532): a migration failure must
		// identify WHICH migration failed, not just carry the SQL error.
		// golang-migrate leaves the failed version dirty, so m.Version()
		// names it; the filename comes from the embedded FS. The returned
		// error, the structured log line, and the system_events row (when the
		// table exists) all carry it.
		version, _, verr := m.Version()
		name := migrationFileForVersion(version)
		elapsed := time.Since(start)

		logger.Error().
			Err(upErr).
			Str(logger.FieldEvent, "migration_failed").
			Str(logger.FieldComponent, "migration").
			Uint("version", version).
			Str("migration", name).
			Int64(logger.FieldDurationMS, elapsed.Milliseconds()).
			Msg("migration failed")
		recordMigrationFailedEvent(db, startVersion, version, elapsed, upErr)

		if verr == nil {
			return fmt.Errorf("failed to apply migrations: version %d (%s): %w", version, name, upErr)
		}
		return fmt.Errorf("failed to apply migrations: %w", upErr)
	}

	// Get final version
	version, _, err = m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get final version: %w", err)
	}

	elapsed := time.Since(start)
	schemaAdvanced := upErr != migrate.ErrNoChange && version != startVersion

	switch {
	case err == migrate.ErrNilVersion:
		logger.Info().Msg("No migrations applied (database is empty)")
	case schemaAdvanced:
		logger.Info().
			Str(logger.FieldEvent, "migration_completed").
			Str(logger.FieldComponent, "migration").
			Uint("from_version", startVersion).
			Int64(logger.FieldDurationMS, elapsed.Milliseconds()).
			Uint("version", version).
			Str(logger.FieldResult, logger.ResultSuccess).
			Msg("Migrations applied successfully")
	default:
		logger.Info().Uint("version", version).Msg("No pending migrations")
	}

	// Best-effort operational event when migrations actually advanced the
	// schema (issue #424). Written by raw SQL on the same *sql.DB — the
	// system_events table exists once migration 000038 has run, and any
	// earlier-schema path where it does not yet exist simply drops the row.
	if schemaAdvanced {
		recordMigrationEvent(db, startVersion, version, elapsed)
	}

	return nil
}

// recordMigrationEvent inserts one migration_completed row, swallowing every
// error: a diagnostic write must never be able to fail a boot.
func recordMigrationEvent(db *sql.DB, from, to uint, elapsed time.Duration) {
	defer func() { _ = recover() }()
	now := time.Now().UTC()
	ms := elapsed.Milliseconds()
	_, err := db.Exec(
		`INSERT INTO system_events (created_at, occurred_at, event_type, severity, component, operation, duration_ms, result, detail)
		 VALUES (?, ?, 'migration_completed', 'info', 'migration', 'run_migrations', ?, 'success', ?)`,
		now, now, ms, fmt.Sprintf("from_version=%d to_version=%d", from, to),
	)
	if err != nil {
		logger.Debug().Err(err).Msg("could not record migration_completed system event (pre-000038 schema?)")
	}
}

// recordMigrationFailedEvent inserts one migration_failed row, swallowing every
// error: the table may not exist yet (a migration before 000038 failed) and a
// diagnostic write must never be able to fail a boot. The error text is
// sanitized and length-capped like every other persisted diagnostic field —
// the raw golang-migrate error embeds the whole failing migration body, which
// would otherwise bloat the row.
func recordMigrationFailedEvent(db *sql.DB, from, version uint, elapsed time.Duration, upErr error) {
	defer func() { _ = recover() }()
	now := time.Now().UTC()
	ms := elapsed.Milliseconds()
	errText := logger.SanitizeLogField(upErr.Error())
	if runes := []rune(errText); len(runes) > maxMigrationEventErrorLen {
		errText = string(runes[:maxMigrationEventErrorLen])
	}
	_, err := db.Exec(
		`INSERT INTO system_events (created_at, occurred_at, event_type, severity, component, operation, duration_ms, result, error, detail)
		 VALUES (?, ?, 'migration_failed', 'error', 'migration', 'run_migrations', ?, 'failure', ?, ?)`,
		now, now, ms, errText, fmt.Sprintf("from_version=%d to_version=%d", from, version),
	)
	if err != nil {
		logger.Debug().Err(err).Msg("could not record migration_failed system event (pre-000038 schema?)")
	}
}

// maxMigrationEventErrorLen caps the persisted migration_failed error text,
// matching the 1024-rune discipline the models package applies to system_event
// free-text fields.
const maxMigrationEventErrorLen = 1024

// migrationFileForVersion returns the NNNNNN_name.up.sql filename for the
// given migration version from the embedded FS, or "" when no file matches.
// Used by the migration-failure path (issue #532) so a failed boot names the
// migration that failed rather than only echoing the SQL error.
func migrationFileForVersion(version uint) string {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, found := strings.Cut(name, "_")
		if !found {
			continue
		}
		v, err := strconv.ParseUint(prefix, 10, 32)
		if err != nil {
			continue
		}
		if uint(v) == version {
			return name
		}
	}
	return ""
}

// MigrateUp applies every pending migration to the database at dbPath. Thin
// path-taking wrapper over RunMigrations so cmd/migrate does not have to
// duplicate the DSN pragmas — the CLI reaching for its own sql.Open and its own
// migration source is exactly how it drifted out of sync with the app before
// (see MigrateDown's note below).
func MigrateUp(dbPath string) error {
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer sqlDB.Close()

	return RunMigrations(sqlDB)
}

// MigrationVersion reports the applied migration version and whether the
// database is in a dirty state. Returns ok=false when no migration has ever
// been applied.
func MigrationVersion(dbPath string) (version uint, dirty bool, ok bool, err error) {
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil {
		return 0, false, false, fmt.Errorf("failed to open database: %w", err)
	}
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	if err != nil {
		return 0, false, false, err
	}
	defer closeMigrator(m)

	version, dirty, err = m.Version()
	if err == migrate.ErrNilVersion {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, fmt.Errorf("failed to get migration version: %w", err)
	}
	return version, dirty, true, nil
}

// MigrateDown rolls back exactly ONE migration.
//
// It is `m.Steps(-1)`, deliberately, and must stay that way. cmd/migrate used
// to call golang-migrate's `m.Down()` directly instead of this function —
// which rolls back *every* migration and drops the whole schema — while the
// Makefile advertised the target as "Rollback the last migration". With the
// migrations squashed to a single initial schema, running the documented
// command would have destroyed the database. The CLI now delegates here so
// there is one implementation of "down" rather than two that disagree.
func MigrateDown(dbPath string) error {
	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer sqlDB.Close()

	m, err := newMigrator(sqlDB)
	if err != nil {
		return err
	}
	defer closeMigrator(m)

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	logger.Info().Msg("Migration rolled back successfully")
	return nil
}

// LatestMigrationVersion reports the highest migration version bundled in the
// embedded migrations FS — i.e. the version a freshly-migrated database should
// be at. It parses the numeric prefix of every `NNNNNN_*.up.sql` entry and
// returns the maximum. Used by the readiness/deep-health checks (issue #421)
// to detect a database that is behind the binary's schema without opening a
// second connection or running a migration.
func LatestMigrationVersion() (uint, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return 0, fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	var latest uint
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		prefix, _, found := strings.Cut(name, "_")
		if !found {
			continue
		}
		// bitSize 32 (not 64): migration prefixes are 6-digit NNNNNN, and
		// parsing to a width no larger than uint's guaranteed minimum keeps the
		// uint(v) conversion provably in range (CodeQL go/incorrect-integer-conversion).
		v, err := strconv.ParseUint(prefix, 10, 32)
		if err != nil {
			continue
		}
		if uint(v) > latest {
			latest = uint(v)
		}
	}

	if latest == 0 {
		return 0, fmt.Errorf("no migration files found in embedded FS")
	}
	return latest, nil
}

// AppliedMigrationVersion reads the currently-applied migration version and
// dirty flag straight from the live database's schema_migrations table, using
// the app's existing *gorm.DB (no second sql.Open, unlike MigrationVersion,
// which the frequently-polled readiness probe should not pay per request).
// ok is false when no migration has ever been applied (empty table).
func AppliedMigrationVersion(db *gorm.DB) (version uint, dirty bool, ok bool, err error) {
	var row struct {
		Version uint
		Dirty   bool
	}
	res := db.Raw("SELECT version, dirty FROM " + defaultMigrationsTable + " LIMIT 1").Scan(&row)
	if res.Error != nil {
		return 0, false, false, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, false, false, nil
	}
	return row.Version, row.Dirty, true, nil
}
