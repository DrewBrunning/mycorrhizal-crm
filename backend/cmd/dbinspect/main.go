// Command dbinspect reports a SQLite database's migration state and PRAGMA
// integrity_check result straight off disk, using the app's own connection
// pragmas (database.OpenMigratedFile). It exists for the external-fault CI
// job (issue #434, .github/workflows/chaos-tests.yml): after a chaos test
// interrupts a database mid-operation — SIGKILL mid-migration, ENOSPC during
// backup — this is how the harness asserts the defined outcome (not dirty at
// the latest version, integrity ok). It doubles as an operator's quick
// "is this file a valid, up-to-date database" check.
//
// The path is read from the positional argument, or SQLITE_DB_PATH (the same
// variable the server, cmd/migrate, and cmd/backup read).
package main

import (
	"errors"
	"fmt"
	"os"

	"mycorrhizal/database"
)

// defaultDBPath matches the other CLIs' fallback when SQLITE_DB_PATH is unset.
const defaultDBPath = "mycorrhizal.db"

// dbPath resolves the database path from the positional argument, then
// SQLITE_DB_PATH, then the default. More than one argument is a usage error
// (the harness may pass a path; anything else is a mistake worth failing on).
func dbPath() (string, error) {
	if len(os.Args) > 2 {
		return "", fmt.Errorf("usage: %s [DB_PATH] (or set SQLITE_DB_PATH)", os.Args[0])
	}
	if len(os.Args) > 1 {
		return os.Args[1], nil
	}
	if p := os.Getenv("SQLITE_DB_PATH"); p != "" {
		return p, nil
	}
	return defaultDBPath, nil
}

func main() {
	os.Exit(runCLI()) // # pragma: no cover — os.Exit terminates the process; tests exercise runCLI directly
}

// runCLI wires dbPath + run to the process exit code: 0 on success, 1 when
// the database cannot be inspected, 2 on a usage error. Split out of main so
// the exit paths are covered by tests.
func runCLI() int {
	path, err := dbPath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	line, err := run(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(line)
	return 0
}

// run opens dbPath and returns the integrity + migration state line. It
// returns an error when the file cannot be opened, fails PRAGMA
// integrity_check, or has no schema_migrations row (never migrated). A dirty
// flag is reported, not failed: a dirty database is a recoverable state the
// harness asserts on.
//
// The output is a single exact string — the chaos jobs `case` against it
// verbatim — so both reads go through the database package's raw-sql helpers
// (IntegrityCheck / MigrationVersion), never GORM: GORM's slow-SQL logger
// writes to os.Stdout, and on a large database the integrity check is slow
// enough to trip it, which would interleave a "SLOW SQL >= 200ms" line into
// the parseable output.
func run(dbPath string) (string, error) {
	// Fail on a missing file rather than letting the driver create an empty
	// database — a typo'd path must not silently materialize a fresh DB.
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			err = errors.New("no such file")
		}
		return "", fmt.Errorf("open %q: %w", dbPath, err)
	}

	result, err := database.IntegrityCheck(dbPath)
	if err != nil {
		return "", fmt.Errorf("integrity check on %q: %w", dbPath, err)
	}
	if result != "ok" {
		// # pragma: no cover — SQLite also surfaces most corruptions as errors,
		// not row results; the integrity_check result is "ok" for every intact
		// database the harness inspects.
		return "", fmt.Errorf("integrity check on %q failed: %s", dbPath, result)
	}

	version, dirty, ok, err := database.MigrationVersion(dbPath)
	if err != nil {
		return "", fmt.Errorf("read migration version of %q: %w", dbPath, err) // # pragma: no cover — a file that opened is version-readable
	}
	if !ok {
		return "", fmt.Errorf("%q has no schema_migrations row (never migrated)", dbPath)
	}
	return fmt.Sprintf("integrity_check=%s version=%d dirty=%v", result, version, dirty), nil
}
