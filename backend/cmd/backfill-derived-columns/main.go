// Command backfill-derived-columns rebuilds the denormalized contact columns
// (issue #497) from the authoritative nested Card: the flat contacts.*
// projection scalars, contacts.sort_name, contacts.addresses_flat and
// contacts.phones_normalized. Idempotent and safe to run at any time — these
// columns are derived data, so a rebuild always converges on exactly what a
// plain re-save through the ordinary write path would have produced. Run it
// after a bulk import that INSERTed contact rows directly, a raw-SQL
// migration that touched a base column, or a restore from backup.
//
// This is the path for an operator with a Go toolchain and filesystem access
// to the database. A stock Docker deployment has neither: it uses the
// admin-gated POST /api/v1/admin/contacts/rebuild-derived endpoint instead
// (same rebuild, same guarantees). It is the flat-column analogue of
// cmd/backfill-search-index; run both after a restore.
//
// Usage:
//
//	go run cmd/backfill-derived-columns/main.go [-db <path>]
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/database"
	"mycorrhizal/services"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) // pragma: no cover — os.Exit terminates; tests call run directly
}

// run parses args, opens the database, and rebuilds. Exit codes: 0 on
// success, 1 on any failure.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill-derived-columns", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", "mycorrhizal.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		return 1
	}

	db, err := database.InitDB(*dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "failed to open database: %v\n", err)
		return 1
	}

	stats, err := services.RebuildDerivedContactColumns(context.Background(), db)
	if err != nil {
		fmt.Fprintf(stderr, "failed to rebuild derived contact columns: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Derived contact columns rebuilt: scanned=%d updated=%d\n",
		stats.ContactsScanned, stats.ContactsUpdated)
	for col, n := range stats.ColumnUpdates {
		fmt.Fprintf(stdout, "  %s: %d row(s) rewritten\n", col, n)
	}
	return 0
}
