package services

// TestConfirmVCF_RealDB_ControlCharactersAndInvalidUTF8_StaySanitizedOnSave
// is the VCF-import-assistant regression test for the fix that came out of
// issue #512's CardDAV hostile-input work: SanitizeImportedContact
// (BuildImportRowPreview's issue #416 fix-up) cleans the flat
// Firstname/Lastname fields, but until this fix left contact.Card.Name
// (the neutral full-fidelity copy ApplyRecordToContact also populates)
// untouched. Contact.BeforeSave's cardSetDirectly branch (models/contact.go)
// re-derives Firstname/Lastname/FN from contact.Card on every save that
// follows ApplyRecordToContact -- exactly what ConfirmVCF's "add" branch
// does (services/import_session.go: `contact := *vcfData.Contact;
// tx.Create(&contact)`) -- so the sanitized flat fields were silently
// reverted to the original hostile bytes the moment the contact was
// actually saved. This drives ParseVCF -> ConfirmVCF's exact "add" sequence
// against a real database.InitDB-migrated schema to prove the save, not
// just the in-memory preview, ends up clean.
import (
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfirmVCF_RealDB_ControlCharactersAndInvalidUTF8_StaySanitizedOnSave(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "vcf-hostile-controlchars.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "vcf-hostile", Password: "password123!A", Email: "vcf-hostile@example.com"}
	require.NoError(t, db.Create(&user).Error)

	// 0x01/0x02 are C0 control characters (dropped); 0xff is not valid
	// standalone UTF-8 (replaced with U+FFFD) -- same two hostilities
	// services/import_sanitize_test.go's CSV-path test pins, now exercised
	// through the VCF adapter instead of BuildContactFromRow.
	raw := "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:vcf-hostile-uid\r\nFN:Ada\x01Lovelace\r\nN:Lovelace\xff;Ada\x02;;;\r\nEND:VCARD\r\n"

	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)

	// The preview (BuildImportRowPreview's own SanitizeImportedContact call)
	// already reflects clean data before this fix; the bug was save-time
	// reversion, not the preview.
	assert.Equal(t, "Ada", contacts[0].Contact.Firstname)
	assert.Equal(t, "Lovelace�", contacts[0].Contact.Lastname)

	// Mirrors ConfirmVCF's "add" branch exactly (services/import_session.go):
	// a value copy of the previewed contact, UserID set, then tx.Create.
	contact := *contacts[0].Contact
	contact.UserID = user.ID
	require.NoError(t, db.Create(&contact).Error)

	var persisted models.Contact
	require.NoError(t, db.First(&persisted, contact.ID).Error)
	assert.Equal(t, "Ada", persisted.Firstname, "BeforeSave's cardSetDirectly re-derivation must not resurrect the control character from contact.Card.Name")
	assert.Equal(t, "Lovelace�", persisted.Lastname, "BeforeSave's cardSetDirectly re-derivation must not resurrect the invalid UTF-8 byte from contact.Card.Name")
	assert.False(t, strings.ContainsRune(persisted.Firstname, 0x01))
	assert.False(t, strings.ContainsRune(persisted.Lastname, 0xff))
}
