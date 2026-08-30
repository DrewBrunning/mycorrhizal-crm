package schemafixture

import (
	"database/sql"
	"fmt"

	"mycorrhizal/database"

	"gorm.io/gorm"
)

// TransplantDataToVersion copies every populated row of srcDB — a database at
// the CURRENT schema — into a fresh database at outPath built to exactly the
// given migration version, and returns. It is the exported, non-test form of
// the machinery Load uses: the large-dataset migration harness (issue #495)
// and cmd/migratebench build historical-schema databases at scale with it,
// because canonicalfixture.Populate drives GORM models that assume the current
// schema and cannot insert into an older one directly.
//
// The historical schema is built by running the embedded migration chain up to
// version (the same bytes a release tag shipped — the chain is frozen and
// append-only), then the data is copied with the version-appropriate column
// intersection (extractData/copyData), so a column added by a later migration
// is simply absent at this version. FTS content is deliberately not copied:
// the historical schema's FTS triggers rebuild the index from the copied base
// rows.
//
// outPath is checkpointed (WAL folded into the main file) before it returns,
// so the result is a single self-contained file that can be copied or moved
// wholesale — the shape the measurement harness and the disk-exhaustion job
// need.
func TransplantDataToVersion(srcDB *gorm.DB, version uint, outPath string) error {
	data, err := extractData(srcDB)
	if err != nil { // # pragma: no cover — extractData's own error paths are unit-tested; the source is a real migrated DB
		return err
	}

	if err := database.MigrateUpTo(outPath, version); err != nil { // # pragma: no cover — a fresh file always migrates; the migration tests exercise the failure vocabulary
		return fmt.Errorf("schemafixture: building version %d schema: %w", version, err)
	}

	// A plain open, deliberately without foreign_keys(1): copyData inserts in
	// deterministic alphabetical order, which is not dependency order, so FK
	// enforcement must be off for the copy connection exactly as the committed
	// schema dump disables it (openHistoricalSchema). The fixture is reopened
	// through the app's own pragmas (OpenMigratedFile) by its consumer.
	conn, err := sql.Open("sqlite", outPath)
	if err != nil { // # pragma: no cover — a driver already registered cannot fail to open a file DSN
		return fmt.Errorf("schemafixture: opening %s for data copy: %w", outPath, err)
	}
	defer conn.Close()

	if err := copyData(conn, data); err != nil { // # pragma: no cover — copyData's own error paths are unit-tested; the source data came from a real migrated DB
		return fmt.Errorf("schemafixture: copying data into version %d schema: %w", version, err)
	}

	// Fold the -wal/-shm sidecars into the main file so outPath is complete on
	// its own.
	if _, err := conn.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil { // # pragma: no cover — a freshly-written database always accepts a checkpoint
		return fmt.Errorf("schemafixture: checkpointing %s: %w", outPath, err)
	}
	return nil
}
