// SEARCH-03 (issue #463): mutation-path search tests. #462 detects index
// divergence; this file prevents it, by proving every path that mutates
// searchable data leaves the trigger-maintained FTS index correct — enumerated
// rather than sampled, and checked *after* the operation by searching for the
// changed value and by comparing against a fresh RebuildSearchIndex (#461),
// never by asserting a trigger fired.
//
// Scope of this file (the paths reachable from package services):
//
//   - the trigger state-transition matrix for all three indexes
//     (contacts_fts / notes_fts / activities_fts): create, update a searchable
//     field, update a non-searchable field (index must not change),
//     soft-delete, restore-from-soft-delete, hard-delete;
//   - CardDAV sync reconciliation (reconcileContactSync): remote create,
//     remote update (searchable content moves), remote delete (archive);
//   - bulk VCF import (ImportSessionManager.ConfirmVCF): add and merge-into-
//     existing;
//   - a structural guard (issue #463 action 5) against "a searchable field
//     added without extending the triggers".
//
// REST create/update/delete, contact merge, archive/unarchive and the audit
// Undo path are the handler-level paths and live in
// controllers/search_mutation_path_test.go.
//
// Everything here runs against the real migrated schema (dbtest.New / a
// database.InitDB template): CLAUDE.md trap #1 — the contacts_fts virtual
// table and its triggers live only in the hand-written migration SQL (000007,
// widened by 000010/000020) and do not exist under GORM AutoMigrate, so this
// coverage is meaningless without it.
package services

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/emersion/go-webdav/carddav"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mpUser makes a user for a mutation-path test.
func mpUser(t *testing.T, db *gorm.DB, tag string) models.User {
	t.Helper()
	u := models.User{Username: "search-mp-" + tag, Password: "password123!A", Email: "search-mp-" + tag + "@example.com"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// mpContactFindable reports whether a free-text search for term returns a
// contact whose id is wantID, for userID — the "search for the changed value"
// half of issue #463 action 2.
func mpContactFindable(t *testing.T, db *gorm.DB, userID uint, term string, wantID uint) bool {
	t.Helper()
	res, err := Search(db, userID, term, 50, nil)
	require.NoError(t, err)
	for _, hit := range res.Contacts {
		if hit.ID == wantID {
			return true
		}
	}
	return false
}

func mpNoteFindable(t *testing.T, db *gorm.DB, userID uint, term string, wantID uint) bool {
	t.Helper()
	res, err := Search(db, userID, term, 50, nil)
	require.NoError(t, err)
	for _, hit := range res.Notes {
		if hit.ID == wantID {
			return true
		}
	}
	return false
}

func mpActivityFindable(t *testing.T, db *gorm.DB, userID uint, term string, wantID uint) bool {
	t.Helper()
	res, err := Search(db, userID, term, 50, nil)
	require.NoError(t, err)
	for _, hit := range res.Activities {
		if hit.ID == wantID {
			return true
		}
	}
	return false
}

// assertFTSCanonical is the post-operation check every mutation-path test runs:
// the trigger-maintained index matches canonical data strictly (every live
// base row indexed with identical columns, no orphans — ftsDivergence),
// contract-aware (CheckSearchIndexConsistency clean), and equal to a
// from-scratch rebuild ("Index state after each path equals a fresh rebuild",
// issue #463 "How to verify").
func assertFTSCanonical(t *testing.T, db *gorm.DB) {
	t.Helper()

	assert.Empty(t, ftsDivergence(t, db), "trigger-maintained index diverged from canonical data")

	cons, err := CheckSearchIndexConsistency(db)
	require.NoError(t, err)
	assert.Truef(t, cons.Clean(), "consistency check found divergence: %s", cons.Summary())

	before, err := snapshotFTSLive(db)
	require.NoError(t, err)
	require.NoError(t, RebuildSearchIndex(db))
	after, err := snapshotFTSLive(db)
	require.NoError(t, err)
	assert.Equal(t, before, after, "index state after the mutation path must equal a fresh rebuild")
}

// ftsRowSnapshot returns the indexed columns of one FTS row as a deterministic
// string, or "" when the row is absent. Used to pin "update a non-searchable
// field must not change the index".
func ftsRowSnapshot(t *testing.T, db *gorm.DB, table string, rowid uint) string {
	t.Helper()
	return dumpRowsT(t, db, fmt.Sprintf("SELECT * FROM %s WHERE rowid = %d", table, rowid))
}

func dumpRowsT(t *testing.T, db *gorm.DB, query string) string {
	t.Helper()
	s, err := dumpRows(db, query)
	require.NoError(t, err)
	return s
}

// ---------------------------------------------------------------------------
// State-transition matrix — contacts_fts
// ---------------------------------------------------------------------------

// TestSearchMutationPath_ContactStateTransitions walks one contact through
// every trigger state transition and asserts index correctness after each,
// including that an update to a non-indexed column leaves the FTS row
// byte-identical.
func TestSearchMutationPath_ContactStateTransitions(t *testing.T) {
	db := dbtest.New(t)
	user := mpUser(t, db, "contact-transitions")
	// A second user's contact that must never move, to keep the per-row
	// user_id scoping honest across every transition below.
	other := mpUser(t, db, "contact-transitions-other")
	require.NoError(t, db.Create(&models.Contact{UserID: other.ID, Firstname: "Otherperson", Organization: "OtherOrg"}).Error)

	// create
	c := models.Contact{UserID: user.ID, Firstname: "Zaphod", Lastname: "Beeblebrox", Organization: "Betelgeuse Holdings"}
	require.NoError(t, db.Create(&c).Error)
	assert.True(t, mpContactFindable(t, db, user.ID, "Zaphod", c.ID), "a created contact is findable")
	assert.True(t, mpContactFindable(t, db, user.ID, "Betelgeuse", c.ID), "on an indexed non-name column too")
	assert.False(t, mpContactFindable(t, db, other.ID, "Zaphod", c.ID), "not findable by another user")
	assertFTSCanonical(t, db)

	// update a searchable field (org): the new value is findable, the old is not
	require.NoError(t, db.Exec("UPDATE contacts SET org = ? WHERE id = ?", "Milliways Restaurant", c.ID).Error)
	assert.True(t, mpContactFindable(t, db, user.ID, "Milliways", c.ID), "the new indexed value is findable")
	assert.False(t, mpContactFindable(t, db, user.ID, "Betelgeuse", c.ID), "the replaced indexed value is gone from the index")
	assert.True(t, mpContactFindable(t, db, user.ID, "Zaphod", c.ID), "an untouched indexed column still matches")
	assertFTSCanonical(t, db)

	// update a non-searchable field (birthday): the index must not change
	before := ftsRowSnapshot(t, db, "contacts_fts", c.ID)
	require.NotEmpty(t, before)
	require.NoError(t, db.Exec("UPDATE contacts SET birthday = ? WHERE id = ?", "1975-04-25", c.ID).Error)
	assert.Equal(t, before, ftsRowSnapshot(t, db, "contacts_fts", c.ID), "updating a non-indexed column must not change the FTS row")
	assertFTSCanonical(t, db)

	// soft-delete: drops out of the index and out of search
	require.NoError(t, db.Delete(&c).Error)
	assert.False(t, mpContactFindable(t, db, user.ID, "Zaphod", c.ID), "a soft-deleted contact is not findable")
	var inIndex int64
	require.NoError(t, db.Raw("SELECT count(*) FROM contacts_fts WHERE rowid = ?", c.ID).Scan(&inIndex).Error)
	assert.Equal(t, int64(0), inIndex, "the AFTER UPDATE trigger's deleted_at guard removed the row")
	assertFTSCanonical(t, db)

	// restore-from-soft-delete: the row comes back
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Where("id = ?", c.ID).Update("deleted_at", nil).Error)
	assert.True(t, mpContactFindable(t, db, user.ID, "Zaphod", c.ID), "restoring a soft-deleted contact brings it back into search")
	assert.True(t, mpContactFindable(t, db, user.ID, "Milliways", c.ID), "with all its indexed columns")
	assertFTSCanonical(t, db)

	// hard-delete: gone for good, no orphan row
	require.NoError(t, db.Unscoped().Delete(&models.Contact{}, c.ID).Error)
	require.NoError(t, db.Raw("SELECT count(*) FROM contacts_fts WHERE rowid = ?", c.ID).Scan(&inIndex).Error)
	assert.Equal(t, int64(0), inIndex, "the AFTER DELETE trigger removed the index row")
	assertFTSCanonical(t, db)
}

// ---------------------------------------------------------------------------
// State-transition matrix — notes_fts
// ---------------------------------------------------------------------------

func TestSearchMutationPath_NoteStateTransitions(t *testing.T) {
	db := dbtest.New(t)
	user := mpUser(t, db, "note-transitions")
	contact := models.Contact{UserID: user.ID, Firstname: "Trillian"}
	require.NoError(t, db.Create(&contact).Error)

	// create
	n := models.Note{UserID: user.ID, ContactID: &contact.ID, Content: "discussed the improbability drive", Date: time.Now()}
	require.NoError(t, db.Create(&n).Error)
	assert.True(t, mpNoteFindable(t, db, user.ID, "improbability", n.ID), "a created note is findable")
	assertFTSCanonical(t, db)

	// update the searchable field (content)
	require.NoError(t, db.Exec("UPDATE notes SET content = ? WHERE id = ?", "rescheduled the Vogon poetry reading", n.ID).Error)
	assert.True(t, mpNoteFindable(t, db, user.ID, "Vogon", n.ID), "the new content is findable")
	assert.False(t, mpNoteFindable(t, db, user.ID, "improbability", n.ID), "the replaced content is gone")
	assertFTSCanonical(t, db)

	// update a non-searchable field (date): the index must not change
	before := ftsRowSnapshot(t, db, "notes_fts", n.ID)
	require.NoError(t, db.Exec("UPDATE notes SET date = ? WHERE id = ?", time.Now().Add(48*time.Hour), n.ID).Error)
	assert.Equal(t, before, ftsRowSnapshot(t, db, "notes_fts", n.ID), "updating a non-indexed column must not change the FTS row")
	assertFTSCanonical(t, db)

	// soft-delete → restore → hard-delete
	require.NoError(t, db.Delete(&n).Error)
	assert.False(t, mpNoteFindable(t, db, user.ID, "Vogon", n.ID), "a soft-deleted note is not findable")
	assertFTSCanonical(t, db)

	require.NoError(t, db.Unscoped().Model(&models.Note{}).Where("id = ?", n.ID).Update("deleted_at", nil).Error)
	assert.True(t, mpNoteFindable(t, db, user.ID, "Vogon", n.ID), "restoring a soft-deleted note brings it back")
	assertFTSCanonical(t, db)

	require.NoError(t, db.Unscoped().Delete(&models.Note{}, n.ID).Error)
	var inIndex int64
	require.NoError(t, db.Raw("SELECT count(*) FROM notes_fts WHERE rowid = ?", n.ID).Scan(&inIndex).Error)
	assert.Equal(t, int64(0), inIndex, "a hard-deleted note leaves no index row")
	assertFTSCanonical(t, db)
}

// ---------------------------------------------------------------------------
// State-transition matrix — activities_fts
// ---------------------------------------------------------------------------

func TestSearchMutationPath_ActivityStateTransitions(t *testing.T) {
	db := dbtest.New(t)
	user := mpUser(t, db, "activity-transitions")

	// create
	a := models.Activity{UserID: user.ID, Title: "Lunch at Milliways", Description: "end of the universe", Location: "Frogstar", Date: time.Now()}
	require.NoError(t, db.Create(&a).Error)
	assert.True(t, mpActivityFindable(t, db, user.ID, "Milliways", a.ID), "matched on title")
	assert.True(t, mpActivityFindable(t, db, user.ID, "Frogstar", a.ID), "matched on location")
	assertFTSCanonical(t, db)

	// update a searchable field (description)
	require.NoError(t, db.Exec("UPDATE activities SET description = ? WHERE id = ?", "pan galactic gargle blaster tasting", a.ID).Error)
	assert.True(t, mpActivityFindable(t, db, user.ID, "gargle", a.ID), "the new description is findable")
	assert.False(t, mpActivityFindable(t, db, user.ID, "universe", a.ID), "the replaced description is gone")
	assertFTSCanonical(t, db)

	// update a non-searchable field (date): the index must not change
	before := ftsRowSnapshot(t, db, "activities_fts", a.ID)
	require.NoError(t, db.Exec("UPDATE activities SET date = ? WHERE id = ?", time.Now().Add(72*time.Hour), a.ID).Error)
	assert.Equal(t, before, ftsRowSnapshot(t, db, "activities_fts", a.ID), "updating a non-indexed column must not change the FTS row")
	assertFTSCanonical(t, db)

	// soft-delete → restore → hard-delete
	require.NoError(t, db.Delete(&a).Error)
	assert.False(t, mpActivityFindable(t, db, user.ID, "Milliways", a.ID), "a soft-deleted activity is not findable")
	assertFTSCanonical(t, db)

	require.NoError(t, db.Unscoped().Model(&models.Activity{}).Where("id = ?", a.ID).Update("deleted_at", nil).Error)
	assert.True(t, mpActivityFindable(t, db, user.ID, "Milliways", a.ID), "restoring a soft-deleted activity brings it back")
	assertFTSCanonical(t, db)

	require.NoError(t, db.Unscoped().Delete(&models.Activity{}, a.ID).Error)
	var inIndex int64
	require.NoError(t, db.Raw("SELECT count(*) FROM activities_fts WHERE rowid = ?", a.ID).Scan(&inIndex).Error)
	assert.Equal(t, int64(0), inIndex, "a hard-deleted activity leaves no index row")
	assertFTSCanonical(t, db)
}

// ---------------------------------------------------------------------------
// CardDAV sync reconciliation
// ---------------------------------------------------------------------------

// TestSearchMutationPath_CardDAVReconcile drives reconcileContactSync — the
// CardDAV incremental-sync ingestion path — through remote create, remote
// update (searchable content changes) and remote delete (archive), asserting
// index correctness after each. A synced contact is written with
// tx.Create/tx.Save through GORM, so the triggers fire; the value of covering
// it explicitly is that this is a second, independent ingestion point for
// searchable data (issue #463 action 1).
func TestSearchMutationPath_CardDAVReconcile(t *testing.T) {
	db := dbtest.New(t)
	cfg := contactSyncTestConfig()
	user := mpUser(t, db, "carddav")
	sub := newContactTestSubscription(t, db, cfg, user.ID, "https://example.com/addressbooks/mp/", "", "")

	// remote create. The email token is deliberately unrelated to either
	// name so that a "name gone from the index" assertion below cannot be
	// satisfied by a prefix-match on the address.
	obj := carddav.AddressObject{
		Path: "/addressbooks/mp/ford.vcf",
		ETag: "\"etag-1\"",
		Card: testCard(t, "ford-uid", "Zarniwoop", "Prefect", "zw@hitch.example"),
	}
	stats, err := reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.NoError(t, err)
	require.Equal(t, ContactSyncStats{Created: 1}, stats)

	var synced models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "ford-uid").First(&synced).Error)
	assert.True(t, mpContactFindable(t, db, user.ID, "Zarniwoop", synced.ID), "a contact created by CardDAV sync is findable")
	assert.True(t, mpContactFindable(t, db, user.ID, "hitch", synced.ID), "including on its synced email")
	assertFTSCanonical(t, db)

	// remote update: the vCard's given name changes — the new value must be
	// searchable and the old must not (full-overwrite reconcile).
	obj.ETag = "\"etag-2\""
	obj.Card = testCard(t, "ford-uid", "Ixchel", "Prefect", "zw@hitch.example")
	stats, err = reconcileContactSync(db, sub, []carddav.AddressObject{obj}, nil, false, "")
	require.NoError(t, err)
	require.Equal(t, ContactSyncStats{Updated: 1}, stats)
	assert.True(t, mpContactFindable(t, db, user.ID, "Ixchel", synced.ID), "the updated synced name is findable")
	assert.False(t, mpContactFindable(t, db, user.ID, "Zarniwoop", synced.ID), "the replaced synced name is gone from the index")
	assertFTSCanonical(t, db)

	// remote delete: reconcile archives (does not delete). An archived
	// contact is still live (deleted_at IS NULL), so it deliberately stays in
	// the index — search correctness for it is the outer query's job, not the
	// index's (ADR 0012 INV-D9). The index must stay canonical.
	stats, err = reconcileContactSync(db, sub, nil, []string{obj.Path}, false, "")
	require.NoError(t, err)
	require.Equal(t, ContactSyncStats{Archived: 1}, stats)

	var archived models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, "ford-uid").First(&archived).Error)
	require.True(t, archived.Archived, "reconcile archived the remotely-deleted contact")
	var stillIndexed int64
	require.NoError(t, db.Raw("SELECT count(*) FROM contacts_fts WHERE rowid = ?", archived.ID).Scan(&stillIndexed).Error)
	assert.Equal(t, int64(1), stillIndexed, "an archived (still-live) contact stays in the index by contract")
	assertFTSCanonical(t, db)
}

