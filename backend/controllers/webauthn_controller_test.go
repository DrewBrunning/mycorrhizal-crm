package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	waTestOrigin = "https://crm.example.com"
	waTestRPID   = "crm.example.com"
)

// waEnv is a real-migrated-DB router with the TOTP and WebAuthn routes wired
// (mirroring routes.go) and one seeded password user.
type waEnv struct {
	t      *testing.T
	db     *gorm.DB
	router *gin.Engine
	cfg    *config.Config
	user   models.User
}

func newWAEnv(t *testing.T) *waEnv {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() {
		models.AuditFlush()
		models.RegisterAuditDB(nil)
	})
	cfg := &config.Config{JWTSecretKey: testJWTSecret, JWTExpiryHours: 24, FrontendURL: waTestOrigin}

	username := strings.ToLower("wa_" + strings.ReplaceAll(strings.TrimPrefix(t.Name(), "Test"), "/", "_"))
	if len(username) > 50 {
		username = username[:50]
	}
	hashed, err := services.HashPassword(strongPassword)
	require.NoError(t, err)
	user := models.User{Username: username, Email: username + "@example.com", Password: hashed}
	require.NoError(t, db.Create(&user).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.POST("/login", func(c *gin.Context) { LoginUser(c, cfg) })
	router.POST("/login/2fa", func(c *gin.Context) { Complete2FALogin(c, cfg) })
	router.POST("/webauthn/login/begin", func(c *gin.Context) { WebAuthnLoginBegin(c, cfg) })
	router.POST("/webauthn/login/finish", func(c *gin.Context) { WebAuthnLoginFinish(c, cfg) })
	p := router.Group("/")
	p.Use(middleware.AuthMiddleware(cfg))
	p.GET("/users/2fa/status", GetTwoFactorStatus)
	p.POST("/users/2fa/setup", SetupTwoFactor)
	p.POST("/users/2fa/confirm", ConfirmTwoFactor)
	p.POST("/users/2fa/disable", DisableTwoFactor)
	p.POST("/users/2fa/recovery-codes/regenerate", RegenerateRecoveryCodes)
	p.POST("/webauthn/register/begin", WebAuthnRegisterBegin)
	p.POST("/webauthn/register/finish", WebAuthnRegisterFinish)
	p.POST("/webauthn/assert/begin", WebAuthnProofBegin)
	p.GET("/webauthn/credentials", ListWebAuthnCredentials)
	p.DELETE("/webauthn/credentials/:id", DeleteWebAuthnCredential)

	return &waEnv{t: t, db: db, router: router, cfg: cfg, user: user}
}

func (e *waEnv) session() string {
	tok, err := services.IssueSession(e.db, e.reload(), e.cfg, "", "")
	require.NoError(e.t, err)
	return tok
}

func (e *waEnv) reload() models.User {
	var u models.User
	require.NoError(e.t, e.db.First(&u, e.user.ID).Error)
	return u
}

func (e *waEnv) auth() *virtualAuthenticator {
	return newVirtualAuthenticator(e.t, waTestOrigin, waTestRPID)
}

func (e *waEnv) do(method, path string, body any, token string) (*httptest.ResponseRecorder, map[string]*http.Cookie) {
	return doRequest(e.router, sessionRequest(method, path, body, token))
}

// register runs the whole enrollment ceremony with va and returns the finish
// response recorder + cookies. token must be a live session.
func (e *waEnv) register(va *virtualAuthenticator, name, token string) (*httptest.ResponseRecorder, map[string]*http.Cookie) {
	e.t.Helper()
	w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"name": name}, token)
	require.Equal(e.t, http.StatusOK, w.Code, "register/begin: %s", w.Body.String())
	return e.do("POST", "/webauthn/register/finish", va.attestation(challengeFrom(e.t, w.Body.Bytes())), token)
}

