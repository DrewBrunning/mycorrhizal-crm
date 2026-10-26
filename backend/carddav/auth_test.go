package carddav

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// newAuthTestRouter builds an in-memory-SQLite-backed gin router with
// BasicAuthMiddleware wired ahead of a probe route that echoes the
// authenticated userID, mirroring controllers/activity_controller_test.go's
// setupRouter pattern.
func newAuthTestRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.Use(BasicAuthMiddleware())
	router.GET("/probe", func(c *gin.Context) {
		userID, _ := c.Get("userID")
		c.JSON(http.StatusOK, gin.H{"userID": userID})
	})

	return db, router
}

func newDAVPathTestRouter(t *testing.T) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.Use(BasicAuthMiddleware())
	probe := func(c *gin.Context) {
		userID, _ := c.Get("userID")
		c.JSON(http.StatusOK, gin.H{"userID": userID})
	}
	for _, path := range []string{"/carddav/*path", "/caldav/*path"} {
		router.GET(path, probe)
		router.Handle("PROPFIND", path, probe)
	}

	return db, router
}

func doDAVRequest(router *gin.Engine, method, path, username, password string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest(method, path, http.NoBody)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func createTestToken(t *testing.T, db *gorm.DB, userID uint, scope string, opts ...func(*models.ApiToken)) string {
	t.Helper()
	rawBytes := make([]byte, 32)
	_, err := rand.Read(rawBytes)
	require.NoError(t, err)
	plaintext := "mycorrhizal_" + base64.RawURLEncoding.EncodeToString(rawBytes)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))

	expiresAt := time.Now().Add(90 * 24 * time.Hour)
	token := models.ApiToken{UserID: userID, Name: "test-token", TokenHash: hash, Scope: scope, ExpiresAt: &expiresAt}
	for _, opt := range opts {
		opt(&token)
	}
	require.NoError(t, db.Create(&token).Error)
	return plaintext
}

