package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validDiagnosticsConfig returns a config that passes config.Validate() and
// points every storage dir at a real, writable temp dir — the healthy baseline
// for the sweep.
func validDiagnosticsConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		JWTSecretKey:                "diagnostics-test-secret-key-that-is-long-enough-for-validation",
		JWTExpiryHours:              96,
		DBPath:                      filepath.Join(t.TempDir(), "myco.db"),
		ProfilePhotoDir:             t.TempDir(),
		AttachmentsDir:              t.TempDir(),
		Port:                        "8080",
		ReminderTime:                "06:00",
		ReminderTimezone:            "UTC",
		FrontendURL:                 "http://localhost:5173",
		ReadTimeout:                 15,
		WriteTimeout:                15,
		IdleTimeout:                 60,
		DBRestoreDrillEnabled:       true,
		DBRestoreDrillIntervalHours: 168,
	}
}

func findCheck(t *testing.T, d Diagnostics, name string) DiagnosticCheck {
	t.Helper()
	for _, c := range d.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no check named %q in diagnostics output (%d checks)", name, len(d.Checks))
	return DiagnosticCheck{}
}

// TestRunDiagnosticsHealthyInstall: a well-configured install with a migrated
// database and a fresh restore-drill result reports all-ok with an exact
// summary — the issue's "healthy install reports all-green with no errors".
func TestRunDiagnosticsHealthyInstall(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	require.NoError(t, db.Create(&models.OperationalCheckResult{
		CheckName: models.JobNameRestoreDrill, Status: models.OpCheckStatusOK, CheckedAt: time.Now(),
	}).Error)

	d := RunDiagnostics(context.Background(), db, cfg)

	assert.Equal(t, "ok", d.Summary.Status)
	assert.Zero(t, d.Summary.Errors)
	assert.Zero(t, d.Summary.Warnings)
	// 5 config/db/migrations/filesystem/backup + 1 data_integrity + 4
	// notifications + 6 integrations + 4 disk/background-jobs/search-index/
	// version = 20 rows, all ok.
	assert.Equal(t, 20, d.Summary.OK)
	assert.Len(t, d.Checks, 20)
	assert.Equal(t, DiagStatusOK, findCheck(t, d, "data_integrity").Status)

	for _, c := range d.Checks {
		assert.Equal(t, DiagStatusOK, c.Status, "check %q should be ok", c.Name)
		assert.NotEmpty(t, c.Message, "check %q should carry a message", c.Name)
	}
}

// TestRunDiagnosticsConfigProblems: a broken config (here a missing
// JWT_SECRET_KEY) is an error row naming the failing variables — never their
// values — and drives the summary to error.
func TestRunDiagnosticsConfigProblems(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	cfg.JWTSecretKey = ""

	d := RunDiagnostics(context.Background(), db, cfg)

	c := findCheck(t, d, "config")
	assert.Equal(t, DiagStatusError, c.Status)
	assert.Contains(t, c.Message, "JWT_SECRET_KEY")
	assert.Equal(t, "error", d.Summary.Status)
}

// TestRunDiagnosticsNoSecretLeak: even when config is broken, the response
// never echoes the configured value — only the variable name.
func TestRunDiagnosticsNoSecretLeak(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	cfg.JWTSecretKey = ""

	raw, err := json.Marshal(RunDiagnostics(context.Background(), db, cfg))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "diagnostics-test-secret-key")
}

