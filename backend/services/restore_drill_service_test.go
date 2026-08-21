package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// TestLiveTablesErrorsOnClosedConnection covers liveTables' own query-error
// return, distinct from a healthy call that simply finds no tables.
func TestLiveTablesErrorsOnClosedConnection(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "live-tables-closed.db"))
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = liveTables(db)
	require.Error(t, err)
}

// TestCountTableRowsErrorsOnUnknownTable pins countTableRows' error-wrapping
// contract: a table name that doesn't exist must surface as an error naming
// the table, not a silent zero count (which would masquerade as "0 rows",
// indistinguishable from a genuinely empty table).
func TestCountTableRowsErrorsOnUnknownTable(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "count-unknown-table.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	_, err = countTableRows(db, "does_not_exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does_not_exist")
}

// TestCompareTableCountsErrorsWhenScratchTableMissing covers the case where a
// table counted live has no counterpart in the restored snapshot at all —
// a real restore failure (not merely a count mismatch), which must surface
// as an error rather than being silently skipped.
func TestCompareTableCountsErrorsWhenScratchTableMissing(t *testing.T) {
	scratchDB, err := database.InitDB(filepath.Join(t.TempDir(), "compare-missing-table.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := scratchDB.DB(); err == nil {
			sqlDB.Close()
		}
	})

	liveCounts := map[string]int64{"table_that_does_not_exist_in_scratch": 5}
	_, _, err = compareTableCounts(liveCounts, scratchDB)
	require.Error(t, err)
}

// TestRestoreDrillMinIntervalClampsToMargin mirrors
// TestDBIntegrityCheckMinIntervalClampsToMargin: a configured interval at or
// below the margin must floor at the margin rather than going zero/negative.
func TestRestoreDrillMinIntervalClampsToMargin(t *testing.T) {
	got := restoreDrillMinInterval(config.Config{DBRestoreDrillIntervalHours: 0})
	assert.Equal(t, restoreDrillMinIntervalMargin, got)
}

func TestRestoreDrillMinIntervalAboveMargin(t *testing.T) {
	got := restoreDrillMinInterval(config.Config{DBRestoreDrillIntervalHours: 168})
	assert.Equal(t, 168*time.Hour-restoreDrillMinIntervalMargin, got)
}

// newTestWebhookServer spins up an httptest server recording hits, and
// registers an active webhook for user subscribed to eventType. Shared by
// the RunRestoreDrillScheduled full-path tests below.
func newTestWebhookServer(t *testing.T, db *gorm.DB, userID uint, eventType string) *int32 {
	t.Helper()
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	wh := models.Webhook{UserID: userID, Name: "alerts", URL: server.URL, Events: []string{eventType}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)
	return &hits
}

// TestRunRestoreDrillScheduledFiresWebhookOnMismatch exercises
// RunRestoreDrillScheduled end to end through a genuine row-count mismatch,
// forced deterministically rather than by racing a timing window: a GORM
// "before raw query" hook injects one extra live write the instant
// countTableRows queries the "notes" table, which happens *after*
// BackupSnapshot has already captured the (now stale) snapshot. The restored
// copy is then guaranteed to disagree with the live count by exactly one
// row, with no dependence on real-world timing.
func TestRunRestoreDrillScheduledFiresWebhookOnMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-mismatch-live.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	user := seedNotes(t, db, "mismatchtester", 5)

	var injected int32
	require.NoError(t, db.Callback().Row().Before("gorm:row").Register("test:inject-post-snapshot-write", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), `FROM "notes"`) {
			return
		}
		if !atomic.CompareAndSwapInt32(&injected, 0, 1) {
			return
		}
		require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "post-snapshot write", Date: time.Now()}).Error)
	}))

	hits := newTestWebhookServer(t, db, user.ID, EventRestoreDrillFailed)

	cfg := config.Config{DBPath: path, DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	assert.Equal(t, int32(1), atomic.LoadInt32(&injected), "the injected write must actually have landed for this test to prove anything")

	var deliveries []models.WebhookDelivery
	require.Eventually(t, func() bool {
		if err := db.Find(&deliveries).Error; err != nil {
			return false
		}
		return len(deliveries) >= 1
	}, 3*time.Second, 10*time.Millisecond, "a row-count mismatch must trigger a db.restore_drill_failed webhook")
	assert.GreaterOrEqual(t, atomic.LoadInt32(hits), int32(1))

	require.Len(t, deliveries, 1)
	assert.Equal(t, EventRestoreDrillFailed, deliveries[0].EventType)

	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			Detail string `json:"detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &envelope))
	assert.Contains(t, envelope.Data.Detail, "notes")

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameRestoreDrill).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock must be released even when the drill finds a mismatch")
}

// TestRunRestoreDrillScheduledFiresWebhookOnError exercises the distinct
// "the drill itself failed to run" path (as opposed to running and finding a
// mismatch): cfg.DBPath deliberately does not exist, so BackupSnapshot fails
// at its very first os.Stat check, deterministically forcing
// runRestoreDrill's error return.
func TestRunRestoreDrillScheduledFiresWebhookOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-error-live.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillerrortester", 3)

	hits := newTestWebhookServer(t, db, user.ID, EventRestoreDrillFailed)

	cfg := config.Config{DBPath: filepath.Join(t.TempDir(), "does-not-exist.db"), DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	var deliveries []models.WebhookDelivery
	require.Eventually(t, func() bool {
		if err := db.Find(&deliveries).Error; err != nil {
			return false
		}
		return len(deliveries) >= 1
	}, 3*time.Second, 10*time.Millisecond, "a failed backup/restore must trigger a db.restore_drill_failed webhook")
	assert.GreaterOrEqual(t, atomic.LoadInt32(hits), int32(1))

	require.Len(t, deliveries, 1)
	assert.Equal(t, EventRestoreDrillFailed, deliveries[0].EventType)

	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			Error string `json:"error"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &envelope))
	assert.NotEmpty(t, envelope.Data.Error)

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameRestoreDrill).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock must be released even when the drill itself errors")
}

// TestRunRestoreDrillScheduledHealthyRun exercises the full success path
// through the scheduled entry point (as opposed to calling runRestoreDrill
// directly, as TestRunRestoreDrillMatchingCounts does): the job lock must
// cycle cleanly and no failure webhook may fire.
func TestRunRestoreDrillScheduledHealthyRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-healthy.db")
	db, err := database.InitDB(path)
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillhealthytester", 10)

	hits := newTestWebhookServer(t, db, user.ID, EventRestoreDrillFailed)

	cfg := config.Config{DBPath: path, DBRestoreDrillEnabled: true, DBRestoreDrillIntervalHours: 168}
	require.NotPanics(t, func() { RunRestoreDrillScheduled(db, cfg) })

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameRestoreDrill).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock must be released after a healthy run")
	assert.WithinDuration(t, time.Now(), job.LastRunAt, 5*time.Second)

	assert.Zero(t, atomic.LoadInt32(hits), "a healthy drill must not fire the failure webhook")
	var count int64
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&count).Error)
	assert.Zero(t, count)
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
