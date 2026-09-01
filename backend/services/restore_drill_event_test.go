package services

import (
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRestoreDrillScheduled_EmitsRestoreTestCompleted covers the operational
// event on the healthy path (issue #424): one restore_test_completed row with
// a duration and result=success.
func TestRestoreDrillScheduled_EmitsRestoreTestCompleted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-ev-ok.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, db, "drilleventok", 6)

	cfg := config.Config{DBPath: path, DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	RunRestoreDrillScheduled(db, cfg)

	var ev models.SystemEvent
	require.NoError(t, db.Where("event_type = ?", models.SysEventRestoreTestCompleted).Order("id desc").First(&ev).Error)
	assert.Equal(t, logger.ComponentBackup, ev.Component)
	require.NotNil(t, ev.Result)
	assert.Equal(t, logger.ResultSuccess, *ev.Result)
	require.NotNil(t, ev.DurationMS)
	// No DB_RESTORE_DRILL_MAX_DURATION_SECONDS budget configured, so the row
	// carries the recorded duration but no RTO-budget annotation (issue #506).
	assert.Empty(t, ev.Detail)
}

// TestRestoreDrillScheduled_EmitsBackupFailed covers the failure path: the
// drill cannot run (DBPath does not exist), so one backup_failed row with
// severity=error is written.
func TestRestoreDrillScheduled_EmitsBackupFailed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-ev-fail.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, db, "drilleventfail", 3)

	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "nope.db"), DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	RunRestoreDrillScheduled(db, cfg)

	var ev models.SystemEvent
	require.NoError(t, db.Where("event_type = ?", models.SysEventBackupFailed).Order("id desc").First(&ev).Error)
	assert.Equal(t, logger.SeverityError, ev.Severity)
	require.NotNil(t, ev.Result)
	assert.Equal(t, logger.ResultFailure, *ev.Result)
	assert.NotEmpty(t, ev.Error)
}
