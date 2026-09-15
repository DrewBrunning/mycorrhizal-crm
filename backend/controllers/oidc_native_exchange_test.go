package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// RFC 7636 Appendix B vector, reused so the controller tests exercise the same
// verifier->challenge binding the service tests pin.
const (
	nativeTestVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	nativeTestChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

func exchangeTestConfig() *config.Config {
	return &config.Config{
		JWTSecretKey:   "native-exchange-test-secret-key",
		JWTExpiryHours: 24,
		CookieSecure:   false,
	}
}

func postExchange(t *testing.T, router *gin.Engine, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, _ := http.NewRequest("POST", "/exchange", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// mints a valid code for the seeded user (id 1) bound to the RFC vector
// challenge, so a test only has to supply the right verifier.
func TestOIDCNativeExchangeHandler_Success(t *testing.T) {
	db, router := setupRouter()
	cfg := exchangeTestConfig()
	router.POST("/exchange",
		middleware.ValidateJSONMiddleware(&models.OIDCNativeExchangeInput{}),
		OIDCNativeExchangeHandler(cfg),
	)

	var user models.User
	require.NoError(t, db.Where("username = ?", "tester").First(&user).Error)
	code, err := services.MintOIDCNativeExchangeCode(user, nativeTestChallenge, cfg)
	require.NoError(t, err)

	w := postExchange(t, router, models.OIDCNativeExchangeInput{
		Code:         code,
		CodeVerifier: nativeTestVerifier,
	})

	require.Equal(t, http.StatusOK, w.Code)
	var resp models.OIDCNativeExchangeResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Token)
	assert.Equal(t, 3, len(splitJWT(resp.Token)), "expected a session JWT")

	// A server-side session row was created, exactly like a password login.
	var sessions int64
	require.NoError(t, db.Model(&models.Session{}).Count(&sessions).Error)
	assert.EqualValues(t, 1, sessions)

	// The session token is a real session (no purpose claim).
	_, _, isExchangeCode := services.ParseOIDCNativeExchangeCode(resp.Token, cfg)
	assert.False(t, isExchangeCode, "the returned token is a session, not an exchange code")
}

func TestOIDCNativeExchangeHandler_ValidationAndRejections(t *testing.T) {
	db, router := setupRouter()
	cfg := exchangeTestConfig()
	router.POST("/exchange",
		middleware.ValidateJSONMiddleware(&models.OIDCNativeExchangeInput{}),
		OIDCNativeExchangeHandler(cfg),
	)

	var user models.User
	require.NoError(t, db.Where("username = ?", "tester").First(&user).Error)
	validCode, err := services.MintOIDCNativeExchangeCode(user, nativeTestChallenge, cfg)
	require.NoError(t, err)

	t.Run("missing fields is a 400", func(t *testing.T) {
		w := postExchange(t, router, map[string]string{"code": validCode})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed code is a 401", func(t *testing.T) {
		w := postExchange(t, router, models.OIDCNativeExchangeInput{
			Code: "not-a-jwt", CodeVerifier: nativeTestVerifier,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong verifier is a 401", func(t *testing.T) {
		w := postExchange(t, router, models.OIDCNativeExchangeInput{
			Code: validCode, CodeVerifier: "the-wrong-verifier",
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("missing verifier is a 400", func(t *testing.T) {
		w := postExchange(t, router, map[string]string{"code": validCode})
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("non-existent user is a 401", func(t *testing.T) {
		ghost := user
		ghost.ID = 999999
		ghostCode, mintErr := services.MintOIDCNativeExchangeCode(ghost, nativeTestChallenge, cfg)
		require.NoError(t, mintErr)
		w := postExchange(t, router, models.OIDCNativeExchangeInput{
			Code: ghostCode, CodeVerifier: nativeTestVerifier,
		})
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// The handler is defensive if it is ever invoked without the JSON-validation
// middleware in front of it (GetValidated finds nothing in context): it must
// reject rather than nil-panic.
func TestOIDCNativeExchangeHandler_MissingValidatedBodyIsRejected(t *testing.T) {
	_, router := setupRouter()
	cfg := exchangeTestConfig()
	router.POST("/exchange", OIDCNativeExchangeHandler(cfg))

	w := postExchange(t, router, map[string]string{"code": "x", "code_verifier": "y"})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// The exchange code must never be usable as a bearer session, even though it is
// signed by the same secret: AuthMiddleware rejects any token carrying a
// `purpose` claim. This is the property that makes leaking the deep link
// non-fatal.
func TestOIDCNativeExchangeCode_RejectedAsBearer(t *testing.T) {
	db, router := setupRouter()
	cfg := exchangeTestConfig()

	var user models.User
	require.NoError(t, db.Where("username = ?", "tester").First(&user).Error)
	code, err := services.MintOIDCNativeExchangeCode(user, nativeTestChallenge, cfg)
	require.NoError(t, err)

	router.GET("/protected", middleware.AuthMiddleware(cfg), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+code)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"a purpose-scoped exchange code must never authenticate a session route")
}
