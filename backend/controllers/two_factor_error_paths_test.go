package controllers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupBare2FARouter wires the 2FA-management handlers directly (no
// AuthMiddleware), so a test can control whether userID is present.
func setupBare2FARouter(db *gorm.DB, withUserID bool, cfg config.Config) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		if withUserID {
			c.Set("userID", uint(1))
		}
		c.Set("cfg", cfg)
		c.Next()
	})
	router.GET("/status", GetTwoFactorStatus)
	router.POST("/setup", SetupTwoFactor)
	router.POST("/confirm", ConfirmTwoFactor)
	router.POST("/disable", DisableTwoFactor)
	router.POST("/regenerate", RegenerateRecoveryCodes)
	return router
}

func closedDB2FA(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	return db
}

// seed2FAUser creates a user under the given state, returning it plus a router
// whose context carries that user's ID.
func seed2FAUser(t *testing.T, db *gorm.DB, enabled bool, secret *string) (models.User, *gin.Engine) {
	t.Helper()
	name := "tf_" + strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(t.Name(), "Test"), "/", "_"))
	if len(name) > 50 {
		name = name[:50]
	}
	user := models.User{
		Username: name, Email: name + "@example.com", Password: strongPassword,
		TOTPEnabled: enabled, TOTPSecretEncrypted: secret,
	}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.POST("/confirm", ConfirmTwoFactor)
	router.POST("/disable", DisableTwoFactor)
	router.POST("/regenerate", RegenerateRecoveryCodes)
	return user, router
}

// --- 401 branches: handler-level currentUserID failures ---

func TestTwoFactorHandlers_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	router := setupBare2FARouter(db, false, config.Config{})
	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/status"},
		{http.MethodPost, "/setup"},
		{http.MethodPost, "/confirm"},
		{http.MethodPost, "/disable"},
		{http.MethodPost, "/regenerate"},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "%s %s", tc.method, tc.path)
	}
}

// --- DB-error branches (closed connection) ---

func TestTwoFactorHandlers_DatabaseError(t *testing.T) {
	router := setupBare2FARouter(closedDB2FA(t), true, config.Config{})
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/status", ""},
		{http.MethodPost, "/setup", ""},
		{http.MethodPost, "/confirm", `{"code":"000000"}`},
		{http.MethodPost, "/disable", `{"code":"000000"}`},
		{http.MethodPost, "/regenerate", `{"code":"000000"}`},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
	}
}

// --- state-guard branches ---

func TestConfirmTwoFactor_NoPendingSecret(t *testing.T) {
	db := dbtest.New(t)
	_, router := seed2FAUser(t, db, false, nil)

	req, _ := http.NewRequest(http.MethodPost, "/confirm", bytes.NewBufferString(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code, "confirming without a pending secret must 409")
}

func TestConfirmTwoFactor_MissingCode(t *testing.T) {
	db := dbtest.New(t)
	secret := "somesampleencryptedsecret"
	_, router := seed2FAUser(t, db, false, &secret)

	req, _ := http.NewRequest(http.MethodPost, "/confirm", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestDisableTwoFactor_NotEnabled(t *testing.T) {
	db := dbtest.New(t)
	_, router := seed2FAUser(t, db, false, nil)

	req, _ := http.NewRequest(http.MethodPost, "/disable", bytes.NewBufferString(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestRegenerateRecoveryCodes_NotEnabled(t *testing.T) {
	db := dbtest.New(t)
	_, router := seed2FAUser(t, db, false, nil)

	req, _ := http.NewRequest(http.MethodPost, "/regenerate", bytes.NewBufferString(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestRegenerateRecoveryCodes_MissingCode(t *testing.T) {
	db := dbtest.New(t)
	secret := "somesampleencryptedsecret"
	_, router := seed2FAUser(t, db, true, &secret)

	req, _ := http.NewRequest(http.MethodPost, "/regenerate", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

// --- reissueSessionToken guard branches ---

func TestReissueSessionToken_APITokenCallerIsSkipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	c.Set("isAPIToken", true)
	c.Set("cfg", config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24})

	reissueSessionToken(c, models.User{Username: "apitoken", Model: gorm.Model{ID: 1}})
	assert.Empty(t, c.Writer.Header().Get("Set-Cookie"), "an API-token caller must not receive a session cookie")
}

func TestReissueSessionToken_MissingSecretWarns(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
	c.Set("cfg", config.Config{}) // empty JWT secret

	require.NotPanics(t, func() { reissueSessionToken(c, models.User{Username: "nosecret", Model: gorm.Model{ID: 1}}) })
}

// --- Complete2FALogin error branches ---

func TestComplete2FALogin_MissingCode(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("cfg", config.Config{}); c.Next() })
	router.POST("/login/2fa", func(c *gin.Context) { Complete2FALogin(c, &config.Config{}) })

	req, _ := http.NewRequest(http.MethodPost, "/login/2fa", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestComplete2FALogin_NoPendingChallenge(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("cfg", config.Config{}); c.Next() })
	router.POST("/login/2fa", func(c *gin.Context) { Complete2FALogin(c, &config.Config{}) })

	req, _ := http.NewRequest(http.MethodPost, "/login/2fa", bytes.NewBufferString(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}

func TestComplete2FALogin_InvalidChallengeToken(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) { c.Set("cfg", config.Config{JWTSecretKey: testJWTSecret}); c.Next() })
	router.POST("/login/2fa", func(c *gin.Context) { Complete2FALogin(c, &config.Config{JWTSecretKey: testJWTSecret}) })

	req, _ := http.NewRequest(http.MethodPost, "/login/2fa", bytes.NewBufferString(`{"code":"000000"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "2fa_pending", Value: "garbage-not-a-challenge"})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}
