package controllers

// DATA-04 (issue #444) — the selective-export × DATA-02 (issue #442)
// interaction: what belongs in the export loss report and what does not.
//
// The boundary has three omission classes:
//
//  1. user-selected omission (a section the user did not pick) — NOT fidelity
//     loss, must not be reported;
//  2. sensitivity-default omission (a private/secret item excluded without the
//     IncludeSensitive opt-in) — NOT fidelity loss, must not be reported;
//  3. format-driven omission (the target format cannot represent the field)
//     — IS fidelity loss, must be reported.
//
// Conflating 1/2 with 3 makes the loss report untrustworthy: it would teach
// users to ignore the report because it is noisy with things they deliberately
// asked to leave out. The preflight endpoint and the export response header
// both consume the same shared computation, so these tests run preflight.

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// boundaryContact builds a contact carrying an email and phone (representable
// in every format) plus a vCard 4.0-only language (a vCard 3.0 format-driven
// loss), through the same ApplyRecordToContact path the REST API uses.
func boundaryContact(t *testing.T, db *gorm.DB, user models.User) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	models.ApplyRecordToContact(&contact, &contactmodel.Record{Card: contactmodel.Card{
		UID:      "urn:uuid:boundary-ada",
		Language: "en",
		Emails:   []contactmodel.Email{{Address: "ada@example.com"}},
		Phones:   []contactmodel.Phone{{Number: "555-0100"}},
	}}, "")
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

// TestSelectiveExport_LossReportBoundary pins the three-class boundary in one
// export: with sections=emails on vCard 3.0, the deselected phones are not
// reported (user-selected omission), the selected emails are not reported
// (they are present), and the vCard 4.0-only language IS reported (format
// driven — it is in the record and the user could not have deselected it).
func TestSelectiveExport_LossReportBoundary(t *testing.T) {
	db, router, ds := matrixFixture(t)
	contact := boundaryContact(t, db, ds.User)
	scope := "&vcard_uid=" + contact.VCardUID

	resp := preflightReports(t, router, "?format=vcard3&sections=emails"+scope)

	concepts := map[string]bool{}
	for _, d := range resp.Diagnostics {
		concepts[d.Concept] = true
	}

	// Class 3: language has no vCard 3.0 home and the user selected all the
	// sections that could carry it away — it must be reported.
	assert.True(t, concepts["language"],
		"a format-driven omission must be reported even under a sections= filter")

	// Class 1: phones were deselected by the user's sections=emails — a
	// deliberate omission, not fidelity loss, so no report.
	assert.False(t, concepts["phone"],
		"a user-selected omission must not be reported as fidelity loss")

	// The selected email is present in the would-be file; no loss to report.
	assert.False(t, concepts["email"],
		"a selected, representable field is not a loss")

	// The full export (all sections) reports the same language loss — the
	// format-driven report is not an artifact of the sections= filter.
	full := preflightReports(t, router, "?format=vcard3"+scope)
	assert.True(t, func() bool {
		for _, d := range full.Diagnostics {
			if d.Concept == "language" {
				return true
			}
		}
		return false
	}(), "the format-driven language loss must appear with all sections too")
}

// TestSelectiveExport_LossReportBoundary_SectionSelectedButFormatCant pins the
// inverse of the user-selected-omission case: a section the user DID select,
// whose data the format cannot represent, is reported. ada (from the fixture)
// carries speakToAs pronouns — selected via sections=speak_to_as — which
// vCard 3.0 has no home for.
func TestSelectiveExport_LossReportBoundary_SectionSelectedButFormatCant(t *testing.T) {
	_, router, ds := matrixFixture(t)
	ada := ds.Contacts["ada"]

	resp := preflightReports(t, router, "?format=vcard3&sections=speak_to_as&vcard_uid="+ada.VCardUID)

	concepts := map[string]bool{}
	for _, d := range resp.Diagnostics {
		concepts[d.Concept] = true
	}
	assert.True(t, concepts["pronouns"],
		"PRONOUNS selected but with no vCard 3.0 home must be reported as fidelity loss")

	// The deselected sections are absent from the report entirely (class 1).
	assert.False(t, concepts["email"], "deselected emails are not a loss")
	assert.False(t, concepts["phone"], "deselected phones are not a loss")

	// The same selection against vCard 4.0 (which represents PRONOUNS) is not
	// a loss — the report is format-specific.
	v4 := preflightReports(t, router, "?format=vcard4&sections=speak_to_as&vcard_uid="+ada.VCardUID)
	for _, d := range v4.Diagnostics {
		assert.NotEqual(t, "pronouns", d.Concept, "PRONOUNS has a vCard 4.0 home; not a loss there")
	}
}

// TestSelectiveExport_LossReportBoundary_OptInInclusionNotReported pins that
// the opt-in's inclusion is not a loss either way: the sensitive item appears
// in the file with include_sensitive=true, and the export header reports
// nothing for it — the same way the default-deny exclusion reports nothing
// (TestExportLossHeader_SensitivityPolicyExclusionNotReported).
func TestSelectiveExport_LossReportBoundary_OptInInclusionNotReported(t *testing.T) {
	db, router, ds := matrixFixture(t)

	alice := models.Contact{UserID: ds.User.ID, Firstname: "Alice", Lastname: "Anderson"}
	bob := models.Contact{UserID: ds.User.ID, Firstname: "Bob", Lastname: "Brown"}
	require.NoError(t, db.Create(&alice).Error)
	require.NoError(t, db.Create(&bob).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID:      ds.User.ID,
		SourceID:    alice.VCardUID,
		TargetID:    bob.VCardUID,
		Type:        "spouse_of",
		Source:      models.RelationshipSourceUserConfirmed,
		Confidence:  1.0,
		Status:      models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)

	// Scope to the source contact so the loss header is a small, predictable
	// set (the full fixture's own losses exceed the header budget).
	req, _ := http.NewRequest("GET", "/export/vcf?version=4&sections=related_to&include_sensitive=true&vcard_uid="+alice.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "RELATED",
		"the opt-in must project the secret edge into the file")

	hdr := parseLossHeader(t, w)
	require.False(t, hdr.Truncated)
	assert.Empty(t, hdr.Diagnostics,
		"including a sensitive item via the opt-in is not fidelity loss and must not be reported")
}