// enroll registers va and returns the credential id, recovery codes and the
// live session token to use afterwards (re-issued when it was the first factor).
func (e *waEnv) enroll(va *virtualAuthenticator, name, token string) (id string, codes []string, session string) {
	e.t.Helper()
	w, cookies := e.register(va, name, token)
	require.Equal(e.t, http.StatusCreated, w.Code, "register/finish: %s", w.Body.String())
	var out struct {
		ID            string   `json:"id"`
		RecoveryCodes []string `json:"recovery_codes"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &out))
	session = token
	if c := cookies["auth_token"]; c != nil {
		session = c.Value
	}
	return out.ID, out.RecoveryCodes, session
}

// passwordStep runs POST /login and returns the 2fa_pending cookie + methods.
func (e *waEnv) passwordStep() (*http.Cookie, []string) {
	e.t.Helper()
	w, cookies := doRequest(e.router, mustPost("/login", map[string]string{"identifier": e.user.Username, "password": strongPassword}))
	require.Equal(e.t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		Required bool     `json:"two_factor_required"`
		Methods  []string `json:"methods"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &out))
	require.True(e.t, out.Required)
	require.NotNil(e.t, cookies["2fa_pending"])
	return cookies["2fa_pending"], out.Methods
}

func withPending(req *http.Request, pending *http.Cookie) *http.Request {
	req.AddCookie(&http.Cookie{Name: "2fa_pending", Value: pending.Value})
	return req
}

// passkeyLogin runs the login ceremony for va; returns the finish recorder and cookies.
func (e *waEnv) passkeyLogin(va *virtualAuthenticator, pending *http.Cookie) (*httptest.ResponseRecorder, map[string]*http.Cookie) {
	e.t.Helper()
	w, _ := doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/begin", nil, ""), pending))
	require.Equal(e.t, http.StatusOK, w.Code, "login/begin: %s", w.Body.String())
	return doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/finish", va.assertion(challengeFrom(e.t, w.Body.Bytes())), ""), pending))
}

func (e *waEnv) credentialCount() int64 {
	var n int64
	require.NoError(e.t, e.db.Model(&models.WebAuthnCredential{}).Where("user_id = ?", e.user.ID).Count(&n).Error)
	return n
}

func (e *waEnv) recoveryCount() int64 {
	var n int64
	require.NoError(e.t, e.db.Model(&models.RecoveryCode{}).Where("user_id = ?", e.user.ID).Count(&n).Error)
	return n
}

func (e *waEnv) auditCount(op string) int64 {
	models.AuditFlush()
	var n int64
	require.NoError(e.t, e.db.Model(&models.AuditEvent{}).Where("user_id = ? AND operation = ?", e.user.ID, op).Count(&n).Error)
	return n
}

// enableTOTP enrolls TOTP through the real endpoints and returns the secret and
// the re-issued session.
func (e *waEnv) enableTOTP(token string) (secret string, session string) {
	e.t.Helper()
	w, _ := e.do("POST", "/users/2fa/setup", nil, token)
	require.Equal(e.t, http.StatusOK, w.Code)
	var setup struct {
		Secret string `json:"secret"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &setup))
	w, cookies := e.do("POST", "/users/2fa/confirm", map[string]string{"code": totpCodeAt(e.t, setup.Secret, time.Now().Add(-30*time.Second))}, token)
	require.Equal(e.t, http.StatusOK, w.Code, w.Body.String())
	return setup.Secret, cookies["auth_token"].Value
}

func TestWebAuthn_RegistrationRoundTripCreatesExactlyOneRow(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	tok := e.session()

	id, codes, session := e.enroll(va, "YubiKey", tok)

	require.Equal(t, int64(1), e.credentialCount())
	var row models.WebAuthnCredential
	require.NoError(t, e.db.Where("user_id = ? AND credential_id = ?", e.user.ID, va.credentialID).First(&row).Error)
	assert.Equal(t, id, row.ID)
	assert.Equal(t, "YubiKey", row.Name)
	assert.NotEmpty(t, row.PublicKey)
	assert.Nil(t, row.LastUsedAt)

	// First second-factor of any kind mints a recovery set.
	assert.Len(t, codes, recoveryCodeCount)
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount())
	assert.Equal(t, int64(1), e.auditCount(models.AuditOpWebAuthnRegister))

	// First factor bumps the token version: the pre-enrollment token dies, the
	// re-issued one is live.
	w, _ := e.do("GET", "/webauthn/credentials", nil, tok)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = e.do("GET", "/webauthn/credentials", nil, session)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestWebAuthn_ListNeverLeaksKeyMaterial(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	_, _, session := e.enroll(va, "Laptop", e.session())

	w, _ := e.do("GET", "/webauthn/credentials", nil, session)
	require.Equal(t, http.StatusOK, w.Code)
	var raw struct {
		Credentials []map[string]any `json:"credentials"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Len(t, raw.Credentials, 1)
	keys := []string{}
	for k := range raw.Credentials[0] {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"id", "name", "created_at", "last_used_at"}, keys)
	assert.NotContains(t, w.Body.String(), b64(va.credentialID))

	// Empty list is [] not null (frontend trap #8).
	other := newWAEnv(t)
	w, _ = other.do("GET", "/webauthn/credentials", nil, other.session())
	assert.JSONEq(t, `{"credentials":[]}`, w.Body.String())
}

