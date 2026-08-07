package services

import (
	"mycorrhizal/database"
	"mycorrhizal/models"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMergeImportedContact_RealDB_VCF is the real-DB regression test for T49
// (docs/fork-plan/tickets/58-T49-vcf-import-merge-corrupts-existing-contact.md),
// modeled directly on the ticket's own reproduction: create a contact with a
// real phone/email and a linked Gift, then run it through the exact sequence
// ConfirmVCF's "update" branch uses -- MergeImportedContact(&existing,
// incoming) followed by tx.Save(&existing) -- with an incoming contact whose
// VCardUID is freshly generated (the common case for a real-world vCard with
// no UID: property) and whose Phones/Emails don't repeat every value the
// existing contact already has.
//
// This runs against database.InitDB's real migrated schema, not AutoMigrate,
// per /CLAUDE.md's backend trap 1 -- and because the entity_id/vcard_uid
// reachability this test pins is exactly a real-schema concern.
func TestMergeImportedContact_RealDB_VCF(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "vcf-merge-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "vcf-merge-realdb", Password: "password123!A", Email: "vcf-merge-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := models.Contact{
		UserID:    user.ID,
		Firstname: "Elizabeth",
		Lastname:  "Bennet",
		Emails:    []models.ContactEmail{{Type: "home", Value: "elizabeth@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "+1-608-000-0000"}},
	}
	require.NoError(t, db.Create(&existing).Error)
	require.NotEmpty(t, existing.VCardUID, "BeforeCreate should have minted a stable VCardUID")
	originalUID := existing.VCardUID

	gift := models.Gift{UserID: user.ID, EntityID: originalUID, Description: "Piano sheet music"}
	require.NoError(t, db.Create(&gift).Error)

	// Simulates a real-world vCard: no UID: property (ParseVCF mints a fresh
	// random UUID for it), and it only knows about a work email/second phone
	// -- it doesn't repeat the ones already on file, the way a partial or
	// differently-sourced export commonly wouldn't.
	incoming := &models.Contact{
		VCardUID: "freshly-generated-uuid-from-parsevcf",
		Emails:   []models.ContactEmail{{Type: "work", Value: "e.bennet@example.com"}},
		Phones:   []models.ContactPhone{{Type: "home", Value: "+1-608-111-1111"}},
	}

	MergeImportedContact(&existing, incoming)
	require.NoError(t, db.Save(&existing).Error)

	// 1. Identity must never move.
	assert.Equal(t, originalUID, existing.VCardUID, "an update import must never reassign the existing contact's identity")

	// 2. Existing data survives, and the new entry is added alongside it --
	// not "last import wins".
	require.Len(t, existing.Emails, 2)
	assert.Equal(t, "elizabeth@example.com", existing.Emails[0].Value)
	assert.Equal(t, "e.bennet@example.com", existing.Emails[1].Value)
	require.Len(t, existing.Phones, 2)
	assert.Equal(t, "+1-608-000-0000", existing.Phones[0].Value)
	assert.Equal(t, "+1-608-111-1111", existing.Phones[1].Value)

	// 3. The Gift row must still be reachable by the contact's (unchanged)
	// VCardUID -- the exact "gifts was also wiped out" symptom the ticket
	// reproduced (the Gift row itself was never deleted, it was orphaned).
	var reachableGifts int64
	require.NoError(t, db.Model(&models.Gift{}).Where("entity_id = ?", existing.VCardUID).Count(&reachableGifts).Error)
	assert.Equal(t, int64(1), reachableGifts)

	// Re-fetch from the DB to prove this isn't just an in-memory artifact of
	// the struct passed to Save.
	var persisted models.Contact
	require.NoError(t, db.First(&persisted, existing.ID).Error)
	assert.Equal(t, originalUID, persisted.VCardUID)
	assert.Len(t, persisted.Emails, 2)
	assert.Len(t, persisted.Phones, 2)
}

// TestMergeImportedContact_RealDB_CSV proves the same fix covers CSV import's
// merge path, not just VCF's -- MergeImportedContact is shared by both call
// sites (import_session.go), so a fix scoped to VCF alone would leave CSV
// exploitable the same way.
func TestMergeImportedContact_RealDB_CSV(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "csv-merge-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "csv-merge-realdb", Password: "password123!A", Email: "csv-merge-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := models.Contact{
		UserID:    user.ID,
		Firstname: "Fitzwilliam",
		Lastname:  "Darcy",
		Emails:    []models.ContactEmail{{Type: "home", Value: "darcy@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "+1-608-222-2222"}},
	}
	require.NoError(t, db.Create(&existing).Error)
	originalUID := existing.VCardUID
	require.NotEmpty(t, originalUID)

	gift := models.Gift{UserID: user.ID, EntityID: originalUID, Description: "First edition novel"}
	require.NoError(t, db.Create(&gift).Error)

	// A CSV row with a mapped email column but an empty cell for it -- the
	// same "len() > 0 but no content" shape T50's vCard 2.1 gap produces,
	// reachable here via BuildContactFromRow (the CSV parse path).
	headers := []string{"First Name", "Email"}
	row := []string{"Fitzwilliam", ""}
	mappings := []models.ColumnMapping{
		{CSVColumn: "First Name", ContactField: "firstname"},
		{CSVColumn: "Email", ContactField: "email", Group: 0},
	}
	incoming := BuildContactFromRow(user.ID, headers, row, mappings)
	// BuildContactFromRow only appends an email entry for a non-empty cell
	// (putValue skips ""), so simulate the len()>0-blank-value shape directly
	// to prove the fix isn't just "CSV happens not to produce this case".
	incoming.Emails = []models.ContactEmail{{Type: "home", Value: ""}}
	incoming.VCardUID = "csv-import-has-no-vcard-uid-at-all-but-this-proves-it-would-be-ignored"

	MergeImportedContact(&existing, &incoming)
	require.NoError(t, db.Save(&existing).Error)

	assert.Equal(t, originalUID, existing.VCardUID)
	require.Len(t, existing.Emails, 1, "a blank-valued incoming email must not wipe the existing one")
	assert.Equal(t, "darcy@example.com", existing.Emails[0].Value)

	var reachableGifts int64
	require.NoError(t, db.Model(&models.Gift{}).Where("entity_id = ?", existing.VCardUID).Count(&reachableGifts).Error)
	assert.Equal(t, int64(1), reachableGifts)
}
