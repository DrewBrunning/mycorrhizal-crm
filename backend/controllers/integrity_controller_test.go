package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func integrityEndpointConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		JWTSecretKey:     "integrity-controller-secret-key-that-is-long-enough",
		JWTExpiryHours:   96,
		DBPath:           filepath.Join(t.TempDir(), "myco.db"),
		ProfilePhotoDir:  t.TempDir(),
		AttachmentsDir:   t.TempDir(),
		Port:             "8080",
		ReminderTime:     "06:00",
		ReminderTimezone: "UTC",
		FrontendURL:      "http://localhost:5173",
		ReadTimeout:      15,
		WriteTimeout:     15,
		IdleTimeout:      60,
	}
}

func integrityRouter(t *testing.T, db *gorm.DB) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", integrityEndpointConfig(t))
		c.Next()
	})
	r.GET("/admin/integrity-check", RunIntegrityCheck)
	return r
}

func getIntegrityCheck(t *testing.T, r *gin.Engine) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/integrity-check", nil)
	r.ServeHTTP(w, req)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return w.Code, body
}

func TestIntegrityCheckEndpoint_HealthyDB(t *testing.T) {
	db := dbtest.New(t)
	code, body := getIntegrityCheck(t, integrityRouter(t, db))

	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, true, body["ok"])

	storage := body["storage"].(map[string]any)
	assert.Equal(t, true, storage["ok"])
	assert.Equal(t, "ok", storage["integrity_check"])
	assert.Equal(t, "ok", storage["foreign_key_check"])

	data := body["data"].(map[string]any)
	assert.Equal(t, true, data["ok"])
}

func TestIntegrityCheckEndpoint_ReportsDataViolation(t *testing.T) {
	db := dbtest.New(t)
	u := models.User{Username: "alice", Email: "a@example.com", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	c := models.Contact{UserID: u.ID, Firstname: "A"}
	require.NoError(t, db.Create(&c).Error)
	// Edge whose target contact does not exist.
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: u.ID, SourceID: c.VCardUID, TargetID: "00000000-0000-4000-8000-000000000000",
		Type: "friend_of", Source: models.RelationshipSourceUserConfirmed, Confidence: 1,
		Status: models.RelationshipStatusConfirmed, Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	code, body := getIntegrityCheck(t, integrityRouter(t, db))
	require.Equal(t, http.StatusOK, code, "a broken database still returns 200 — the diagnosis is the payload")
	assert.Equal(t, false, body["ok"])

	data := body["data"].(map[string]any)
	assert.Equal(t, false, data["ok"])
	findings := data["findings"].([]any)
	require.NotEmpty(t, findings)
	first := findings[0].(map[string]any)
	assert.Equal(t, "INV-D1", first["invariant"])
	assert.Equal(t, "relationship_edge.endpoint_missing", first["check"])
}

func TestIntegrityCheckEndpoint_NoSecretLeak(t *testing.T) {
	db := dbtest.New(t)
	// A file path / stored name must never reach the body — only counts.
	u := models.User{Username: "alice", Email: "a@example.com", Password: "x"}
	require.NoError(t, db.Create(&u).Error)
	c := models.Contact{UserID: u.ID, Firstname: "A"}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Create(&models.Attachment{
		UserID: u.ID, ContactVCardUID: c.VCardUID, StoredName: "secret-stored-name-uuid",
		OriginalName: "私物.pdf", ContentType: "application/pdf", SizeBytes: 1,
	}).Error)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/integrity-check", nil)
	integrityRouter(t, db).ServeHTTP(w, req)
	assert.NotContains(t, w.Body.String(), "secret-stored-name-uuid")
	assert.NotContains(t, w.Body.String(), "私物")
}

func TestIntegrityCheckEndpoint_NilDB(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("cfg", integrityEndpointConfig(t))
		c.Next()
	})
	r.GET("/admin/integrity-check", RunIntegrityCheck)

	code, body := getIntegrityCheck(t, r)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, false, body["ok"])
}

// Guard: the response type stays in sync with the service report shape.
func TestIntegrityCheckEndpoint_ResponseDecodesToTypedReport(t *testing.T) {
	db := dbtest.New(t)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/admin/integrity-check", nil)
	integrityRouter(t, db).ServeHTTP(w, req)

	var typed struct {
		OK      bool                            `json:"ok"`
		Storage services.StorageIntegrityReport `json:"storage"`
		Data    services.DataIntegrityReport    `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &typed))
	assert.True(t, typed.OK)
	assert.True(t, typed.Data.OK)
}