func doBasicAuthRequest(router *gin.Engine, username, password string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", "/probe", nil)
	req.SetBasicAuth(username, password)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestBasicAuthMiddleware_PasswordStillWorks(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	user := models.User{Username: "pw-user", Email: "pw-user@example.com", Password: string(hashed), AllowDAVPassword: true}
	require.NoError(t, db.Create(&user).Error)

	w := doBasicAuthRequest(router, "pw-user", "correct-password")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBasicAuthMiddleware_PasswordRejectedWhenSettingOff(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	user := models.User{Username: "pw-off", Email: "pw-off@example.com", Password: string(hashed)}
	require.NoError(t, db.Create(&user).Error)

	w := doBasicAuthRequest(router, "pw-off", "correct-password")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "account-password-not-allowed-for-dav", w.Header().Get("X-Mycorrhizal-Auth-Error"))
	assert.Contains(t, w.Body.String(), "app password")

	plaintext := createTestToken(t, db, user.ID, "carddav")
	assert.Equal(t, http.StatusOK, doBasicAuthRequest(router, "pw-off", plaintext).Code)
}

func TestBasicAuthMiddleware_FullScopeTokenAuthenticates(t *testing.T) {
	db, router := newAuthTestRouter(t)

	user := models.User{Username: "full-token-user", Email: "full-token-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&user).Error)
	plaintext := createTestToken(t, db, user.ID, "full")

	w := doBasicAuthRequest(router, "full-token-user", plaintext)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBasicAuthMiddleware_CardDAVScopeTokenAuthenticates(t *testing.T) {
	db, router := newAuthTestRouter(t)

	user := models.User{Username: "carddav-token-user", Email: "carddav-token-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&user).Error)
	plaintext := createTestToken(t, db, user.ID, "carddav")

	w := doBasicAuthRequest(router, "carddav-token-user", plaintext)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBasicAuthMiddleware_TwoFactorRejectsAccountPassword(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	user := models.User{
		Username:            "twofa-sync-user",
		Email:               "twofa-sync-user@example.com",
		Password:            string(hashed),
		AllowDAVPassword:    true,
		TOTPEnabled:         true,
		TOTPSecretEncrypted: &[]string{"encrypted-secret-blob"}[0],
	}
	require.NoError(t, db.Create(&user).Error)

	w := doBasicAuthRequest(router, "twofa-sync-user", "correct-password")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "account-password-not-allowed-for-dav", w.Header().Get("X-Mycorrhizal-Auth-Error"))
	assert.Contains(t, w.Body.String(), "app password")

	plaintext := createTestToken(t, db, user.ID, "carddav")
	assert.Equal(t, http.StatusOK, doBasicAuthRequest(router, "twofa-sync-user", plaintext).Code)
}

func TestBasicAuthMiddleware_PasskeyRejectsAccountPassword(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	user := models.User{
		Username:         "passkey-user",
		Email:            "passkey-user@example.com",
		Password:         string(hashed),
		AllowDAVPassword: true,
	}
	require.NoError(t, db.Create(&user).Error)
	require.NoError(t, db.Create(&models.WebAuthnCredential{
		UserID:       user.ID,
		CredentialID: []byte("credential-id"),
		PublicKey:    []byte("public-key"),
		Name:         "YubiKey",
	}).Error)

	w := doBasicAuthRequest(router, "passkey-user", "correct-password")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, "account-password-not-allowed-for-dav", w.Header().Get("X-Mycorrhizal-Auth-Error"))

	// An app password still works: 2FA gates the account password, not DAV.
	plaintext := createTestToken(t, db, user.ID, "carddav")
	assert.Equal(t, http.StatusOK, doBasicAuthRequest(router, "passkey-user", plaintext).Code)
}

func TestBasicAuthMiddleware_SecondFactorLookupFailureFailsClosed(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	user := models.User{
		Username:         "sf-failure",
		Email:            "sf-failure@example.com",
		Password:         string(hashed),
		AllowDAVPassword: true,
	}
	require.NoError(t, db.Create(&user).Error)
	dbtest.HideTable(t, db, "webauthn_credentials")

	w := doBasicAuthRequest(router, "sf-failure", "correct-password")
	assert.Equal(t, http.StatusForbidden, w.Code, "an unanswerable second-factor check must deny the account password")
	assert.Equal(t, "account-password-not-allowed-for-dav", w.Header().Get("X-Mycorrhizal-Auth-Error"))

	plaintext := createTestToken(t, db, user.ID, "carddav")
	assert.Equal(t, http.StatusOK, doBasicAuthRequest(router, "sf-failure", plaintext).Code)
}

func TestBasicAuthMiddleware_DAVScopesAreSeparate(t *testing.T) {
	db, router := newDAVPathTestRouter(t)

	user := models.User{Username: "dav-scope-user", Email: "dav-scope@example.com", Password: "unused"}
	require.NoError(t, db.Create(&user).Error)

	cardOnly := createTestToken(t, db, user.ID, "carddav")
	assert.Equal(t, http.StatusOK, doDAVRequest(router, "PROPFIND", "/carddav/probe", "dav-scope-user", cardOnly).Code)

	explicitCardOnly := createTestToken(t, db, user.ID, "carddav", func(tok *models.ApiToken) {
		tok.Scopes = []string{"carddav"}
	})
	assert.Equal(t, http.StatusOK, doDAVRequest(router, "PROPFIND", "/carddav/probe", "dav-scope-user", explicitCardOnly).Code)
	assert.Equal(t, http.StatusForbidden, doDAVRequest(router, "PROPFIND", "/caldav/probe", "dav-scope-user", explicitCardOnly).Code)

	explicitCalOnly := createTestToken(t, db, user.ID, "custom", func(tok *models.ApiToken) {
		tok.Scopes = []string{"caldav"}
	})
	assert.Equal(t, http.StatusOK, doDAVRequest(router, "PROPFIND", "/caldav/probe", "dav-scope-user", explicitCalOnly).Code)
	assert.Equal(t, http.StatusForbidden, doDAVRequest(router, "PROPFIND", "/carddav/probe", "dav-scope-user", explicitCalOnly).Code)
}

func TestBasicAuthMiddleware_TokenCannotAuthenticateAnotherUsersUsername(t *testing.T) {
	db, router := newAuthTestRouter(t)

	userA := models.User{Username: "alice", Email: "alice@example.com", Password: "unused-a"}
	userB := models.User{Username: "bob", Email: "bob@example.com", Password: "unused-b"}
	require.NoError(t, db.Create(&userA).Error)
	require.NoError(t, db.Create(&userB).Error)

	tokenB := createTestToken(t, db, userB.ID, "carddav")

	assert.Equal(t, http.StatusUnauthorized,
		doBasicAuthRequest(router, "alice", tokenB).Code,
		"bob's app password must not authenticate as alice")
	assert.Equal(t, http.StatusOK,
		doBasicAuthRequest(router, "bob", tokenB).Code,
		"the token still works for its owner")
}

func TestBasicAuthMiddleware_SSOUserWithEmptyPasswordAuthenticatesViaToken(t *testing.T) {
	db, router := newAuthTestRouter(t)

	// OIDC-provisioned users get Password: "" -- bcrypt can never match it,
	// so a scoped token is the only way for them to use CardDAV.
	user := models.User{Username: "sso-user", Email: "sso-user@example.com", Password: ""}
	require.NoError(t, db.Create(&user).Error)
	plaintext := createTestToken(t, db, user.ID, "carddav")

	w := doBasicAuthRequest(router, "sso-user", plaintext)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestBasicAuthMiddleware_TokenForDifferentUserRejected(t *testing.T) {
	db, router := newAuthTestRouter(t)

	owner := models.User{Username: "owner-user", Email: "owner-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&owner).Error)
	victim := models.User{Username: "victim-user", Email: "victim-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&victim).Error)

	plaintext := createTestToken(t, db, owner.ID, "full")

	// Typed username belongs to victim, but the token belongs to owner.
	w := doBasicAuthRequest(router, "victim-user", plaintext)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuthMiddleware_RevokedTokenRejected(t *testing.T) {
	db, router := newAuthTestRouter(t)

	user := models.User{Username: "revoked-user", Email: "revoked-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&user).Error)
	now := time.Now()
	plaintext := createTestToken(t, db, user.ID, "full", func(a *models.ApiToken) { a.RevokedAt = &now })

	w := doBasicAuthRequest(router, "revoked-user", plaintext)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuthMiddleware_ExpiredTokenRejected(t *testing.T) {
	db, router := newAuthTestRouter(t)

	user := models.User{Username: "expired-user", Email: "expired-user@example.com", Password: "unused"}
	require.NoError(t, db.Create(&user).Error)
	past := time.Now().Add(-time.Hour)
	plaintext := createTestToken(t, db, user.ID, "full", func(a *models.ApiToken) { a.ExpiresAt = &past })

	w := doBasicAuthRequest(router, "expired-user", plaintext)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestBasicAuthMiddleware_WrongPasswordAndNoValidTokenRejected(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	user := models.User{Username: "wrong-pw-user", Email: "wrong-pw-user@example.com", Password: string(hashed)}
	require.NoError(t, db.Create(&user).Error)

	w := doBasicAuthRequest(router, "wrong-pw-user", "totally-wrong")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestBasicAuthMiddleware_UnknownUserRejected is a regression guard for the
// issue #862 refactor: the unknown-identifier branch (which now burns its bcrypt
// cost via the shared services.SpendDummyPasswordHash instead of a local dummy
// hash) must still reject with 401.
func TestBasicAuthMiddleware_UnknownUserRejected(t *testing.T) {
	_, router := newAuthTestRouter(t)

	w := doBasicAuthRequest(router, "no-such-carddav-user", "whatever")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, `Basic realm="CardDAV"`, w.Header().Get("WWW-Authenticate"),
		"a DAV client needs the Basic challenge to re-prompt")
	assert.NotContains(t, w.Body.String(), "userID", "the protected handler must not run for an unknown user")
}

// TestBasicAuthMiddleware_Griefing_DoesNotLockLegitimateIP pins issue #867 on
// the CardDAV path: it shares the AccountRateLimiter with interactive login, so
// failures from an attacker's IP must not lock the real user out from theirs.
func TestBasicAuthMiddleware_Griefing_DoesNotLockLegitimateIP(t *testing.T) {
	db, router := newAuthTestRouter(t)

	hashed, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.DefaultCost)
	require.NoError(t, err)
	require.NoError(t, db.Create(&models.User{Username: "dav-grief", Email: "dav-grief@example.com", Password: string(hashed), AllowDAVPassword: true}).Error)

	req := func(ip, pw string) *httptest.ResponseRecorder {
		r, _ := http.NewRequest("GET", "/probe", nil)
		r.SetBasicAuth("dav-grief", pw)
		r.RemoteAddr = ip + ":40000"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}

	const attackerIP, victimIP = "203.0.113.31", "198.51.100.31"
	var last int
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		last = req(attackerIP, "wrong").Code
	}
	assert.Equal(t, http.StatusTooManyRequests, last, "attacker IP should be locked")
	assert.Equal(t, http.StatusOK, req(victimIP, "correct-password").Code,
		"victim's IP must not be locked by the attacker's failures (issue #867)")
}
