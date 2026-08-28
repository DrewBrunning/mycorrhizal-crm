// Command migrate is a thin CLI over the database package's migration entry
// points. It exists for the operator workflows the server process does not
// cover — rolling a migration back, and inspecting the applied version. Normal
// startup does not need it: database.InitDB runs every pending migration from
// the embedded FS automatically.
//
// It deliberately owns no migration logic of its own. It used to, and both
// halves were wrong:
//
//   - "down" called golang-migrate's m.Down(), which rolls back EVERY
//     migration, while the Makefile documented the target as "Rollback the
//     last migration". With the migrations squashed to one initial schema that
//     command dropped the entire database.
//   - the database path was hardcoded to "mycorrhizal.db", ignoring
//     SQLITE_DB_PATH. Since .env.example ships SQLITE_DB_PATH=./static/
//     mycorrhizal.db, the documented setup had this CLI migrating a different
//     file than the one the server opened, while the Makefile echoed the path
//     it was not using.
//
// Both are now delegated to database.MigrateUp/MigrateDown/MigrationVersion,
// which read the embedded migrations and the same DSN pragmas the server uses.
package main

import (
	"fmt"
	"log"
	"os"

	"mycorrhizal/database"
	"mycorrhizal/logger"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/migrate/main.go [up|down|version]")
	}

	// run's own defers execute before main ever calls log.Fatal, unlike
	// inlining this logic directly in main: log.Fatal calls os.Exit, which
	// skips every pending defer in the process.
	if err := run(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

func dbPath() string {
	if p := os.Getenv("SQLITE_DB_PATH"); p != "" {
		return p
	}
	return defaultDBPath
}

// initCLILogger mirrors the server's LOG_LEVEL / LOG_PRETTY env contract so
// the database package's structured logs reach stderr/stdout. Without an
// initialized logger the zero-value zerolog.Logger discards every event, so a
// bare `make migrate-up` produced NO diagnostics — no migration_failed line,
// and no issue #434 injection-pause marker for the external-fault CI job to
// grep. Split out of run so the env branches are covered by tests.
func initCLILogger() {
	level := os.Getenv("LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	pretty := os.Getenv("LOG_PRETTY") == "true" || os.Getenv("LOG_PRETTY") == "1"
	logger.InitLogger(logger.Config{Level: level, Pretty: pretty})
}

func run(command string) error {
	initCLILogger()

	path := dbPath()

	switch command {
	case "up":
		if err := database.MigrateUp(path); err != nil {
			return fmt.Errorf("failed to run migrations: %w", err)
		}
		fmt.Printf("Migrations applied successfully to %s\n", path)
	case "down":
		// Exactly one migration — see database.MigrateDown's doc comment for
		// why this must never become "roll back everything".
		if err := database.MigrateDown(path); err != nil {
			return fmt.Errorf("failed to roll back migration: %w", err)
		}
		fmt.Printf("Rolled back one migration on %s\n", path)
	case "version":
		version, dirty, ok, err := database.MigrationVersion(path)
		if err != nil {
			return fmt.Errorf("failed to get migration version: %w", err)
		}
		if !ok {
			fmt.Printf("No migrations applied yet on %s\n", path)
		} else {
			fmt.Printf("Current version on %s: %d (dirty: %v)\n", path, version, dirty)
		}
	default:
		return fmt.Errorf("unknown command: %s. Use 'up', 'down', or 'version'", command)
	}
	return nil
}