func TestWebAuthn_TOTPUserCanAddPasskeyAndUseEither(t *testing.T) {
	e := newWAEnv(t)
	secret, session := e.enableTOTP(e.session())
	recoveryBefore := e.recoveryCount()
	require.Equal(t, int64(10), recoveryBefore)

	va := e.auth()
	_, codes, session := e.enroll(va, "Phone", session)
	// Not the first factor: no session churn, and recovery codes already exist.
	assert.Empty(t, codes)
	assert.Equal(t, recoveryBefore, e.recoveryCount())
	w, _ := e.do("GET", "/webauthn/credentials", nil, session)
	require.Equal(t, http.StatusOK, w.Code)

	// Login offers both methods.
	pending, methods := e.passwordStep()
	assert.Equal(t, []string{"totp", "webauthn"}, methods)

	// Passkey completes login...
	w, cookies := e.passkeyLogin(va, pending)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookies["auth_token"])
	assert.Empty(t, cookies["2fa_pending"].Value, "challenge cookie must be cleared")
	w, _ = e.do("GET", "/users/2fa/status", nil, cookies["auth_token"].Value)
	assert.Equal(t, http.StatusOK, w.Code, "minted token must be a live session")

	// ...and so does TOTP (the step-2 endpoint still works unchanged).
	pending, _ = e.passwordStep()
	w, cookies = doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": totpCode(t, secret)}, ""), pending))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookies["auth_token"])

	// Usage was recorded.
	var row models.WebAuthnCredential
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).First(&row).Error)
	require.NotNil(t, row.LastUsedAt)
	assert.Equal(t, uint32(1), row.SignCount)
}

func TestWebAuthn_DisablingTOTPLeavesPasskeyAndRecoveryCodes(t *testing.T) {
	e := newWAEnv(t)
	secret, session := e.enableTOTP(e.session())
	va := e.auth()
	_, _, session = e.enroll(va, "Phone", session)

	// Step past the TOTP step consumed at enrollment.
	code := totpCodeAt(t, secret, time.Now().Add(30*time.Second))
	w, cookies := e.do("POST", "/users/2fa/disable", map[string]string{"code": code}, session)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	session = cookies["auth_token"].Value

	assert.False(t, e.reload().TOTPEnabled)
	assert.Equal(t, int64(1), e.credentialCount(), "passkey must survive TOTP disable")
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount(), "recovery codes must survive while a passkey remains")

	// Login still demands the passkey.
	pending, methods := e.passwordStep()
	assert.Equal(t, []string{"webauthn"}, methods)
	w, _ = e.passkeyLogin(va, pending)
	assert.Equal(t, http.StatusOK, w.Code)
	_ = session
}

func TestWebAuthn_DisablingTOTPWithoutPasskeyStillDeletesRecoveryCodes(t *testing.T) {
	e := newWAEnv(t)
	secret, session := e.enableTOTP(e.session())
	w, _ := e.do("POST", "/users/2fa/disable", map[string]string{"code": totpCodeAt(t, secret, time.Now().Add(30*time.Second))}, session)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(0), e.recoveryCount())
}

