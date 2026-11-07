package services

import (
	"context"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issues #2186 / #2187: rows with errors can be fixed in the review step, an
// empty name falls back to the email/phone, and the final report lists every
// skipped or failed row with a reason.

func strp(s string) *string { return &s }

var errorRowHeaders = []string{"First", "Last", "Email", "Phone", "Org"}

var errorRowMappings = []models.ColumnMapping{
	{CSVColumn: "First", ContactField: "firstname"},
	{CSVColumn: "Last", ContactField: "lastname"},
	{CSVColumn: "Email", ContactField: "email"},
	{CSVColumn: "Phone", ContactField: "phone"},
	{CSVColumn: "Org", ContactField: "organization"},
}

type errorRowFixture struct {
	m    *ImportSessionManager
	db   *gorm.DB
	id   string
	uid  uint
	prev *models.ImportPreviewResponse
}

func newErrorRowFixture(t *testing.T, rows [][]string) *errorRowFixture {
	t.Helper()
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "err-rows")
	m := NewImportSessionManager()
	id := m.CreateCSVSession(user.ID, errorRowHeaders, rows)
	prev, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{SessionID: id, Mappings: errorRowMappings})
	require.Nil(t, appErr)
	return &errorRowFixture{m: m, db: db, id: id, uid: user.ID, prev: prev}
}

func (f *errorRowFixture) confirm(t *testing.T, actions []models.RowImportAction, edits []models.ImportRowEdit) *models.ImportResult {
	t.Helper()
	res, appErr := f.m.Confirm(f.db, f.uid, models.ImportConfirmRequest{
		SessionID: f.id, Actions: actions, RowEdits: edits,
	}, testImportLogger())
	require.Nil(t, appErr)
	return res
}

func (f *errorRowFixture) contacts(t *testing.T) []models.Contact {
	t.Helper()
	var cs []models.Contact
	require.NoError(t, f.db.Where("user_id = ?", f.uid).Order("id").Find(&cs).Error)
	return cs
}

func TestPreview_EmptyNameFallsBackToEmailThenPhone(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{
		{"", "", "ann@example.com", "+14155550101", ""},
		{"", "", "", "+14155550102", ""},
		{"", "", "", "", "Acme"}, // nothing to fall back on: stays an error
	})
	rows := f.prev.Rows
	require.Len(t, rows, 3)
	assert.Empty(t, rows[0].ValidationErrors)
	assert.Equal(t, "ann@example.com", rows[0].ParsedContact["firstname"])
	assert.Empty(t, rows[1].ValidationErrors)
	assert.Equal(t, "+14155550102", rows[1].ParsedContact["firstname"])
	assert.Equal(t, []string{"First name is required"}, rows[2].ValidationErrors)
	assert.Equal(t, 1, f.prev.ErrorCount)
}

func TestConfirm_EmptyNameFallbackIsPersisted(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"", "", "ann@example.com", "", ""}})
	res := f.confirm(t, []models.RowImportAction{{RowIndex: 0, Action: "add"}}, nil)
	assert.Equal(t, 1, res.Created)
	cs := f.contacts(t)
	require.Len(t, cs, 1)
	assert.Equal(t, "ann@example.com", cs[0].Firstname)
}

func TestConfirm_ErrorRowWithoutEditIsNeverImported(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"", "", "", "", "Acme"}})
	res := f.confirm(t, []models.RowImportAction{{RowIndex: 0, Action: "add"}}, nil)
	assert.Zero(t, res.Created)
	assert.Equal(t, 1, res.Skipped)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "skipped", res.Issues[0].Status)
	assert.Equal(t, 1, res.Issues[0].Row)
	assert.Contains(t, res.Issues[0].Reason, "First name is required")
	assert.Empty(t, f.contacts(t))
}

func TestConfirm_RowEditFixesErrorRow(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{
		{"", "", "", "", "Acme"},
		{"Bob", "Lee", "not-an-email", "", ""},
	})
	res := f.confirm(t,
		[]models.RowImportAction{{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"}},
		[]models.ImportRowEdit{
			{RowIndex: 0, Firstname: strp("Acme Corp")},
			{RowIndex: 1, Email: strp("bob@example.com")},
		})
	assert.Equal(t, 2, res.Created)
	assert.Empty(t, res.Issues)
	cs := f.contacts(t)
	require.Len(t, cs, 2)
	assert.Equal(t, "Acme Corp", cs[0].Firstname)
	assert.Equal(t, "bob@example.com", cs[1].Email)
	require.Len(t, cs[1].Emails, 1)
	assert.Equal(t, "bob@example.com", cs[1].Emails[0].Value)
}

func TestConfirm_RowEditThatStillFailsIsReported(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"Bob", "Lee", "not-an-email", "", ""}})
	res := f.confirm(t,
		[]models.RowImportAction{{RowIndex: 0, Action: "add"}},
		[]models.ImportRowEdit{{RowIndex: 0, Email: strp("still bad")}})
	assert.Zero(t, res.Created)
	require.Len(t, res.Issues, 1)
	assert.Contains(t, res.Issues[0].Reason, "Invalid email format")
	assert.Empty(t, f.contacts(t))
}

