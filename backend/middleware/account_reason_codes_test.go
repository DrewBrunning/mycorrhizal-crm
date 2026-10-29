package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2102 (ADR 0038): the auth and admin middleware rejections carry a
// reason_code in the raw JSON so Android (which does not use the web's
// SessionExpiredGate) can show them localized. These tests read the wire body,
// so a dropped .WithReason fails them.

// wireReasonCode returns error.reason_code from the raw response body.
func wireReasonCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			ReasonCode string `json:"reason_code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), "body: %s", w.Body.String())
	return body.Error.ReasonCode
}

func TestAuthMiddlewareReasonCodes_JWT(t *testing.T) {
	h := newJWTRejectionHarness(t)
	otherSecret := func(claims jwt.MapClaims) string {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("some-other-secret-32-chars-minimum"))
		require.NoError(t, err)
		return tok
	}
	expired := h.validClaims()
	expired["exp"] = time.Now().Add(-time.Hour).Unix()
	purpose := h.validClaims()
	purpose["purpose"] = "2fa"
	unknownSid := h.validClaims()
	unknownSid["sid"] = "no-such-session"

	cases := []struct {
		name  string
		token string
		code  string
	}{
		{"expired", signJWT(t, expired), "auth_bearer_token_expired"},
		{"malformed", "not-a-jwt", "auth_bearer_token_malformed"},
		{"bad signature", otherSecret(h.validClaims()), "auth_bearer_token_signature_invalid"},
		{"purpose-scoped token", signJWT(t, purpose), "auth_bearer_token_invalid"},
		{"unknown session", signJWT(t, unknownSid), "auth_session_expired"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := jwtRequest(h.router, tc.token)
			require.Equal(t, http.StatusUnauthorized, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, tc.code, wireReasonCode(t, w), "body: %s", w.Body.String())
		})
	}
}

func TestAuthMiddlewareReasonCodes_Headers(t *testing.T) {
	_, router := setupAuthTestRouter(t)

	t.Run("no authorization", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/protected", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "auth_authorization_token_required", wireReasonCode(t, w))
	})

	t.Run("not a bearer header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Basic abc")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "auth_authorization_header_not_bearer", wireReasonCode(t, w))
	})

	t.Run("unknown api token", func(t *testing.T) {
		w := jwtRequest(router, "mycorrhizal_doesnotexist")
		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "auth_bearer_token_invalid", wireReasonCode(t, w))
	})
}

func TestAuthMiddlewareReasonCodes_ScopedAPITokens(t *testing.T) {
	for scope, code := range map[string]string{
		"carddav": "auth_token_scoped_carddav",
		"caldav":  "auth_token_scoped_caldav",
	} {
		t.Run(scope, func(t *testing.T) {
			db, router := setupAuthTestRouter(t)
			var user models.User
			require.NoError(t, db.First(&user).Error)
			plaintext := "mycorrhizal_reason2102" + scope
			require.NoError(t, db.Create(&models.ApiToken{
				UserID: user.ID, Name: scope, TokenHash: hashToken(plaintext), Scope: scope,
			}).Error)
			w := jwtRequest(router, plaintext)
			require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, code, wireReasonCode(t, w))
		})
	}
}

func TestAdminMiddlewareReasonCodes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	regular := models.User{Username: "reg2102", Email: "reg2102@example.com", Password: "pw"}
	require.NoError(t, db.Create(&regular).Error)

	serve := func(set func(c *gin.Context)) *httptest.ResponseRecorder {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("db", db); set(c); c.Next() })
		router.Use(AdminMiddleware())
		router.GET("/admin", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		req, _ := http.NewRequest("GET", "/admin", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	cases := []struct {
		name   string
		set    func(c *gin.Context)
		status int
		code   string
	}{
		{"no user in context", func(c *gin.Context) {}, http.StatusUnauthorized, "auth_required"},
		{"user id of the wrong type", func(c *gin.Context) { c.Set("userID", "7") }, http.StatusUnauthorized, "auth_user_id_invalid"},
		{"api token", func(c *gin.Context) { c.Set("userID", regular.ID); c.Set("isAPIToken", true) }, http.StatusForbidden, "admin_api_token_forbidden"},
		{"user missing", func(c *gin.Context) { c.Set("userID", uint(999999)) }, http.StatusUnauthorized, "auth_user_not_found"},
		{"not an admin", func(c *gin.Context) { c.Set("userID", regular.ID) }, http.StatusForbidden, "admin_access_required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := serve(tc.set)
			require.Equal(t, tc.status, w.Code, "body: %s", w.Body.String())
			assert.Equal(t, tc.code, wireReasonCode(t, w))
		})
	}
}
