package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// --- liveness ---------------------------------------------------------------

func TestLiveness_AlwaysUpAndCheap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New() // deliberately NO db / cfg middleware
	r.GET("/health/live", LivenessCheck)

	start := time.Now()
	code, body := getJSON(t, r, "/health/live")
	elapsed := time.Since(start)

	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "live", body["status"])
	require.Less(t, elapsed, 100*time.Millisecond, "liveness must not do real work")
}

func TestLiveness_UpEvenWhenDatabaseIsDown(t *testing.T) {
	db, _, r := migratedHealthRouter(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	code, body := getJSON(t, r, "/health/live")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "live", body["status"])
}

// --- readiness ------------------------------------------------------------

func TestReadiness_HappyPath(t *testing.T) {
	_, _, r := migratedHealthRouter(t)

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "ready", body["status"])

	checks, _ := body["checks"].(map[string]any)
	for _, name := range []string{"database", "migrations", "filesystem"} {
		facet, _ := checks[name].(map[string]any)
		require.Equal(t, "ok", facet["status"], "facet %s", name)
	}
}

func TestReadiness_DatabaseDown(t *testing.T) {
	db, _, r := migratedHealthRouter(t)
	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Close())

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, "not_ready", body["status"])
}

func TestReadiness_DirtyMigration(t *testing.T) {
	db, _, r := migratedHealthRouter(t)
	require.NoError(t, db.Exec("UPDATE schema_migrations SET dirty = 1").Error)

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, code)
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["migrations"].(map[string]any)
	require.Equal(t, "failed", facet["status"])
	require.Contains(t, facet["reason"], "dirty")
}

func TestReadiness_PendingMigration(t *testing.T) {
	db, _, r := migratedHealthRouter(t)
	// Rewind the recorded version so applied < latest (a database that a newer
	// binary has not finished migrating).
	require.NoError(t, db.Exec("UPDATE schema_migrations SET version = version - 1").Error)

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, code)
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["migrations"].(map[string]any)
	require.Equal(t, "failed", facet["status"])
	require.Contains(t, facet["reason"], "pending migrations")
}

func TestReadiness_FilesystemUnavailable(t *testing.T) {
	_, cfg, r := migratedHealthRouter(t)
	cfg.ProfilePhotoDir = filepath.Join(t.TempDir(), "nonexistent")

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, code)
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["filesystem"].(map[string]any)
	require.Equal(t, "failed", facet["status"])
	reason, _ := facet["reason"].(string)
	require.Contains(t, reason, "missing")
	// No absolute path in the unauthenticated body.
	require.NotContains(t, reason, cfg.ProfilePhotoDir)
}

func TestReadiness_FilesystemPathIsAFile(t *testing.T) {
	_, cfg, r := migratedHealthRouter(t)
	f := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(f, []byte("x"), 0o600))
	cfg.AttachmentsDir = f

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, code)
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["filesystem"].(map[string]any)
	require.Equal(t, "failed", facet["status"])
	require.Contains(t, facet["reason"], "not a directory")
}

func TestReadiness_FilesystemDirNotWritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory mode bits")
	}
	_, cfg, r := migratedHealthRouter(t)
	ro := filepath.Join(t.TempDir(), "readonly")
	require.NoError(t, os.Mkdir(ro, 0o500))
	t.Cleanup(func() { _ = os.Chmod(ro, 0o700) })
	cfg.ProfilePhotoDir = ro

	code, body := getJSON(t, r, "/health/ready")
	require.Equal(t, http.StatusServiceUnavailable, code)
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["filesystem"].(map[string]any)
	require.Equal(t, "failed", facet["status"])
	require.Contains(t, facet["reason"], "not writable")
}

// --- deep health --------------------------------------------------------

func TestDeepHealth_DegradedWhenIntegrityCheckNeverRecorded(t *testing.T) {
	_, cfg, r := migratedHealthRouter(t)
	cfg.DBIntegrityCheckEnabled = true

	code, body := getJSON(t, r, "/health")
	require.Equal(t, http.StatusOK, code, "degraded is still 200 — degraded-but-alive is not down")
	require.Equal(t, "degraded", body["status"])

	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["integrity_check"].(map[string]any)
	require.Equal(t, "degraded", facet["status"])
	require.Contains(t, facet["reason"], "never recorded")
}