func TestWebAuthn_PasskeyOnlyUserCanRegenerateAndConsumeRecoveryCodes(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	_, codes, session := e.enroll(va, "Key", e.session())
	require.False(t, e.reload().TOTPEnabled, "precondition: TOTP never enabled")

	// Regenerate (was refused for !TOTPEnabled), proving with a recovery code.
	w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": codes[0]}, session)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var regen struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &regen))
	require.Len(t, regen.RecoveryCodes, recoveryCodeCount)
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount())

	// Old codes are dead; a new one logs in through /login/2fa, exactly once.
	pending, _ := e.passwordStep()
	w, _ = doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": codes[1]}, ""), pending))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, cookies := doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": regen.RecoveryCodes[0]}, ""), pending))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotNil(t, cookies["auth_token"])
	w, _ = doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": regen.RecoveryCodes[0]}, ""), pending))
	assert.Equal(t, http.StatusBadRequest, w.Code, "recovery code is single-use")
}

func TestWebAuthn_RegenerateRefusedWithNoFactor(t *testing.T) {
	e := newWAEnv(t)
	w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": "AAAAA-BBBBB-CCCCC"}, e.session())
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestWebAuthn_TOTPEnrollmentReplacesPasskeyOnlyRecoverySet(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	_, codes, session := e.enroll(va, "Key", e.session())
	_, _ = e.enableTOTP(session)
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount(), "one set, not two")
	// The passkey-era codes were replaced.
	assert.False(t, services.ConsumeRecoveryCode(e.db, e.user.ID, codes[0]))
}

func TestWebAuthn_DeleteRequiresSecondFactorProof(t *testing.T) {
	e := newWAEnv(t)
	va1, va2 := e.auth(), e.auth()
	id1, codes, session := e.enroll(va1, "One", e.session())
	_, _, session = e.enroll(va2, "Two", session)
	require.Equal(t, int64(2), e.credentialCount())

	path := "/webauthn/credentials/" + id1

	// No body / empty / wrong proof → rejected, row intact.
	for _, body := range []any{nil, map[string]string{}, map[string]string{"code": "ZZZZZ-ZZZZZ-ZZZZZ"}} {
		w, _ := e.do("DELETE", path, body, session)
		require.Contains(t, []int{http.StatusBadRequest}, w.Code, w.Body.String())
	}
	assert.Equal(t, int64(2), e.credentialCount())
	assert.Equal(t, int64(0), e.auditCount(models.AuditOpWebAuthnRevoke))

	// A live recovery code is accepted; hard delete, audit recorded.
	w, _ := e.do("DELETE", path, map[string]string{"code": codes[0]}, session)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), e.credentialCount())
	var n int64
	require.NoError(t, e.db.Model(&models.WebAuthnCredential{}).Where("id = ?", id1).Count(&n).Error)
	assert.Zero(t, n)
	assert.Equal(t, int64(1), e.auditCount(models.AuditOpWebAuthnRevoke))
	// Recovery code was consumed as proof.
	assert.Equal(t, int64(recoveryCodeCount-1), e.recoveryCount())
}

