// SEARCH-02 (issue #462): the FTS index consistency check. These tests pin
// the ticket's "How to verify" list:
//
//   - deliberate divergence in each real class is detected and reported
//     specifically (which index, which row, which direction);
//   - soft-deleted and archived rows present in the index produce NO finding
//     — the false-positive class that would make the check untrustworthy —
//     asserted explicitly;
//   - a healthy database (generated corpus, TEST-02 shape) reports clean;
//   - hand-verify per CLAUDE.md: delete one row from contacts_fts directly and
//     confirm the check reports exactly that row.
package services

import (
	"fmt"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func consistencyUser(t *testing.T, db *gorm.DB, tag string) models.User {
	t.Helper()
	u := models.User{Username: "fts-cons-" + tag, Password: "password123!A", Email: "fts-cons-" + tag + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// seedConsistencyCorpus makes n contacts (each with a note and an activity),
// all live and trigger-maintained, for userID.
func seedConsistencyCorpus(t *testing.T, db *gorm.DB, userID uint, n int) []models.Contact {
	t.Helper()
	var out []models.Contact
	for i := 0; i < n; i++ {
		c := models.Contact{UserID: userID, Firstname: fmt.Sprintf("Fn%d", i), Lastname: fmt.Sprintf("Ln%d", i), Email: fmt.Sprintf("c%d@example.com", i)}
		require.NoError(t, db.Create(&c).Error)
		require.NoError(t, db.Create(&models.Note{UserID: userID, ContactID: &c.ID, Content: fmt.Sprintf("note %d", i), Date: time.Now()}).Error)
		require.NoError(t, db.Create(&models.Activity{UserID: userID, Title: fmt.Sprintf("act %d", i), Description: "d", Location: "l", Date: time.Now()}).Error)
		out = append(out, c)
	}
	return out
}

// findDivergence returns the first divergence matching index+class+rowid, or
// nil.
func findDivergence(res FTSConsistencyResult, index, class string, rowid int64) *FTSDivergence {
	for i := range res.Divergences {
		d := res.Divergences[i]
		if d.Index == index && d.Class == class && d.RowID == rowid {
			return &res.Divergences[i]
		}
	}
	return nil
}

func TestCheckSearchIndexConsistency_CleanCorpusReportsClean(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "clean")
	other := consistencyUser(t, db, "clean-other")
	seedConsistencyCorpus(t, db, u.ID, 8)
	seedConsistencyCorpus(t, db, other.ID, 4)

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.True(t, res.Clean(), "a trigger-maintained corpus is consistent: %v", res.Divergences)
	assert.Equal(t, "search index consistent with canonical data", res.Summary())
}

// The generated-corpus ("TEST-02 fixture, #430") verification lives in
// search_index_property_test.go's TestSearchIndex_RebuildMatchesIncremental,
// which builds contacts through the same ApplyRecordToContact path the API
// uses and now also asserts CheckSearchIndexConsistency reports clean.

func TestCheckSearchIndexConsistency_DetectsMissingFromIndex(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "missing")
	cs := seedConsistencyCorpus(t, db, u.ID, 5)

	// A trigger-bypassing delete from the index only.
	require.NoError(t, db.Exec(`DELETE FROM contacts_fts WHERE rowid = ?`, cs[2].ID).Error)

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	d := findDivergence(res, "contacts_fts", FTSDivergenceMissingFromIndex, int64(cs[2].ID))
	require.NotNil(t, d, "the missing row must be reported: %v", res.Divergences)
	assert.Len(t, res.Divergences, 1, "exactly that row, nothing else")
}

func TestCheckSearchIndexConsistency_DetectsOrphanInIndex(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "orphan")
	seedConsistencyCorpus(t, db, u.ID, 3)

	// An index row with no base row at all (a hard delete the AFTER DELETE
	// trigger somehow missed, or a raw insert).
	require.NoError(t, db.Exec(`INSERT INTO notes_fts(rowid, user_id, content) VALUES (777777, ?, 'ghost')`, u.ID).Error)

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	d := findDivergence(res, "notes_fts", FTSDivergenceOrphanInIndex, 777777)
	require.NotNil(t, d, "the orphan must be reported: %v", res.Divergences)
	assert.Len(t, res.Divergences, 1)
}

