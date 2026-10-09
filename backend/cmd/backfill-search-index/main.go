// Command backfill-search-index rebuilds the FTS5 full-text search index
// (T11, SEARCH-01 issue #461) from the live base tables. Idempotent and safe
// to run at any time — the index is derived data, so a rebuild is always the
// same as what the triggers would have produced. Run it after a bulk import
// or a raw-SQL migration that bypassed the FTS triggers.
//
// This is the path for an operator with a Go toolchain and filesystem access
// to the database. A stock Docker deployment has neither: it uses the
// admin-gated POST /admin/search/rebuild endpoint instead (same rebuild, same
// guarantees). See docs/operations/search-index.md.
//
// Usage:
//
//	go run cmd/backfill-search-index/main.go [-db <path>]
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"mycorrhizal/database"
	"mycorrhizal/services"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr)) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable. Exit codes: 0 on
// success, 2 on a usage/parse error, 1 on any runtime failure. env is part of
// the shared operator-CLI signature for symmetry; this rebuild needs no
// environment input.
func run(args []string, env func(string) string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("backfill-search-index", flag.ContinueOnError)
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

	stats, err := services.RebuildSearchIndexReport(db)
	if err != nil {
		fmt.Fprintf(stderr, "failed to rebuild search index: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Search index rebuilt successfully: contacts=%d notes=%d activities=%d\n",
		stats.Contacts, stats.Notes, stats.Activities)
	return 0
}
