package services

import (
	"context"
	"encoding/json"
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

	result, appErr := m.ConfirmVCF(context.Background(), db, userID, models.ImportConfirmRequest{
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

	result, appErr := m.ConfirmVCF(context.Background(), db, user.ID, models.ImportConfirmRequest{
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

// Issue #2463: the per-row issues of a confirmed import are persisted with the
// run (reason + row reference only; email and phone are never stored).

func TestConfirm_CSV_PersistsRowIssuesWithoutEmailOrPhone(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "csv-issues")
	m := NewImportSessionManager()

	headers := []string{"First Name", "Last Name", "Email"}
	rows := [][]string{{"Alice", "Smith", "alice@example.com"}, {"Bob", "Jones", "bob@example.com"}}
	id := m.CreateCSVSession(user.ID, headers, rows)
	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Last Name", ContactField: "lastname"},
			{CSVColumn: "Email", ContactField: "email"},
		},
	})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions: []models.RowImportAction{
			{RowIndex: 0, Action: "add"},
			{RowIndex: 1, Action: "skip"},
		},
	}, testImportLogger())
	require.Nil(t, appErr)
	require.Len(t, result.Issues, 1)
	require.NotEmpty(t, result.Issues[0].Email, "the confirm response still carries the email")

	runs := importRunsFor(t, db, user.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, 1, runs[0].IssueCount)

	var raw string
	require.NoError(t, db.Raw("SELECT issues FROM import_runs WHERE id = ?", runs[0].ID).Scan(&raw).Error)
	var stored []models.ImportRowIssue
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	require.Len(t, stored, 1)
	assert.Equal(t, result.Issues[0].Row, stored[0].Row)
	assert.Equal(t, result.Issues[0].Name, stored[0].Name)
	assert.Equal(t, "skipped", stored[0].Status)
	assert.Equal(t, result.Issues[0].Reason, stored[0].Reason)
	assert.Empty(t, stored[0].Email, "email must not be persisted")
	assert.Empty(t, stored[0].Phone, "phone must not be persisted")
	assert.NotContains(t, raw, "example.com")
}

func TestRecordImportRun_CapsStoredIssues(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "cap-issues")
	issues := make([]models.ImportRowIssue, models.MaxImportRunIssues+25)
	for i := range issues {
		issues[i] = models.ImportRowIssue{Row: i + 1, Name: "n", Status: "skipped", Reason: "r"}
	}
	models.RecordImportRun(context.Background(), db, models.ImportRun{
		UserID: user.ID, Format: models.ImportFormatCSV, Issues: issues,
	})
	runs := importRunsFor(t, db, user.ID)
	require.Len(t, runs, 1)
	assert.Equal(t, models.MaxImportRunIssues, runs[0].IssueCount)
	var stored []models.ImportRowIssue
	require.NoError(t, json.Unmarshal([]byte(runs[0].IssuesJSON), &stored))
	assert.Len(t, stored, models.MaxImportRunIssues)
	assert.Equal(t, 1, stored[0].Row, "the first issues are kept")
}

func TestSourceImportResultFromReport_MapsSkippedAndInvalidIssues(t *testing.T) {
	report := &ImportReport{Issues: []ImportIssue{
		{Record: "contact 7 (Jane Doe)", Field: "contact", Category: ImportIssueCategorySkipped, Message: "excluded in the import review step"},
		{Record: "contact 9 (Bob)", Field: "email", Category: ImportIssueCategoryInvalid, Message: "not an address"},
		{Record: "contact 9 (Bob)", Field: "birthday", Category: ImportIssueCategoryLossy, Message: "truncated"},
	}}
	got := sourceImportResultFromReport(report, 3)
	require.Len(t, got.Issues, 2, "only skipped and invalid issues are row issues")
	assert.Equal(t, models.ImportRowIssue{Row: 1, Name: "contact 7 (Jane Doe)", Status: "skipped", Reason: "excluded in the import review step"}, got.Issues[0])
	assert.Equal(t, models.ImportRowIssue{Row: 2, Name: "contact 9 (Bob)", Status: "failed", Reason: "email: not an address"}, got.Issues[1])
}

