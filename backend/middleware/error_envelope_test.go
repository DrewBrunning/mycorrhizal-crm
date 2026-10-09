package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// Issue #1605: every middleware-written error must use the same apperrors
// envelope as the rest of the API — {"error":{"code","message"},"request_id",
// "timestamp"} — never the legacy bare {"error":"<message>"} string. A client
// that decodes error.code got a type mismatch (string, not object) on exactly
// the most common unauthenticated failure. These tests pin the shape, not just
// "some non-2xx".

// newEnvelopeAuthRouter installs RequestIDMiddleware so the full envelope
// (including request_id) can be asserted, then the real AuthMiddleware.
func newEnvelopeAuthRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	require.NoError(t, db.Create(&models.User{
		Username: "envelope", Email: "envelope@example.com", Password: "password",
	}).Error)

	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	router.Use(AuthMiddleware(&config.Config{JWTSecretKey: testJWTSecret}))
	router.GET("/protected", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return router
}

// TestAuthMiddleware_MissingToken_ReturnsStandardEnvelope is the concrete pin
// for issue #1605: no cookie, no Authorization header. The body must be the
// full documented envelope, with error as an *object* and code UNAUTHORIZED —
// not the bare string.
func TestAuthMiddleware_MissingToken_ReturnsStandardEnvelope(t *testing.T) {
	router := newEnvelopeAuthRouter(t)

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("X-Request-ID", "req-1605")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())

	// Decode into raw JSON first: this is what proves "error" is an object and
	// that the envelope carries request_id + timestamp, rather than merely
	// decoding whatever shape into a Go struct.
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw), "body: %s", w.Body.String())
	require.Contains(t, raw, "error")
	require.Contains(t, raw, "request_id")
	require.Contains(t, raw, "timestamp")

	assert.Equal(t, `"req-1605"`, string(raw["request_id"]), "request_id must round-trip from the request")

	var ts string
	require.NoError(t, json.Unmarshal(raw["timestamp"], &ts))
	_, err := time.Parse(time.RFC3339, ts)
	assert.NoError(t, err, "timestamp must be RFC3339, got %q", ts)

	detail := mwErrorEnvelope(t, w)
	assert.Equal(t, "UNAUTHORIZED", detail["code"])
	assert.Equal(t, "Authorization token required", detail["message"])
}

