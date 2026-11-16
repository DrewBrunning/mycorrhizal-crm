package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passwordlessBegin starts a client-side discoverable login and returns the
// ceremony token and challenge.
func (e *waEnv) passwordlessBegin() (token, challenge string) {
	e.t.Helper()
	w, _ := e.do("POST", "/webauthn/login/passwordless/begin", nil, "")
	require.Equal(e.t, http.StatusOK, w.Code, "passwordless/begin: %s", w.Body.String())
	var out struct {
		CeremonyToken string `json:"ceremony_token"`
		PublicKey     struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &out))
	require.NotEmpty(e.t, out.CeremonyToken)
	require.NotEmpty(e.t, out.PublicKey.Challenge)
	return out.CeremonyToken, out.PublicKey.Challenge
}

// passwordlessFinish submits the assertion for a begun passwordless ceremony.
func (e *waEnv) passwordlessFinish(token string, assertion map[string]any) (*httptest.ResponseRecorder, map[string]*http.Cookie) {
	e.t.Helper()
	return e.do("POST", "/webauthn/login/passwordless/finish", map[string]any{
		"ceremony_token": token,
		"assertion":      assertion,
	}, "")
}

// TestWebAuthn_PasswordlessLoginNeedsNeitherUsernameNorPassword is the core
// regression for discussion #2355: a discoverable passkey signs in directly,
// with no /login step and no 2fa_pending challenge cookie.
func TestWebAuthn_PasswordlessLoginNeedsNeitherUsernameNorPassword(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	token, challenge := e.passwordlessBegin()
	w, cookies := e.passwordlessFinish(token, va.assertion(challenge))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookies["auth_token"], "passwordless login must mint a session")
	assert.Empty(t, cookies["2fa_pending"].Value, "no second-factor challenge is involved")

	// The minted token is a real session, not just a cookie-shaped response.
	w, _ = e.do("GET", "/users/2fa/status", nil, cookies["auth_token"].Value)
	assert.Equal(t, http.StatusOK, w.Code)

	var row models.WebAuthnCredential
	require.NoError(t, e.db.Where("user_id = ?", e.user.ID).First(&row).Error)
	require.NotNil(t, row.LastUsedAt, "passwordless use must be recorded")
	assert.Equal(t, uint32(1), row.SignCount)

	// The ceremony token is single-use: a replayed finish is refused even with
	// the same (valid) assertion.
	w, _ = e.passwordlessFinish(token, va.assertion(challenge))
	assert.Equal(t, http.StatusConflict, w.Code)
}

// TestWebAuthn_PasswordlessLoginIsAvailableToTOTPAccounts pins the chosen
// policy: a passkey is a full credential, so it alone mints a session even
// when the account also has TOTP enrolled.
func TestWebAuthn_PasswordlessLoginIsAvailableToTOTPAccounts(t *testing.T) {
	e := newWAEnv(t)
	_, session := e.enableTOTP(e.session())
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Phone", session)

	token, challenge := e.passwordlessBegin()
	w, cookies := e.passwordlessFinish(token, va.assertion(challenge))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookies["auth_token"])
}

// TestWebAuthn_PasswordlessLoginRequiresUserVerification: without UV the
// passkey would be a bare possession factor, not a password replacement.
func TestWebAuthn_PasswordlessLoginRequiresUserVerification(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	token, challenge := e.passwordlessBegin()
	va.omitUV = true
	w, cookies := e.passwordlessFinish(token, va.assertion(challenge))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, cookies["auth_token"])
}

// TestWebAuthn_PasswordlessLoginUnknownCredential: a credential the server has
// never seen must not resolve an account, and must not be attributed to one.
func TestWebAuthn_PasswordlessLoginUnknownCredential(t *testing.T) {
	e := newWAEnv(t)
	token, challenge := e.passwordlessBegin()
	stranger := e.auth()
	stranger.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	w, cookies := e.passwordlessFinish(token, stranger.assertion(challenge))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, cookies["auth_token"])
	assert.Zero(t, e.auditCount(models.AuditOpLoginFailed), "an unknown credential is not a known-account failure")
}

// TestWebAuthn_PasswordlessLoginRejectsSpoofedUserHandle: the credential id is
// the lookup key, and the library must reject a user handle that does not match
// its owner.
func TestWebAuthn_PasswordlessLoginRejectsSpoofedUserHandle(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	token, challenge := e.passwordlessBegin()
	va.userHandle = []byte("user-999999")
	w, cookies := e.passwordlessFinish(token, va.assertion(challenge))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, cookies["auth_token"])
}

