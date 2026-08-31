package services

// TEST-04 (issue #432) integration tests: the adversarial corpus's tiers
// are asserted at the adapter level in internal/adversarial/adversarial_test.go;
// this file proves the tiers that only the full import pipeline can — the
// bounded-failure guarantees for multi-record files, the splitter's handling
// of END:VCARD inside a value, BOM and vCard 2.1 routing through ParseVCF,
// and that formula-prefixed values survive import intact for the export
// boundary to neutralize.

import (
	"strings"
	"testing"

	"mycorrhizal/internal/adversarial"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newAdversarialImportTest(t *testing.T, username string) (*gorm.DB, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: username, Password: "password123!A", Email: username + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return db, user
}

func parseAdversarialVCF(t *testing.T, fixture string) ([]VCFContactData, []models.ImportRowPreview, ImportStats) {
	t.Helper()
	db, user := newAdversarialImportTest(t, "adv-"+strings.TrimSuffix(fixture, ".vcf"))
	contacts, previews, stats, err := ParseVCF(strings.NewReader(string(adversarial.LoadFixture(fixture))), db, user.ID)
	require.NoError(t, err)
	return contacts, previews, stats
}

// TestParseVCF_BoundedFailure_MalformedMiddle drives
// multi-malformed-middle.vcf: good + invalid-END + good. The malformed
// record must be isolated to a named skip row, never abort the batch, and
// never leave a partial contact.
func TestParseVCF_BoundedFailure_MalformedMiddle(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "multi-malformed-middle.vcf")

	require.Len(t, contacts, 2, "both valid records must import")
	require.Len(t, previews, 3, "all three records must be surfaced, the failure named")
	assert.Equal(t, 2, stats.ValidCount)
	assert.Equal(t, 1, stats.ErrorCount)
	assert.Equal(t, "Good One", contacts[0].Contact.Firstname)
	assert.Equal(t, "Good Two", contacts[1].Contact.Firstname)

	skip := findSkipPreview(t, previews)
	assert.Contains(t, strings.Join(skip.ValidationErrors, " "), "Failed to parse vCard")
	assert.Contains(t, strings.Join(skip.ValidationErrors, " "), "invalid END value")
}

// TestParseVCF_BoundedFailure_TruncatedMiddle drives
// multi-truncated-middle.vcf: good + truncated(missing END) + good. The
// truncated card must be its own failing row — not silently merged into the
// next card (the bug the line-based splitter fixed).
func TestParseVCF_BoundedFailure_TruncatedMiddle(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "multi-truncated-middle.vcf")

	require.Len(t, contacts, 2, "both valid records must import")
	require.Len(t, previews, 3)
	assert.Equal(t, 2, stats.ValidCount)
	assert.Equal(t, 1, stats.ErrorCount)
	assert.Equal(t, "Good One", contacts[0].Contact.Firstname)
	assert.Equal(t, "Good Two", contacts[1].Contact.Firstname, "the truncated card must not swallow the following card's data")

	skip := findSkipPreview(t, previews)
	assert.Contains(t, strings.Join(skip.ValidationErrors, " "), "Failed to parse vCard")
	assert.Contains(t, strings.Join(skip.ValidationErrors, " "), "no END field found")
}

func findSkipPreview(t *testing.T, previews []models.ImportRowPreview) models.ImportRowPreview {
	t.Helper()
	for _, p := range previews {
		if p.SuggestedAction == "skip" {
			return p
		}
	}
	t.Fatalf("no skip preview row found among %d previews", len(previews))
	return models.ImportRowPreview{}
}

// TestParseVCF_NoteValueContainingEndMarker_Preserved drives
// str-note-ends-marker.vcf: a NOTE whose value contains the literal text
// "END:VCARD" must survive the splitter whole — the regression that turned
// the splitter from a regex into a line-based walker.
func TestParseVCF_NoteValueContainingEndMarker_Preserved(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "str-note-ends-marker.vcf")
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)

	require.Len(t, contacts[0].Contact.Card.Notes, 1)
	assert.Equal(t, "see END:VCARD inside this note value", contacts[0].Contact.Card.Notes[0].Note,
		"the splitter must not truncate the NOTE at the in-value END:VCARD marker")
}

// TestParseVCF_ControlCharsAndInvalidUTF8_AdversarialFixtures round-trips
// the encoding/injection fixtures through the full preview pipeline and
// asserts the flat fields come out sanitized — the same guarantee
// import_vcf_hostile_input_test.go pins for a hand-built card, now driven by
// the corpus.
func TestParseVCF_ControlCharsAndInvalidUTF8_AdversarialFixtures(t *testing.T) {
	db, user := newAdversarialImportTest(t, "adv-enc-inj")

	raw := strings.Join([]string{
		string(adversarial.LoadFixture("enc-invalid-utf8.vcf")),
		string(adversarial.LoadFixture("inj-control-chars.vcf")),
	}, "\r\n")
	contacts, _, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 2)

	for _, c := range contacts {
		assert.NotContains(t, c.Contact.Firstname, "\xff", "invalid UTF-8 must be sanitized at the flat layer")
		for _, r := range c.Contact.Firstname {
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
				t.Errorf("control character %#U survived sanitization in Firstname %q", r, c.Contact.Firstname)
			}
		}
	}
}

// TestParseVCF_UTF8BOM_Preserved drives str-bom.vcf through the full
// pipeline: the BOM must be stripped by ParseVCF before block splitting.
func TestParseVCF_UTF8BOM_Preserved(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "str-bom.vcf")
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Equal(t, "Ada", contacts[0].Contact.Firstname)
}

