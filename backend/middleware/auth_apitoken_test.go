package middleware

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAuthTestRouter(t testing.TB) (*gorm.DB, *gin.Engine) {
	return setupAuthTestRouterWithIdle(t, 0)
}

// setupAuthTestRouterWithIdle is setupAuthTestRouter with a non-zero session
// idle timeout (issue #866); 0 disables idle enforcement.
func setupAuthTestRouterWithIdle(t testing.TB, idleHours int) (*gorm.DB, *gin.Engine) {
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)

	user := models.User{Username: "authtest", Email: "authtest@example.com", Password: "password"}
	if err := db.Create(&user).Error; err != nil {
		panic("failed to seed user")
	}

	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!", SessionIdleTimeoutHours: idleHours}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.Use(AuthMiddleware(cfg))

	router.GET("/protected", func(c *gin.Context) {
		userID, _ := c.Get("userID")
		isAPIToken, _ := c.Get("isAPIToken")
		c.JSON(http.StatusOK, gin.H{"user_id": userID, "is_api_token": isAPIToken})
	})

	return db, router
}

// mwErrorEnvelope decodes a middleware error response and asserts it uses the
// apperrors envelope — `{"error":{"code","message"}}` — not the legacy bare
// `{"error":"..."}` string (issue #1605). It returns the decoded detail object
// so callers can pin the code and message.
func mwErrorEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	detail, ok := body["error"].(map[string]any)
	require.True(t, ok, "error must be an object (apperrors envelope), got %T in %s", body["error"], w.Body.String())
	return detail
}

// assertMWErrorCode pins the response status and the envelope's error.code, so
// a test can never be satisfied by "some 4xx" carrying an unrelated error.
func assertMWErrorCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	assert.Equal(t, status, w.Code, w.Body.String())
	detail := mwErrorEnvelope(t, w)
	assert.Equal(t, code, detail["code"], w.Body.String())
}

// rejectedBody decodes a middleware rejection body, asserts the protected
// handler did NOT run (its payload carries user_id; a rejection never does),
// and returns the envelope's human-readable message. It requires the apperrors
// envelope shape, so the bare-string regression cannot pass.
func rejectedBody(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotContains(t, body, "user_id", "protected handler must not have run")
	assert.NotContains(t, body, "is_api_token", "protected handler must not have run")
	detail := mwErrorEnvelope(t, w)
	msg, _ := detail["message"].(string)
	return msg
}

func hashToken(plaintext string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
}

func TestAuthMiddleware_ValidApiToken(t *testing.T) {
	db, router := setupAuthTestRouter(t)

	var user models.User
	db.First(&user)

	plaintext := "mycorrhizal_validtoken123456789"
	db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "test",
		TokenHash: hashToken(plaintext),
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	json.Unmarshal(w.Body.Bytes(), &body)
	assert.Equal(t, float64(user.ID), body["user_id"])
	assert.Equal(t, true, body["is_api_token"])
}

func TestAuthMiddleware_RevokedApiToken(t *testing.T) {
	db, router := setupAuthTestRouter(t)

	var user models.User
	db.First(&user)

	plaintext := "mycorrhizal_revokedtoken9876"
	now := time.Now()
	db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "revoked",
		TokenHash: hashToken(plaintext),
		RevokedAt: &now,
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusUnauthorized, "UNAUTHORIZED")
	assert.Equal(t, "Invalid token", rejectedBody(t, w))
}

func TestAuthMiddleware_UnknownApiToken(t *testing.T) {
	_, router := setupAuthTestRouter(t)

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer mycorrhizal_doesnotexistXXXX")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusUnauthorized, "UNAUTHORIZED")
	assert.Equal(t, "Invalid token", rejectedBody(t, w))
}

