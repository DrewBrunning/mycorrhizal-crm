package services

import (
	"context"
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2741: a row that imported with a dropped invalid birthday/anniversary
// is recorded as a "warning" issue (with its concept) in the confirm result and
// in the persisted import-run issues. skipped/failed semantics and every
// counter stay as they were: a warning row is a normal Created/Updated row.

func TestConfirmCSV_InvalidBirthdayIsRecordedAsWarningIssue(t *testing.T) {
	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	id := m.CreateCSVSession(1, []string{"First Name", "Last Name", "Email", "Birthday", "Anniversary"}, [][]string{
		{"Ada", "Lovelace", "ada.w@example.com", "1991-02-30", "1990-02-31"},
		{"Bob", "Fine", "bob.w@example.com", "1991-02-28", ""},
		{"", "", "nameless.w@example.com", "notadate", ""},
	})
	maps := append(csvMappings(),
		models.ColumnMapping{CSVColumn: "Birthday", ContactField: "birthday"},
		models.ColumnMapping{CSVColumn: "Anniversary", ContactField: "anniversary"})
	_, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: maps})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{
		{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"}, {RowIndex: 2, Action: "add"},
	}}, testImportLogger())
	require.Nil(t, appErr)

	assert.Equal(t, 3, result.Created)
	assert.Equal(t, 3, result.TotalProcessed)
	assert.Zero(t, result.Skipped, "a warning row is not skipped")
	assert.Empty(t, result.Errors, "a warning row is not an error")

	require.Len(t, result.Issues, 3)
	for _, iss := range result.Issues {
		assert.Equal(t, "warning", iss.Status)
	}
	assert.Equal(t, 1, result.Issues[0].Row)
	assert.Equal(t, "birthday", result.Issues[0].Concept)
	assert.Equal(t, "Ada Lovelace", result.Issues[0].Name)
	assert.Equal(t, 1, result.Issues[1].Row)
	assert.Equal(t, "anniversary", result.Issues[1].Concept)
	assert.Equal(t, 3, result.Issues[2].Row)
	assert.Equal(t, "birthday", result.Issues[2].Concept)

	// The persisted run carries them, with the nameless row's email never
	// stored as a name (#2563/#2575).
	var run models.ImportRun
	require.NoError(t, db.Where("user_id = ?", 1).Order("id DESC").First(&run).Error)
	assert.Equal(t, 3, run.Created)
	assert.Zero(t, run.Skipped)
	assert.Zero(t, run.ErrorCount)
	assert.Equal(t, 3, run.IssueCount)
	d := run.Detail()
	require.Len(t, d.Issues, 3)
	assert.Equal(t, "warning", d.Issues[0].Status)
	assert.Equal(t, "birthday", d.Issues[0].Concept)
	assert.Equal(t, "Ada Lovelace", d.Issues[0].Name)
	assert.Equal(t, "", d.Issues[2].Name)
	assert.Empty(t, d.Issues[2].Email)
	assert.NotContains(t, run.IssuesJSON, "nameless.w@example.com")
}

func TestConfirmVCF_InvalidBdayIsRecordedAsWarningIssue(t *testing.T) {
	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	card := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nEMAIL:ada.vw@example.com\r\nBDAY:notadate\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Bob Fine\r\nN:Fine;Bob;;;\r\nEMAIL:bob.vw@example.com\r\nBDAY:1990-05-01\r\nEND:VCARD\r\n"
	contacts, previews, _, err := ParseVCF(strings.NewReader(card), db, 1)
	require.NoError(t, err)
	id := m.CreateVCFSession(1, contacts, previews)
	result, appErr := m.ConfirmVCF(context.Background(), db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{
		{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"},
	}}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)

	assert.Equal(t, 2, result.Created)
	assert.Zero(t, result.Skipped)
	assert.Empty(t, result.Errors)
	require.Len(t, result.Issues, 1)
	assert.Equal(t, models.ImportRowIssue{Row: 1, Name: "Ada Lovelace", Email: "ada.vw@example.com", Status: "warning", Concept: "birthday", Reason: "birthday not imported: invalid value"}, result.Issues[0])

	var run models.ImportRun
	require.NoError(t, db.Where("user_id = ?", 1).Order("id DESC").First(&run).Error)
	d := run.Detail()
	require.Len(t, d.Issues, 1)
	assert.Equal(t, "warning", d.Issues[0].Status)
	assert.Equal(t, "birthday", d.Issues[0].Concept)
	assert.Empty(t, d.Issues[0].Email)
}

func TestConfirmCSV_SkippedWarningRowIsOnlySkipped(t *testing.T) {
	db := setupImportSessionTestDB(t)
	m := NewImportSessionManager()
	id := m.CreateCSVSession(1, []string{"First Name", "Last Name", "Email", "Birthday"}, [][]string{{"Ada", "Lovelace", "ada.sk@example.com", "1991-02-30"}})
	_, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: append(csvMappings(), models.ColumnMapping{CSVColumn: "Birthday", ContactField: "birthday"})})
	require.Nil(t, appErr)
	result, appErr := m.Confirm(db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "skip"}}}, testImportLogger())
	require.Nil(t, appErr)
	require.Len(t, result.Issues, 1)
	assert.Equal(t, "skipped", result.Issues[0].Status)
	assert.Empty(t, result.Issues[0].Concept)
}

func TestConfirmCSV_UpdatedRowWithInvalidBirthdayIsRecordedAsWarning(t *testing.T) {
	db := setupImportSessionTestDB(t)
	existing := models.Contact{UserID: 1, Firstname: "Ada", Lastname: "Lovelace", Email: "ada.up@example.com"}
	require.NoError(t, db.Create(&existing).Error)
	m := NewImportSessionManager()
	id := m.CreateCSVSession(1, []string{"First Name", "Last Name", "Email", "Birthday"}, [][]string{{"Ada", "Lovelace", "ada.up@example.com", "1991-02-30"}})
	prev, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: append(csvMappings(), models.ColumnMapping{CSVColumn: "Birthday", ContactField: "birthday"})})
	require.Nil(t, appErr)
	require.NotNil(t, prev.Rows[0].DuplicateMatch)

	result, appErr := m.Confirm(db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "update"}}}, testImportLogger())
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Updated)
	assert.Zero(t, result.Skipped)
	require.Len(t, result.Issues, 1)
	assert.Equal(t, "warning", result.Issues[0].Status)
	assert.Equal(t, "birthday", result.Issues[0].Concept)
}
