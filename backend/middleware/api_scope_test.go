package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newScopedTokenRouter(t *testing.T, scopes ...string) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "scope-user", Email: "scope@example.com", Password: "pw"}
	require.NoError(t, db.Create(&user).Error)

	plaintext := "mycorrhizal_scope_test"
	require.NoError(t, db.Create(&models.ApiToken{
		UserID:    user.ID,
		Name:      "scope",
		TokenHash: fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext))),
		Scope:     models.LegacyScopeForScopes(scopes),
		Scopes:    scopes,
	}).Error)

	cfg := &config.Config{JWTSecretKey: "test-secret-key-32-chars-minimum!"}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.Use(AuthMiddleware(cfg))
	router.Use(RequireAPIScopeForMethod())
	router.GET("/read", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.POST("/write", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router, plaintext
}

func dispatchScoped(router *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestRequireAPIScopeForMethod_ReadAndWriteAreSeparate(t *testing.T) {
	readRouter, readToken := newScopedTokenRouter(t, models.TokenScopeAPIRead)
	assert.Equal(t, http.StatusOK, dispatchScoped(readRouter, http.MethodGet, "/read", readToken).Code)
	assert.Equal(t, http.StatusForbidden, dispatchScoped(readRouter, http.MethodPost, "/write", readToken).Code)

	writeRouter, writeToken := newScopedTokenRouter(t, models.TokenScopeAPIWrite)
	assert.Equal(t, http.StatusOK, dispatchScoped(writeRouter, http.MethodPost, "/write", writeToken).Code)
	assert.Equal(t, http.StatusForbidden, dispatchScoped(writeRouter, http.MethodGet, "/read", writeToken).Code)

	fullRouter, fullToken := newScopedTokenRouter(t, models.FullTokenScopes...)
	assert.Equal(t, http.StatusOK, dispatchScoped(fullRouter, http.MethodGet, "/read", fullToken).Code)
	assert.Equal(t, http.StatusOK, dispatchScoped(fullRouter, http.MethodPost, "/write", fullToken).Code)
}

func TestRequireAPIScopeForMethod_MCPOnlyTokenIsNotREST(t *testing.T) {
	router, token := newScopedTokenRouter(t, models.TokenScopeMCP)
	assert.Equal(t, http.StatusForbidden, dispatchScoped(router, http.MethodGet, "/read", token).Code)
}

func TestIsCredentialManagementRoute(t *testing.T) {
	for _, path := range []string{
		"/api/v1/account",
		"/api/v1/users/change-password",
		"/api/v1/users/dav-password-policy",
		"/api/v1/users/2fa/status",
		"/api/v1/users/2fa/setup",
		"/api/v1/api-tokens",
		"/api/v1/api-tokens/:id",
		"/api/v1/sessions/:id",
		"/api/v1/webauthn/register/begin",
		"/api/v1/webauthn/assert/begin",
		"/api/v1/webauthn/credentials/:id",
		"/api/v1/auth/device/grants",
		"/api/v1/feeds",
		"/api/v1/feeds/:id/rotate",
	} {
		assert.Truef(t, IsCredentialManagementRoute(path), "%s must be credential-management", path)
	}
	for _, path := range []string{
		"/api/v1/contacts",
		"/api/v1/contacts/:id",
		"/api/v1/users/me",
		"/api/v1/feeds/atom",
		"/api/v1/webhooks",
	} {
		assert.Falsef(t, IsCredentialManagementRoute(path), "%s must not be credential-management", path)
	}
}

func TestRequireInteractiveSessionForCredentials(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("isAPIToken", true)
		c.Next()
	})
	router.Use(RequireInteractiveSessionForCredentials())
	router.GET("/api/v1/api-tokens", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v1/users/2fa/status", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v1/contacts", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v1/feeds/atom", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{"/api/v1/api-tokens", "/api/v1/users/2fa/status"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equalf(t, http.StatusForbidden, w.Code, "%s must reject API tokens", path)
	}
	for _, path := range []string{"/api/v1/contacts", "/api/v1/feeds/atom"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equalf(t, http.StatusOK, w.Code, "%s must allow API tokens", path)
	}

	sessionRouter := gin.New()
	sessionRouter.Use(RequireInteractiveSessionForCredentials())
	sessionRouter.GET("/api/v1/api-tokens", func(c *gin.Context) { c.Status(http.StatusOK) })
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/api-tokens", nil)
	w := httptest.NewRecorder()
	sessionRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "an interactive session must reach credential management")
}