// TestWebAuthn_PasswordlessLoginRejectsWrongChallenge covers a forged signature
// over the wrong challenge and the clone signal.
func TestWebAuthn_PasswordlessLoginRejectsWrongChallenge(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	token, _ := e.passwordlessBegin()
	w, cookies := e.passwordlessFinish(token, va.assertion("d3JvbmctY2hhbGxlbmdl"))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Nil(t, cookies["auth_token"])
	assert.Equal(t, int64(1), e.auditCount(models.AuditOpLoginFailed))

	token, challenge := e.passwordlessBegin()
	va.signCount = 10
	w, _ = e.passwordlessFinish(token, va.assertion(challenge))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	token, challenge = e.passwordlessBegin()
	w, _ = e.passwordlessFinish(token, va.assertionWithCount(challenge, 3))
	assert.Equal(t, http.StatusUnauthorized, w.Code, "a non-increasing counter signals a clone")
}

// TestWebAuthn_PasswordlessLoginLockout: repeated failures hit the same
// per-account lockout as every other sign-in path.
func TestWebAuthn_PasswordlessLoginLockout(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	locked := false
	for i := 0; i < 20 && !locked; i++ {
		token, _ := e.passwordlessBegin()
		w, _ := e.passwordlessFinish(token, va.assertion("d3JvbmctY2hhbGxlbmdl"))
		if w.Code == http.StatusTooManyRequests {
			locked = true
		}
	}
	require.True(t, locked, "repeated bad assertions must trigger the per-account lockout")

	token, challenge := e.passwordlessBegin()
	w, cookies := e.passwordlessFinish(token, va.assertion(challenge))
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Nil(t, cookies["auth_token"])
}

// TestWebAuthn_PasswordlessLoginRejectsBadRequests covers the missing/bad
// ceremony envelope and a non-concrete FRONTEND_URL.
func TestWebAuthn_PasswordlessLoginRejectsBadRequests(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	w, _ := e.do("POST", "/webauthn/login/passwordless/finish", nil, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = e.do("POST", "/webauthn/login/passwordless/finish", map[string]any{"ceremony_token": "x"}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = e.do("POST", "/webauthn/login/passwordless/finish", map[string]any{"ceremony_token": "unknown", "assertion": va.assertion("eA")}, "")
	assert.Equal(t, http.StatusConflict, w.Code)

	e.cfg.FrontendURL = "*"
	w, _ = e.do("POST", "/webauthn/login/passwordless/begin", nil, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	w, _ = e.do("POST", "/webauthn/login/passwordless/finish", map[string]any{"ceremony_token": "x", "assertion": va.assertion("eA")}, "")
	assert.Equal(t, http.StatusConflict, w.Code)
}

// TestWebAuthn_PasswordlessLoginEnvelopeBranches covers the missing-token and
// unparseable-assertion paths.
func TestWebAuthn_PasswordlessLoginEnvelopeBranches(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())

	w, _ := e.do("POST", "/webauthn/login/passwordless/finish", map[string]any{
		"assertion": va.assertion("eA"),
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "a blank ceremony token is a missing field")

	token, _ := e.passwordlessBegin()
	w, _ = e.do("POST", "/webauthn/login/passwordless/finish", map[string]any{
		"ceremony_token": token,
		"assertion":      "not-an-object",
	}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, w.Result().Cookies())
}

// TestWebAuthn_PasswordlessLoginTokenGenerationFailure: the otherwise
// unreachable crypto/rand failure still answers 500, never a session.
func TestWebAuthn_PasswordlessLoginTokenGenerationFailure(t *testing.T) {
	e := newWAEnv(t)
	old := generateCeremonyToken
	generateCeremonyToken = func() (string, error) { return "", errors.New("entropy exhausted") }
	t.Cleanup(func() { generateCeremonyToken = old })

	w, cookies := e.do("POST", "/webauthn/login/passwordless/begin", nil, "")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Nil(t, cookies["auth_token"])
}

// TestWebAuthn_PasswordlessLoginStoreFailureIs500: a failing store must never
// become a silently accepted login or a misleading 401.
func TestWebAuthn_PasswordlessLoginStoreFailureIs500(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	va.userHandle = []byte(fmt.Sprintf("user-%d", e.user.ID))
	_, _, _ = e.enroll(va, "Laptop", e.session())
	token, challenge := e.passwordlessBegin()

	dbtest.HideTable(t, e.db, "users")
	w, cookies := e.passwordlessFinish(token, va.assertion(challenge))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Nil(t, cookies["auth_token"])

	e2 := newWAEnv(t)
	va2 := e2.auth()
	va2.userHandle = []byte(fmt.Sprintf("user-%d", e2.user.ID))
	_, _, _ = e2.enroll(va2, "Laptop", e2.session())
	token, challenge = e2.passwordlessBegin()
	dbtest.HideTable(t, e2.db, "webauthn_credentials")
	w, cookies = e2.passwordlessFinish(token, va2.assertion(challenge))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Nil(t, cookies["auth_token"])
}