func TestCheckSearchIndexConsistency_DetectsContentMismatch(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "content")
	cs := seedConsistencyCorpus(t, db, u.ID, 4)

	// A migration-style raw UPDATE to the base table that bypasses the AFTER
	// UPDATE trigger... except triggers always fire. Simulate the real
	// outcome — a stale index row — by editing the index directly.
	require.NoError(t, db.Exec(`UPDATE contacts_fts SET firstname = 'STALE', org = 'STALE' WHERE rowid = ?`, cs[1].ID).Error)

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	d := findDivergence(res, "contacts_fts", FTSDivergenceContentMismatch, int64(cs[1].ID))
	require.NotNil(t, d, "the drifted row must be reported: %v", res.Divergences)
	assert.ElementsMatch(t, []string{"firstname", "org"}, d.Columns, "the report names the drifted columns, not values")
	assert.Len(t, res.Divergences, 1)
}

func TestCheckSearchIndexConsistency_DetectsScopeMismatch(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "scope")
	cs := seedConsistencyCorpus(t, db, u.ID, 3)

	require.NoError(t, db.Exec(`UPDATE activities_fts SET user_id = user_id + 999 WHERE rowid = (SELECT min(rowid) FROM activities_fts)`).Error)
	_ = cs

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	require.Len(t, res.Divergences, 1)
	assert.Equal(t, "activities_fts", res.Divergences[0].Index)
	assert.Equal(t, FTSDivergenceScopeMismatch, res.Divergences[0].Class)
}

// TestCheckSearchIndexConsistency_SoftDeletedAndArchivedAreNotDivergence is
// the load-bearing false-positive assertion: the index is deliberately not
// authoritative on deletion / archive state, so a soft-deleted or archived
// row sitting in the index is NOT a finding.
func TestCheckSearchIndexConsistency_SoftDeletedAndArchivedAreNotDivergence(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "tolerate")
	cs := seedConsistencyCorpus(t, db, u.ID, 5)

	// Archived contact: still trigger-indexed, filtered by the outer query.
	require.NoError(t, db.Model(&models.Contact{}).Where("id = ?", cs[0].ID).Update("archived", true).Error)

	// A row inserted already-soft-deleted (a bulk import / hand-written
	// migration): the AFTER INSERT trigger has no deleted_at guard, so it
	// lands in the index. That is acceptable under the contract.
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (user_id, firstname, lastname, created_at, updated_at, deleted_at)
		 VALUES (?, 'Ghosted', 'Import', ?, ?, ?)`,
		u.ID, time.Now(), time.Now(), time.Now()).Error)
	var ghostID int64
	require.NoError(t, db.Raw(`SELECT id FROM contacts WHERE firstname = 'Ghosted'`).Scan(&ghostID).Error)
	var inIndex int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM contacts_fts WHERE rowid = ?`, ghostID).Scan(&inIndex).Error)
	require.Equal(t, int64(1), inIndex, "sanity: the AFTER INSERT trigger indexed the already-soft-deleted row")

	// Also soft-delete a normal contact the ordinary way (AFTER UPDATE
	// trigger drops it) — the index no longer has it, and that is fine.
	require.NoError(t, db.Delete(&cs[1]).Error)

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.True(t, res.Clean(), "no soft-deleted or archived row is a divergence: %v", res.Divergences)
}