// TestParseJSContact_UTF8BOM_Preserved drives js-bom.json through
// ParseJSContact.
func TestParseJSContact_UTF8BOM_Preserved(t *testing.T) {
	db, user := newAdversarialImportTest(t, "adv-js-bom")
	contacts, previews, stats, err := ParseJSContact(strings.NewReader(string(adversarial.LoadFixture("js-bom.json"))), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Equal(t, "Ada", contacts[0].Contact.Firstname)
}

// TestParseVCF_VCard21_BareParams_Preserved drives
// ven-vcard21-bare-params.vcf (VERSION:2.1, bare ;CELL;PREF: tokens) through
// ParseVCF's 2.1 normalization + vcard3 routing.
func TestParseVCF_VCard21_BareParams_Preserved(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "ven-vcard21-bare-params.vcf")
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Equal(t, "Ada", contacts[0].Contact.Firstname)
	assert.Equal(t, "Lovelace", contacts[0].Contact.Lastname)
	require.Len(t, contacts[0].Contact.Phones, 2, "both bare-token TELs must land")
}

// TestParseVCF_VCard21_QuotedPrintable_Preserved drives
// ven-vcard21-quoted-printable.vcf (ENCODING=QUOTED-PRINTABLE) through the
// 2.1 normalizer's QP decode.
func TestParseVCF_VCard21_QuotedPrintable_Preserved(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "ven-vcard21-quoted-printable.vcf")
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)
	require.NotNil(t, contacts[0].Contact.Card.Name)
	assert.Equal(t, "Ada Lovelace", contacts[0].Contact.Card.Name.Full, "QP-encoded FN must be decoded")
	require.Len(t, contacts[0].Contact.Card.Notes, 1)
	assert.Equal(t, "first line\nsecond line", contacts[0].Contact.Card.Notes[0].Note)
}

// TestParseVCF_DuplicateUIDWithinFile_BothParse drives sem-duplicate-uid.vcf
// through ParseVCF: both cards parse as "add" candidates (the create-time
// UID collision is the separate controllers/import_duplicate_uid_test.go
// surface, already graceful).
func TestParseVCF_DuplicateUIDWithinFile_BothParse(t *testing.T) {
	contacts, previews, stats := parseAdversarialVCF(t, "sem-duplicate-uid.vcf")
	require.Len(t, contacts, 2)
	require.Len(t, previews, 2)
	assert.Equal(t, 2, stats.ValidCount)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", contacts[0].Contact.VCardUID)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", contacts[1].Contact.VCardUID)
	assert.Equal(t, "add", previews[0].SuggestedAction, "different names/emails must not trip within-batch duplicate detection")
	assert.Equal(t, "add", previews[1].SuggestedAction)
}

// TestParseVCF_CSVFormulaPrefixesSurviveToExportableFields drives
// inj-csv-formula-note.vcf: formula prefixes (=, +, -, @) in exportable
// fields must arrive at the flat Contact intact — the import side never
// mangles them, so the export boundary's csvSafe neutralization
// (controllers/export_csv_injection_test.go) is the sole defense and works.
func TestParseVCF_CSVFormulaPrefixesSurviveToExportableFields(t *testing.T) {
	contacts, _, _ := parseAdversarialVCF(t, "inj-csv-formula-note.vcf")
	require.Len(t, contacts, 1)
	c := contacts[0].Contact

	assert.Equal(t, "Ada", c.Firstname)
	assert.Equal(t, "+SUM(A1)", c.Lastname, "leading-+ surname must survive import untouched")
	assert.Equal(t, "-1", c.MiddleName, "leading-- additional-name must survive")
	assert.Equal(t, `@HYPERLINK("http://x")`, c.Suffix, "leading-@ suffix must survive")
	assert.Equal(t, "@EVALUATE(1)", c.Nickname, "leading-@ nickname must survive")
	assert.Equal(t, "-Acme Corp", c.Organization, "leading-- org must survive")
	require.NotNil(t, c.Card.Name)
	assert.Equal(t, "=1+1", c.Card.Name.Full, "leading-= FN must survive into the neutral Card")
	require.Len(t, c.Card.Notes, 1)
	assert.Equal(t, "=cmd|'/c calc'!A1", c.Card.Notes[0].Note, "leading-= NOTE must survive")
}

// TestParseJSContact_UnknownTopLevel_PreservedThroughImport proves the
// js-unknown-top-level.json passthrough survives the full ParseJSContact
// pipeline onto the persisted flat Contact (whose Passthrough column is
// what ConfirmVCF writes), not just the in-memory adapter record.
func TestParseJSContact_UnknownTopLevel_PreservedThroughImport(t *testing.T) {
	db, user := newAdversarialImportTest(t, "adv-js-unknown")
	contacts, previews, stats, err := ParseJSContact(strings.NewReader(string(adversarial.LoadFixture("js-unknown-top-level.json"))), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	assert.Equal(t, 1, stats.ValidCount)
	assert.Equal(t, "Ada", contacts[0].Contact.Firstname, "known data must land alongside passthrough")

	require.NotNil(t, contacts[0].Contact.Passthrough.JSContact)
	for _, key := range []string{"/xCustomThing", "/extensions"} {
		_, ok := contacts[0].Contact.Passthrough.JSContact[key]
		assert.Truef(t, ok, "unknown top-level key %s must survive ParseJSContact onto the flat Contact", key)
	}
}
