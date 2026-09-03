package services

import (
	"strings"
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/diskspace"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #498: the import-confirm and source-import transactions run a disk
// preflight before they open, so a too-full disk is a clear 507 with the
// session unconsumed and zero rows written — not an ENOSPC deep inside the
// transaction and a raw SQLite error in the logs.

func TestSqliteMainFilePath_ReturnsPathForOnDiskDatabase(t *testing.T) {
	db := dbtest.New(t)
	path, ok := sqliteMainFilePath(db)
	require.True(t, ok, "an on-disk database has a resolvable main file path")
	assert.True(t, strings.HasSuffix(path, "x.db"), "path %q should point at the test database file", path)
}

func TestSqliteMainFilePath_FalseForInMemoryDatabase(t *testing.T) {
	db := setupImportSessionTestDB(t) // gorm sqlite :memory:
	path, ok := sqliteMainFilePath(db)
	assert.False(t, ok, "an in-memory database has no on-disk path")
	assert.Empty(t, path)
}

func TestPreflightImportDiskSpace_NoopForInMemoryDatabase(t *testing.T) {
	db := setupImportSessionTestDB(t)
	restore := diskspace.StubForTest(1 << 10) // 1 KiB free — would refuse if it ran
	t.Cleanup(restore)
	assert.Nil(t, preflightImportDiskSpace(db, 100_000), "in-memory DB skips the preflight")
}

func TestConfirm_RefusesWhenDiskTooFull(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "csv-preflight")
	m := NewImportSessionManager()

	headers := []string{"First Name", "Last Name"}
	rows := [][]string{{"Alice", "Smith"}, {"Bob", "Jones"}}
	id := m.CreateCSVSession(user.ID, headers, rows)
	_, appErr := m.PreviewCSV(db, user.ID, models.ImportPreviewRequest{
		SessionID: id,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Last Name", ContactField: "lastname"},
		},
	})
	require.Nil(t, appErr)

	req := models.ImportConfirmRequest{
		SessionID: id,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}, {RowIndex: 1, Action: "add"}},
	}

	restore := diskspace.StubForTest(1 << 20) // 1 MiB free
	result, appErr := m.Confirm(db, user.ID, req, testImportLogger())
	restore()

	require.NotNil(t, appErr, "a too-full disk must refuse the confirm")
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)
	assert.Equal(t, 507, appErr.HTTPStatus)

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "a refused confirm writes no rows")

	// Session was not consumed: a retry once space is available applies cleanly.
	result, appErr = m.Confirm(db, user.ID, req, testImportLogger())
	require.Nil(t, appErr)
	require.NotNil(t, result)
	assert.Equal(t, 2, result.Created)
}

func TestConfirmVCF_RefusesWhenDiskTooFull(t *testing.T) {
	db := dbtest.New(t)
	user := newImportHistoryUser(t, db, "vcf-preflight")
	m := NewImportSessionManager()
	cfg := testImportConfig(t)

	contact := &models.Contact{Firstname: "Ada", Lastname: "Lovelace", Email: "ada@example.com"}
	id := m.CreateVCFSession(user.ID, []VCFContactData{{Contact: contact}}, []models.ImportRowPreview{{RowIndex: 0, SuggestedAction: "add"}})
	req := models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}}}

	restore := diskspace.StubForTest(1 << 20)
	result, appErr := m.ConfirmVCF(db, user.ID, req, cfg, testImportLogger())
	restore()

	require.NotNil(t, appErr)
	assert.Nil(t, result)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)

	result, appErr = m.ConfirmVCF(db, user.ID, req, cfg, testImportLogger())
	require.Nil(t, appErr)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Created)
}

func TestExecuteSourceImport_RefusesWhenDiskTooFull(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	plan := &ImportSourcePlan{System: "test"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")},
		{Ref: ref("contact/2"), Record: minimalRecord("Ben", "Babbage")},
	}

	restore := diskspace.StubForTest(1 << 20)
	report, err := ExecuteSourceImport(db, user.ID, plan)
	restore()

	require.Error(t, err, "a too-full disk must refuse the source import")
	assert.Nil(t, report)
	var appErr *apperrors.AppError
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, apperrors.ErrCodeInsufficientStorage, appErr.Code)

	var count int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count, "a refused source import writes no contacts")

	// A retry with space available lands every contact.
	report, err = ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	assert.Equal(t, 2, report.ContactsCreated)
}
