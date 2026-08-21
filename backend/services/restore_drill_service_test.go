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

// seedNotes creates n plain Note rows for a fresh user in db and returns the
// user, for building tables of a known, controllable row count.
func seedNotes(t *testing.T, db *gorm.DB, username string, n int) models.User {
	t.Helper()
	user := models.User{Username: username, Password: "password123!A", Email: username + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	for i := 0; i < n; i++ {
		require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "drill note", Date: time.Now()}).Error)
	}
	return user
}

func TestRunRestoreDrillMatchingCounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-live.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	seedNotes(t, db, "drilltester", 10)

	cfg := config.Config{DBPath: path}
	ok, detail, err := runRestoreDrill(db, cfg)
	require.NoError(t, err)
	assert.True(t, ok, "detail: %s", detail)
	assert.Empty(t, detail)
}

func TestCompareTableCountsDetectsMismatch(t *testing.T) {
	livePath := filepath.Join(t.TempDir(), "compare-live.db")
	liveDB, err := database.InitDB(livePath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := liveDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, liveDB, "livecompare", 10)

	scratchPath := filepath.Join(t.TempDir(), "compare-scratch.db")
	scratchDB, err := database.InitDB(scratchPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := scratchDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	// Deliberately different: one fewer note than the live database, so the
	// two databases' "notes" row counts disagree.
	seedNotes(t, scratchDB, "scratchcompare", 9)

	liveNames, err := liveTables(liveDB)
	require.NoError(t, err)
	liveCounts := make(map[string]int64, len(liveNames))
	for _, name := range liveNames {
		n, err := countTableRows(liveDB, name)
		require.NoError(t, err)
		liveCounts[name] = n
	}

	ok, detail, err := compareTableCounts(liveCounts, scratchDB)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, detail, "notes")
	assert.Contains(t, detail, "live=")
	assert.Contains(t, detail, "restored=")
}

func TestCompareTableCountsMatching(t *testing.T) {
	livePath := filepath.Join(t.TempDir(), "compare-match-live.db")
	liveDB, err := database.InitDB(livePath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := liveDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, liveDB, "matchlive", 7)

	scratchPath := filepath.Join(t.TempDir(), "compare-match-scratch.db")
	scratchDB, err := database.InitDB(scratchPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := scratchDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, scratchDB, "matchscratch", 7)

	liveNames, err := liveTables(liveDB)
	require.NoError(t, err)
	liveCounts := make(map[string]int64, len(liveNames))
	for _, name := range liveNames {
		n, err := countTableRows(liveDB, name)
		require.NoError(t, err)
		liveCounts[name] = n
	}

	ok, detail, err := compareTableCounts(liveCounts, scratchDB)
	require.NoError(t, err)
	assert.True(t, ok, "detail: %s", detail)
}

// TestLiveTablesExcludesJobExecutions pins a real false-positive found while
// hand-verifying this feature: job_executions is written by every scheduled
// job's own lock acquisition, including a burst of concurrent initial runs
// at boot, so including it in the drift comparison produces false alarms
// unrelated to actual restore fidelity.
func TestLiveTablesExcludesJobExecutions(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "exclude-job-executions.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	require.NoError(t, db.Create(&models.JobExecution{JobName: "some_job", LastRunAt: time.Now()}).Error)

	names, err := liveTables(db)
	require.NoError(t, err)
	assert.NotContains(t, names, "job_executions")
}

func TestRunRestoreDrillScheduledSkipsWhenLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-locked.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	seededAt := time.Now().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&models.JobExecution{
		JobName: models.JobNameRestoreDrill, LastRunAt: seededAt,
	}).Error)

	cfg := config.Config{DBPath: path, DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameRestoreDrill).First(&job).Error)
	assert.WithinDuration(t, seededAt, job.LastRunAt, time.Second, "a run within the min interval must be skipped, not re-run")
	assert.Nil(t, job.LockedAt)
}

func TestRunRestoreDrillScheduledDisabledByConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-disabled.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	cfg := config.Config{DBPath: path, DBRestoreDrillEnabled: false, DBRestoreDrillIntervalHours: 168}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	var count int64
	require.NoError(t, db.Model(&models.JobExecution{}).Where("job_name = ?", models.JobNameRestoreDrill).Count(&count).Error)
	assert.Zero(t, count, "a disabled job must never touch the job_executions table")
}
