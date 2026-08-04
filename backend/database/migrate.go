package database

import (
	"database/sql"
	"embed"
	"fmt"
	"mycorrhizal/logger"

	"github.com/glebarez/sqlite"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// openDSN appends this app's standard connection pragmas to a plain file
// path:
//   - journal_mode(WAL): what makes docs/deployment.md's backup instructions
//     ("copy the database file while the app is running") actually safe --
//     the default rollback journal can leave a plain `cp` with a torn,
//     inconsistent file mid-write. Persisted in the database file itself
//     once set, so this only needs to run against a real file, never
//     ":memory:".
//   - foreign_keys(1): turns on real FK enforcement (Tier 3c item 8) --
//     unlike journal_mode this is a per-connection setting, not persisted in
//     the file, so it must be supplied via the DSN (applied by the driver on
//     every new physical connection it opens) rather than a one-time
//     PRAGMA statement, which would only affect whichever single connection
//     ran it.
//   - busy_timeout(5000): wait up to 5s for a write lock instead of
//     immediately returning SQLITE_BUSY. The scheduled jobs fire concurrently
//     at startup and the first one to INSERT into job_executions fails the
//     others; a timeout lets them queue instead.
func openDSN(dbPath string) string {
	return dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
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
	db, err := gorm.Open(sqlite.Open(openDSN(dbPath)), &gorm.Config{})
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

// RunMigrations runs all pending database migrations
func RunMigrations(db *sql.DB) error {
	m, err := newMigrator(db)
	if err != nil {
		return err
	}

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

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Get final version
	version, _, err = m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get final version: %w", err)
	}

	if err == migrate.ErrNilVersion {
		logger.Info().Msg("No migrations applied (database is empty)")
	} else {
		logger.Info().Uint("version", version).Msg("Migrations applied successfully")
	}

	return nil
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

	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	logger.Info().Msg("Migration rolled back successfully")
	return nil
}