// TestCheckSearchIndexConsistency_HandVerify is the CLAUDE.md "break it and
// confirm" step: delete one specific row from contacts_fts and confirm the
// check names exactly that row, then restore it and confirm clean.
func TestCheckSearchIndexConsistency_HandVerify(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "hand")
	cs := seedConsistencyCorpus(t, db, u.ID, 6)
	target := int64(cs[3].ID)

	before, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	require.True(t, before.Clean())

	require.NoError(t, db.Exec(`DELETE FROM contacts_fts WHERE rowid = ?`, target).Error)

	after, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	require.Len(t, after.Divergences, 1)
	assert.Equal(t, FTSDivergence{Index: "contacts_fts", RowID: target, Class: FTSDivergenceMissingFromIndex}, after.Divergences[0])

	// Restore and re-check.
	require.NoError(t, db.Exec(
		`INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized)
		 SELECT id, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat, phones_normalized
		 FROM contacts WHERE id = ?`, target).Error)
	restored, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.True(t, restored.Clean(), "%v", restored.Divergences)
}

// TestCheckSearchIndexConsistency_PropagatesQueryErrors covers the error
// paths: a missing FTS table fails a rowid check, and an injected failure on
// the content-mismatch query fails that pass — both surface as an error, not
// a false "clean".
func TestCheckSearchIndexConsistency_PropagatesQueryErrors(t *testing.T) {
	t.Run("missing fts table", func(t *testing.T) {
		db := dbtest.New(t)
		u := consistencyUser(t, db, "err-table")
		seedConsistencyCorpus(t, db, u.ID, 2)
		require.NoError(t, db.Exec(`DROP TABLE contacts_fts`).Error)

		_, err := CheckSearchIndexConsistency(db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "contacts_fts")
	})

	t.Run("content query fails on a column the index is missing", func(t *testing.T) {
		db := dbtest.New(t)
		u := consistencyUser(t, db, "err-content")
		seedConsistencyCorpus(t, db, u.ID, 2)

		// A botched migration that recreated contacts_fts without the
		// phones_normalized column: the rowid/user_id checks still run, but
		// the content comparison references a column that no longer exists.
		require.NoError(t, db.Exec(`DROP TABLE contacts_fts`).Error)
		require.NoError(t, db.Exec(`CREATE VIRTUAL TABLE contacts_fts USING fts5(
			firstname, lastname, nickname, email, phone, org, addresses_flat, user_id UNINDEXED)`).Error)
		require.NoError(t, db.Exec(`INSERT INTO contacts_fts(rowid, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat)
			SELECT id, user_id, firstname, lastname, nickname, email, phone, org, addresses_flat
			FROM contacts WHERE deleted_at IS NULL`).Error)

		_, err := CheckSearchIndexConsistency(db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), FTSDivergenceContentMismatch)
	})
}

// TestCheckSearchIndexConsistency_TruncatesLargeDrift pins that a
// catastrophically desynchronised index is reported as a capped sample with
// Truncated set, not an unbounded id stream.
func TestCheckSearchIndexConsistency_TruncatesLargeDrift(t *testing.T) {
	db := dbtest.New(t)
	u := consistencyUser(t, db, "trunc")
	seedConsistencyCorpus(t, db, u.ID, ftsDivergenceCapPerClass+25)

	// Every indexed contact both drifts in content and is missing its
	// activity/note counterparts — exercises the content-mismatch cap and the
	// rowid-class cap together.
	require.NoError(t, db.Exec(`UPDATE contacts_fts SET firstname = 'DRIFT'`).Error)
	require.NoError(t, db.Exec(`DELETE FROM notes_fts`).Error)

	res, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.True(t, res.Truncated)

	byClass := map[string]int{}
	for _, d := range res.Divergences {
		if d.Index == "contacts_fts" {
			byClass[d.Class]++
		}
	}
	assert.Equal(t, ftsDivergenceCapPerClass, byClass[FTSDivergenceContentMismatch], "content-mismatch class is capped")
	assert.Contains(t, res.Summary(), "(sampled)")
}