// TestAuthMiddleware_Rejections_PinEnvelopeCodes exercises each rejection arm
// and pins the status + envelope code, so a wrong-code regression cannot pass
// as "some 401/403".
func TestAuthMiddleware_Rejections_PinEnvelopeCodes(t *testing.T) {
	db, router := setupAuthTestRouter(t)

	var user models.User
	require.NoError(t, db.First(&user).Error)
	sid := seedSession(t, db, user.ID, time.Now())

	validClaims := func() jwt.MapClaims {
		return jwt.MapClaims{
			"user_id":       user.ID,
			"username":      user.Username,
			"token_version": user.TokenVersion,
			"sid":           sid,
			"exp":           time.Now().Add(time.Hour).Unix(),
		}
	}
	mutated := func(mutate func(jwt.MapClaims)) string {
		claims := validClaims()
		mutate(claims)
		return signJWT(t, claims)
	}

	wrongSig, err := jwt.NewWithClaims(jwt.SigningMethodHS256, validClaims()).
		SignedString([]byte("some-other-secret-32-chars-minimum"))
	require.NoError(t, err)

	revokedPlain := "mycorrhizal_revoked_envelope"
	revokedAt := time.Now()
	require.NoError(t, db.Create(&models.ApiToken{
		UserID: user.ID, Name: "revoked", TokenHash: hashToken(revokedPlain), RevokedAt: &revokedAt,
	}).Error)
	carddavPlain := "mycorrhizal_carddav_envelope"
	require.NoError(t, db.Create(&models.ApiToken{
		UserID: user.ID, Name: "carddav", TokenHash: hashToken(carddavPlain), Scope: "carddav",
	}).Error)

	cases := []struct {
		name        string
		header      string
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"missing header", "", http.StatusUnauthorized, "UNAUTHORIZED", "Authorization token required"},
		{"non-bearer scheme", "Basic dXNlcjpwYXNz", http.StatusUnauthorized, "UNAUTHORIZED", "Authorization header must start with Bearer"},
		{"malformed token", "Bearer not-a-jwt", http.StatusUnauthorized, "UNAUTHORIZED", "Malformed token"},
		{"wrong signature", "Bearer " + wrongSig, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token signature"},
		{"expired token", "Bearer " + mutated(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }), http.StatusUnauthorized, "UNAUTHORIZED", "Token expired"},
		{"missing token_version", "Bearer " + mutated(func(c jwt.MapClaims) { delete(c, "token_version") }), http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token"},
		{"missing sid", "Bearer " + mutated(func(c jwt.MapClaims) { delete(c, "sid") }), http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token"},
		{"unknown api token", "Bearer mycorrhizal_unknown_envelope", http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token"},
		{"revoked api token", "Bearer " + revokedPlain, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token"},
		{"carddav-scoped api token", "Bearer " + carddavPlain, http.StatusForbidden, "FORBIDDEN", "This token is scoped to CardDAV and cannot be used for the API"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/protected", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assertMWErrorCode(t, w, tc.wantStatus, tc.wantCode)
			assert.Equal(t, tc.wantMessage, mwErrorEnvelope(t, w)["message"])
			// The protected handler emits user_id on success; a rejection must
			// never have run it.
			assert.NotContains(t, w.Body.String(), `"user_id"`, "protected handler must not have run")
		})
	}
}

// TestAdminMiddleware_Rejections_PinEnvelopeCodes covers every AdminMiddleware
// arm (issue #1608) and asserts the envelope code, not just the status.
func TestAdminMiddleware_Rejections_PinEnvelopeCodes(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)

	admin := models.User{Username: "env-admin", Email: "env-admin@example.com", Password: "pw", IsAdmin: true}
	require.NoError(t, db.Create(&admin).Error)
	nonAdmin := models.User{Username: "env-plain", Email: "env-plain@example.com", Password: "pw"}
	require.NoError(t, db.Create(&nonAdmin).Error)

	run := func(setup func(c *gin.Context)) *httptest.ResponseRecorder {
		ran := false
		r := gin.New()
		r.Use(RequestIDMiddleware())
		r.Use(func(c *gin.Context) { c.Set("db", db); setup(c); c.Next() })
		r.Use(AdminMiddleware())
		r.GET("/admin", func(c *gin.Context) { ran = true; c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))
		require.False(t, ran, "admin handler must not run on a rejection")
		return w
	}

	cases := []struct {
		name        string
		setup       func(c *gin.Context)
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			"no userID in context",
			func(*gin.Context) {},
			http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required",
		},
		{
			"non-uint userID",
			func(c *gin.Context) { c.Set("userID", "1") },
			http.StatusUnauthorized, "UNAUTHORIZED", "Invalid user ID",
		},
		{
			"user not found",
			func(c *gin.Context) { c.Set("userID", uint(999999)) },
			http.StatusUnauthorized, "UNAUTHORIZED", "User not found",
		},
		{
			"api token",
			func(c *gin.Context) { c.Set("userID", admin.ID); c.Set("isAPIToken", true) },
			http.StatusForbidden, "FORBIDDEN", "API tokens cannot access admin endpoints",
		},
		{
			"non-admin user",
			func(c *gin.Context) { c.Set("userID", nonAdmin.ID) },
			http.StatusForbidden, "FORBIDDEN", "Admin access required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := run(tc.setup)
			assertMWErrorCode(t, w, tc.wantStatus, tc.wantCode)
			assert.Equal(t, tc.wantMessage, mwErrorEnvelope(t, w)["message"])
		})
	}
}

// TestBodySizeLimitMiddleware_413_UsesErrorEnvelope pins the body-limit
// middleware's rejection shape: 413 with PAYLOAD_TOO_LARGE in the envelope.
func TestBodySizeLimitMiddleware_413_UsesErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(BodySizeLimitMiddleware(16))
	handlerRan := false
	router.POST("/upload", func(c *gin.Context) { handlerRan = true; c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(bytes.Repeat([]byte("x"), 17)))
	req.Header.Set("Content-Length", "17")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assertMWErrorCode(t, w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
	assert.Equal(t, "request body too large", mwErrorEnvelope(t, w)["message"])
	assert.False(t, handlerRan, "handler must not run for an oversized body")
}

// TestRateLimitMiddleware_429_UsesErrorEnvelope pins the rate-limiter
// rejection shape: 429 with RATE_LIMIT_EXCEEDED in the envelope, and the
// handler never reached.
func TestRateLimitMiddleware_429_UsesErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewIPRateLimiter(rate.Every(time.Hour), 1)

	handlerRan := 0
	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(RateLimitMiddleware(limiter))
	router.GET("/test", func(c *gin.Context) { handlerRan++; c.Status(http.StatusOK) })

	do := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "203.0.113.9:1234"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusOK, do().Code, "the first request must pass")
	w := do()
	assertMWErrorCode(t, w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED")
	assert.Equal(t, "Rate limit exceeded. Please try again later.", mwErrorEnvelope(t, w)["message"])
	assert.Equal(t, 1, handlerRan, "the rate-limited request must not reach the handler")
}
