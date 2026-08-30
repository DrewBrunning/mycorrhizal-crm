// Command migrate is a thin CLI over the database package's migration entry
// points. It exists for the operator workflows the server process does not
// cover — rolling a migration back, inspecting the applied version, and the
// operator-only recovery for a dirty (interrupted) migration (`force`, which
// prompts). Normal startup does not need it: database.InitDB runs every pending
// migration from the embedded FS automatically.
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
// Both are now delegated to database.MigrateUp/MigrateDown/MigrationVersion
// (and MigrateForce for the prompted dirty-state recovery), which read the
// embedded migrations and the same DSN pragmas the server uses.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"mycorrhizal/database"
	"mycorrhizal/logger"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

// stdin is the reader the force command's confirmation prompt reads from.
// Split out so tests can drive the prompt without touching os.Stdin.
var stdin io.Reader = os.Stdin

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run cmd/migrate/main.go [up|down|force|version]")
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
	case "force":
		// Operator-only recovery for a dirty database (MIG-04, issue #439 /
		// issue #546). Unlike the old startup path, which force-cleared a
		// dirty flag automatically at every boot (fail-open), this is a
		// deliberate, human-invoked command that prompts before doing anything
		// and only acts on the CURRENT dirty version — never a version the
		// operator has not been told about. The server's startup path has no
		// equivalent.
		if err := confirmForce(path, stdin); err != nil {
			return err
		}
		if err := database.MigrateForce(path); err != nil {
			return fmt.Errorf("failed to force migrations: %w", err)
		}
		fmt.Printf("Forced the dirty migration state and ran pending migrations on %s\n", path)
	default:
		return fmt.Errorf("unknown command: %s. Use 'up', 'down', 'force', or 'version'", command)
	}
	return nil
}

// confirmForce shows the operator what a dirty database actually means and
// requires an explicit "yes" before the force proceeds. It refuses up front
// when there is nothing to force (not dirty, or never migrated), so the
// command can never be triggered non-interactively by accident — the state it
// is about to change is stated in full before the prompt.
func confirmForce(path string, in io.Reader) error {
	version, dirty, ok, err := database.MigrationVersion(path)
	if err != nil {
		return fmt.Errorf("failed to get migration version: %w", err)
	}
	if !ok {
		return errors.New("no migrations have been applied; there is no dirty state to force")
	}
	if !dirty {
		return fmt.Errorf("database is not dirty (version %d is clean); force is only for an interrupted migration", version)
	}

	fmt.Printf("Database %s is dirty at version %d: a migration started and did not finish, so the schema may be only partially applied.\n", path, version)
	fmt.Printf("'force' marks version %d as complete and re-runs pending migrations from there.\n", version)
	fmt.Println("Only do this after verifying the schema actually matches that version; the normal recovery is restore-from-backup (docs/deployment.md).")
	fmt.Print("Type 'yes' to continue: ")

	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	if strings.TrimSpace(line) != "yes" {
		return errors.New("aborted: force requires explicit confirmation")
	}
	return nil
}