// ---------------------------------------------------------------------------
// Bulk VCF import
// ---------------------------------------------------------------------------

// TestSearchMutationPath_BulkVCFImport covers the import engine's two write
// branches — "add" (tx.Create) and "update"/merge-into-existing (tx.Save after
// MergeImportedContact) — asserting the imported/merged searchable content is
// findable and the index stays canonical.
func TestSearchMutationPath_BulkVCFImport(t *testing.T) {
	db := dbtest.New(t)
	user := mpUser(t, db, "bulk-import")
	log := testImportLogger()
	cfg := &config.Config{}

	// An existing contact the import will merge into (matched on email).
	existing := models.Contact{UserID: user.ID, Firstname: "Slartibartfast", Email: "slarti@magrathea.example"}
	require.NoError(t, db.Create(&existing).Error)

	m := NewImportSessionManager()
	vcfContacts := []VCFContactData{
		{Contact: &models.Contact{Firstname: "Deep", Lastname: "Thought", Organization: "Magrathea Computing", Email: "deepthought@magrathea.example"}},
		{Contact: &models.Contact{Firstname: "Slartibartfast", Lastname: "Fjordmaker", Email: "slarti@magrathea.example"}},
	}
	previews := []models.ImportRowPreview{
		{RowIndex: 0, SuggestedAction: "add"},
		{RowIndex: 1, DuplicateMatch: &models.DuplicateMatch{ExistingContactID: existing.ID, MatchReason: "email"}, SuggestedAction: "update"},
	}
	sessionID := m.CreateVCFSession(user.ID, vcfContacts, previews)

	result, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: sessionID,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "add"},
			{RowIndex: 1, Action: "update"},
		},
	}, cfg, log)
	require.Nil(t, appErr)
	require.NotNil(t, result)
	require.Empty(t, result.Errors)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Updated)

	// The added contact is findable on a column only it carries.
	var added models.Contact
	require.NoError(t, db.Where("user_id = ? AND email = ?", user.ID, "deepthought@magrathea.example").First(&added).Error)
	assert.True(t, mpContactFindable(t, db, user.ID, "Magrathea", added.ID), "an added contact's org is searchable after import")

	// The merged contact absorbed the incoming lastname.
	var merged models.Contact
	require.NoError(t, db.Where("id = ?", existing.ID).First(&merged).Error)
	require.Equal(t, "Fjordmaker", merged.Lastname, "import merge filled the empty lastname")
	assert.True(t, mpContactFindable(t, db, user.ID, "Fjordmaker", merged.ID), "the merged-in lastname is searchable")

	assertFTSCanonical(t, db)
}

