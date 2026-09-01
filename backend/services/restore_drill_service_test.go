package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mycorrhizal/atrest"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
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
	db := dbtest.NewAt(t, path)
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

// TestRestoreDrillPassesWithEncryptedDatabase pins that the decryptability
// check added in #420 does not false-positive on a legitimately at-rest
// encrypted database: a snapshot whose wrapped DEK unwraps under the current
// master key must pass the drill exactly like an unencrypted one.
func TestRestoreDrillPassesWithEncryptedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-atrest-healthy.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	kek := bytes.Repeat([]byte{0x42}, 32)
	require.NoError(t, atrest.Initialize(db, kek))
	t.Cleanup(atrest.ResetForTest)

	seedNotes(t, db, "drillatrest", 10)
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(kek))

	cfg := config.Config{DBPath: path}
	ok, detail, err := runRestoreDrill(db, cfg)
	require.NoError(t, err)
	assert.True(t, ok, "detail: %s", detail)
}

// TestRestoreDrillFailsWhenSnapshotNotDecryptable is the hand-verified
// opposite of the healthy test: the live database is encrypted under key B,
// but the drill resolves key A from the environment — the exact mismatch a
// real restore hits after a master-key rotation. The restored snapshot's
// wrapped DEK must NOT unwrap under A, and the drill must fail loudly rather
// than pass on matching row counts (the encrypted columns count the same
// regardless of key).
func TestRestoreDrillFailsWhenSnapshotNotDecryptable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-atrest-key-mismatch.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	liveKEK := bytes.Repeat([]byte{0x42}, 32)
	require.NoError(t, atrest.Initialize(db, liveKEK))
	t.Cleanup(atrest.ResetForTest)

	seedNotes(t, db, "drillatrestkey", 10)

	// The drill resolves the master key from the environment; make it a
	// different key than the one the live database (and therefore the
	// snapshot) is wrapped under.
	t.Setenv("DATA_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x41}, 32)))

	cfg := config.Config{DBPath: path}
	_, _, err := runRestoreDrill(db, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not decryptable under the current master key")
}

func TestCompareTableCountsDetectsMismatch(t *testing.T) {
	livePath := filepath.Join(t.TempDir(), "compare-live.db")
	liveDB := dbtest.NewAt(t, livePath)
	t.Cleanup(func() {
		if sqlDB, err := liveDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, liveDB, "livecompare", 10)

	scratchPath := filepath.Join(t.TempDir(), "compare-scratch.db")
	scratchDB := dbtest.NewAt(t, scratchPath)
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
	liveDB := dbtest.NewAt(t, livePath)
	t.Cleanup(func() {
		if sqlDB, err := liveDB.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, liveDB, "matchlive", 7)

	scratchPath := filepath.Join(t.TempDir(), "compare-match-scratch.db")
	scratchDB := dbtest.NewAt(t, scratchPath)
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
	db := dbtest.New(t)
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
	db := dbtest.New(t)
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
	db := dbtest.New(t)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	_, err := countTableRows(db, "does_not_exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does_not_exist")
}

// TestCompareTableCountsErrorsWhenScratchTableMissing covers the case where a
// table counted live has no counterpart in the restored snapshot at all —
// a real restore failure (not merely a count mismatch), which must surface
// as an error rather than being silently skipped.
func TestCompareTableCountsErrorsWhenScratchTableMissing(t *testing.T) {
	scratchDB := dbtest.New(t)
	t.Cleanup(func() {
		if sqlDB, err := scratchDB.DB(); err == nil {
			sqlDB.Close()
		}
	})

	liveCounts := map[string]int64{"table_that_does_not_exist_in_scratch": 5}
	_, _, err := compareTableCounts(liveCounts, scratchDB)
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

// newTestWebhookServer spins up an httptest server recording hits and the
// raw request bodies, and registers an active webhook for user subscribed to
// eventType. Both the hit counter and the captured body are returned — the
// body because, since issue #622, the stored delivery receipt no longer keeps
// the entity body, so the wire body is the only place the payload's data is
// still observable. Shared by the RunRestoreDrillScheduled full-path tests
// below.
func newTestWebhookServer(t *testing.T, db *gorm.DB, userID uint, eventType string) (*int32, *[]byte) {
	t.Helper()
	var hits int32
	var bodies []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, b...)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	wh := models.Webhook{UserID: userID, Name: "alerts", URL: server.URL, Events: []string{eventType}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)
	return &hits, &bodies
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
	db := dbtest.NewAt(t, path)
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

	hits, deliveredBody := newTestWebhookServer(t, db, user.ID, EventRestoreDrillFailed)

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

	// The wire body carries the full payload; the stored receipt is the
	// trimmed envelope (issue #622: a successful delivery never needs the
	// entity body it carried).
	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			Detail string `json:"detail"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(*deliveredBody, &envelope))
	assert.Contains(t, envelope.Data.Detail, "notes")

	var storedEnvelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &storedEnvelope))
	assert.Equal(t, EventRestoreDrillFailed, storedEnvelope.Event)
	assert.Nil(t, storedEnvelope.Data, "the stored receipt must not retain the entity body")

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
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillerrortester", 3)

	hits, deliveredBody := newTestWebhookServer(t, db, user.ID, EventRestoreDrillFailed)

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

	// The wire body carries the full payload; the stored receipt is the
	// trimmed envelope (issue #622).
	var envelope struct {
		Event string `json:"event"`
		Data  struct {
			Error string `json:"error"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(*deliveredBody, &envelope))
	assert.NotEmpty(t, envelope.Data.Error)

	var storedEnvelope struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &storedEnvelope))
	assert.Equal(t, EventRestoreDrillFailed, storedEnvelope.Event)
	assert.Nil(t, storedEnvelope.Data, "the stored receipt must not retain the entity body")

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
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillhealthytester", 10)

	hits, _ := newTestWebhookServer(t, db, user.ID, EventRestoreDrillFailed)

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
	db := dbtest.NewAt(t, path)
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
	db := dbtest.NewAt(t, path)
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

// --- BACKUP-02 (issue #454): the drill also reconciles the backup set -------

// TestRunRestoreDrillPassesWithCompleteBackupSet pins that adding the
// completeness check does not false-positive on a healthy instance: every
// live attachment/photo row's file present in the (live, in this test)
// directories must still pass.
func TestRunRestoreDrillPassesWithCompleteBackupSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-set-complete.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillsetok", 1)

	photoDir := t.TempDir()
	attachmentsDir := t.TempDir()
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo) VALUES (100, 'C', ?, 'uid-set-ok', 'ok_photo.jpg')`,
		user.ID).Error)
	require.NoError(t, os.WriteFile(filepath.Join(photoDir, "ok_photo.jpg"), []byte("x"), 0o600))
	require.NoError(t, db.Exec(
		`INSERT INTO attachments (user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes)
		 VALUES (?, 'uid-set-ok', 'ok-file', 'd.pdf', 'application/pdf', 1)`, user.ID).Error)
	require.NoError(t, os.WriteFile(filepath.Join(attachmentsDir, "ok-file"), []byte("x"), 0o600))

	cfg := config.Config{DBPath: path, ProfilePhotoDir: photoDir, AttachmentsDir: attachmentsDir}
	ok, detail, err := runRestoreDrill(db, cfg)
	require.NoError(t, err)
	assert.True(t, ok, "detail: %s", detail)
}

// TestRunRestoreDrillFailsOnMissingAttachmentFile is the hand-verified
// opposite: a live attachment row whose file is absent from the (live)
// attachments directory — exactly what an operator's backup would miss if the
// directory copy was skipped or partial — must fail the drill and name the
// file, through the same channel a row-count mismatch uses.
func TestRunRestoreDrillFailsOnMissingAttachmentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-set-missing.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillsetmiss", 1)

	photoDir := t.TempDir()
	attachmentsDir := t.TempDir()
	require.NoError(t, db.Exec(
		`INSERT INTO attachments (user_id, contact_vcard_uid, stored_name, original_name, content_type, size_bytes)
		 VALUES (?, 'uid-x', 'missing-file', 'd.pdf', 'application/pdf', 1)`, user.ID).Error)
	// deliberately no file written for "missing-file"

	cfg := config.Config{DBPath: path, ProfilePhotoDir: photoDir, AttachmentsDir: attachmentsDir}
	ok, detail, err := runRestoreDrill(db, cfg)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, detail, "backup set incomplete")
	assert.Contains(t, detail, "missing-file")
}

// TestRunRestoreDrillFailsOnMissingPhotoFile is the photo-side twin of
// TestRunRestoreDrillFailsOnMissingAttachmentFile — a missing profile-photo
// file must also fail the drill and be named, not just a missing attachment.
func TestRunRestoreDrillFailsOnMissingPhotoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-set-missing-photo.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	user := seedNotes(t, db, "drillsetmissphoto", 1)

	photoDir := t.TempDir()
	attachmentsDir := t.TempDir()
	require.NoError(t, db.Exec(
		`INSERT INTO contacts (id, firstname, user_id, vcard_uid, photo) VALUES (200, 'C', ?, 'uid-photo-gone', 'gone_photo.jpg')`,
		user.ID).Error)
	// deliberately no file written for "gone_photo.jpg"

	cfg := config.Config{DBPath: path, ProfilePhotoDir: photoDir, AttachmentsDir: attachmentsDir}
	ok, detail, err := runRestoreDrill(db, cfg)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Contains(t, detail, "backup set incomplete")
	assert.Contains(t, detail, "gone_photo.jpg")
}

// TestRunRestoreDrillPassesWithOrphanBackupFile pins that an orphan file — no
// owning row — is informational and never fails the drill: a file whose row
// hasn't landed in the snapshot yet is a routine race against a live
// directory, not a backup defect.
func TestRunRestoreDrillPassesWithOrphanBackupFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drill-set-orphan.db")
	db := dbtest.NewAt(t, path)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	seedNotes(t, db, "drillsetorphan", 1)

	photoDir := t.TempDir()
	attachmentsDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(attachmentsDir, "orphan-file"), []byte("x"), 0o600))

	cfg := config.Config{DBPath: path, ProfilePhotoDir: photoDir, AttachmentsDir: attachmentsDir}
	ok, detail, err := runRestoreDrill(db, cfg)
	require.NoError(t, err)
	assert.True(t, ok, "detail: %s", detail)
}

// TestCheckBackupSetCompletenessSkippedWhenDirsUnset pins the guard that
// keeps every dirs-unset test above (config.Config{DBPath: path}) green: with
// no photo/attachment directories configured there is nothing to reconcile,
// so the check passes without even touching the database path.
func TestCheckBackupSetCompletenessSkippedWhenDirsUnset(t *testing.T) {
	ok, detail, err := checkBackupSetCompleteness("/does/not/exist.db", config.Config{})
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, detail)
}

// TestCheckBackupSetCompletenessPropagatesVerifyError pins that a failure in
// database.VerifyBackupSet itself (not a missing file it found, but the check
// being unable to run at all) surfaces as a hard error from the drill, the
// same way runRestoreDrill's other pre-flight checks do — never silently
// treated as "complete".
func TestCheckBackupSetCompletenessPropagatesVerifyError(t *testing.T) {
	_, _, err := checkBackupSetCompleteness(filepath.Join(t.TempDir(), "no-such-snapshot.db"), config.Config{
		ProfilePhotoDir: t.TempDir(),
		AttachmentsDir:  t.TempDir(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify backup set completeness")
}
