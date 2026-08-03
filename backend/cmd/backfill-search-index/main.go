// Command backfill-search-index rebuilds the FTS5 full-text search index
// (T11) from the live base tables. Idempotent and safe to run at any time —
// the index is derived data, so a rebuild is always the same as what the
// triggers would have produced. Run it after a bulk import or a raw-SQL
// migration that bypassed the FTS triggers.
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

	if err := services.RebuildSearchIndex(db); err != nil {
		log.Fatalf("failed to rebuild search index: %v", err)
	}

	log.Println("Search index rebuilt successfully")
}
