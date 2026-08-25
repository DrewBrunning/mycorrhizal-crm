// Command audit-backfill completes the tamper-evident audit hash chain
// (issue #381, ASVS V7.3) for rows that predate migration 000034. The server
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

// run backfills the chain on the database at path. Split out of main so the
// failure path is covered by tests.
func run(path string) error {
	db, err := database.InitDB(path)
	if err != nil {
		return fmt.Errorf("failed to open database %s: %w", path, err)
	}
	if err := models.RecomputeAuditChain(db); err != nil {
		return fmt.Errorf("audit hash chain backfill failed: %w", err)
	}
	fmt.Printf("Audit hash chain is complete on %s\n", path)
	return nil
}

func main() {
	if err := run(dbPath()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