func TestDeepHealth_HealthyWithRecordedOKResults(t *testing.T) {
	db, cfg, r := migratedHealthRouter(t)
	cfg.DBIntegrityCheckEnabled = true
	cfg.DBRestoreDrillEnabled = true

	now := time.Now()
	for _, name := range []string{models.JobNameDBIntegrityCheck, models.JobNameRestoreDrill} {
		require.NoError(t, db.Create(&models.OperationalCheckResult{
			CheckName: name, Status: models.OpCheckStatusOK, CheckedAt: now,
		}).Error)
	}

	code, body := getJSON(t, r, "/health")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "healthy", body["status"])
}

func TestDeepHealth_DegradedWhenIntegrityCheckFailed(t *testing.T) {
	db, cfg, r := migratedHealthRouter(t)
	cfg.DBIntegrityCheckEnabled = true

	require.NoError(t, db.Create(&models.OperationalCheckResult{
		CheckName: models.JobNameDBIntegrityCheck,
		Status:    models.OpCheckStatusFailed,
		Detail:    "page 42 is corrupt",
		CheckedAt: time.Now(),
	}).Error)

	code, body := getJSON(t, r, "/health")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "degraded", body["status"])
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["integrity_check"].(map[string]any)
	require.Equal(t, "degraded", facet["status"])
	reason, _ := facet["reason"].(string)
	require.Contains(t, reason, "failed")
	// The stored detail (table names / schema internals) must NOT leak into
	// this unauthenticated response — it goes to the log + failure webhook.
	require.NotContains(t, reason, "page 42 is corrupt")
}

func TestDeepHealth_DegradedOnStuckJobLock(t *testing.T) {
	db, _, r := migratedHealthRouter(t)

	locked := time.Now().Add(-30 * time.Minute)
	require.NoError(t, db.Create(&models.JobExecution{
		JobName:   "calendar_sync",
		LastRunAt: locked,
		LockedAt:  &locked,
		LockedBy:  "dead-worker",
	}).Error)

	code, body := getJSON(t, r, "/health")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "degraded", body["status"])
	checks, _ := body["checks"].(map[string]any)
	facet, _ := checks["background_jobs"].(map[string]any)
	require.Equal(t, "degraded", facet["status"])
	require.Contains(t, facet["reason"], "calendar_sync")
}

func TestDeepHealth_StillUnhealthyWhenDatabaseDown(t *testing.T) {
	db, _, r := migratedHealthRouter(t)
	sqlDB, _ := db.DB()
	require.NoError(t, sqlDB.Close())

	code, body := getJSON(t, r, "/health")
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, "unhealthy", body["status"])
}

// The deep endpoint is unauthenticated, so its reason/detail strings must not
// leak internals — absolute paths, the operator's SMTP host, raw dial errors,
// or the integrity/restore-drill detail (which can carry table names + row
// counts). Detail belongs in the server log + failure webhook, not this body.
func TestDeepHealth_ResponseBodyLeaksNoInternals(t *testing.T) {
	db, cfg, r := migratedHealthRouter(t)
	cfg.DBIntegrityCheckEnabled = true
	cfg.UseSMTP = true
	cfg.SMTPHost = "sekret-internal-mailhost.corp.example"
	cfg.SMTPPort = 2525
	cfg.FCMServiceAccountFile = "/very/secret/path/fcm-service-account.json"

	require.NoError(t, db.Create(&models.OperationalCheckResult{
		CheckName: models.JobNameDBIntegrityCheck,
		Status:    models.OpCheckStatusFailed,
		Detail:    "contacts: live=1234 restored=1200; secret_table page 7 malformed",
		CheckedAt: time.Now(),
	}).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)
	raw := w.Body.String()

	for _, needle := range []string{
		"sekret-internal-mailhost.corp.example", // SMTP host
		"/very/secret/path",                     // FCM file path
		"contacts: live=1234",                   // restore-drill row counts
		"secret_table",                          // integrity-check schema hint
		"page 7 malformed",
		"dial tcp", // raw net error text
	} {
		require.NotContains(t, raw, needle, "deep /health body must not expose %q", needle)
	}
	// It should still be a useful degraded report.
	require.Contains(t, raw, "degraded")
}

// Sanity: the deep body still decodes into the typed HealthResponse (the
// pre-split shape is preserved, with checks added).
func TestDeepHealth_TypedShapeUnchanged(t *testing.T) {
	_, _, r := migratedHealthRouter(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	var resp HealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "healthy", resp.Status)
	require.Equal(t, "healthy", resp.Database.Status)
	require.NotEmpty(t, resp.Version)
}