func TestConfirm_RowEditCanClearBadEmail(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"Bob", "Lee", "not-an-email", "", ""}})
	res := f.confirm(t,
		[]models.RowImportAction{{RowIndex: 0, Action: "add"}},
		[]models.ImportRowEdit{{RowIndex: 0, Email: strp("")}})
	assert.Equal(t, 1, res.Created)
	cs := f.contacts(t)
	require.Len(t, cs, 1)
	assert.Empty(t, cs[0].Emails)
}

func TestConfirm_IssuesListSkippedRowsWithReasonAndName(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{
		{"Ann", "Ok", "ann@example.com", "", ""},
		{"Cy", "Skip", "cy@example.com", "+14155550103", ""},
		{"Ann", "Ok", "ann@example.com", "", ""}, // duplicate of row 1 in the file
	})
	res := f.confirm(t, []models.RowImportAction{
		{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "skip"}, {RowIndex: 2, Action: "skip"},
	}, nil)
	assert.Equal(t, 1, res.Created)
	require.NotNil(t, res.Issues)
	require.Len(t, res.Issues, 2)
	assert.Equal(t, models.ImportRowIssue{Row: 2, Name: "Cy Skip", Email: "cy@example.com", Phone: "+14155550103", Status: "skipped", Reason: "Skipped"}, res.Issues[0])
	assert.Equal(t, 3, res.Issues[1].Row)
	assert.Contains(t, res.Issues[1].Reason, "row 1")
}

func TestConfirm_NoIssuesIsEmptyNotNil(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"Ann", "Ok", "", "", ""}})
	res := f.confirm(t, []models.RowImportAction{{RowIndex: 0, Action: "add"}}, nil)
	require.NotNil(t, res.Issues)
	assert.Empty(t, res.Issues)
}

func TestConfirmVCF_RowEditNamesNamelessCard(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "vcf-err-rows")
	m := NewImportSessionManager()
	contact := &models.Contact{Organization: "Acme"} // no name, email or phone
	prevRow := models.ImportRowPreview{
		RowIndex: 0, ValidationErrors: []string{"First name is required"}, SuggestedAction: "skip",
		ParsedContact: map[string]interface{}{"organization": "Acme"},
	}
	id := m.CreateVCFSession(user.ID, []VCFContactData{{Contact: contact}}, []models.ImportRowPreview{prevRow})

	// No edit: skipped and reported.
	res, appErr := m.ConfirmVCF(context.Background(), db, user.ID, models.ImportConfirmRequest{
		SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	assert.Zero(t, res.Created)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "skipped", res.Issues[0].Status)

	id = m.CreateVCFSession(user.ID, []VCFContactData{{Contact: contact}}, []models.ImportRowPreview{prevRow})
	res, appErr = m.ConfirmVCF(context.Background(), db, user.ID, models.ImportConfirmRequest{
		SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		RowEdits: []models.ImportRowEdit{{RowIndex: 0, Firstname: strp("Acme Corp")}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, res.Created)
	assert.Empty(t, res.Issues)
	var got models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&got).Error)
	assert.Equal(t, "Acme Corp", got.Firstname)
	assert.Equal(t, "Acme Corp", got.FN)
	assert.Empty(t, contact.Firstname, "the session's stored contact is not mutated by an edit")
}

