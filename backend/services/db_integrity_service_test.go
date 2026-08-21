package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// corruptTailPage truncates one page's worth of bytes off the end of a
// closed SQLite file. The header (page 1) still claims the original page
// count, so the file still opens fine — PRAGMA integrity_check is what
// notices the missing/short pages, exactly the class of corruption this job
// exists to catch.
func corruptTailPage(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	const pageSize = 4096
	require.Greater(t, info.Size(), int64(4*pageSize), "need enough pages for truncation to hit real data, not just the header")
	require.NoError(t, os.Truncate(path, info.Size()-pageSize))
}

// seededLiveDB builds a real migrated database (CLAUDE.md trap #1 — never
// AutoMigrate) with enough bulk data that it spans multiple SQLite pages, so
// corruptTailPage has real data to land on.
func seededLiveDB(t *testing.T, name string) (db *gorm.DB, path string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), name)
	db, err := database.InitDB(path)
	require.NoError(t, err)

	user := models.User{Username: "integritytester", Password: "password123!A", Email: "integrity@example.com"}
	require.NoError(t, db.Create(&user).Error)

	for i := 0; i < 500; i++ {
		note := models.Note{UserID: user.ID, Content: "bulk note content padding the file with real pages so truncation lands on data", Date: time.Now()}
		require.NoError(t, db.Create(&note).Error)
	}

	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db, path
}

// closeDB is a t.Cleanup-free explicit close, used when the test needs the
// file closed (and corrupted) partway through, before its own t.Cleanup would
// otherwise fire.
func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	// WAL mode means recent writes may still be sitting in the -wal sidecar
	// rather than the main file; truncate-checkpoint first so the bulk data
	// corruptTailPage depends on is actually in the file it truncates.
	require.NoError(t, db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

// openRaw opens a plain (non-migrating) connection to an existing file —
// used for the corrupted-file case, where running database.InitDB's
// migration check against a corrupted schema page would be its own
// (unrelated) failure mode.
func openRaw(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func TestCheckDBIntegrityHealthyDB(t *testing.T) {
	db, _ := seededLiveDB(t, "healthy.db")

	ok, detail, err := checkDBIntegrity(db)
	require.NoError(t, err)
	assert.True(t, ok, "detail: %s", detail)
	assert.Empty(t, detail)
}

func TestCheckDBIntegrityDetectsCorruption(t *testing.T) {
	db, path := seededLiveDB(t, "corrupt.db")
	closeDB(t, db)

	corruptTailPage(t, path)

	raw := openRaw(t, path)
	ok, detail, err := checkDBIntegrity(raw)
	require.NoError(t, err, "integrity_check itself must not error on a corrupted-but-openable file")
	assert.False(t, ok)
	assert.NotEmpty(t, detail, "must report what integrity_check found")
}

func TestCheckDBIntegrityScheduledFiresWebhookOnCorruption(t *testing.T) {
	db, path := seededLiveDB(t, "corrupt-scheduled.db")
	user := models.User{}
	require.NoError(t, db.Where("username = ?", "integritytester").First(&user).Error)
	closeDB(t, db)

	corruptTailPage(t, path)
	raw := openRaw(t, path)

	// job_executions and webhooks tables are created early by migrations and
	// were empty at corruption time (all the bulk data went into notes), so
	// they're expected to have survived the tail truncation intact.
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "alerts", URL: server.URL, Events: []string{EventDBIntegrityCheckFailed}, Secret: "s", IsActive: true}
	require.NoError(t, raw.Create(&wh).Error)

	cfg := config.Config{DBIntegrityCheckEnabled: true, DBIntegrityCheckIntervalHours: 24}
	require.NotPanics(t, func() { CheckDBIntegrityScheduled(raw, cfg) })

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&hits) >= 1
	}, 3*time.Second, 10*time.Millisecond, "corruption must trigger a db.integrity_check_failed webhook")

	var deliveries []models.WebhookDelivery
	require.NoError(t, raw.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, EventDBIntegrityCheckFailed, deliveries[0].EventType)

	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			Detail string `json:"detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &envelope))
	assert.NotEmpty(t, envelope.Data.Detail)

	var job models.JobExecution
	require.NoError(t, raw.Where("job_name = ?", models.JobNameDBIntegrityCheck).First(&job).Error)
	assert.Nil(t, job.LockedAt, "lock must be released after the job completes")
}

func TestCheckDBIntegrityScheduledSkipsWhenLocked(t *testing.T) {
	db, _ := seededLiveDB(t, "locked.db")

	seededAt := time.Now().Add(-1 * time.Minute)
	require.NoError(t, db.Create(&models.JobExecution{
		JobName: models.JobNameDBIntegrityCheck, LastRunAt: seededAt,
	}).Error)

	cfg := config.Config{DBIntegrityCheckEnabled: true, DBIntegrityCheckIntervalHours: 24}
	require.NotPanics(t, func() { CheckDBIntegrityScheduled(db, cfg) })

	var job models.JobExecution
	require.NoError(t, db.Where("job_name = ?", models.JobNameDBIntegrityCheck).First(&job).Error)
	assert.WithinDuration(t, seededAt, job.LastRunAt, time.Second, "a run within the min interval must be skipped, not re-run")
	assert.Nil(t, job.LockedAt)
}

func TestCheckDBIntegrityScheduledDisabledByConfig(t *testing.T) {
	db, _ := seededLiveDB(t, "disabled.db")

	cfg := config.Config{DBIntegrityCheckEnabled: false, DBIntegrityCheckIntervalHours: 24}
	require.NotPanics(t, func() { CheckDBIntegrityScheduled(db, cfg) })

	var count int64
	require.NoError(t, db.Model(&models.JobExecution{}).Where("job_name = ?", models.JobNameDBIntegrityCheck).Count(&count).Error)
	assert.Zero(t, count, "a disabled job must never touch the job_executions table")
}
