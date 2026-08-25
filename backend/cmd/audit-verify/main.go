// Command audit-verify checks the tamper-evident audit hash chain (issue #381,
// ASVS V7.3) and reports the first gap. Read-only — it never repairs anything.
// Exit status 0 means the chain is intact; 1 means a gap was found (a row was
// edited, deleted, inserted, or reordered after recording, or the chain
// backfill has not run yet — see `make audit-backfill`); 2 means the check
// itself failed.
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

// run verifies the chain on the database at path and returns the process exit
// code (0 intact / 1 gap / 2 check failed). Split out of main so the exit
// paths are covered by tests.
func run(path string) int {
	db, err := database.InitDB(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open database %s: %v\n", path, err)
		return 1
	}

	gaps, err := models.VerifyAuditChain(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "audit hash chain verification failed on %s: %v\n", path, err)
		return 2
	}
	if len(gaps) == 0 {
		fmt.Printf("Audit hash chain is intact on %s\n", path)
		return 0
	}
	gap := gaps[0]
	fmt.Fprintf(os.Stderr, "Audit hash chain BROKEN on %s at event %d: %s\n", path, gap.EventID, gap.Message)
	return 1
}

func main() {
	os.Exit(run(dbPath()))
}
