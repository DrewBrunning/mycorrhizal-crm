package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSeafileConfig_RealMigratedSchema is the real-DB check for the
// SeafileConfig partial unique index (migration 000026): the table soft-deletes
// AND is user-unique, so per T26 the unique index must be partial
// (WHERE deleted_at IS NULL). AutoMigrate cannot see this, so this runs the
// config save → delete → re-save round trip against a real
// database.InitDB-migrated file.
func TestSeafileConfig_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "seafile-cfg-realdb", Password: "password123!A", Email: "seafile-cfg-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef"})
		c.Next()
	})
	router.PUT("/seafile/config", withValidated(func() any { return &models.SeafileConfigInput{} }), SaveSeafileConfig)
	router.DELETE("/seafile/config", DeleteSeafileConfig)

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

	create1 := do("PUT", "/seafile/config", models.SeafileConfigInput{BaseURL: "https://seafile.example", APIToken: "token-1"})
	require.Equal(t, http.StatusOK, create1.Code, create1.Body.String())

	del := do("DELETE", "/seafile/config", nil)
	require.Equal(t, http.StatusOK, del.Code)

	create2 := do("PUT", "/seafile/config", models.SeafileConfigInput{BaseURL: "https://seafile.example", APIToken: "token-2"})
	assert.Equal(t, http.StatusOK, create2.Code, create2.Body.String())
}
