package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/buildinfo"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// systemStatusEnv wires the real AdminMiddleware in front of GetSystemStatus,
// with a stub auth middleware that sets db / cfg / userID exactly the way
// main.go's context-injection middleware + AuthMiddleware do in production.
// userID == 0 means "no authenticated user" (the stub does not set it).
func systemStatusEnv(t *testing.T, db *gorm.DB, cfg config.Config, userID uint) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	services.ResetStorageUsageCache()
	services.ResetDeepHealthCache()
	services.ResetUpdateCheckCache()
	t.Cleanup(func() {
		services.ResetStorageUsageCache()
		services.ResetDeepHealthCache()
		services.ResetUpdateCheckCache()
	})

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", cfg)
		if userID != 0 {
			c.Set("userID", userID)
		}
		c.Next()
	})
	router.Use(middleware.AdminMiddleware())
	router.GET("/admin/system-status", GetSystemStatus)
	return router
}

func getSystemStatus(t *testing.T, router *gin.Engine) (*httptest.ResponseRecorder, []byte) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/system-status", nil)
	router.ServeHTTP(w, req)
	return w, w.Body.Bytes()
}

func seedAdmin(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{Username: "sys-admin", Email: "sys-admin@example.com", Password: "password123", IsAdmin: true}
	require.NoError(t, db.Create(&u).Error)
	return u
}

func validSystemStatusConfig(t *testing.T, dbPath string) config.Config {
	t.Helper()
	return config.Config{
		JWTSecretKey:     "a-sufficiently-long-and-random-testing-jwt-secret-value",
		DBPath:           dbPath,
		ProfilePhotoDir:  t.TempDir(),
		AttachmentsDir:   t.TempDir(),
		FrontendURL:      "http://localhost:5173",
		Port:             "7300",
		ReminderTime:     "09:00",
		ReminderTimezone: "UTC",
		JWTExpiryHours:   96,
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
	}
}

func TestGetSystemStatus_Unauthenticated_401(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	router := systemStatusEnv(t, db, validSystemStatusConfig(t, dbPath), 0)

	w, _ := getSystemStatus(t, router)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestGetSystemStatus_NonAdmin_403(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)

	nonAdmin := models.User{Username: "plain", Email: "plain@example.com", Password: "password123"}
	require.NoError(t, db.Create(&nonAdmin).Error)

	router := systemStatusEnv(t, db, validSystemStatusConfig(t, dbPath), nonAdmin.ID)

	w, _ := getSystemStatus(t, router)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestGetSystemStatus_MigratedDB_ReportsCleanState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)
	router := systemStatusEnv(t, db, validSystemStatusConfig(t, dbPath), admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	assert.Contains(t, []string{"healthy", "degraded", "unhealthy"}, resp.Overall)
	assert.Equal(t, resp.Overall, resp.Health.Status, "overall must be DeepHealth.Status verbatim")

	// On a DB migrated to head.
	assert.Greater(t, resp.Migration.Applied, uint(0))
	assert.Equal(t, resp.Migration.Latest, resp.Migration.Applied)
	assert.Equal(t, 0, resp.Migration.Pending)
	assert.False(t, resp.Migration.Dirty)

	assert.NotEmpty(t, resp.Version.Version)
	assert.NotEmpty(t, resp.Uptime.StartedAt)
	assert.GreaterOrEqual(t, resp.Uptime.UptimeSeconds, int64(0))

	assert.NotEmpty(t, resp.Database.SQLiteVersion)
	assert.Equal(t, "wal", strings.ToLower(resp.Database.JournalMode))

	assert.Greater(t, resp.Storage.DatabaseBytes, int64(0))
	assert.Greater(t, resp.Storage.Filesystem.TotalBytes, int64(0))
	assert.GreaterOrEqual(t, resp.Storage.Filesystem.FreeBytes, int64(0))
}

