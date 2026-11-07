package services

import (
	"context"
	"testing"

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