// TestRunDiagnosticsFilesystemErrors: a storage directory that is missing or
// not a directory (the deterministic stand-ins for the issue's "chmod 000"
// scenario) makes the filesystem check an error and the sweep red.
func TestRunDiagnosticsFilesystemErrors(t *testing.T) {
	db := dbtest.New(t)

	t.Run("missing attachment directory is an error", func(t *testing.T) {
		cfg := validDiagnosticsConfig(t)
		cfg.AttachmentsDir = filepath.Join(t.TempDir(), "missing-attachments")
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "filesystem")
		assert.Equal(t, DiagStatusError, c.Status)
		assert.Contains(t, c.Message, "attachments directory")
		assert.Equal(t, "error", d.Summary.Status)
	})

	t.Run("attachment path that is a file is an error", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "not-a-dir")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
		cfg := validDiagnosticsConfig(t)
		cfg.AttachmentsDir = file
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "filesystem")
		assert.Equal(t, DiagStatusError, c.Status)
		assert.Contains(t, c.Message, "is not a directory")
	})

	t.Run("unreadable FCM service account is a warning", func(t *testing.T) {
		cfg := validDiagnosticsConfig(t)
		cfg.FCMServiceAccountFile = filepath.Join(t.TempDir(), "missing-sa.json")
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "filesystem")
		assert.Equal(t, DiagStatusWarning, c.Status)
		assert.Contains(t, c.Message, "FCM service-account file")
	})
}

func TestProbeWritableDir(t *testing.T) {
	assert.Equal(t, "", ProbeWritableDir(t.TempDir()))
	assert.Equal(t, "is missing", ProbeWritableDir(filepath.Join(t.TempDir(), "nope")))

	f := filepath.Join(t.TempDir(), "regular-file")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	assert.Equal(t, "is not a directory", ProbeWritableDir(f))
}

// TestRunDiagnosticsBackup: the backup row folds the persisted restore-drill
// result — a disabled drill is ok, a failed drill is a warning, and a stale
// fresh-enough result is ok.
func TestRunDiagnosticsBackup(t *testing.T) {
	db := dbtest.New(t)

	t.Run("disabled drill reports ok", func(t *testing.T) {
		cfg := validDiagnosticsConfig(t)
		cfg.DBRestoreDrillEnabled = false
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "backup")
		assert.Equal(t, DiagStatusOK, c.Status)
		assert.Contains(t, c.Message, "disabled")
	})

	t.Run("enabled drill with a passing result is ok", func(t *testing.T) {
		cfg := validDiagnosticsConfig(t)
		require.NoError(t, db.Save(&models.OperationalCheckResult{
			CheckName: models.JobNameRestoreDrill, Status: models.OpCheckStatusOK, CheckedAt: time.Now(),
		}).Error)
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "backup")
		assert.Equal(t, DiagStatusOK, c.Status)
		assert.Contains(t, c.Message, "passed")
	})

	t.Run("enabled drill with a failing result is a warning", func(t *testing.T) {
		cfg := validDiagnosticsConfig(t)
		require.NoError(t, db.Save(&models.OperationalCheckResult{
			CheckName: models.JobNameRestoreDrill, Status: models.OpCheckStatusFailed,
			Detail: "some table: live=1 restored=0", CheckedAt: time.Now(),
		}).Error)
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "backup")
		assert.Equal(t, DiagStatusWarning, c.Status)
		assert.NotEmpty(t, c.Message)
	})
}

// TestRunDiagnosticsNotificationFailing: a configured channel whose last
// delivery failed is an error row with the (capped) reason, and the sweep goes
// red — mirroring the notification_health surface the check reuses.
func TestRunDiagnosticsNotificationFailing(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	cfg.UseSMTP = true

	user := models.User{Username: "diag-notif", Email: "diag-notif@example.com", Password: "password123", NotifyGotify: true}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.NotificationConfig{
		UserID: user.ID, GotifyURL: "https://gotify.example.com", GotifyTokenEncrypted: "encrypted",
	}).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Diag", Lastname: "Notify"}
	require.NoError(t, db.Create(&contact).Error)
	byMail := false
	reminder := models.Reminder{
		UserID: user.ID, ContactID: &contact.ID, Message: "m", ByMail: &byMail,
		RemindAt: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), Recurrence: "once",
	}
	require.NoError(t, db.Create(&reminder).Error)
	reason := "HTTP 401"
	require.NoError(t, db.Create(&models.NotificationDelivery{
		ReminderID: reminder.ID, Channel: "gotify", Status: "failed", Error: &reason,
	}).Error)

	d := RunDiagnostics(context.Background(), db, cfg)

	gotify := findCheck(t, d, "notification_gotify")
	assert.Equal(t, DiagStatusError, gotify.Status)
	assert.Contains(t, gotify.Message, "failed")
	email := findCheck(t, d, "notification_email")
	assert.Equal(t, DiagStatusOK, email.Status, "a configured-but-unused email channel is healthy, not failing")
	assert.Equal(t, "error", d.Summary.Status)
}

