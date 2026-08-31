package models

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/correspondence"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestLossReportsFor_CarriesContactAndMatrixContext pins the DATA-02 report
// shape (issue #442): a report names which contact lost which field to which
// format, and the why comes from the DATA-01 matrix, not the adapter message.
func TestLossReportsFor_CarriesContactAndMatrixContext(t *testing.T) {
	contact := &Contact{Model: gorm.Model{ID: 7}, Firstname: "Ada", Lastname: "Lovelace", VCardUID: "urn:uuid:ada"}
	diags := []contactmodel.Diagnostic{
		// Gender: the issue #515 canary. The envelope-loss diagnostic has the
		// concept; the report must carry the matrix bucket+reason.
		{Severity: "warn", Concept: "crm.gender", Message: "CRM-only field has no home in this export format and is dropped from the file"},
		// A non-reportable adapter warn (empty-FN, an exact cell) must not
		// surface as a fidelity loss.
		{Severity: "warn", Concept: "name.full", Message: "no Name.Full and no components to derive FN from"},
	}

	reports := LossReportsFor("vcard4", contact, diags)
	require.Len(t, reports, 1)

	got := reports[0]
	assert.Equal(t, "vcard4", got.Format)
	assert.Equal(t, uint(7), got.ContactID)
	assert.Equal(t, "Ada Lovelace", got.ContactName)
	assert.Equal(t, "urn:uuid:ada", got.VCardUID)
	assert.Equal(t, "warn", got.Severity)
	assert.Equal(t, "crm.gender", got.Concept)
	assert.Equal(t, string(correspondence.BucketUnsupported), got.Bucket)
	assert.NotEmpty(t, got.Reason, "the report must carry the DATA-01 matrix reason, not just the adapter message")
	assert.Equal(t, "CRM-only field has no home in this export format and is dropped from the file", got.Message)
}

// TestLossReportsFor_FormatSpecificClassification pins that the same diagnostic
// concept can be reportable in one format and not another, and that the bucket
// is the format's own cell. E.g. gramgender is lossy in JSContact (scalar
// collapse) but unsupported in vCard 3.0; anniversary.birth is lossy in vCard
// 3.0 (date-only) but exact in vCard 4.0.
func TestLossReportsFor_FormatSpecificClassification(t *testing.T) {
	contact := &Contact{Firstname: "Grace", Lastname: "Hopper", VCardUID: "urn:uuid:grace"}

	gg := contactmodel.Diagnostic{Severity: "warn", Concept: "gramgender", Message: "collapsed"}
	js := LossReportsFor("jscontact", contact, []contactmodel.Diagnostic{gg})
	require.Len(t, js, 1, "gramgender is reportable in JSContact")
	assert.Equal(t, string(correspondence.BucketLossy), js[0].Bucket)

	v3 := LossReportsFor("vcard3", contact, []contactmodel.Diagnostic{gg})
	require.Len(t, v3, 1, "gramgender is reportable in vCard 3.0")
	assert.Equal(t, string(correspondence.BucketUnsupported), v3[0].Bucket)

	// anniversary.birth is lossy only in vCard 3.0 (time-of-day dropped).
	ab := contactmodel.Diagnostic{Severity: "warn", Concept: "anniversary.birth", Message: "date-only"}
	assert.Len(t, LossReportsFor("vcard3", contact, []contactmodel.Diagnostic{ab}), 1)
	assert.Empty(t, LossReportsFor("vcard4", contact, []contactmodel.Diagnostic{ab}),
		"anniversary.birth has an exact vCard 4.0 home and is not a loss there")
}

// TestLossReportsFor_NonWarnAndUnknownConceptsExcluded pins that only warn
// diagnostics corresponding to a matrix unsupported/lossy cell become reports:
// info diagnostics and foreign concepts never surface.
func TestLossReportsFor_NonWarnAndUnknownConceptsExcluded(t *testing.T) {
	contact := &Contact{Firstname: "Katherine", Lastname: "Johnson", VCardUID: "urn:uuid:katherine"}
	diags := []contactmodel.Diagnostic{
		{Severity: "info", Concept: "crm.gender", Message: "info, not a loss"},
		{Severity: "warn", Concept: "not.a.concept", Message: "foreign"},
	}
	assert.Empty(t, LossReportsFor("vcard4", contact, diags))
}

// TestLossReportsFor_EveryMatrixLossIsReachable pins the matrix -> report half
// of the DATA-02 correspondence at the model level: for every registry entry,
// a warn diagnostic carrying that concept resolves to a report carrying the
// registry's own bucket+reason. Combined with the adapter round-trip suite
// (which proves the adapters emit those warns when the field is present), this
// is the "every field the matrix marks unsupported or lossy must be capable of
// producing a report" guarantee.
func TestLossReportsFor_EveryMatrixLossIsReachable(t *testing.T) {
	contact := &Contact{Firstname: "A", Lastname: "Contact", VCardUID: "urn:uuid:a"}
	for _, lr := range correspondence.LossReports() {
		reports := LossReportsFor(string(lr.Format), contact, []contactmodel.Diagnostic{
			{Severity: "warn", Concept: lr.Concept, Message: "adapter reported"},
		})
		require.Len(t, reports, 1, "matrix loss %s/%s must produce a report", lr.Concept, lr.Format)
		assert.Equal(t, string(lr.Bucket), reports[0].Bucket)
		assert.Equal(t, lr.Reason, reports[0].Reason)
	}
}

// TestExportLossPreflightResponse_EmptySerializesAsArray pins the frontend
// trap #8 shape: an empty diagnostics list must marshal as `[]`, never `null`,
// so a client can always read `.length`.
func TestExportLossPreflightResponse_EmptySerializesAsArray(t *testing.T) {
	raw, err := json.Marshal(ExportLossPreflightResponse{Format: "vcard4", Diagnostics: []LossReport{}})
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"diagnostics":[]`)
	assert.NotContains(t, string(raw), `"diagnostics":null`)
}