// Issue #2563: a row with no name is reported under its email or phone in the
// live confirm response, but that fallback must never reach the persisted
// history (docs/security/pii-inventory.md: "never email, phone").

func rawStoredIssues(t *testing.T, db *gorm.DB, userID uint) (string, []models.ImportRowIssue) {
	t.Helper()
	runs := importRunsFor(t, db, userID)
	require.Len(t, runs, 1)
	var raw string
	require.NoError(t, db.Raw("SELECT issues FROM import_runs WHERE id = ?", runs[0].ID).Scan(&raw).Error)
	var stored []models.ImportRowIssue
	require.NoError(t, json.Unmarshal([]byte(raw), &stored))
	return raw, stored
}

func TestConfirm_CSV_NamelessRowsDoNotPersistEmailOrPhoneAsName(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "csv-nameless")
	m := NewImportSessionManager()

	headers := []string{"First Name", "Email", "Phone"}
	rows := [][]string{{"", "jane.private@example.org", ""}, {"", "", "+14155550142"}}
	id := m.CreateCSVSession(user.ID, headers, rows)
	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Email", ContactField: "email"},
			{CSVColumn: "Phone", ContactField: "phone"},
		},
	})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "skip"}, {RowIndex: 1, Action: "skip"}},
	}, testImportLogger())
	require.Nil(t, appErr)
	require.Len(t, result.Issues, 2)
	assert.Equal(t, "jane.private@example.org", result.Issues[0].Name, "the live response keeps the fallback name")
	assert.Equal(t, "+14155550142", result.Issues[1].Name)

	raw, stored := rawStoredIssues(t, db, user.ID)
	require.Len(t, stored, 2)
	assert.NotContains(t, raw, "jane.private@example.org")
	assert.NotContains(t, raw, "4155550142")
	assert.Equal(t, 1, stored[0].Row, "the row reference survives")
	assert.Equal(t, 2, stored[1].Row)
}

func TestConfirmVCF_NamelessRowsDoNotPersistEmailOrPhoneAsName(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "vcf-nameless")
	m := NewImportSessionManager()

	id := m.CreateVCFSession(user.ID,
		[]VCFContactData{{Contact: &models.Contact{}}, {Contact: &models.Contact{}}},
		[]models.ImportRowPreview{
			{RowIndex: 0, ParsedContact: map[string]any{"email": "jane.private@example.org"}, SuggestedAction: "skip"},
			{RowIndex: 1, ParsedContact: map[string]any{"phone": "+14155550142"}, SuggestedAction: "skip"},
		})
	result, appErr := m.ConfirmVCF(context.Background(), db, user.ID, models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "skip"}, {RowIndex: 1, Action: "skip"}},
	}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	require.Len(t, result.Issues, 2)
	assert.Equal(t, "jane.private@example.org", result.Issues[0].Name, "the live response keeps the fallback name")

	raw, stored := rawStoredIssues(t, db, user.ID)
	require.Len(t, stored, 2)
	assert.NotContains(t, raw, "jane.private@example.org")
	assert.NotContains(t, raw, "4155550142")
}

// A real name that merely resembles nothing sensitive is kept.
func TestStoredImportIssues_KeepsRealNamesAndBlanksDerivedOnes(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "name-keep")
	models.RecordImportRun(context.Background(), db, models.ImportRun{
		UserID: user.ID, Format: models.ImportFormatCSV,
		Issues: []models.ImportRowIssue{
			{Row: 1, Name: "Alice Smith", Email: "alice@example.com", Status: "skipped", Reason: "r"},
			{Row: 2, Name: "bob@example.com", Email: "bob@example.com", Status: "skipped", Reason: "r"},
			{Row: 3, Name: "+14155550142", Phone: "+14155550142", Status: "skipped", Reason: "r"},
		},
	})
	_, stored := rawStoredIssues(t, db, user.ID)
	require.Len(t, stored, 3)
	assert.Equal(t, "Alice Smith", stored[0].Name)
	assert.Empty(t, stored[1].Name)
	assert.Empty(t, stored[2].Name)
}