// TestRunDiagnosticsPushNoDevices: push provisioned but no subscription is a
// warning, not an error — the no_devices remedy is distinct from a failure.
func TestRunDiagnosticsPushNoDevices(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	require.NoError(t, db.Create(&models.ServerSetting{Key: "vapid_public_key", Value: "pub"}).Error)
	require.NoError(t, db.Create(&models.ServerSetting{Key: "vapid_private_key", Value: "priv"}).Error)

	d := RunDiagnostics(context.Background(), db, cfg)
	push := findCheck(t, d, "notification_push")
	assert.Equal(t, DiagStatusWarning, push.Status)
	assert.Equal(t, "warning", d.Summary.Status)
}

// TestRunDiagnosticsFailingJob: a scheduled job whose last executed run failed
// folds into a background_jobs warning naming the job.
func TestRunDiagnosticsFailingJob(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	now := time.Now()
	require.NoError(t, db.Create(&models.JobRun{
		JobName: models.JobNameDailyReminders, Trigger: models.JobTriggerScheduled,
		StartedAt: now, FinishedAt: now, DurationMS: 5, Result: models.JobRunResultFailure, Error: "boom",
	}).Error)

	d := RunDiagnostics(context.Background(), db, cfg)
	c := findCheck(t, d, "background_jobs")
	assert.Equal(t, DiagStatusWarning, c.Status)
	assert.Contains(t, c.Message, models.JobNameDailyReminders)
	assert.Equal(t, "warning", d.Summary.Status)
}

// TestRunDiagnosticsSearchIndexDivergence: a desynchronised FTS index folds
// into a search_index warning that points at the rebuild endpoint (SEARCH-02,
// issue #462).
func TestRunDiagnosticsSearchIndexDivergence(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	u := models.User{Username: "diag-fts", Password: "password123!A", Email: "diag-fts@example.com"}
	require.NoError(t, db.Create(&u).Error)
	c := models.Contact{UserID: u.ID, Firstname: "Diag", Lastname: "Fts"}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Exec(`DELETE FROM contacts_fts WHERE rowid = ?`, c.ID).Error)

	d := RunDiagnostics(context.Background(), db, cfg)
	check := findCheck(t, d, "search_index")
	assert.Equal(t, DiagStatusWarning, check.Status)
	assert.Contains(t, check.Message, "rebuild")
	assert.Equal(t, "warning", d.Summary.Status)
}

// TestRunDiagnosticsSearchIndexCheckError: if the consistency check itself
// cannot run, the sweep degrades to a warning rather than failing.
func TestRunDiagnosticsSearchIndexCheckError(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	require.NoError(t, db.Exec(`DROP TABLE contacts_fts`).Error)

	d := RunDiagnostics(context.Background(), db, cfg)
	check := findCheck(t, d, "search_index")
	assert.Equal(t, DiagStatusWarning, check.Status)
	assert.Contains(t, check.Message, "could not run")
}

