package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #392: pins the CSRF posture rather than leaving it assumed —
// SameSite=Strict on every session cookie (auth_token/2fa_pending/id_token),
// SameSite=Lax retained only on the transient OIDC state/nonce/PKCE cookies
// (required for the cross-site redirect back from the IdP — see the comments
// at oidc_controller.go's OIDCLoginHandler), and a protected mutating
// endpoint rejecting a request with no session cookie at all — the
// server-side floor a cross-site browser request (denied its cookie by
// SameSite) reduces to.

func TestLoginUser_AuthTokenCookie_SameSiteStrict(t *testing.T) {
	_, router, cfg, user := twoFactorTestEnv(t)

	w, cookies := doRequest(router, mustPost("/login", map[string]string{
		"identifier": user.Username,
		"password":   strongPassword,
	}))
	require.Equal(t, http.StatusOK, w.Code, "login: %s", w.Body.String())

	authCookie := cookies["auth_token"]
	require.NotNil(t, authCookie, "auth_token cookie must be set")
	assert.Equal(t, http.SameSiteStrictMode, authCookie.SameSite)
	assert.True(t, authCookie.HttpOnly)
	assert.Equal(t, cfg.CookieSecure, authCookie.Secure)
}

func TestLogin_TwoFactorPendingCookie_SameSiteStrict(t *testing.T) {
	db, router, cfg, user := twoFactorTestEnv(t)
	_, _, _ = enableTwoFactor(t, db, router, cfg, user)

	w, cookies := doRequest(router, mustPost("/login", map[string]string{
		"identifier": user.Username,
		"password":   strongPassword,
	}))
	require.Equal(t, http.StatusOK, w.Code, "login: %s", w.Body.String())

	pendingCookie := cookies["2fa_pending"]
	require.NotNil(t, pendingCookie, "2fa_pending challenge cookie must be set")
	assert.Equal(t, http.SameSiteStrictMode, pendingCookie.SameSite)
	assert.True(t, pendingCookie.HttpOnly)
}

func TestComplete2FALogin_AuthTokenCookie_SameSiteStrict(t *testing.T) {
	db, router, cfg, user := twoFactorTestEnv(t)
	secret, _, _ := enableTwoFactor(t, db, router, cfg, user)

	req := loginWith2FA(router, user, totpCode(t, secret))
	w, cookies := doRequest(router, req)
	require.Equal(t, http.StatusOK, w.Code, "login/2fa: %s", w.Body.String())

	authCookie := cookies["auth_token"]
	require.NotNil(t, authCookie, "auth_token cookie must be set after completing 2FA")
	assert.Equal(t, http.SameSiteStrictMode, authCookie.SameSite)
	assert.True(t, authCookie.HttpOnly)
}

func TestLogoutUser_ClearsAuthTokenCookie_SameSiteStrict(t *testing.T) {
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}
	_, router := setupRouter()
	router.POST("/logout", func(c *gin.Context) { LogoutUser(c, cfg, nil) })

	req, _ := http.NewRequest("POST", "/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	authCookie := findCookie(w.Result().Cookies(), "auth_token")
	require.NotNil(t, authCookie, "logout must clear the auth_token cookie")
	assert.Equal(t, http.SameSiteStrictMode, authCookie.SameSite)
	assert.Equal(t, -1, authCookie.MaxAge)
}

// TestOIDCLoginHandler_StateCookies_RemainSameSiteLax is a regression guard:
// unlike the session cookies above, oidc_state/oidc_nonce/oidc_pkce MUST stay
// Lax. They are read at /auth/oidc/callback, reached via a cross-site
// top-level redirect from the IdP — a Strict cookie would never arrive there,
// silently breaking every OIDC login. Do not "fix" these to Strict.
func TestOIDCLoginHandler_StateCookies_RemainSameSiteLax(t *testing.T) {
	provider := newFakeOIDCProviderForLogout(t, "")
	cfg := &config.Config{CookieDomain: "", CookieSecure: false}

	_, router := setupRouter()
	router.GET("/login", OIDCLoginHandler(provider, cfg))

	req, _ := http.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusFound, w.Code)

	for _, name := range []string{"oidc_state", "oidc_nonce", "oidc_pkce"} {
		cookie := findCookie(w.Result().Cookies(), name)
		require.NotNil(t, cookie, "%s cookie must be set", name)
		assert.Equal(t, http.SameSiteLaxMode, cookie.SameSite, "%s must stay Lax for the cross-site IdP redirect", name)
	}
}

// TestProtectedEndpoint_RejectsRequestWithNoSessionCookie is the concrete
// "cross-site POST is rejected" pin the issue asks for: a browser denied the
// cookie by SameSite (Strict or Lax) makes exactly this request — a mutating
// call with no session cookie attached — and it must be rejected.
func TestProtectedEndpoint_RejectsRequestWithNoSessionCookie(t *testing.T) {
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24}
	_, router := setupRouter()
	protected := router.Group("/")
	protected.Use(middleware.AuthMiddleware(cfg))
	protected.PATCH("/users/language", UpdateLanguage)

	body, _ := json.Marshal(map[string]string{"language": "en"})
	req, _ := http.NewRequest("PATCH", "/users/language", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
