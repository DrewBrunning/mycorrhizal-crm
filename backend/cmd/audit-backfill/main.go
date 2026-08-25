// Command audit-backfill completes the tamper-evident audit hash chain
// (issue #381, ASVS V7.3) for rows that predate migration 000033. The server
// does this automatically on every startup; this CLI exists for the
// `make migrate-up`-only workflow (migrating a database without booting the
// app) and for an operator who wants to confirm the chain is complete on
// demand.
//
// It is a maintenance operation, not a verifier: it recalculates hash/prev_hash
// for any row whose chain link is stale or missing. See VerifyAuditChain and
// `make audit-verify` for the read-only integrity check.
package main

import (
	"fmt"
	"log"
	"os"

	"mycorrhizal/database"
	"mycorrhizal/models"
)

// defaultDBPath matches config.Config's own SQLITE_DB_PATH default so the CLI
// and the server agree when the variable is unset.
const defaultDBPath = "mycorrhizal.db"

func dbPath() string {
	if p := os.Getenv("SQLITE_DB_PATH"); p != "" {
		return p
	}
	return defaultDBPath
}

func main() {
	path := dbPath()
	db, err := database.InitDB(path)
	if err != nil {
		log.Fatalf("failed to open database %s: %v", path, err)
	}
	if err := models.RecomputeAuditChain(db); err != nil {
		log.Fatalf("audit hash chain backfill failed: %v", err)
	}
	fmt.Printf("Audit hash chain is complete on %s\n", path)
}
