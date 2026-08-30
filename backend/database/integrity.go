package database

import (
	"database/sql"
	"fmt"
	"os"
)

// IntegrityCheck reports the PRAGMA integrity_check result of the database at
// dbPath, opening it with the app's own connection pragmas (openDSN) but via a
// raw *sql.DB — deliberately NOT through GORM, whose slow-SQL logger writes to
// os.Stdout and would pollute the stdout of machine-readable callers (dbinspect
// prints one parseable state line; a GORM "SLOW SQL >= 200ms" warning breaks
// the exact-string contract the chaos jobs assert on). Mirrors
// MigrationVersion, the other raw-sql, GORM-free path in this package.
func IntegrityCheck(dbPath string) (string, error) {
	// Refuse a missing path up front: sql.Open is lazy and SQLite would
	// otherwise create an empty database, silently reporting a typo'd path as
	// "ok" — the same silent-materialization hazard dbinspect's own guard
	// exists to prevent.
	if _, err := os.Stat(dbPath); err != nil {
		return "", fmt.Errorf("open %q: %w", dbPath, err)
	}

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	if err != nil { // # pragma: no cover — the driver cannot fail to open a file DSN; the error surfaces at the first query
		return "", fmt.Errorf("open %q: %w", dbPath, err)
	}
	defer sqlDB.Close()

	var result string
	if err := sqlDB.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return "", fmt.Errorf("integrity check on %q: %w", dbPath, err)
	}
	return result, nil
}