func TestGetSystemStatus_CleanConfig_ValidationMarshalsAsEmptyArray_NoSecrets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	cfg.MetricsToken = "0123456789abcdef-metrics-token-value"
	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Asserted on the RAW bytes: decoding into the struct makes "absent" and
	// "[]" indistinguishable (CLAUDE.md frontend trap #8).
	raw := string(body)
	assert.Contains(t, raw, `"validation":[]`)
	assert.Contains(t, raw, `"directories":[`)

	// No secret value anywhere in the body.
	assert.NotContains(t, raw, cfg.JWTSecretKey)
	assert.NotContains(t, raw, cfg.MetricsToken)

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.NotNil(t, resp.Config.Validation)
	assert.Empty(t, resp.Config.Validation)
	// The metrics feature flag is on (token set) but the token itself is not leaked.
	assert.True(t, resp.Config.Features.Metrics)
}

func TestGetSystemStatus_ReportsNonFatalConfigValidationError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	// A too-short JWT secret: cfg.Validate() reports it, but the process is
	// already running, so the endpoint should surface it rather than 500.
	cfg.JWTSecretKey = "too-short"
	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	require.NotEmpty(t, resp.Config.Validation, "a bad config value must show up in config.validation, not an empty list")
	var fields []string
	for _, v := range resp.Config.Validation {
		fields = append(fields, v.Field)
		assert.NotEmpty(t, v.Message)
	}
	assert.Contains(t, fields, "JWT_SECRET_KEY")

	// Even the message must not echo the offending value.
	assert.NotContains(t, string(body), "too-short")
}

func TestGetSystemStatus_StorageDirectoriesMatchDiskUsage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)

	// Known fixture tree under the profile-photo dir.
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ProfilePhotoDir, "one.jpg"), make([]byte, 1024), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(cfg.ProfilePhotoDir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ProfilePhotoDir, "nested", "two.jpg"), make([]byte, 2048), 0o644))

	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))

	require.Len(t, resp.Storage.Directories, 2) // ProfilePhotoDir + AttachmentsDir
	var photo *services.DirectoryUsage
	for i := range resp.Storage.Directories {
		if resp.Storage.Directories[i].Path == cfg.ProfilePhotoDir {
			photo = &resp.Storage.Directories[i]
		}
	}
	require.NotNil(t, photo)
	assert.EqualValues(t, 3072, photo.Bytes)
	assert.Equal(t, 2, photo.FileCount)
	assert.False(t, photo.Truncated)
}

func TestGetSystemStatus_SecondRequestServedFromCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ProfilePhotoDir, "a.jpg"), make([]byte, 500), 0o644))

	router := systemStatusEnv(t, db, cfg, admin.ID)

	_, body1 := getSystemStatus(t, router)
	var r1 SystemStatusResponse
	require.NoError(t, json.Unmarshal(body1, &r1))

	// Grow the tree; within the 5-minute cache TTL the walk must not re-run.
	require.NoError(t, os.WriteFile(filepath.Join(cfg.ProfilePhotoDir, "b.jpg"), make([]byte, 5000), 0o644))

	_, body2 := getSystemStatus(t, router)
	var r2 SystemStatusResponse
	require.NoError(t, json.Unmarshal(body2, &r2))

	findPhoto := func(r SystemStatusResponse) services.DirectoryUsage {
		for _, d := range r.Storage.Directories {
			if d.Path == cfg.ProfilePhotoDir {
				return d
			}
		}
		t.Fatalf("profile photo dir missing from storage.directories")
		return services.DirectoryUsage{}
	}
	assert.Equal(t, findPhoto(r1).Bytes, findPhoto(r2).Bytes,
		"a second request inside the cache TTL must not re-walk the filesystem")
}

// recordingRoundTripper counts RoundTrip calls and always fails, so a test can
// assert that a disabled update check never touches the network.
type recordingRoundTripper struct {
	calls int
}

func (r *recordingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return nil, errors.New("recordingRoundTripper: unexpected outbound call")
}

// setBuildVersionForTest overrides the link-time build identity for the
// duration of a test (issue #650's version comparison feeds off it).
func setBuildVersionForTest(t *testing.T, v string) {
	t.Helper()
	orig := buildinfo.Version
	buildinfo.Version = v
	t.Cleanup(func() { buildinfo.Version = orig })
}

