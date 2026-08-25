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
	"log"

	"mycorrhizal/atrest"
	"mycorrhizal/database"
)

func main() {
	dbPath := flag.String("db", "mycorrhizal.db", "path to the SQLite database file")
	flag.Parse()

	db, err := database.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	kek, err := atrest.EncryptionKey()
	if err != nil {
		log.Fatalf("failed to resolve at-rest encryption master key: %v", err)
	}
	if err := atrest.Initialize(db, kek); err != nil {
		log.Fatalf("failed to initialize at-rest encryption: %v", err)
	}
	if err := atrest.Backfill(db); err != nil {
		log.Fatalf("failed to backfill at-rest encryption: %v", err)
	}

	log.Println("At-rest encryption backfill complete")
}