func TestApplyImportRowEdit_PhoneAndCardEntries(t *testing.T) {
	base := models.Contact{
		Phones: []models.ContactPhone{{Value: "+14155550101"}, {Value: "abc"}},
		Emails: []models.ContactEmail{{Value: "ok@example.com"}},
	}
	base.Card.Phones = []contactmodel.Phone{{Number: "+14155550101"}, {Number: "abc"}}
	base.Card.Emails = []contactmodel.Email{{Address: "ok@example.com"}}

	// Replaces the first INVALID phone, leaves the valid one, mirrors the Card.
	fixed := applyImportRowEdit(base, models.ImportRowEdit{Phone: strp("+14155550102")})
	assert.Equal(t, "+14155550101", fixed.Phones[0].Value)
	assert.Equal(t, "+14155550102", fixed.Phones[1].Value)
	assert.Equal(t, "+14155550102", fixed.Card.Phones[1].Number)
	assert.Equal(t, "abc", base.Phones[1].Value, "the original is not mutated")

	// An empty phone removes the invalid entry from flat and Card.
	cleared := applyImportRowEdit(base, models.ImportRowEdit{Phone: strp("")})
	require.Len(t, cleared.Phones, 1)
	require.Len(t, cleared.Card.Phones, 1)
	assert.Equal(t, "+14155550101", cleared.Phone)

	// An email edit with no invalid entry replaces the first one and the Card copy.
	swapped := applyImportRowEdit(base, models.ImportRowEdit{Email: strp("new@example.com")})
	assert.Equal(t, "new@example.com", swapped.Email)
	assert.Equal(t, "new@example.com", swapped.Card.Emails[0].Address)

	// Removing the only email empties the flat primary.
	none := applyImportRowEdit(base, models.ImportRowEdit{Email: strp("")})
	assert.Empty(t, none.Emails)
	assert.Empty(t, none.Email)
}

func TestApplyImportRowEdit_AddsMissingEmailPhoneAndNames(t *testing.T) {
	base := models.Contact{Firstname: "Ann", Lastname: "Old"}
	base.Card.Name = &contactmodel.Name{Components: []contactmodel.NameComponent{
		{Kind: "given", Value: "Ann"}, {Kind: "surname", Value: "Old"},
	}}
	fixed := applyImportRowEdit(base, models.ImportRowEdit{
		Lastname: strp("New"), Email: strp("a@example.com"), Phone: strp("+14155550104"),
	})
	assert.Equal(t, "New", fixed.Lastname)
	assert.Equal(t, "Ann New", fixed.Card.Name.Full)
	assert.Equal(t, "Old", base.Card.Name.Components[1].Value, "the original Card name is not mutated")
	assert.Equal(t, "a@example.com", fixed.Card.Emails[0].Address)
	assert.Equal(t, "+14155550104", fixed.Card.Phones[0].Number)

	// Clearing the surname drops the component.
	cleared := applyImportRowEdit(base, models.ImportRowEdit{Lastname: strp("")})
	assert.Len(t, cleared.Card.Name.Components, 1)
}

func TestReportRowFailure_AddsErrorAndIssue(t *testing.T) {
	res := models.ImportResult{Errors: []string{}, Issues: []models.ImportRowIssue{}}
	p := models.ImportRowPreview{RowIndex: 4, ParsedContact: map[string]interface{}{"email": "z@example.com"}}
	reportRowFailure(testImportLogger(), &res, p, "create contact", assert.AnError)
	assert.Equal(t, []string{"Row 5: Failed to create contact"}, res.Errors)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, models.ImportRowIssue{Row: 5, Name: "z@example.com", Email: "z@example.com", Status: "failed", Reason: "Failed to create contact"}, res.Issues[0])
}

func TestConfirm_UpdateWithoutMatchIsReportedAsFailed(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"Ann", "Ok", "ann@example.com", "", ""}})
	res := f.confirm(t, []models.RowImportAction{{RowIndex: 0, Action: "update"}}, nil)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "failed", res.Issues[0].Status)
	assert.Contains(t, res.Issues[0].Reason, "no existing contact")
}

func TestConfirm_SkippedErrorRowReportsItsValidationErrors(t *testing.T) {
	f := newErrorRowFixture(t, [][]string{{"", "", "", "", "Acme"}})
	res := f.confirm(t, []models.RowImportAction{{RowIndex: 0, Action: "skip"}}, nil)
	require.Len(t, res.Issues, 1)
	assert.Equal(t, "First name is required", res.Issues[0].Reason)
	assert.Equal(t, "", res.Issues[0].Name)
}

func TestConfirm_VCFSessionOnCSVEndpointAppliesRowEdits(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "vcf-on-confirm")
	m := NewImportSessionManager()
	contact := &models.Contact{Organization: "Acme"}
	prev := models.ImportRowPreview{RowIndex: 0, ValidationErrors: []string{"First name is required"}, SuggestedAction: "skip"}
	id := m.CreateVCFSession(user.ID, []VCFContactData{{Contact: contact}}, []models.ImportRowPreview{prev})
	res, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		RowEdits: []models.ImportRowEdit{{RowIndex: 0, Firstname: strp("Acme Corp")}},
	}, testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, res.Created)
}