// TestRunDiagnosticsDiskSpace: the sweep reuses the alert evaluator's statfs
// fold. Above the configured threshold is a warning; essentially full is an
// error.
func TestRunDiagnosticsDiskSpace(t *testing.T) {
	db := dbtest.New(t)
	origDisk := diskUsageFn
	t.Cleanup(func() { diskUsageFn = origDisk })

	cfg := validDiagnosticsConfig(t)
	cfg.AlertDiskUsagePercent = 80

	t.Run("below threshold is ok", func(t *testing.T) {
		diskUsageFn = func(string) (int, error) { return 40, nil }
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "disk_space")
		assert.Equal(t, DiagStatusOK, c.Status)
	})

	t.Run("at threshold is a warning", func(t *testing.T) {
		diskUsageFn = func(string) (int, error) { return 95, nil }
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "disk_space")
		assert.Equal(t, DiagStatusWarning, c.Status)
		assert.Contains(t, c.Message, "95%")
		assert.Equal(t, "warning", d.Summary.Status)
	})

	t.Run("filesystem full is an error", func(t *testing.T) {
		diskUsageFn = func(string) (int, error) { return 100, nil }
		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "disk_space")
		assert.Equal(t, DiagStatusError, c.Status)
		assert.Equal(t, "error", d.Summary.Status)
	})
}

// TestRunDiagnosticsIntegrationReachability: distinct configured integration
// base URLs are probed with the short timeout; a reachable one is ok, an
// unreachable one is a warning (an optional integration degrades, not fails).
func TestRunDiagnosticsIntegrationReachability(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer up.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	down.Close()

	user := models.User{Username: "diag-int", Email: "diag-int@example.com", Password: "password123"}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.ImmichConfig{
		UserID: user.ID, BaseURL: up.URL, APIKeyEncrypted: "enc",
	}).Error)
	require.NoError(t, db.Create(&models.PaperlessConfig{
		UserID: user.ID, BaseURL: down.URL, APITokenEncrypted: "enc",
	}).Error)

	d := RunDiagnostics(context.Background(), db, cfg)

	immich := findCheck(t, d, "integration_immich")
	assert.Equal(t, DiagStatusOK, immich.Status)
	paperless := findCheck(t, d, "integration_paperless")
	assert.Equal(t, DiagStatusWarning, paperless.Status)
	assert.Equal(t, "warning", d.Summary.Status)

	carddav := findCheck(t, d, "integration_carddav")
	assert.Equal(t, DiagStatusOK, carddav.Status)
	assert.Equal(t, "disabled", carddav.Message)

	// The base URLs themselves must never appear in the response.
	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), up.URL)
	assert.NotContains(t, string(raw), down.URL)
}

// TestRunDiagnosticsIntegrationProbeCap: beyond maxIntegrationProbes distinct
// endpoints the sweep stops probing and says exactly that — the sweep must
// stay bounded on a large instance, and must not guess about the unprobed
// remainder.
func TestRunDiagnosticsIntegrationProbeCap(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)

	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close()

	// immich_configs is one row per user (partial unique index on user_id), so
	// a distinct base URL per config means a distinct user per config.
	for i := 0; i < maxIntegrationProbes+1; i++ {
		user := models.User{
			Username: fmt.Sprintf("diag-cap-%d", i),
			Email:    fmt.Sprintf("diag-cap-%d@example.com", i),
			Password: "password123",
		}
		require.NoError(t, db.Create(&user).Error)
		require.NoError(t, db.Create(&models.ImmichConfig{
			UserID: user.ID, BaseURL: fmt.Sprintf("%s/%d", dead.URL, i), APIKeyEncrypted: "enc",
		}).Error)
	}

	d := RunDiagnostics(context.Background(), db, cfg)
	immich := findCheck(t, d, "integration_immich")
	assert.Equal(t, DiagStatusWarning, immich.Status)
	assert.Contains(t, immich.Message, "probe cap reached")
	assert.Contains(t, immich.Message, fmt.Sprintf("%d of %d", maxIntegrationProbes, maxIntegrationProbes+1))
}

