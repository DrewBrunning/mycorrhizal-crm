// Command backfill-at-rest encrypts any rows in the at-rest-encrypted
// columns that still hold plaintext (issue #380). It is the operator-side
// companion to the automatic startup backfill in main.go: the server runs
// atrest.Backfill on every boot right after migrations, so normal upgrades
// get existing rows encrypted without any manual step. This command exists
// for the case where migrations were applied without booting the server
// (e.g. `make migrate-up` alone) and the operator wants the data half of the
// migration applied immediately.
//
// Idempotent and row-count-preserving — it only rewrites values that lack
// the "encv1:" ciphertext prefix, never inserts or deletes.
//
// Usage:
//
//	go run cmd/backfill-at-rest/main.go [-db <path>]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable. Exit codes: 0 on
// success, 2 on a usage/parse error, 1 on any runtime failure.
func run(args []string, env func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill-at-rest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "mycorrhizal.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	db, err := database.InitDB(*dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open database: %v\n", err)
		return 1
	}

	kek, err := atrest.ResolveMasterKey(env("DATA_ENCRYPTION_KEY"), env("DATA_ENCRYPTION_KEY_FILE"), env("JWT_SECRET_KEY"))
	if err != nil {
		fmt.Fprintf(stderr, "failed to resolve at-rest encryption master key: %v\n", err)
		return 1
	}
	if kek == nil {
		// Fail closed: with no key the backfill would be a silent no-op,
		// leaving every plaintext row unencrypted while reporting success.
		fmt.Fprintln(stderr, "failed to resolve at-rest encryption master key: no key configured (DATA_ENCRYPTION_KEY, DATA_ENCRYPTION_KEY_FILE and JWT_SECRET_KEY all unset); refusing to run")
		return 1
	}
	if err := atrest.Initialize(db, kek); err != nil {
		fmt.Fprintf(stderr, "failed to initialize at-rest encryption: %v\n", err)
		return 1
	}
	if err := atrest.Backfill(db); err != nil {
		fmt.Fprintf(stderr, "failed to backfill at-rest encryption: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, "At-rest encryption backfill complete")
	return 0
}
