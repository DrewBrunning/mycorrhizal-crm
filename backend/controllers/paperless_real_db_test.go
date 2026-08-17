package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPaperlessConfig_RealMigratedSchema is the real-DB check for the
// PaperlessConfig partial unique index (migration 000025): the table
// soft-deletes AND is user-unique, so per T26 the unique index must be partial
// (WHERE deleted_at IS NULL) — otherwise removing the connection would block
// re-creating it forever. AutoMigrate cannot see this, so this runs the config
// save → delete → re-save round trip against a real database.InitDB-migrated
// file.
func TestPaperlessConfig_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "paperless-config-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "paperless-cfg-realdb", Password: "password123!A", Email: "paperless-cfg-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef"})
		c.Next()
	})
	router.PUT("/paperless/config", withValidated(func() any { return &models.PaperlessConfigInput{} }), SavePaperlessConfig)
	router.DELETE("/paperless/config", DeletePaperlessConfig)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Create → delete (soft) → re-create must all succeed on the real schema.
	create1 := do("PUT", "/paperless/config", models.PaperlessConfigInput{BaseURL: "https://paperless.example", APIToken: "token-1"})
	require.Equal(t, http.StatusOK, create1.Code, create1.Body.String())

	del := do("DELETE", "/paperless/config", nil)
	require.Equal(t, http.StatusOK, del.Code)

	create2 := do("PUT", "/paperless/config", models.PaperlessConfigInput{BaseURL: "https://paperless.example", APIToken: "token-2"})
	assert.Equal(t, http.StatusOK, create2.Code, create2.Body.String())
}