// TestRunDiagnosticsSummaryPrecedence: an error dominates a warning in the
// overall summary — one broken check must not read as "all clear".
func TestRunDiagnosticsSummaryPrecedence(t *testing.T) {
	db := dbtest.New(t)
	cfg := validDiagnosticsConfig(t)
	cfg.AttachmentsDir = filepath.Join(t.TempDir(), "missing")
	cfg.AlertDiskUsagePercent = 50

	origDisk := diskUsageFn
	t.Cleanup(func() { diskUsageFn = origDisk })
	diskUsageFn = func(string) (int, error) { return 60, nil }

	d := RunDiagnostics(context.Background(), db, cfg)
	assert.Equal(t, "error", d.Summary.Status)
	assert.Equal(t, 1, d.Summary.Errors)
	assert.GreaterOrEqual(t, d.Summary.Warnings, 1)
}

// TestRunDiagnosticsNilDB: a nil database handle is reported per-check as an
// error/warning and the sweep still completes — the endpoint must degrade, not
// panic.
func TestRunDiagnosticsNilDB(t *testing.T) {
	cfg := validDiagnosticsConfig(t)
	d := RunDiagnostics(context.Background(), nil, cfg)

	dbCheck := findCheck(t, d, "database")
	assert.Equal(t, DiagStatusError, dbCheck.Status)
	assert.Equal(t, "error", d.Summary.Status)
	// A nil db must not break the remaining checks: config/database/migrations/
	// filesystem/backup (5) + data_integrity (1) + one folded notifications
	// check + carddav/caldav (2) + disk_space/background_jobs/search_index/
	// version (4) = 13 rows.
	assert.Len(t, d.Checks, 13)
	assert.Equal(t, DiagStatusWarning, findCheck(t, d, "data_integrity").Status)
}

// TestRunDiagnosticsDataIntegrity: the data-invariant pass (DB-01, issue #460)
// surfaces in the operator sweep as its own row — ok when the last recorded
// result passed, warning when it failed or is stale, ok when the check is
// disabled (a deliberate configuration).
func TestRunDiagnosticsDataIntegrity(t *testing.T) {
	t.Run("passed", func(t *testing.T) {
		db := dbtest.New(t)
		cfg := validDiagnosticsConfig(t)
		cfg.DBIntegrityCheckEnabled = true
		cfg.DBIntegrityCheckIntervalHours = 24
		require.NoError(t, db.Create(&models.OperationalCheckResult{
			CheckName: models.CheckNameDataIntegrity, Status: models.OpCheckStatusOK, CheckedAt: time.Now(),
		}).Error)

		c := findCheck(t, RunDiagnostics(context.Background(), db, cfg), "data_integrity")
		assert.Equal(t, DiagStatusOK, c.Status)
	})

	t.Run("failed is a warning", func(t *testing.T) {
		db := dbtest.New(t)
		cfg := validDiagnosticsConfig(t)
		cfg.DBIntegrityCheckEnabled = true
		cfg.DBIntegrityCheckIntervalHours = 24
		require.NoError(t, db.Create(&models.OperationalCheckResult{
			CheckName: models.CheckNameDataIntegrity, Status: models.OpCheckStatusFailed,
			Detail: "circle_member.orphaned_contact x3", CheckedAt: time.Now(),
		}).Error)

		d := RunDiagnostics(context.Background(), db, cfg)
		c := findCheck(t, d, "data_integrity")
		assert.Equal(t, DiagStatusWarning, c.Status)
		assert.Equal(t, "warning", d.Summary.Status)
		// The row.Detail can name tables — it must not reach the response body.
		raw, err := json.Marshal(d)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "circle_member.orphaned_contact")
	})

	t.Run("disabled is ok", func(t *testing.T) {
		db := dbtest.New(t)
		cfg := validDiagnosticsConfig(t) // DBIntegrityCheckEnabled stays false
		c := findCheck(t, RunDiagnostics(context.Background(), db, cfg), "data_integrity")
		assert.Equal(t, DiagStatusOK, c.Status)
		assert.Contains(t, c.Message, "disabled")
	})
}
