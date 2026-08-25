package services

// Issue #416: import data hygiene. sanitizeImportedText/SanitizeImportedContact
// fix invalid UTF-8 and disallowed control characters on the free-text
// Contact fields an import path can populate -- see SanitizeImportedContact's
// doc comment for why length/HTML are deliberately NOT touched here.

import (
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeImportedText_CleanInputUnchanged(t *testing.T) {
	cleaned, changed := sanitizeImportedText("Ada Lovelace, met at PyCon\tin 2019\n")
	assert.False(t, changed)
	assert.Equal(t, "Ada Lovelace, met at PyCon\tin 2019\n", cleaned)
}

func TestSanitizeImportedText_InvalidUTF8Replaced(t *testing.T) {
	// 0xFF is never valid as a standalone UTF-8 byte.
	hostile := "Ada\xffLovelace"
	cleaned, changed := sanitizeImportedText(hostile)
	assert.True(t, changed)
	assert.True(t, strings.Contains(cleaned, "Ada"))
	assert.True(t, strings.Contains(cleaned, "Lovelace"))
	assert.False(t, strings.ContainsRune(cleaned, 0xFF))
}

func TestSanitizeImportedText_ControlCharactersStrippedTabNewlineKept(t *testing.T) {
	hostile := "Ada\x00\x1bLovelace\tsecond line\ncarriage\rreturn"
	cleaned, changed := sanitizeImportedText(hostile)
	assert.True(t, changed)
	assert.Equal(t, "AdaLovelace\tsecond line\ncarriage\rreturn", cleaned)
}

func TestSanitizeImportedText_C1ControlCharacterStripped(t *testing.T) {
	// U+0085 (NEL) is a C1 control character, distinct from the C0 range.
	hostile := "AdaLovelace"
	cleaned, changed := sanitizeImportedText(hostile)
	assert.True(t, changed)
	assert.Equal(t, "AdaLovelace", cleaned)
}

func TestSanitizeImportedContact_AppliesToAllListedTextFields_LeavesOthersAlone(t *testing.T) {
	contact := &models.Contact{
		Firstname:          "Ada\x00",
		Lastname:           "Lovelace\x1b",
		Nickname:           "AL\x07",
		Gender:             "F\x02",
		Address:            "1 Main St\x03",
		HowWeMet:           "PyCon\x04",
		WorkInformation:    "Engineer\x05",
		ContactInformation: "Prefers email\x06",
		Email:              "ada@example.com", // untouched: has its own validator
	}

	notes := SanitizeImportedContact(contact)

	assert.Equal(t, "Ada", contact.Firstname)
	assert.Equal(t, "Lovelace", contact.Lastname)
	assert.Equal(t, "AL", contact.Nickname)
	assert.Equal(t, "F", contact.Gender)
	assert.Equal(t, "1 Main St", contact.Address)
	assert.Equal(t, "PyCon", contact.HowWeMet)
	assert.Equal(t, "Engineer", contact.WorkInformation)
	assert.Equal(t, "Prefers email", contact.ContactInformation)
	assert.Equal(t, "ada@example.com", contact.Email, "Email is not in the sanitized field list -- its own format validator handles it")
	assert.Len(t, notes, 8, "one diagnostic note per changed field")
}

func TestSanitizeImportedContact_NothingChanged_NoNotes(t *testing.T) {
	contact := &models.Contact{Firstname: "Ada", Lastname: "Lovelace"}
	notes := SanitizeImportedContact(contact)
	assert.Empty(t, notes)
}

// TestBuildContactFromRow_HostileControlCharactersAndInvalidUTF8_ImportsCleanly
// is the CSV-import integration case: a hostile byte sequence in a mapped
// free-text column survives BuildContactFromRow + BuildImportRowPreview as
// clean text with a diagnostic, not a crash and not mojibake in the stored
// row.
func TestBuildContactFromRow_HostileControlCharactersAndInvalidUTF8_ImportsCleanly(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	headers := []string{"First Name", "Last Name", "Notes"}
	row := []string{"Ada\x00", "Lovelace\xff", "Met at PyCon\x1b in 2019"}
	mappings := SuggestColumnMappings(headers)

	contact := BuildContactFromRow(user.ID, headers, row, mappings)
	stats := &ImportStats{}
	preview := BuildImportRowPreview(db, user.ID, &contact, 0, nil, nil, stats)

	require.Empty(t, preview.ValidationErrors, "sanitized input must still pass validation")
	assert.Equal(t, "Ada", contact.Firstname)
	// The invalid byte is replaced (U+FFFD), not silently dropped -- \xff
	// is not a control character, so only the UTF-8 fixup touches it.
	assert.Equal(t, "Lovelace�", contact.Lastname)
	assert.Equal(t, "Met at PyCon in 2019", contact.HowWeMet)
	assert.NotEmpty(t, preview.Diagnostics, "the sanitization must be surfaced as a diagnostic, not applied silently")
}

// TestBuildContactFromRow_HTMLScriptInFreeTextField_StoredLiterallyNotStripped
// pins the "malicious HTML/embedded scripts in free-text fields" bullet of
// issue #416 by proof, not assertion: this repo does not sanitize/strip
// HTML on import (see sanitizeImportedText's doc comment for why), and the
// frontend renders every free-text field via plain JSX interpolation with
// no dangerouslySetInnerHTML or markdown rendering anywhere in frontend/src
// -- so the payload is inert wherever it is displayed. This test proves the
// backend half of that claim: the payload round-trips byte-for-byte.
func TestBuildContactFromRow_HTMLScriptInFreeTextField_StoredLiterallyNotStripped(t *testing.T) {
	db, _ := setupRouter()
	var user models.User
	db.First(&user)

	const payload = `<script>alert(1)</script>`
	headers := []string{"First Name", "Last Name", "Notes"}
	row := []string{"Ada", "Lovelace", payload}
	mappings := SuggestColumnMappings(headers)

	contact := BuildContactFromRow(user.ID, headers, row, mappings)
	stats := &ImportStats{}
	preview := BuildImportRowPreview(db, user.ID, &contact, 0, nil, nil, stats)

	require.Empty(t, preview.ValidationErrors)
	assert.Equal(t, payload, contact.HowWeMet, "HTML/script in free text is stored verbatim -- neutralizing it here would destroy real user data to close a risk the frontend already doesn't have")
}
