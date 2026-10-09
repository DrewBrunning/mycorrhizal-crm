package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1472: the JWT session verifier's rejection branches. ASVS 3.2.4 claims
// the `*jwt.SigningMethodHMAC` pin defeats `alg: none` and RSA/HMAC confusion;
// these tests are the failing-test counterpart to that claim.

// jwtRejectionHarness is a real AuthMiddleware router whose protected handler
// records whether it ran, so every rejection can assert it never did.
type jwtRejectionHarness struct {
	router     *gin.Engine
	handlerRan bool
	userID     uint
	tokenVer   uint
	sid        string
}

func newJWTRejectionHarness(t *testing.T) *jwtRejectionHarness {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "jwtrej", Email: "jwtrej@example.com", Password: "password"}
	require.NoError(t, db.Create(&user).Error)

	h := &jwtRejectionHarness{userID: user.ID, tokenVer: user.TokenVersion}
	h.sid = seedSession(t, db, user.ID, time.Now())

	cfg := &config.Config{JWTSecretKey: testJWTSecret}
	h.router = gin.New()
	h.router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	h.router.Use(AuthMiddleware(cfg))
	h.router.GET("/protected", func(c *gin.Context) {
		h.handlerRan = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return h
}

func (h *jwtRejectionHarness) validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"user_id":       h.userID,
		"username":      "jwtrej",
		"token_version": h.tokenVer,
		"sid":           h.sid,
		"exp":           time.Now().Add(time.Hour).Unix(),
	}
}

// do issues the request and decodes the response message. envelope reports
// whether the body used the apperrors `{"error":{...,"message"}}` shape
// (issue #1605) rather than the legacy bare `{"error":"..."}` string.
func (h *jwtRejectionHarness) do(token string) (code int, msg string, envelope bool) {
	h.handlerRan = false
	w := jwtRequest(h.router, token)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	detail, ok := body["error"].(map[string]any)
	if !ok {
		return w.Code, "", false
	}
	msg, _ = detail["message"].(string)
	return w.Code, msg, true
}

func (h *jwtRejectionHarness) requireRejected(t *testing.T, token, wantMsg string) {
	t.Helper()
	code, msg, envelope := h.do(token)
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.True(t, envelope, "rejection must use the apperrors envelope, not a bare-string error")
	assert.Equal(t, wantMsg, msg)
	assert.False(t, h.handlerRan, "protected handler must not run for a rejected token")
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func TestAuthMiddleware_RejectsAlgNone(t *testing.T) {
	h := newJWTRejectionHarness(t)
	forged, err := jwt.NewWithClaims(jwt.SigningMethodNone, h.validClaims()).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	h.requireRejected(t, forged, "Invalid token")

	// Variant: header says none but a signature segment is appended.
	parts := strings.Split(forged, ".")
	h.requireRejected(t, parts[0]+"."+parts[1]+"."+b64([]byte("sig")), "Invalid token")
}

func TestAuthMiddleware_RejectsRSAHMACConfusion(t *testing.T) {
	h := newJWTRejectionHarness(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	t.Run("RS256 token signed by an RSA key", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, h.validClaims()).SignedString(key)
		require.NoError(t, err)
		h.requireRejected(t, tok, "Invalid token")
	})

	t.Run("RS256 header with HMAC(public key PEM) signature", func(t *testing.T) {
		claims, err := json.Marshal(h.validClaims())
		require.NoError(t, err)
		signingInput := b64([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + b64(claims)
		mac := hmac.New(sha256.New, pubPEM)
		mac.Write([]byte(signingInput))
		h.requireRejected(t, signingInput+"."+b64(mac.Sum(nil)), "Invalid token")
	})

	t.Run("HS256 signed with the public key PEM as the secret", func(t *testing.T) {
		tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, h.validClaims()).SignedString(pubPEM)
		require.NoError(t, err)
		h.requireRejected(t, tok, "Invalid token signature")
	})
}

func TestAuthMiddleware_RejectsWrongSecret(t *testing.T) {
	h := newJWTRejectionHarness(t)
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, h.validClaims()).
		SignedString([]byte("some-other-secret-32-chars-minimum"))
	require.NoError(t, err)
	h.requireRejected(t, tok, "Invalid token signature")
}

func TestAuthMiddleware_RejectsMalformedTokens(t *testing.T) {
	h := newJWTRejectionHarness(t)
	valid := signJWT(t, h.validClaims())
	parts := strings.Split(valid, ".")

	cases := map[string]string{
		"garbage":           "not-a-jwt",
		"two segments":      parts[0] + "." + parts[1],
		"four segments":     valid + ".extra",
		"truncated":         valid[:len(valid)/2],
		"empty segments":    "..",
		"empty header":      "." + parts[1] + "." + parts[2],
		"non-base64 header": "!!!." + parts[1] + "." + parts[2],
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			h.requireRejected(t, tok, "Malformed token")
		})
	}

	t.Run("empty bearer value", func(t *testing.T) {
		h.handlerRan = false
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer ")
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, h.handlerRan)
	})
}

