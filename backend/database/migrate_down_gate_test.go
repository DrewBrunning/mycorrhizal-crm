package database

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEveryMigrationHasADownMigration is MIG-02's "a new migration with no
// .down.sql fails CI" gate (issue #437 action 6): issue #530 keeps the down
// half of every migration required, and the up/down/up round-trip test
// (schemafixture.TestEveryMigrationRoundTripsUpDownUp) needs it to exist.
// This check is the immediate, named failure — before any migration executes
// — for a migration that ships without its down half. It reads the same
// embedded FS the migrator runs from, so it cannot drift from what boots.
func TestEveryMigrationHasADownMigration(t *testing.T) {
	t.Parallel()
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	require.NoError(t, err)

	downs := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".down.sql") {
			continue
		}
		downs[strings.TrimSuffix(e.Name(), ".down.sql")] = true
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".up.sql")
		require.Truef(t, downs[base],
			"migration %s has no matching .down.sql — issue #530 keeps the down half required, and the up/down/up round-trip test needs it", e.Name())
	}
}
