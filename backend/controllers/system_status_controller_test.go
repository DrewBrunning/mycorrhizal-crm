package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	t.Cleanup(func() {
		services.ResetStorageUsageCache()
		services.ResetDeepHealthCache()
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