// ---------------------------------------------------------------------------
// Structural guard (issue #463 action 5)
// ---------------------------------------------------------------------------

// ftsInsertColRE pulls the column list out of an FTS-maintenance trigger body:
// `INSERT INTO <index>(rowid, user_id, col, ...) ...`.
var ftsInsertColRE = regexp.MustCompile(`(?is)INSERT\s+INTO\s+\w+_fts\s*\(([^)]*)\)`)

// ftsCreateColRE pulls the column list out of `CREATE VIRTUAL TABLE <index>
// USING fts5( ... )`.
var ftsCreateColRE = regexp.MustCompile(`(?is)USING\s+fts5\s*\((.*)\)`)

// splitCols normalises a comma-separated SQL column list to a sorted set of
// bare names: lower-cased, `UNINDEXED`/quotes stripped, `rowid` dropped (it is
// the implicit FTS key, never a content column).
func splitCols(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		f := strings.Fields(strings.TrimSpace(part))
		if len(f) == 0 {
			continue
		}
		name := strings.Trim(strings.ToLower(f[0]), `"'`)
		if name == "" || name == "rowid" {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// TestSearchMutationPath_TriggerColumnsCoverSearchableFields is the structural
// guard against issue #462's "blind spot 2" (a write to a column the trigger
// does not project). It pins that, for every FTS index, four independently
// maintained column lists agree exactly:
//
//   - the virtual table's own column list (the migration's CREATE VIRTUAL TABLE);
//   - the AFTER INSERT trigger's projection;
//   - the AFTER UPDATE trigger's projection (the subtlest one — a column added
//     to the table and the AI trigger but forgotten here is silently
//     un-maintained on every edit);
//   - ftsConsistencySpecs — the set SEARCH-02's consistency check and
//     RebuildSearchIndex treat as the indexed content (ADR 0012 INV-D9's
//     "add it to RebuildSearchIndex and ftsConsistencySpecs").
//
// Adding a searchable field without extending all four fails here, so this
// ticket's coverage does not decay with the next feature that widens search.
func TestSearchMutationPath_TriggerColumnsCoverSearchableFields(t *testing.T) {
	db := dbtest.New(t)

	specByIndex := map[string]ftsConsistencySpec{}
	for _, s := range ftsConsistencySpecs {
		specByIndex[s.index] = s
	}

	for _, index := range ftsTables {
		t.Run(index, func(t *testing.T) {
			spec, ok := specByIndex[index]
			require.Truef(t, ok, "ftsConsistencySpecs has no entry for %s", index)

			// The virtual table's declared columns (minus the UNINDEXED
			// user_id, which is scoping, not searchable content).
			var createSQL string
			require.NoError(t, db.Raw(
				`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, index).Scan(&createSQL).Error)
			require.NotEmpty(t, createSQL)
			cm := ftsCreateColRE.FindStringSubmatch(createSQL)
			require.Lenf(t, cm, 2, "could not parse columns from %q", createSQL)
			tableCols := splitCols(cm[1])
			tableContentCols := withoutUserID(tableCols)

			// The two maintenance triggers' INSERT projections.
			for _, suffix := range []string{"ai", "au"} {
				name := index + "_" + suffix
				var trigSQL string
				require.NoError(t, db.Raw(
					`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = ?`, name).Scan(&trigSQL).Error)
				require.NotEmptyf(t, trigSQL, "trigger %s not found — the index is not trigger-maintained", name)
				im := ftsInsertColRE.FindStringSubmatch(trigSQL)
				require.Lenf(t, im, 2, "could not parse INSERT columns from trigger %s", name)
				assert.Equalf(t, tableCols, splitCols(im[1]),
					"%s projects a different column set than the %s virtual table declares", name, index)
			}

			// ftsConsistencySpecs (SEARCH-02 / RebuildSearchIndex).
			wantContent := append([]string(nil), spec.contentCols...)
			sort.Strings(wantContent)
			assert.Equalf(t, tableContentCols, wantContent,
				"ftsConsistencySpecs[%s].contentCols is out of sync with the indexed columns — "+
					"a searchable field was added to the triggers without updating the consistency check / rebuild (or vice versa)", index)
		})
	}
}

func withoutUserID(cols []string) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		if c == "user_id" {
			continue
		}
		out = append(out, c)
	}
	return out
}