func TestAuthMiddleware_ApiToken_UpdatesLastUsedAt(t *testing.T) {
	db, router := setupAuthTestRouter(t)

	var user models.User
	db.First(&user)

	plaintext := "mycorrhizal_lastusedupdatetoken"
	token := models.ApiToken{
		UserID:    user.ID,
		Name:      "track",
		TokenHash: hashToken(plaintext),
	}
	db.Create(&token)

	assert.Nil(t, token.LastUsedAt)

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// The update runs in a fire-and-forget goroutine (TouchAPIToken), so poll
	// until the write lands instead of sleeping a fixed interval — the old
	// time.Sleep(50ms) flaked when the goroutine hadn't been scheduled yet.
	var updated models.ApiToken
	require.Eventually(t, func() bool {
		db.First(&updated, token.ID)
		return updated.LastUsedAt != nil
	}, 5*time.Second, 10*time.Millisecond, "the async last_used_at update must land")

	assert.WithinDuration(t, time.Now(), *updated.LastUsedAt, 5*time.Second)
}

func TestAuthMiddleware_FullScopeApiTokenAuthenticates(t *testing.T) {
	db, router := setupAuthTestRouter(t)

	var user models.User
	db.First(&user)

	plaintext := "mycorrhizal_fullscopetoken12345"
	db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "full-scope",
		TokenHash: hashToken(plaintext),
		Scope:     "full",
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(user.ID), body["user_id"], "token must resolve to its owner")
	assert.Equal(t, true, body["is_api_token"])
}

func TestAuthMiddleware_DefaultScopeApiTokenAuthenticates(t *testing.T) {
	// Tokens minted before the scope column existed default to "full" via the
	// DB column default, so this is a no-regression check.
	db, router := setupAuthTestRouter(t)

	var user models.User
	db.First(&user)

	plaintext := "mycorrhizal_defaultscopetoken1"
	db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "default-scope",
		TokenHash: hashToken(plaintext),
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, float64(user.ID), body["user_id"], "token must resolve to its owner")
	assert.Equal(t, true, body["is_api_token"])
}

func TestAuthMiddleware_CardDAVScopeApiTokenRejected(t *testing.T) {
	db, router := setupAuthTestRouter(t)

	var user models.User
	db.First(&user)

	plaintext := "mycorrhizal_carddavscopetoken1"
	db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "carddav-scope",
		TokenHash: hashToken(plaintext),
		Scope:     "carddav",
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusForbidden, "FORBIDDEN")
	assert.Equal(t, "This token is scoped to CardDAV and cannot be used for the API", mwErrorEnvelope(t, w)["message"])
}

func TestAuthMiddleware_MissingAuthorizationHeader(t *testing.T) {
	_, router := setupAuthTestRouter(t)

	req, _ := http.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusUnauthorized, "UNAUTHORIZED")
	assert.Equal(t, "Authorization token required", rejectedBody(t, w))
}

func TestAdminMiddleware_BlocksApiToken(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)

	user := models.User{Username: "admintest", Email: "admin@example.com", Password: "pw", IsAdmin: true}
	db.Create(&user)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		// Simulate what AuthMiddleware sets for an API token
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("isAPIToken", true)
		c.Next()
	})
	router.Use(AdminMiddleware())

	router.GET("/admin/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/admin/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusForbidden, "FORBIDDEN")
	assert.Equal(t, "API tokens cannot access admin endpoints", mwErrorEnvelope(t, w)["message"])
}

func TestAdminMiddleware_AllowsAdminUser(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)

	user := models.User{Username: "superadmin", Email: "super@example.com", Password: "pw", IsAdmin: true}
	db.Create(&user)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		// isAPIToken NOT set — regular JWT session
		c.Next()
	})
	router.Use(AdminMiddleware())

	router.GET("/admin/users", func(c *gin.Context) {
		uid, _ := c.Get("userID")
		c.JSON(http.StatusOK, gin.H{"ok": true, "user_id": uid})
	})

	req, _ := http.NewRequest("GET", "/admin/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["ok"], "handler must have run")
	assert.Equal(t, float64(user.ID), body["user_id"], "handler must run as the admin")
}

func TestAdminMiddleware_BlocksNonAdminUser(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)

	user := models.User{Username: "regular", Email: "regular@example.com", Password: "pw", IsAdmin: false}
	db.Create(&user)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.Use(AdminMiddleware())

	router.GET("/admin/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/admin/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusForbidden, "FORBIDDEN")
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "Admin access required", mwErrorEnvelope(t, w)["message"])
	assert.NotContains(t, body, "ok", "admin handler must not have run")
}
