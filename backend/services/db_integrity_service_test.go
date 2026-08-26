package services

import (
	"bytes"
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

// corruptDataPage overwrites one whole page of a closed SQLite file with
// garbage bytes, in place, well past the schema pages so it lands on real
// notes-table data rather than the header.
//
// An earlier version of this helper truncated one page's worth of bytes off
// the end instead. That shrinks the file below what page 1's header still
// claims as the total page count, and whether that reads back as graceful
// ("Page N: never used" / a freelist-count mismatch PRAGMA integrity_check
// can report as text) or as a hard, unopenable "database disk image is
// malformed" I/O error depends entirely on whether the now-missing tail
// page happened to be on the freelist (unreferenced -- safe) or still
// referenced by a live b-tree (a real short read past EOF -- fatal). Which
// one you get is a function of the database's exact page count, which is
// sensitive to incidental things like how many connections happened to
// checkpoint the WAL before this file was sealed -- not something a test
// should be pinned to. Overwriting a page's bytes in place instead never
// changes the file's size, so it can never produce a short read: every page
// the header claims still physically exists, and corruption is always the
// gentler "this page's content doesn't parse" case integrity_check is
// built to report as findings, not the fatal "this page doesn't exist"
// case. See PR #579 review.
func corruptDataPage(t *testing.T, path string) {
	t.Helper()
	const pageSize = 4096
	info, err := os.Stat(path)
	require.NoError(t, err)
	pageCount := info.Size() / pageSize
	require.Greater(t, pageCount, int64(8), "need enough pages for the corruption target to land past the schema pages, on real notes data")

	// The schema pages (sqlite_master, and the small users/webhooks/
	// job_executions tables) sit at the front of the file; bulk notes data
	// dominates everything after that. The middle of the file is
	// comfortably inside that notes region without ever risking page 1
	// (the file header) or the tables the webhook-delivery test depends on
	// surviving untouched.
	target := pageCount / 2

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	garbage := bytes.Repeat([]byte{0xFF}, pageSize)
	_, err = f.WriteAt(garbage, (target-1)*pageSize)
	require.NoError(t, err)
}

// seededLiveDB builds a real migrated database (CLAUDE.md trap #1 — never
// AutoMigrate) with enough bulk data that it spans multiple SQLite pages, so
// corruptDataPage has real data to land on.
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
	// corruptDataPage depends on is actually in the file it corrupts.
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

	corruptDataPage(t, path)

	raw := openRaw(t, path)
	ok, detail, err := checkDBIntegrity(raw)
	require.NoError(t, err, "integrity_check itself must not error on a corrupted-but-openable file")
	assert.False(t, ok)
	assert.NotEmpty(t, detail, "must report what integrity_check found")
}

// TestCheckDBIntegrityErrorsOnClosedConnection distinguishes "the
// integrity_check query itself failed" (a real Go error, e.g. a lost
// connection) from "the query ran and found corruption" (TestCheckDBIntegrityDetectsCorruption).
// CheckDBIntegrityScheduled fires a differently-shaped webhook payload for
// each ("error" vs. "detail"), so the two must stay distinguishable.
func TestCheckDBIntegrityErrorsOnClosedConnection(t *testing.T) {
	db, _ := seededLiveDB(t, "integrity-closed.db")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	ok, detail, err := checkDBIntegrity(db)
	require.Error(t, err, "a failed integrity_check query itself (not corruption found) must surface as an error")
	assert.False(t, ok)
	assert.Empty(t, detail)
}

// TestDBIntegrityCheckMinIntervalClampsToMargin pins the same guard
// restoreDrillMinInterval has: a configured interval at or below the margin
// must floor at the margin rather than going zero/negative, which would
// otherwise let the job re-run on every cron tick.
func TestDBIntegrityCheckMinIntervalClampsToMargin(t *testing.T) {
	got := dbIntegrityCheckMinInterval(config.Config{DBIntegrityCheckIntervalHours: 0})
	assert.Equal(t, dbIntegrityCheckMinIntervalMargin, got)
}

func TestDBIntegrityCheckMinIntervalAboveMargin(t *testing.T) {
	got := dbIntegrityCheckMinInterval(config.Config{DBIntegrityCheckIntervalHours: 24})
	assert.Equal(t, 24*time.Hour-dbIntegrityCheckMinIntervalMargin, got)
}

func TestCheckDBIntegrityScheduledFiresWebhookOnCorruption(t *testing.T) {
	db, path := seededLiveDB(t, "corrupt-scheduled.db")
	user := models.User{}
	require.NoError(t, db.Where("username = ?", "integritytester").First(&user).Error)
	closeDB(t, db)

	corruptDataPage(t, path)
	raw := openRaw(t, path)

	// job_executions and webhooks tables are created early by migrations and
	// were empty at corruption time (all the bulk data went into notes), so
	// they're expected to have survived the corruption intact.
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

	// deliverWebhook runs in its own goroutine (see TriggerWebhooks) and only
	// writes the WebhookDelivery row *after* the HTTP round-trip completes, so
	// polling on `hits` alone (set the instant the request arrives, before the
	// response is even sent) races that write. Poll for the row itself instead.
	var deliveries []models.WebhookDelivery
	require.Eventually(t, func() bool {
		if atomic.LoadInt32(&hits) < 1 {
			return false
		}
		if err := raw.Find(&deliveries).Error; err != nil {
			return false
		}
		return len(deliveries) >= 1
	}, 3*time.Second, 10*time.Millisecond, "corruption must trigger a db.integrity_check_failed webhook and persist a delivery")

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