func TestWebAuthn_DeleteWithAssertionFromDifferentCredential(t *testing.T) {
	e := newWAEnv(t)
	va1, va2 := e.auth(), e.auth()
	id1, _, session := e.enroll(va1, "One", e.session())
	_, _, session = e.enroll(va2, "Two", session)
	path := "/webauthn/credentials/" + id1

	proof := func(va *virtualAuthenticator) map[string]any {
		w, _ := e.do("POST", "/webauthn/assert/begin", nil, session)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		return map[string]any{"assertion": va.assertion(challengeFrom(t, w.Body.Bytes()))}
	}

	// Assertion by the credential being deleted proves nothing new → rejected.
	w, _ := e.do("DELETE", path, proof(va1), session)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, int64(2), e.credentialCount())

	// Assertion without a begun ceremony → rejected.
	w, _ = e.do("DELETE", path, map[string]any{"assertion": va2.assertion("bm90LWEtcmVhbC1jaGFsbGVuZ2U")}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Malformed assertion → rejected.
	w, _ = e.do("POST", "/webauthn/assert/begin", nil, session)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = e.do("DELETE", path, map[string]any{"assertion": map[string]string{"nope": "x"}}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Bad signature (wrong challenge) → rejected.
	w, _ = e.do("POST", "/webauthn/assert/begin", nil, session)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = e.do("DELETE", path, map[string]any{"assertion": va2.assertion("d3JvbmctY2hhbGxlbmdl")}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Different remaining credential → accepted.
	w, _ = e.do("DELETE", path, proof(va2), session)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(1), e.credentialCount())
}

func TestWebAuthn_DeletingLastFactorDropsRecoveryCodesAndEndsSessions(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	id, codes, session := e.enroll(va, "Only", e.session())
	w, cookies := e.do("DELETE", "/webauthn/credentials/"+id, map[string]string{"code": codes[0]}, session)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Zero(t, e.credentialCount())
	assert.Zero(t, e.recoveryCount(), "no factor left → recovery codes go")

	// Old session dead, re-issued one live, and login is password-only again.
	w, _ = e.do("GET", "/users/2fa/status", nil, session)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	w, _ = e.do("GET", "/users/2fa/status", nil, cookies["auth_token"].Value)
	assert.Equal(t, http.StatusOK, w.Code)
	w, lc := doRequest(e.router, mustPost("/login", map[string]string{"identifier": e.user.Username, "password": strongPassword}))
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, lc["auth_token"])
	assert.NotContains(t, w.Body.String(), "two_factor_required")
}

func TestWebAuthn_DeleteKeepsRecoveryCodesWhileTOTPRemains(t *testing.T) {
	e := newWAEnv(t)
	secret, session := e.enableTOTP(e.session())
	va := e.auth()
	id, _, session := e.enroll(va, "Phone", session)
	w, _ := e.do("DELETE", "/webauthn/credentials/"+id, map[string]string{"code": totpCodeAt(t, secret, time.Now().Add(30*time.Second))}, session)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount())
	assert.True(t, e.reload().TOTPEnabled)
}

func TestWebAuthn_DeleteScopedToOwner(t *testing.T) {
	e := newWAEnv(t)
	other := newWAEnv(t)
	otherID, _, _ := other.enroll(other.auth(), "Theirs", other.session())
	_, codes, session := e.enroll(e.auth(), "Mine", e.session())

	w, _ := e.do("DELETE", "/webauthn/credentials/"+otherID, map[string]string{"code": codes[0]}, session)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, int64(1), other.credentialCount(), "another user's passkey must be untouchable")
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount(), "proof must not be spent on a 404")
}

func TestWebAuthn_LoginFailurePaths(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	_, _, _ = e.enroll(va, "Key", e.session())
	pending, _ := e.passwordStep()

	post := func(path string, body any, p *http.Cookie) *httptest.ResponseRecorder {
		req := sessionRequest("POST", path, body, "")
		if p != nil {
			withPending(req, p)
		}
		w, _ := doRequest(e.router, req)
		return w
	}

	// No challenge cookie at all.
	assert.Equal(t, http.StatusBadRequest, post("/webauthn/login/begin", nil, nil).Code)
	assert.Equal(t, http.StatusBadRequest, post("/webauthn/login/finish", nil, nil).Code)
	// Garbage challenge.
	assert.Equal(t, http.StatusUnauthorized, post("/webauthn/login/begin", nil, &http.Cookie{Value: "junk"}).Code)

	// finish without begin.
	assert.Equal(t, http.StatusConflict, post("/webauthn/login/finish", va.assertion("eA"), pending).Code)

	// Bad signature: wrong challenge → 401, audited, no session.
	w := post("/webauthn/login/begin", nil, pending)
	require.Equal(t, http.StatusOK, w.Code)
	real := challengeFrom(t, w.Body.Bytes())
	w = post("/webauthn/login/finish", va.assertion("d3JvbmctY2hhbGxlbmdl"), pending)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, int64(1), e.auditCount(models.AuditOpLoginFailed))

	// The failed attempt consumed the ceremony: even the correct assertion for
	// the original challenge cannot be replayed against it.
	w = post("/webauthn/login/finish", va.assertion(real), pending)
	assert.Equal(t, http.StatusConflict, w.Code)

	// Sign counter regression (cloned authenticator) is rejected.
	w = post("/webauthn/login/begin", nil, pending)
	require.Equal(t, http.StatusOK, w.Code)
	va.signCount = 10
	w = post("/webauthn/login/finish", va.assertion(challengeFrom(t, w.Body.Bytes())), pending)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = post("/webauthn/login/begin", nil, pending)
	require.Equal(t, http.StatusOK, w.Code)
	w = post("/webauthn/login/finish", va.assertionWithCount(challengeFrom(t, w.Body.Bytes()), 3), pending)
	assert.Equal(t, http.StatusUnauthorized, w.Code, "a non-increasing counter signals a clone")
}

