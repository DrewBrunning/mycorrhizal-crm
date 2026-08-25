package controllers

// Issue #416: pins the real-DB behavior for a colliding VCardUID on VCF
// import. database/migrations/000001_initial_schema.up.sql:90-92 declares
//
//	CREATE UNIQUE INDEX idx_contacts_vcard_uid_user
//	    ON contacts(user_id, vcard_uid)
//	    WHERE vcard_uid IS NOT NULL AND deleted_at IS NULL
//
// -- so a second contact created with a UID already taken by this user
// cannot silently duplicate the identity every graph-adjacent table joins
// on via VCardUID (Gift, CircleMember, Tag, FieldValue, ...). Neither of the
// tests below use setupImportSessionTestDB-style AutoMigrate: that only
// derives schema from models.Contact's GORM struct tags (a plain `index`,
// not `uniqueIndex` -- see models/contact.go), which would silently miss
// this constraint entirely (CLAUDE.md backend trap 1). database.InitDB runs
// the real hand-written migration, so the constraint is actually exercised.
//
// services/import_session.go's ConfirmVCF "add" branch already checks
// tx.Create(&contact).Error and records a per-row failure rather than
// aborting -- these tests prove that holds (one contact created, the
// colliding one turned into a recorded error, not a crash and not a second
// row), and that SQLite doesn't abort the rest of the transaction after one
// row's UNIQUE violation.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// vcfWithUID builds a minimal, valid vCard 3.0 block carrying an explicit
// UID -- go-vcard's importer preserves an incoming UID verbatim rather than
// minting a fresh one (services/import_service.go only mints a UUID when
// the parsed UID is empty), which is exactly the scenario under test. Both
// FN and the structured N are set: Contact.Firstname/Lastname come from N,
// and ValidateImportedContact requires a non-empty Firstname, so a vCard
// with only FN (no N) would fail validation and default to "skip" rather
// than "add" -- not the collision this test is pinning.
func vcfWithUID(uid, given, family, email string) string {
	return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + given + " " + family +
		"\r\nN:" + family + ";" + given + ";;;\r\nEMAIL:" + email + "\r\nEND:VCARD\r\n"
}

func TestConfirmVCF_DuplicateUIDWithinFile_OneCreated_OneGracefulError(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "dup-uid-within-file.db"))
	require.NoError(t, err)
	user := models.User{Username: "dupuidwithin", Password: "password123!A", Email: "dupuidwithin@example.com"}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, cfg)

	const sharedUID = "11111111-1111-1111-1111-111111111111"
	vcf := vcfWithUID(sharedUID, "Alice", "Anderson", "alice@example.com") +
		vcfWithUID(sharedUID, "Bob", "Brown", "bob@example.com")

	uploadReq := newFileUploadRequest(t, "/contacts/import/vcf/upload", "dup.vcf", []byte(vcf))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))
	require.Len(t, uploadResp.Rows, 2, "both vCards must parse -- this test is about the create-time collision, not a parse rejection")
	require.Equal(t, "add", uploadResp.Rows[0].SuggestedAction, "different name/email, so existing duplicate detection (email/name/phone) must not flag these as the same contact")
	require.Equal(t, "add", uploadResp.Rows[1].SuggestedAction)

	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "add"},
			{RowIndex: 1, Action: "add"},
		},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &result))

	assert.Equal(t, 1, result.Created, "exactly one of the two same-UID rows must be created")
	assert.Equal(t, 1, result.Skipped, "the colliding second row must be skipped, not silently duplicated")
	require.Len(t, result.Errors, 1, "the collision must surface as a recorded per-row error")
	assert.NotEmpty(t, result.Errors[0])

	var contacts []models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, sharedUID).Find(&contacts).Error)
	assert.Len(t, contacts, 1, "the DB must hold exactly one contact for this UID, never two")
}

func TestConfirmVCF_DuplicateUIDAgainstExistingContact_ExistingUntouched(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "dup-uid-existing.db"))
	require.NoError(t, err)
	user := models.User{Username: "dupuidexisting", Password: "password123!A", Email: "dupuidexisting@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := models.Contact{UserID: user.ID, Firstname: "Carol", Lastname: "Clark", Email: "carol@example.com"}
	require.NoError(t, db.Create(&existing).Error)
	require.NotEmpty(t, existing.VCardUID)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, cfg)

	// A different name/email than the existing contact, so DetectDuplicate
	// (email/name/phone) does not flag this as an update candidate -- the
	// only thing the two rows share is the VCardUID.
	vcf := vcfWithUID(existing.VCardUID, "Dave", "Davis", "dave@example.com")

	uploadReq := newFileUploadRequest(t, "/contacts/import/vcf/upload", "dup2.vcf", []byte(vcf))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))
	require.Len(t, uploadResp.Rows, 1)
	require.Equal(t, "add", uploadResp.Rows[0].SuggestedAction, "different name/email must not be flagged as an update of the existing contact")

	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "add"},
		},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &result))

	assert.Equal(t, 0, result.Created, "a create using an already-taken VCardUID must not succeed")
	assert.Equal(t, 1, result.Skipped)
	require.Len(t, result.Errors, 1)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	assert.Equal(t, "Carol", reloaded.Firstname, "the pre-existing contact must be untouched by the failed collision")
	assert.Equal(t, "Clark", reloaded.Lastname)

	var contacts []models.Contact
	require.NoError(t, db.Where("user_id = ? AND vcard_uid = ?", user.ID, existing.VCardUID).Find(&contacts).Error)
	assert.Len(t, contacts, 1, "still exactly one contact for this UID")
}