func TestAuthMiddleware_RejectsBadClaimTypes(t *testing.T) {
	h := newJWTRejectionHarness(t)

	for _, claim := range []string{"user_id", "token_version"} {
		bad := map[string]any{
			"string":      "1",
			"negative":    -1,
			"negative_f":  -1.5,
			"bool":        true,
			"object":      map[string]any{"a": 1},
			"null":        nil,
			"missing":     "__delete__",
			"empty_array": []any{},
		}
		for name, val := range bad {
			t.Run(claim+"/"+name, func(t *testing.T) {
				claims := h.validClaims()
				if val == "__delete__" {
					delete(claims, claim)
				} else {
					claims[claim] = val
				}
				h.requireRejected(t, signJWT(t, claims), "Invalid token")
			})
		}
	}

	t.Run("missing sid", func(t *testing.T) {
		claims := h.validClaims()
		delete(claims, "sid")
		h.requireRejected(t, signJWT(t, claims), "Invalid token")
	})

	t.Run("expired", func(t *testing.T) {
		claims := h.validClaims()
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
		h.requireRejected(t, signJWT(t, claims), "Token expired")
	})
}

func TestUintClaim_Arms(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  uint
		ok    bool
	}{
		{"float", float64(7), 7, true},
		{"negative float", float64(-1), 0, false},
		{"int", 7, 7, true},
		{"negative int", -7, 0, false},
		{"uint", uint(7), 7, true},
		{"string", "7", 0, false},
		{"bool", true, 0, false},
		{"nil", nil, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := uintClaim(jwt.MapClaims{"k": tc.value}, "k")
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
	_, ok := uintClaim(jwt.MapClaims{}, "k")
	assert.False(t, ok)
}

// ASVS 3.2.4 documents the pin as family-wide: HS384/HS512 signed with the real
// secret verify. This pins that stated behavior so a tightening (or loosening)
// is a deliberate change.
func TestAuthMiddleware_AcceptsHMACFamily(t *testing.T) {
	h := newJWTRejectionHarness(t)
	for _, m := range []jwt.SigningMethod{jwt.SigningMethodHS256, jwt.SigningMethodHS384, jwt.SigningMethodHS512} {
		t.Run(m.Alg(), func(t *testing.T) {
			tok, err := jwt.NewWithClaims(m, h.validClaims()).SignedString([]byte(testJWTSecret))
			require.NoError(t, err)
			code, _, _ := h.do(tok)
			assert.Equal(t, http.StatusOK, code)
			assert.True(t, h.handlerRan)
		})
	}
}

func TestAuthMiddleware_RejectsMissingOrNonBearerHeader(t *testing.T) {
	h := newJWTRejectionHarness(t)

	for name, header := range map[string]string{
		"no header":     "",
		"basic scheme":  "Basic dXNlcjpwYXNz",
		"lowercase":     "bearer " + signJWT(t, h.validClaims()),
		"bare token":    signJWT(t, h.validClaims()),
		"token scheme":  "Token abc",
		"bearer no sep": "Bearer",
	} {
		t.Run(name, func(t *testing.T) {
			h.handlerRan = false
			req, _ := http.NewRequest("GET", "/protected", nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			w := httptest.NewRecorder()
			h.router.ServeHTTP(w, req)
			assertMWErrorCode(t, w, http.StatusUnauthorized, "UNAUTHORIZED")
			assert.False(t, h.handlerRan)
		})
	}
}

// The httpOnly auth_token cookie goes through the same verifier: a forged
// token in the cookie is rejected, and a valid one is accepted.
func TestAuthMiddleware_CookieTokenUsesSameVerifier(t *testing.T) {
	h := newJWTRejectionHarness(t)
	do := func(token string) int {
		h.handlerRan = false
		req, _ := http.NewRequest("GET", "/protected", nil)
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
		w := httptest.NewRecorder()
		h.router.ServeHTTP(w, req)
		return w.Code
	}

	forged, err := jwt.NewWithClaims(jwt.SigningMethodNone, h.validClaims()).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	assert.Equal(t, http.StatusUnauthorized, do(forged))
	assert.False(t, h.handlerRan)

	assert.Equal(t, http.StatusOK, do(signJWT(t, h.validClaims())))
	assert.True(t, h.handlerRan)
}

func TestLookupAPIToken_RejectsWrongPrefix(t *testing.T) {
	db := dbtest.New(t)
	tok, ok := LookupAPIToken(db, "not_a_mycorrhizal_token", time.Now())
	assert.False(t, ok)
	assert.Nil(t, tok)
}

// AdminMiddleware's 401 arms.
func TestAdminMiddleware_RejectionArms(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)

	run := func(setUserID func(c *gin.Context)) (int, bool) {
		ran := false
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("db", db); setUserID(c); c.Next() })
		r.Use(AdminMiddleware())
		r.GET("/admin", func(c *gin.Context) { ran = true; c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/admin", nil)
		r.ServeHTTP(w, req)
		return w.Code, ran
	}

	t.Run("no userID in context", func(t *testing.T) {
		code, ran := run(func(*gin.Context) {})
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.False(t, ran)
	})
	t.Run("non-uint userID", func(t *testing.T) {
		for _, v := range []any{"1", 1, float64(1), nil} {
			code, ran := run(func(c *gin.Context) { c.Set("userID", v) })
			assert.Equal(t, http.StatusUnauthorized, code)
			assert.False(t, ran)
		}
	})
	t.Run("user deleted", func(t *testing.T) {
		code, ran := run(func(c *gin.Context) { c.Set("userID", uint(999999)) })
		assert.Equal(t, http.StatusUnauthorized, code)
		assert.False(t, ran)
	})
}
