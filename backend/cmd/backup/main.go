// Command backup writes a consistent online snapshot of the SQLite database
// using VACUUM INTO, without stopping the server. It exists for the operator
// workflow that `make backup` drives (see backend/Makefile and
// docs/deployment.md's Backups section, ticket N6).
//
// It deliberately owns no backup logic of its own: VACUUM INTO is a plain SQL
// statement that the app's own SQLite driver handles normally, so the work
// lives in database.BackupSnapshot (checkpoint-then-snapshot + integrity
// verification) and this CLI is only argument/env plumbing — the same shape
// as cmd/migrate, which delegates all migration logic to the database package.
//
// Source is read from SQLITE_DB_PATH (same variable the server and the rest
// of the Makefile read, so this CLI can never drift onto a different file).
// The output path is, in precedence order: a positional argument, BACKUP_PATH,
// or a timestamped sibling of the source database.
package main

import (
	"fmt"
	"log"
	"os"

	"mycorrhizal/database"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	src := dbPath()

	out := ""
	if len(os.Args) > 1 {
		out = os.Args[1]
	} else if env := os.Getenv("BACKUP_PATH"); env != "" {
		out = env
	} else {
		out = database.DefaultBackupPath(src)
	}

	if err := database.BackupSnapshot(src, out); err != nil {
		return err
	}
	fmt.Printf("Backed up %s to %s\n", src, out)
	return nil
}

func dbPath() string {
	if p := os.Getenv("SQLITE_DB_PATH"); p != "" {
		return p
	}
	return defaultDBPath
}
