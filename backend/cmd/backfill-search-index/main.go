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
	"log"

	"mycorrhizal/database"
	"mycorrhizal/services"
)

func main() {
	dbPath := flag.String("db", "mycorrhizal.db", "path to the SQLite database file")
	flag.Parse()

	db, err := database.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	stats, err := services.RebuildSearchIndexReport(db)
	if err != nil {
		log.Fatalf("failed to rebuild search index: %v", err)
	}

	log.Printf("Search index rebuilt successfully: contacts=%d notes=%d activities=%d",
		stats.Contacts, stats.Notes, stats.Activities)
}
