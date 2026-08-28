package services

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #651: every confirmed import writes exactly one immutable import_runs
// row, carrying the true source format and counts that match the returned
// ImportResult. Driven through the real wizard against the real migrated
// schema (dbtest.New, not AutoMigrate — CLAUDE.md backend trap #1).

func newImportHistoryUser(t *testing.T, db *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{Username: name, Email: name + "@example.com", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func importRunsFor(t *testing.T, db *gorm.DB, userID uint) []models.ImportRun {
	t.Helper()
	var runs []models.ImportRun
	require.NoError(t, db.Where("user_id = ?", userID).Order("created_at DESC").Find(&runs).Error)
	return runs
}

// confirmVCFLikeAdd builds a VCF-like session via create (a closure so the
// caller picks CreateVCFSession / CreateJSContactSession / CreateRecordsSession)
// and confirms a single "add" row, returning the result.
func confirmVCFLikeAdd(
	t *testing.T,
	m *ImportSessionManager,
	db *gorm.DB,
	userID uint,
	create func(uint, []VCFContactData, []models.ImportRowPreview) string,
	firstname string,
) *models.ImportResult {
	t.Helper()
	contact := &models.Contact{Firstname: firstname, Lastname: "History"}
	id := create(userID, []VCFContactData{{Contact: contact}},
		[]models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "add"}})

	result, appErr := m.ConfirmVCF(db, userID, models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	require.NotNil(t, result)
	return result
}

func TestConfirm_CSV_WritesOneImportRunMatchingResult(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "csv-history")
	m := NewImportSessionManager()

	headers := []string{"First Name", "Last Name"}
	rows := [][]string{{"Alice", "Smith"}, {"Bob", "Jones"}, {"Carol", "Lee"}}
	id := m.CreateCSVSession(user.ID, headers, rows)

	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Last Name", ContactField: "lastname"},
		},
	})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "add"},
			{RowIndex: 1, Action: "add"},
			{RowIndex: 2, Action: "skip"},
		},
	}, testImportLogger())
	require.Nil(t, appErr)
	require.Empty(t, result.Errors)

	runs := importRunsFor(t, db, user.ID)
	require.Len(t, runs, 1, "a confirmed CSV import writes exactly one import_runs row")
	got := runs[0]
	assert.Equal(t, models.ImportFormatCSV, got.Format)
	assert.Equal(t, result.TotalProcessed, got.TotalProcessed)
	assert.Equal(t, result.Created, got.Created)
	assert.Equal(t, result.Updated, got.Updated)
	assert.Equal(t, result.Skipped, got.Skipped)
	assert.Equal(t, len(result.Errors), got.ErrorCount)
	assert.Equal(t, user.ID, got.UserID)
	assert.False(t, got.CreatedAt.IsZero(), "created_at must be stamped")
}

func TestConfirmVCF_RecordsFormatPerSource(t *testing.T) {
	cases := []struct {
		name   string
		format string
		create func(m *ImportSessionManager) func(uint, []VCFContactData, []models.ImportRowPreview) string
	}{
		{"vcf", models.ImportFormatVCF, func(m *ImportSessionManager) func(uint, []VCFContactData, []models.ImportRowPreview) string {
			return m.CreateVCFSession
		}},
		{"jscontact", models.ImportFormatJSContact, func(m *ImportSessionManager) func(uint, []VCFContactData, []models.ImportRowPreview) string {
			return m.CreateJSContactSession
		}},
		{"records", models.ImportFormatRecords, func(m *ImportSessionManager) func(uint, []VCFContactData, []models.ImportRowPreview) string {
			return m.CreateRecordsSession
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := dbtest.New(t)
			user := newImportHistoryUser(t, db, tc.name+"-history")
			m := NewImportSessionManager()

			result := confirmVCFLikeAdd(t, m, db, user.ID, tc.create(m), "Dana")
			require.Empty(t, result.Errors)

			runs := importRunsFor(t, db, user.ID)
			require.Len(t, runs, 1)
			assert.Equal(t, tc.format, runs[0].Format)
			assert.Equal(t, 1, runs[0].Created)
			assert.Equal(t, result.TotalProcessed, runs[0].TotalProcessed)
		})
	}
}

func TestConfirmVCF_ErrorRows_RecordedAsErrorCount(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "errcount-history")
	m := NewImportSessionManager()

	// Row 0 references a non-existent existing contact on "update" → one
	// error, batch continues; row 1 adds cleanly.
	id := m.CreateVCFSession(user.ID,
		[]VCFContactData{
			{Contact: &models.Contact{Firstname: "Stale", Lastname: "Ref", Email: "stale-h@example.com"}},
			{Contact: &models.Contact{Firstname: "Good", Lastname: "Row", Email: "good-h@example.com"}},
		},
		[]models.ImportRowPreview{
			{RowIndex: 0, DuplicateMatch: &models.DuplicateMatch{ExistingContactID: 999999, MatchReason: "email"}, SuggestedAction: "update"},
			{RowIndex: 1, SuggestedAction: "add"},
		})

	result, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "update"},
			{RowIndex: 1, Action: "add"},
		},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	require.Len(t, result.Errors, 1)

	runs := importRunsFor(t, db, user.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, 1, runs[0].ErrorCount, "error_count must equal len(result.Errors)")
	assert.Equal(t, result.Skipped, runs[0].Skipped)
	assert.Equal(t, result.Created, runs[0].Created)
}

func TestConfirm_IdempotentReplay_DoesNotDoubleCountHistory(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "replay-history")
	m := NewImportSessionManager()

	id := m.CreateCSVSession(user.ID, []string{"First Name"}, [][]string{{"Solo"}})
	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings:  []models.ColumnMapping{{CSVColumn: "First Name", ContactField: "firstname"}},
	})
	require.Nil(t, appErr)

	req := models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}
	_, appErr = m.Confirm(db, user.ID, req, testImportLogger())
	require.Nil(t, appErr)

	// Second confirm of the same session replays the stored result (T57) and
	// must NOT write a second history row.
	replayed, appErr := m.Confirm(db, user.ID, req, testImportLogger())
	require.Nil(t, appErr)
	require.NotNil(t, replayed)

	assert.Len(t, importRunsFor(t, db, user.ID), 1,
		"the idempotent replay path must not append a second import_runs row")
}

func TestConfirm_HistoryWriteFailure_DoesNotFailTheImport(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "brokenhistory")
	m := NewImportSessionManager()

	// Remove the history table out from under the writer: RecordImportRun must
	// log and swallow the insert error, and the import itself must still
	// succeed with its result intact.
	require.NoError(t, db.Exec("DROP TABLE import_runs").Error)

	id := m.CreateCSVSession(user.ID, []string{"First Name"}, [][]string{{"Persist"}})
	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings:  []models.ColumnMapping{{CSVColumn: "First Name", ContactField: "firstname"}},
	})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}, testImportLogger())
	require.Nil(t, appErr, "a history-write failure must not fail the import")
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Created)

	var contacts int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	assert.Equal(t, int64(1), contacts, "the contact still persisted")
}