func TestWebAuthn_LoginBeginWithoutPasskey(t *testing.T) {
	e := newWAEnv(t)
	_, session := e.enableTOTP(e.session())
	_ = session
	pending, methods := e.passwordStep()
	assert.Equal(t, []string{"totp"}, methods)
	w, _ := doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/begin", nil, ""), pending))
	assert.Equal(t, http.StatusConflict, w.Code)
	w, _ = doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/finish", map[string]string{}, ""), pending))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebAuthn_LoginFinishRateLimited(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	e.enroll(va, "Key", e.session())
	pending, _ := e.passwordStep()

	locked := false
	for i := 0; i < 20 && !locked; i++ {
		w, _ := doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/begin", nil, ""), pending))
		if w.Code != http.StatusOK {
			continue
		}
		w, _ = doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/finish", va.assertion("d3JvbmctY2hhbGxlbmdl"), ""), pending))
		if w.Code == http.StatusTooManyRequests {
			locked = true
		}
	}
	require.True(t, locked, "repeated bad assertions must trigger the per-account lockout")

	// Once locked, even a well-formed ceremony is refused up front.
	w, _ := doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/finish", va.assertion("eA"), ""), pending))
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestWebAuthn_LoginChallengeRejectedAfterAccountLosesFactors(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	e.enroll(va, "Key", e.session())
	pending, _ := e.passwordStep()
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).Delete(&models.WebAuthnCredential{}).Error)
	w, _ := doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": "000000"}, ""), pending))
	assert.Equal(t, http.StatusUnauthorized, w.Code, "challenge minted against a since-removed factor must not work")
}

func TestWebAuthn_RegisterFailurePaths(t *testing.T) {
	e := newWAEnv(t)
	tok := e.session()

	// finish without begin.
	w, _ := e.do("POST", "/webauthn/register/finish", e.auth().attestation("eA"), tok)
	assert.Equal(t, http.StatusConflict, w.Code)

	// name too long / bad JSON.
	w, _ = e.do("POST", "/webauthn/register/begin", map[string]string{"name": strings.Repeat("x", 101)}, tok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	req := sessionRequest("POST", "/webauthn/register/begin", nil, tok)
	req.Body = http.NoBody
	req.ContentLength = 5
	req.Body = nopCloser("{bad")
	w2, _ := doRequest(e.router, req)
	assert.Equal(t, http.StatusBadRequest, w2.Code)

	// A begin with no body at all is fine and gets a default label.
	req = sessionRequest("POST", "/webauthn/register/begin", nil, tok)
	req.Body = http.NoBody
	req.ContentLength = 0
	w2, _ = doRequest(e.router, req)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	va := e.auth()
	w, _ = e.do("POST", "/webauthn/register/finish", va.attestation(challengeFrom(t, w2.Body.Bytes())), tok)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var row models.WebAuthnCredential
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).First(&row).Error)
	assert.True(t, strings.HasPrefix(row.Name, "Passkey "), row.Name)

	// Wrong challenge → 400, nothing stored.
	tok = e.session()
	w, _ = e.do("POST", "/webauthn/register/begin", map[string]string{"name": "x"}, tok)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = e.do("POST", "/webauthn/register/finish", e.auth().attestation("d3JvbmctY2hhbGxlbmdl"), tok)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, int64(1), e.credentialCount())

	// Duplicate credential id (an authenticator ignoring excludeCredentials) →
	// 409 from the (user_id, credential_id) natural key, not a 500.
	w, _ = e.do("POST", "/webauthn/register/begin", map[string]string{"name": "dup"}, tok)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = e.do("POST", "/webauthn/register/finish", va.attestation(challengeFrom(t, w.Body.Bytes())), tok)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, int64(1), e.credentialCount())
}

