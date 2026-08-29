package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupApiTokenRouter wires the API-token handlers against a real migrated
// schema, with a seeded user whose ID is in the request context.
func setupApiTokenRouter(t *testing.T) (*gin.Engine, models.User) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "tokuser", Password: "password123!A", Email: "tokuser@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.GET("/api-tokens", ListApiTokens)
	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)
	router.POST("/api-tokens/revoke-all", RevokeAllApiTokens)
	router.DELETE("/api-tokens/:id", RevokeApiToken)
	router.POST("/api-tokens/:id/rotate", RotateApiToken)
	return router, user
}

func TestListApiTokens_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest(http.MethodGet, "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestListApiTokens_DatabaseError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "tokdberr", Password: "password123!A", Email: "tokdberr@example.com"}
	require.NoError(t, db.Create(&user).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest(http.MethodGet, "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestCreateApiToken_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens", bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestCreateApiToken_InvalidScopeRejectedByRealMiddleware(t *testing.T) {
	router, _ := setupApiTokenRouter(t)
	req, _ := http.NewRequest(http.MethodPost, "/api-tokens", bytes.NewBufferString(`{"name":"x","scope":"bogus"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCreateApiToken_DatabaseError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "tokcrtdberr", Password: "password123!A", Email: "tokcrtdberr@example.com"}
	require.NoError(t, db.Create(&user).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens", bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestRevokeApiToken_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.DELETE("/api-tokens/:id", RevokeApiToken)

	req, _ := http.NewRequest(http.MethodDelete, "/api-tokens/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestRevokeApiToken_NonNumericID(t *testing.T) {
	router, _ := setupApiTokenRouter(t)
	req, _ := http.NewRequest(http.MethodDelete, "/api-tokens/not-a-number", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestRevokeAllApiTokens_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.POST("/api-tokens/revoke-all", RevokeAllApiTokens)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens/revoke-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestRevokeAllApiTokens_DatabaseError(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "tokrevokedberr", Password: "password123!A", Email: "tokrevokedberr@example.com"}
	require.NoError(t, db.Create(&user).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/api-tokens/revoke-all", RevokeAllApiTokens)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens/revoke-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestRotateApiToken_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() }) // no userID
	router.POST("/api-tokens/:id/rotate", RotateApiToken)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens/1/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

// TestCreateApiToken_PersistsHashNotPlaintext pins the security core of
// CreateApiToken against the real schema: the response carries the plaintext
// exactly once, and the database stores only the SHA-256 hash — a later read
// of the row can never recover the token.
func TestCreateApiToken_PersistsHashNotPlaintext(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "tokhash", Password: "password123!A", Email: "tokhash@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens", bytes.NewBufferString(`{"name":"cli","expires_in_days":30,"scope":"carddav"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		Token string `json:"token"`
		Scope string `json:"scope"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Token, "the plaintext must be shown in the create response")
	require.Equal(t, "carddav", resp.Scope, "an explicit scope must override the default")

	var stored models.ApiToken
	require.NoError(t, db.First(&stored).Error)
	require.NotEqual(t, resp.Token, stored.TokenHash, "the stored value must be the hash, not the plaintext")
	require.NotContains(t, resp.Token, stored.TokenHash, "the hash must never leak into the plaintext")

	var revokeAllCount int64
	require.NoError(t, db.Model(&models.ApiToken{}).Where("user_id = ? AND revoked_at IS NOT NULL", user.ID).Count(&revokeAllCount).Error)
	assert.Zero(t, revokeAllCount)
}

func TestRevokeAllApiTokens_RevokesEveryActiveToken(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "tokrevokeall", Password: "password123!A", Email: "tokrevokeall@example.com"}
	require.NoError(t, db.Create(&user).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.ApiToken{
			UserID: user.ID, Name: "t" + strconv.Itoa(i), TokenHash: "h" + strconv.Itoa(i),
		}).Error)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Set("userID", user.ID); c.Next() })
	router.POST("/api-tokens/revoke-all", RevokeAllApiTokens)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens/revoke-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Revoked int64 `json:"revoked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 3, body.Revoked)

	var active int64
	require.NoError(t, db.Model(&models.ApiToken{}).Where("user_id = ? AND revoked_at IS NULL", user.ID).Count(&active).Error)
	assert.Zero(t, active, "every active token must be revoked")
}
