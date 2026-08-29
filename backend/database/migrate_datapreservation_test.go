package database

import (
	"database/sql"
	"mycorrhizal/models"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestFullChainMigrationPreservesRealData is issue #254's regression test,
// reworked for the supported-upgrade floor (issue #529).
//
// CLAUDE.md's orientation section is explicit: real production data exists
// as of v0.2.0-alpha-candidate, and "migrations must preserve existing
// data." Every other migration test in this package proves a migration's SQL
// is syntactically valid (TestMigrationsApplyToEmptyDatabase) or that ONE
// migration in isolation preserves data across itself (e.g.
// TestMigrationsAddGiftURLAndNotes). Neither proves the CUMULATIVE chain —
// every migration after the tag, applied together against a real historical
// snapshot — still preserves rows. A future migration that drops, renames,
// or retypes a column would pass every existing test here and still
// silently destroy data in prod.
//
// Under the floor policy the v0.2.0-alpha-candidate snapshot is a SUB-FLOOR
// database: the normal startup path must refuse to migrate it (asserted
// first, naming v0.6.0 as the required intermediate), and the full chain runs
// only through the documented one-time bridge override — exactly what the
// maintainer's own v0.2.0-alpha-candidate deployment procedure
// (docs/upgrade-compatibility.md) uses. This test therefore: (1) applies
// exactly the migrations that existed at v0.2.0-alpha-candidate
// (000001-000008 — verified identical in shape to the tag itself; see the
// testdata fixture's own comment), (2) loads a seed fixture of
// representative rows written directly against that schema, no GORM involved,
// exactly like real rows that predate every migration since, (3) asserts the
// startup refusal, (4) runs database.InitDB with the bridge override — the
// same entry point the real server uses on startup — to apply every
// migration from there to HEAD, and (5) asserts the data survived: counts,
// key values, and that the soft-deleted contact is still there (via
// Unscoped()), not resurrected and not hard-deleted.
func TestFullChainMigrationPreservesRealData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "full-chain.db")

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)

	m, err := newMigrator(sqlDB)
	require.NoError(t, err)

	// Exactly the migrations present at v0.2.0-alpha-candidate. NOT m.Up():
	// this must stop at the tag's own state, not run the current chain, or
	// the fixture below would be inserted against a schema shape it was
	// never written for.
	require.NoError(t, m.Steps(8))

	fixture, err := os.ReadFile(filepath.Join("migrations", "testdata", "v0_2_0_alpha_candidate_seed.sql"))
	require.NoError(t, err)
	_, err = sqlDB.Exec(string(fixture))
	require.NoError(t, err, "the seed fixture must load cleanly against the v0.2.0-alpha-candidate schema shape")

	require.NoError(t, sqlDB.Close())

	// Issue #529: a sub-floor database refuses to migrate on startup, naming
	// v0.6.0 as the required intermediate — never a partial migration, never
	// a best-effort single hop.
	_, err = InitDB(dbPath)
	require.Error(t, err, "a v0.2.0-alpha-candidate database must refuse to migrate")
	var subFloor *ErrSubFloorMigration
	require.ErrorAs(t, err, &subFloor)
	assert.EqualValues(t, 8, subFloor.Version)
	assert.Contains(t, err.Error(), "v0.6.0", "the refusal must name v0.6.0 as the required intermediate")

	// The one-time bridge (docs/upgrade-compatibility.md): the documented
	// override lets the full chain run in one binary when the v0.6.0
	// intermediate cannot be produced. The production entry point applies
	// every migration from 000008 to HEAD, then opens the GORM connection the
	// rest of the app uses.
	t.Setenv(subFloorMigrationEnvVar, "1")
	db, err := InitDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		conn, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, conn.Close())
	})

	version, dirty, ok, err := MigrationVersion(dbPath)
	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, dirty)
	assert.GreaterOrEqual(t, version, uint(31), "the full current migration chain must have applied on top of the seed")

	// Contacts: three total, two still visible through GORM's default
	// soft-delete scope, Carol only visible with Unscoped(). A lingering
	// soft-deleted row is someone's undo button, not a test fixture.
	var totalContacts, liveContacts int64
	require.NoError(t, db.Model(&models.Contact{}).Unscoped().Count(&totalContacts).Error)
	require.NoError(t, db.Model(&models.Contact{}).Count(&liveContacts).Error)
	assert.EqualValues(t, 3, totalContacts, "all three seeded contacts must still exist at some level")
	assert.EqualValues(t, 2, liveContacts, "exactly the one soft-deleted contact must be excluded from the default scope")

	var carol models.Contact
	require.NoError(t, db.Unscoped().Where("vcard_uid = ?", "vcard-seed-carol").First(&carol).Error)
	assert.True(t, carol.DeletedAt.Valid, "Carol must still be marked soft-deleted, not resurrected")
	assert.Equal(t, "Diaz", carol.Lastname, "a soft-deleted contact's own data must survive untouched")

	var carolLive models.Contact
	err = db.Where("vcard_uid = ?", "vcard-seed-carol").First(&carolLive).Error
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound, "a soft-deleted contact must not be visible through the default scope")

	// Alice: the rich contact. Her `card` column holds nested data (an
	// address's individual components, and a free-text note) that has no
	// flat-column home of its own — exactly what CLAUDE.md backend trap #3
	// warns a wrong plain-save path can silently drop. This test never saves
	// her through GORM, but proves the migration chain itself doesn't touch
	// (and so can't drop) that data either.
	var alice models.Contact
	require.NoError(t, db.Where("vcard_uid = ?", "vcard-seed-alice").First(&alice).Error)
	assert.Equal(t, "Nakamura", alice.Lastname)

	// Read the raw `card` column text rather than the GORM-deserialized
	// contactmodel.Card struct — a raw string Contains check is exactly what
	// this test wants to prove (the bytes made it through untouched), and it
	// can't be fooled by a struct field that happens to round-trip through
	// json serialization/deserialization while silently losing something
	// else along the way.
	var cardJSON string
	require.NoError(t, db.Raw("SELECT card FROM contacts WHERE vcard_uid = ?", "vcard-seed-alice").Scan(&cardJSON).Error)
	assert.Contains(t, cardJSON, "Fungal Networks Ltd", "Alice's organization must survive in the card JSON")
	assert.Contains(t, cardJSON, "742 Spore Lane", "Alice's nested address components must survive in the card JSON")
	assert.Contains(t, cardJSON, "Met at the North American Mycological Association conference.",
		"Alice's card-only note (no flat-column home) must survive the full migration chain")

	// Note, activity (+ its join row), reminder, and relationship edge: one
	// of each, keyed off Alice, all present and unchanged.
	var noteCount int64
	require.NoError(t, db.Model(&models.Note{}).Where("contact_id = ?", alice.ID).Count(&noteCount).Error)
	assert.EqualValues(t, 1, noteCount)
	var note models.Note
	require.NoError(t, db.Where("contact_id = ?", alice.ID).First(&note).Error)
	assert.Equal(t, "Loves truffle hunting and slow correspondence.", note.Content)

	var activityCount int64
	require.NoError(t, db.Model(&models.Activity{}).Where("uuid = ?", "activity-seed-1").Count(&activityCount).Error)
	assert.EqualValues(t, 1, activityCount)

	var activityContactCount int64
	require.NoError(t, db.Table("activity_contacts").
		Joins("JOIN activities ON activities.id = activity_contacts.activity_id").
		Where("activities.uuid = ?", "activity-seed-1").
		Count(&activityContactCount).Error)
	assert.EqualValues(t, 2, activityContactCount, "both linked contacts on the activity must survive")

	var reminder models.Reminder
	require.NoError(t, db.Where("contact_id = ?", alice.ID).First(&reminder).Error)
	assert.Equal(t, "Follow up about the lab opening", reminder.Message)

	var edge models.RelationshipEdge
	require.NoError(t, db.Where("id = ?", "edge-seed-1").First(&edge).Error)
	assert.Equal(t, "vcard-seed-alice", edge.SourceID)
	assert.Equal(t, "vcard-seed-bob", edge.TargetID)
	assert.Equal(t, "friend_of", edge.Type)
	assert.Equal(t, "confirmed", edge.Status, "only confirmed edges are fact — this seeded edge must still read as confirmed")
}
