package services

import (
	"context"
	"errors"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// assertDatabaseHealthy runs the DB-01 storage pragmas and the ADR-0012
// data-invariant probes and fails the test if either reports a problem. This
// is the non-negotiable outcome for issue #498: an interrupted or refused
// operation may fail, but it may never corrupt.
func assertDatabaseHealthy(t *testing.T, db *gorm.DB) {
	t.Helper()
	storage, err := RunStorageIntegrityChecks(db)
	require.NoError(t, err)
	assert.True(t, storage.OK, "PRAGMA integrity_check / foreign_key_check must be clean: %s", storage.Detail())

	data, err := RunDataIntegrityChecks(context.Background(), db, config.Config{})
	require.NoError(t, err)
	assert.True(t, data.OK, "ADR-0012 data invariants must hold: %+v", data.Findings)
}

// TestSourceImport_InjectedFaultFailsClosed pins the services.import.source
// seam (issue #434 / #498): an armed fault fails the whole source import at
// the transaction boundary — no contacts, no source-link ledger rows, no
// partial graph — and a retry after the fault clears re-runs cleanly. The
// database is structurally and semantically intact throughout.
func TestSourceImport_InjectedFaultFailsClosed(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	plan := &ImportSourcePlan{System: "test"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")},
		{Ref: ref("contact/2"), Record: minimalRecord("Ben", "Babbage")},
	}
	plan.Notes = []MappedNote{{Ref: ref("note/1"), Contact: ref("contact/1"), Content: "hi", Date: "2023-01-01T00:00:00Z"}}

	injected := errors.New("injected db failure mid-source-import")
	faults.ArmError(faultImportSource, injected)

	report, err := ExecuteSourceImport(db, user.ID, plan)
	require.Error(t, err)
	assert.ErrorIs(t, err, injected)
	assert.Nil(t, report)

	var contacts, notes, links int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	require.NoError(t, db.Model(&models.Note{}).Where("user_id = ?", user.ID).Count(&notes).Error)
	require.NoError(t, db.Model(&models.ImportSourceLink{}).Where("user_id = ?", user.ID).Count(&links).Error)
	assert.Equal(t, int64(0), contacts, "no contact rows survive a failed-closed source import")
	assert.Equal(t, int64(0), notes, "no note rows survive")
	assert.Equal(t, int64(0), links, "no source-link ledger rows survive")

	assertDatabaseHealthy(t, db)

	// Retry with the fault cleared: the same plan applies in full.
	faults.Disarm(faultImportSource)
	report, err = ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	assert.Equal(t, 2, report.ContactsCreated)
	assert.Equal(t, 1, report.NotesCreated)
	assertDatabaseHealthy(t, db)
}

// TestSearchIndexRebuild_InjectedFaultLeavesPriorIndexIntact pins the
// services.search.rebuild seam (issue #434 / #498): the whole rebuild runs in
// one transaction, so an armed fault rolls the DELETEs back and the
// previously-good FTS index is untouched. Search still works, the index is
// still consistent with canonical data, and a later unarmed rebuild succeeds.
func TestSearchIndexRebuild_InjectedFaultLeavesPriorIndexIntact(t *testing.T) {
	faults.Reset()
	t.Cleanup(faults.Reset)

	db := dbtest.New(t)
	user := models.User{Username: "fts-rebuild", Email: "fts@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	for _, name := range []string{"Zebra", "Yak", "Xerus"} {
		c := models.Contact{UserID: user.ID, Firstname: name, Lastname: "Searchable"}
		require.NoError(t, db.Create(&c).Error)
	}
	// A clean baseline index.
	require.NoError(t, RebuildSearchIndex(db))
	before, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	require.True(t, before.Clean(), "baseline index is consistent: %s", before.Summary())

	injected := errors.New("injected failure mid-FTS-rebuild")
	faults.ArmError(faultSearchRebuild, injected)

	_, err = RebuildSearchIndexReport(db)
	require.Error(t, err)
	assert.ErrorIs(t, err, injected)

	faults.Disarm(faultSearchRebuild)

	// The rollback left the prior index in place — still consistent, no
	// half-populated state, storage intact.
	after, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.True(t, after.Clean(), "a rolled-back rebuild must leave the prior index consistent: %s", after.Summary())
	assertDatabaseHealthy(t, db)

	// And a later unarmed rebuild still succeeds.
	stats, err := RebuildSearchIndexReport(db)
	require.NoError(t, err)
	assert.Equal(t, int64(3), stats.Contacts)
}
