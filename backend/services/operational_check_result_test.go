package services

import (
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func freshDB(t *testing.T, name string) *gorm.DB {
	t.Helper()
	db, err := database.InitDB(filepath.Join(t.TempDir(), name))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRecordOperationalCheckResult_InsertThenUpsert(t *testing.T) {
	db := freshDB(t, "opcheck.db")

	RecordOperationalCheckResult(db, "widget_check", models.OpCheckStatusFailed, "boom")

	var row models.OperationalCheckResult
	require.NoError(t, db.Where("check_name = ?", "widget_check").First(&row).Error)
	assert.Equal(t, models.OpCheckStatusFailed, row.Status)
	assert.Equal(t, "boom", row.Detail)
	firstCheckedAt := row.CheckedAt

	time.Sleep(5 * time.Millisecond)
	RecordOperationalCheckResult(db, "widget_check", models.OpCheckStatusOK, "")

	var rows []models.OperationalCheckResult
	require.NoError(t, db.Where("check_name = ?", "widget_check").Find(&rows).Error)
	require.Len(t, rows, 1, "must upsert in place, not append a second row")
	assert.Equal(t, models.OpCheckStatusOK, rows[0].Status)
	assert.Empty(t, rows[0].Detail)
	assert.True(t, rows[0].CheckedAt.After(firstCheckedAt), "checked_at must advance on upsert")
}

// The scheduled integrity check records its outcome so deep /health can report
// the last actual verdict, not just "did it run".
func TestCheckDBIntegrityScheduled_RecordsOKResult(t *testing.T) {
	db := freshDB(t, "integrity-record.db")
	cfg := config.Config{DBIntegrityCheckEnabled: true, DBIntegrityCheckIntervalHours: 24}

	require.NotPanics(t, func() { CheckDBIntegrityScheduled(db, cfg) })

	var row models.OperationalCheckResult
	require.NoError(t, db.Where("check_name = ?", models.JobNameDBIntegrityCheck).First(&row).Error)
	assert.Equal(t, models.OpCheckStatusOK, row.Status)
}

func TestRunRestoreDrillScheduled_RecordsOKResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-record.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	})
	seedNotes(t, db, "drillrecordtester", 5)

	cfg := config.Config{DBPath: path, DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	var row models.OperationalCheckResult
	require.NoError(t, db.Where("check_name = ?", models.JobNameRestoreDrill).First(&row).Error)
	assert.Equal(t, models.OpCheckStatusOK, row.Status)
}

func TestRunRestoreDrillScheduled_RecordsErrorResult(t *testing.T) {
	db := freshDB(t, "drill-record-err.db")

	// A DBPath that does not exist makes the snapshot step fail -> error path.
	cfg := config.Config{
		DBPath:                      filepath.Join(t.TempDir(), "missing.db"),
		DBRestoreDrillEnabled:       true,
		DBRestoreDrillIntervalHours: 168,
	}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	var row models.OperationalCheckResult
	require.NoError(t, db.Where("check_name = ?", models.JobNameRestoreDrill).First(&row).Error)
	assert.Equal(t, models.OpCheckStatusError, row.Status)
	assert.NotEmpty(t, row.Detail)
}
