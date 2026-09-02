package services

// DB-03 (issue #494), action 4: prove the canonical invariants survive the
// operations most likely to break them, not only a static fixture. Each of
// these is a place where a previously-true invariant can quietly stop being
// true — a repoint that drops an association, an import that writes an edge
// past resolveRelationshipEndpoint, a migration backfill without a guard, a
// restore that loses a file.
//
// Merge (#433) and the delete cascade are driven from package controllers
// (controllers/data_integrity_operations_test.go) because a faithful run needs
// the unexported deleteContactAssociations. This file covers import, migration
// re-run, and backup/restore, all reachable from package services.

import (
	"context"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// assertInvariantsHold runs the full data-integrity sweep and fails with the
// finding list if any violation is present.
func assertInvariantsHold(t *testing.T, db *gorm.DB, cfg config.Config) {
	t.Helper()
	r, err := RunDataIntegrityChecks(context.Background(), db, cfg)
	require.NoError(t, err, "no probe should error")
	assert.True(t, r.OK, "invariants must hold after the operation; got: %+v", r.Findings)
}

// ---------------------------------------------------------------------------
// import — ExecuteSourceImport must not introduce a dangling reference,
// an orphaned join row, an out-of-range enum, or a malformed Card
// ---------------------------------------------------------------------------

func TestDataIntegrity_HoldsAfterImport(t *testing.T) {
	db, cfg, ds := loadCanonicalFixture(t)

	// Import new contacts plus a relationship, a circle spanning two imported
	// contacts, a preference and a custom field — the graph-endpoint,
	// join-row and field-value invariants at once, through the real
	// ApplyRecordToContact path a source import uses.
	plan := &ImportSourcePlan{System: "test"}
	plan.Contacts = []MappedContact{
		{Ref: ref("c/1"), Record: minimalRecord("Grace", "Hopper")},
		{Ref: ref("c/2"), Record: minimalRecord("Alan", "Turing")},
	}
	plan.Relationships = []MappedRelationship{
		{Ref: ref("r/1"), Source: ref("c/1"), Target: ref("c/2"), Type: "coworker_of"},
	}
	plan.Circles = []MappedCircle{
		{Ref: ref("cir/1"), Name: "Bletchley", Members: []SourceRef{ref("c/1"), ref("c/2")}},
	}
	plan.Preferences = []MappedPreference{
		{Ref: ref("p/1"), Contact: ref("c/1"), Category: models.PreferenceCategoryFood, Value: "tea"},
	}
	plan.CustomFields = []MappedCustomField{
		{Ref: ref("f/1"), Contact: ref("c/2"), Key: "machine", Label: "Machine", Value: "Bombe"},
	}

	report, err := ExecuteSourceImport(db, ds.User.ID, plan)
	require.NoError(t, err)
	require.Empty(t, report.Issues, "the import batch is wholly mappable")
	require.Equal(t, 2, report.ContactsCreated)
	require.Equal(t, 1, report.RelationshipsCreated)

	assertInvariantsHold(t, db, cfg)
}

// ---------------------------------------------------------------------------
// migration re-run — re-applying the (idempotent) migration chain over a
// database that already holds the TEST-02 content must not leave a hole a
// forgotten backfill would. The generative "migrate(migrate(db)) preserves
// content" half is internal/propertytest.TestMigrateIsNonDestructive.
// ---------------------------------------------------------------------------

func TestDataIntegrity_HoldsAfterMigrationRerun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mig.db")
	db := dbtest.NewAt(t, path)
	cfg, _ := populateCanonicalFixture(t, db)

	require.NoError(t, database.MigrateUp(path), "re-running the migration chain must be a clean no-op")

	assertInvariantsHold(t, db, cfg)
}

// ---------------------------------------------------------------------------
// backup / restore — a VACUUM INTO snapshot reopened as a fresh database must
// carry every invariant across intact (BACKUP-01 #453 / BACKUP-02 #454 own
// the file-set half; this is the data half).
// ---------------------------------------------------------------------------

func TestDataIntegrity_HoldsAfterBackupRestore(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	srcDB := dbtest.NewAt(t, src)
	cfg, _ := populateCanonicalFixture(t, srcDB)

	// Sanity: the source is clean before we snapshot it.
	assertInvariantsHold(t, srcDB, cfg)

	out := filepath.Join(dir, "restored.db")
	require.NoError(t, database.BackupSnapshot(src, out))

	restored, err := database.InitDB(out)
	require.NoError(t, err)
	t.Cleanup(func() {
		if s, e := restored.DB(); e == nil {
			_ = s.Close()
		}
	})

	assertInvariantsHold(t, restored, cfg)
}