// TestGetSystemStatus_UpdateCheckDisabledByDefault_NoOutboundCall pins the
// default posture: UPDATE_CHECK_ENABLED unset means update.enabled == false
// and the endpoint makes NO outbound request (asserted with an injected
// transport that records calls).
func TestGetSystemStatus_UpdateCheckDisabledByDefault_NoOutboundCall(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	rt := &recordingRoundTripper{}
	restore := services.SetUpdateCheckForTest("http://example.invalid", &http.Client{Transport: rt})
	defer restore()

	cfg := validSystemStatusConfig(t, dbPath) // UpdateCheckEnabled zero-value: false
	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.False(t, resp.Update.Enabled)
	assert.Empty(t, resp.Update.Current)
	assert.Empty(t, resp.Update.Latest)
	assert.False(t, resp.Update.UpdateAvailable)
	assert.Nil(t, resp.Update.CheckedAt)
	assert.Zero(t, rt.calls, "a disabled update check must not dial out")
}

// TestGetSystemStatus_UpdateCheckEnabled_ReportsUpdateAvailable pins the happy
// path: flag on, GitHub API (stubbed) reports a newer tag -> update_available
// true, latest set, checked_at populated.
func TestGetSystemStatus_UpdateCheckEnabled_ReportsUpdateAvailable(t *testing.T) {
	setBuildVersionForTest(t, "v0.6.2")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"tag_name":"v9.9.9"}`)
	}))
	defer srv.Close()
	restore := services.SetUpdateCheckForTest(srv.URL, srv.Client())
	defer restore()

	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	cfg.UpdateCheckEnabled = true
	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.True(t, resp.Update.Enabled)
	assert.Equal(t, "v0.6.2", resp.Update.Current)
	assert.Equal(t, "v9.9.9", resp.Update.Latest)
	assert.True(t, resp.Update.UpdateAvailable)
	require.NotNil(t, resp.Update.CheckedAt)
}

// TestGetSystemStatus_UpdateCheckEnabled_StubErrorIsUnknown pins the fail-soft
// posture: a 500 from the releases API -> latest "unknown" (empty),
// update_available false, and the endpoint still returns 200.
func TestGetSystemStatus_UpdateCheckEnabled_StubErrorIsUnknown(t *testing.T) {
	setBuildVersionForTest(t, "v0.6.2")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	restore := services.SetUpdateCheckForTest(srv.URL, srv.Client())
	defer restore()

	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	cfg.UpdateCheckEnabled = true
	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.True(t, resp.Update.Enabled)
	assert.Equal(t, "v0.6.2", resp.Update.Current)
	assert.Empty(t, resp.Update.Latest)
	assert.False(t, resp.Update.UpdateAvailable)
	assert.Nil(t, resp.Update.CheckedAt)
}

// TestGetSystemStatus_UpdateCheckEnabled_DevBuildNeverAvailable pins that a
// non-release running version ("dev") is never "behind", even when the stub
// reports a newer tag — latest is still reported.
func TestGetSystemStatus_UpdateCheckEnabled_DevBuildNeverAvailable(t *testing.T) {
	setBuildVersionForTest(t, "dev")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"tag_name":"v9.9.9"}`)
	}))
	defer srv.Close()
	restore := services.SetUpdateCheckForTest(srv.URL, srv.Client())
	defer restore()

	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	cfg.UpdateCheckEnabled = true
	router := systemStatusEnv(t, db, cfg, admin.ID)

	w, body := getSystemStatus(t, router)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp SystemStatusResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	assert.True(t, resp.Update.Enabled)
	assert.Equal(t, "dev", resp.Update.Current)
	assert.Equal(t, "v9.9.9", resp.Update.Latest, "latest is still reported")
	assert.False(t, resp.Update.UpdateAvailable)
}

// TestGetSystemStatus_UpdateCheckDisabled_BlockStillMarshals ensures the
// update block is present on the wire (not omitted) even when disabled, so the
// frontend's optional access stays well-defined.
func TestGetSystemStatus_UpdateCheckDisabled_BlockStillMarshals(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "x.db")
	db := dbtest.NewAt(t, dbPath)
	admin := seedAdmin(t, db)

	cfg := validSystemStatusConfig(t, dbPath)
	router := systemStatusEnv(t, db, cfg, admin.ID)

	_, body := getSystemStatus(t, router)

	raw := string(body)
	assert.Contains(t, raw, `"update":{"enabled":false}`)
}