func TestWebAuthn_RegisterBeginExcludesExistingCredentials(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	_, _, session := e.enroll(va, "One", e.session())
	w, _ := e.do("POST", "/webauthn/register/begin", nil, session)
	require.Equal(t, http.StatusOK, w.Code)
	var opts struct {
		PublicKey struct {
			Exclude []struct {
				ID string `json:"id"`
			} `json:"excludeCredentials"`
		} `json:"publicKey"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &opts))
	require.Len(t, opts.PublicKey.Exclude, 1)
	assert.Equal(t, b64(va.credentialID), opts.PublicKey.Exclude[0].ID)
}

func TestWebAuthn_OIDCUserExcluded(t *testing.T) {
	e := newWAEnv(t)
	sub := "oidc-subject"
	require.NoError(t, e.db.Model(&models.User{}).Where("id = ?", e.user.ID).Update("oidc_subject", sub).Error)
	tok := e.session()
	w, _ := e.do("POST", "/webauthn/register/begin", nil, tok)
	assert.Equal(t, http.StatusForbidden, w.Code)
	w, _ = e.do("POST", "/webauthn/register/finish", map[string]string{}, tok)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestWebAuthn_RequiresConcreteFrontendURL(t *testing.T) {
	for _, origin := range []string{"*", "", "not a url", "ftp://crm.example.com"} {
		e := newWAEnv(t)
		e.cfg.FrontendURL = origin
		tok := e.session()
		for _, path := range []string{"/webauthn/register/begin", "/webauthn/register/finish", "/webauthn/assert/begin"} {
			w, _ := e.do("POST", path, nil, tok)
			assert.Equal(t, http.StatusConflict, w.Code, "%q %s", origin, path)
			assert.Contains(t, w.Body.String(), "FRONTEND_URL")
		}
		w, _ := doRequest(e.router, sessionRequest("POST", "/webauthn/login/begin", nil, ""))
		assert.Equal(t, http.StatusConflict, w.Code, origin)
		w, _ = doRequest(e.router, sessionRequest("POST", "/webauthn/login/finish", nil, ""))
		assert.Equal(t, http.StatusConflict, w.Code, origin)
	}
}

func TestWebAuthn_ProofBeginNeedsAPasskey(t *testing.T) {
	e := newWAEnv(t)
	w, _ := e.do("POST", "/webauthn/assert/begin", nil, e.session())
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestWebAuthn_BackupEligibleFlagsRoundTrip(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.backupFlags = 0x08 | 0x10 // synced passkey: BE + BS
	e.enroll(va, "iCloud", e.session())
	var row models.WebAuthnCredential
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).First(&row).Error)
	assert.True(t, row.BackupEligible)
	assert.True(t, row.BackupState)

	// The library rejects an assertion whose BE flag flipped — proves the flag
	// is persisted and fed back, not defaulted.
	pending, _ := e.passwordStep()
	w, _ := e.passkeyLogin(va, pending)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	va.backupFlags = 0
	pending, _ = e.passwordStep()
	w, _ = e.passkeyLogin(va, pending)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestWebAuthn_LoginReportsMethodsOnlyWhenFactorEnrolled(t *testing.T) {
	e := newWAEnv(t)
	w, cookies := doRequest(e.router, mustPost("/login", map[string]string{"identifier": e.user.Username, "password": strongPassword}))
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, cookies["auth_token"])
	assert.NotContains(t, w.Body.String(), "methods")
}

func TestWebAuthn_LoginQueryFailureSurfaces(t *testing.T) {
	e := newWAEnv(t)
	dbtest.HideTable(t, e.db, "webauthn_credentials")
	w, _ := doRequest(e.router, mustPost("/login", map[string]string{"identifier": e.user.Username, "password": strongPassword}))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }

func nopCloser(s string) readCloser { return readCloser{strings.NewReader(s)} }

// A failing store must surface as a 500, never as a silently-empty passkey list
// or a bypassed check (trap #4 spirit: a swallowed DB error on an auth path is a
// security bug, not a cosmetic one).
func TestWebAuthn_StoreFailuresSurfaceAs500(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	id, codes, session := e.enroll(va, "Key", e.session())
	pending, _ := e.passwordStep()
	// A registration ceremony in flight, so finish gets past the ceremony lookup.
	w, _ := e.do("POST", "/webauthn/register/begin", nil, session)
	require.Equal(t, http.StatusOK, w.Code)
	att := e.auth().attestation(challengeFrom(t, w.Body.Bytes()))

	dbtest.HideTable(t, e.db, "webauthn_credentials")

	for _, tc := range []struct {
		method, path string
		body         any
		cookie       *http.Cookie
	}{
		{"POST", "/webauthn/register/begin", nil, nil},
		{"POST", "/webauthn/register/finish", att, nil},
		{"POST", "/webauthn/assert/begin", nil, nil},
		{"GET", "/webauthn/credentials", nil, nil},
		{"DELETE", "/webauthn/credentials/" + id, map[string]string{"code": codes[0]}, nil},
		{"POST", "/webauthn/login/begin", nil, pending},
		{"POST", "/webauthn/login/finish", va.assertion("eA"), pending},
		{"POST", "/login/2fa", map[string]string{"code": "000000"}, pending},
		{"POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": codes[1]}, nil},
	} {
		req := sessionRequest(tc.method, tc.path, tc.body, session)
		if tc.cookie != nil {
			withPending(req, tc.cookie)
		}
		w, _ := doRequest(e.router, req)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
	}
}

func TestWebAuthn_RegisterFinishRecoveryLookupFailure(t *testing.T) {
	e := newWAEnv(t)
	tok := e.session()
	va := e.auth()
	w, _ := e.do("POST", "/webauthn/register/begin", nil, tok)
	require.Equal(t, http.StatusOK, w.Code)
	att := va.attestation(challengeFrom(t, w.Body.Bytes()))
	dbtest.HideTable(t, e.db, "recovery_codes")
	w, _ = e.do("POST", "/webauthn/register/finish", att, tok)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Zero(t, e.credentialCount(), "nothing may be stored when the recovery-code check fails")
}

func TestWebAuthn_PendingChallengeUserGone(t *testing.T) {
	e := newWAEnv(t)
	e.enroll(e.auth(), "Key", e.session())
	pending, _ := e.passwordStep()
	require.NoError(t, e.db.Unscoped().Delete(&models.User{}, e.user.ID).Error)
	for _, path := range []string{"/webauthn/login/begin", "/webauthn/login/finish"} {
		w, _ := doRequest(e.router, withPending(sessionRequest("POST", path, nil, ""), pending))
		assert.Equal(t, http.StatusUnauthorized, w.Code, path)
	}
}

func TestWebAuthn_PendingChallengeUserLookupFailure(t *testing.T) {
	e := newWAEnv(t)
	e.enroll(e.auth(), "Key", e.session())
	pending, _ := e.passwordStep()
	dbtest.HideTable(t, e.db, "users")
	w, _ := doRequest(e.router, withPending(sessionRequest("POST", "/webauthn/login/begin", nil, ""), pending))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestWebAuthn_DeleteAssertionRejectedWhenRelyingPartyUnconfigured(t *testing.T) {
	e := newWAEnv(t)
	va1, va2 := e.auth(), e.auth()
	id1, _, session := e.enroll(va1, "One", e.session())
	_, _, session = e.enroll(va2, "Two", session)
	e.cfg.FrontendURL = "*"
	w, _ := e.do("DELETE", "/webauthn/credentials/"+id1, map[string]any{"assertion": va2.assertion("eA")}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, int64(2), e.credentialCount())
}

func TestWebAuthn_DeleteRejectsMalformedBody(t *testing.T) {
	e := newWAEnv(t)
	id, _, session := e.enroll(e.auth(), "One", e.session())
	req := sessionRequest("DELETE", "/webauthn/credentials/"+id, nil, session)
	req.Body = nopCloser("{not json")
	req.ContentLength = 9
	w, _ := doRequest(e.router, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
