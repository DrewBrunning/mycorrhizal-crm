package controllers

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1337: enrolling an ADDITIONAL second factor needs a live proof, the
// same bar as removing one. The first factor stays proof-free.

// resetEnrollLock clears the enrollment-proof lockout bucket so a lockout test
// cannot leak into another test sharing the process-wide limiter.
func resetEnrollLock(t *testing.T, e *waEnv) {
	t.Helper()
	t.Cleanup(func() { middleware.GetAccountRateLimiter().RecordSuccessfulLogin(enrollProofLockKey(e.user.ID)) })
}

func TestEnrollmentProof_FirstFactorNeedsNoProof(t *testing.T) {
	t.Run("passkey", func(t *testing.T) {
		e := newWAEnv(t)
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"name": "First"}, e.session())
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
	t.Run("totp", func(t *testing.T) {
		e := newWAEnv(t)
		w, _ := e.do("POST", "/users/2fa/setup", nil, e.session())
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
	t.Run("pending totp secret is not a factor", func(t *testing.T) {
		e := newWAEnv(t)
		tok := e.session()
		e.pendingTOTPSecret(tok)
		// A second setup and a first passkey still need nothing: nothing is confirmed.
		w, _ := e.do("POST", "/users/2fa/setup", nil, tok)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w, _ = e.do("POST", "/webauthn/register/begin", nil, tok)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}

func TestEnrollmentProof_PasskeyToPasskeySessionOnlyRejected(t *testing.T) {
	e := newWAEnv(t)
	owner := e.auth()
	_, _, session := e.enroll(owner, "Owner", e.session())
	attacker := e.auth()

	// Begin with only a session (no body, empty body, empty strings) → 400.
	for _, body := range []any{nil, map[string]string{"name": "Evil"}, map[string]string{"name": "Evil", "code": ""}} {
		w, _ := e.do("POST", "/webauthn/register/begin", body, session)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	}
	// Finish cannot be reached without a begun ceremony.
	w, _ := e.do("POST", "/webauthn/register/finish", attacker.attestation("eA"), session)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, int64(1), e.credentialCount())

	// So the attacker passkey can never become the "other passkey" that
	// removes the owner's.
	assert.Equal(t, int64(1), e.credentialCount())
}

func TestEnrollmentProof_PasskeyToPasskeyProofs(t *testing.T) {
	t.Run("assertion from an existing passkey is accepted", func(t *testing.T) {
		e := newWAEnv(t)
		owner := e.auth()
		_, _, session := e.enroll(owner, "Owner", e.session())
		w, _ := e.registerWith(e.auth(), "Second", session, e.proofFrom(owner, session))
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, int64(2), e.credentialCount())
	})
	t.Run("recovery code is accepted and consumed", func(t *testing.T) {
		e := newWAEnv(t)
		_, codes, session := e.enroll(e.auth(), "Owner", e.session())
		w, _ := e.registerWith(e.auth(), "Second", session, map[string]any{"code": codes[0]})
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, int64(recoveryCodeCount-1), e.recoveryCount())
	})
	t.Run("wrong code is rejected and nothing is stored", func(t *testing.T) {
		e := newWAEnv(t)
		resetEnrollLock(t, e)
		_, _, session := e.enroll(e.auth(), "Owner", e.session())
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"code": "ZZZZZ-ZZZZZ-ZZZZZ"}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		w, _ = e.do("POST", "/webauthn/register/finish", e.auth().attestation("eA"), session)
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, int64(1), e.credentialCount())
	})
	t.Run("bad assertions are rejected", func(t *testing.T) {
		e := newWAEnv(t)
		resetEnrollLock(t, e)
		owner := e.auth()
		_, _, session := e.enroll(owner, "Owner", e.session())

		// No proof ceremony begun.
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]any{"assertion": owner.assertion("bm90LWEtcmVhbC1jaGFsbGVuZ2U")}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		// Malformed assertion.
		w, _ = e.do("POST", "/webauthn/assert/begin", nil, session)
		require.Equal(t, http.StatusOK, w.Code)
		w, _ = e.do("POST", "/webauthn/register/begin", map[string]any{"assertion": map[string]string{"nope": "x"}}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		// Attacker's own authenticator answering the owner's ceremony.
		w, _ = e.do("POST", "/webauthn/assert/begin", nil, session)
		require.Equal(t, http.StatusOK, w.Code)
		w, _ = e.do("POST", "/webauthn/register/begin", map[string]any{"assertion": e.auth().assertion(challengeFrom(t, w.Body.Bytes()))}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		// A proof ceremony is single-use: a replayed valid assertion fails.
		w, _ = e.do("POST", "/webauthn/assert/begin", nil, session)
		require.Equal(t, http.StatusOK, w.Code)
		proof := map[string]any{"assertion": owner.assertion(challengeFrom(t, w.Body.Bytes()))}
		w, _ = e.do("POST", "/webauthn/register/begin", proof, session)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w, _ = e.do("POST", "/webauthn/register/begin", proof, session)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, int64(1), e.credentialCount())
	})
}

func TestEnrollmentProof_TOTPToPasskey(t *testing.T) {
	t.Run("session-only rejected", func(t *testing.T) {
		e := newWAEnv(t)
		_, session := e.enableTOTP(e.session())
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"name": "Evil"}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		w, _ = e.do("POST", "/webauthn/register/finish", e.auth().attestation("eA"), session)
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Zero(t, e.credentialCount())
	})
	t.Run("wrong code rejected", func(t *testing.T) {
		e := newWAEnv(t)
		resetEnrollLock(t, e)
		_, session := e.enableTOTP(e.session())
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"code": "000000"}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Zero(t, e.credentialCount())
	})
	t.Run("valid TOTP code accepted", func(t *testing.T) {
		e := newWAEnv(t)
		secret, session := e.enableTOTP(e.session())
		// Step past the one consumed by confirm.
		code := totpCodeAt(t, secret, time.Now().Add(30*time.Second))
		w, _ := e.registerWith(e.auth(), "Phone", session, map[string]any{"code": code})
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, int64(1), e.credentialCount())
	})
	t.Run("valid recovery code accepted", func(t *testing.T) {
		e := newWAEnv(t)
		_, session := e.enableTOTP(e.session())
		w, _ := e.registerWith(e.auth(), "Phone", session, map[string]any{"code": e.codes[0]})
		assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	})
	t.Run("pending secret is not a proof", func(t *testing.T) {
		// A passkey-only account with a never-confirmed TOTP secret: the code
		// from that secret must not authorise a further enrollment (#1306).
		e := newWAEnv(t)
		resetEnrollLock(t, e)
		_, _, session := e.enroll(e.auth(), "Key", e.session())
		secret := e.pendingTOTPSecret(session)
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"code": totpCodeAt(t, secret, time.Now())}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Equal(t, int64(1), e.credentialCount())
	})
}

func TestEnrollmentProof_SetupTwoFactorOnPasskeyOnlyAccount(t *testing.T) {
	pendingSecret := func(t *testing.T, e *waEnv) bool {
		u := e.reload()
		return u.TOTPSecretEncrypted != nil && *u.TOTPSecretEncrypted != ""
	}
	t.Run("without proof rejected and no secret minted", func(t *testing.T) {
		e := newWAEnv(t)
		_, _, session := e.enroll(e.auth(), "Key", e.session())
		for _, body := range []any{nil, map[string]string{}, map[string]string{"code": ""}} {
			w, _ := e.do("POST", "/users/2fa/setup", body, session)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		}
		assert.False(t, pendingSecret(t, e))
		// Confirm has nothing to confirm either.
		w, _ := e.do("POST", "/users/2fa/confirm", map[string]string{"code": "123456"}, session)
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.False(t, e.reload().TOTPEnabled)
	})
	t.Run("wrong code rejected", func(t *testing.T) {
		e := newWAEnv(t)
		resetEnrollLock(t, e)
		_, _, session := e.enroll(e.auth(), "Key", e.session())
		w, _ := e.do("POST", "/users/2fa/setup", map[string]string{"code": "ZZZZZ-ZZZZZ-ZZZZZ"}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.False(t, pendingSecret(t, e))
	})
	t.Run("recovery code accepted", func(t *testing.T) {
		e := newWAEnv(t)
		_, codes, session := e.enroll(e.auth(), "Key", e.session())
		w, _ := e.do("POST", "/users/2fa/setup", map[string]string{"code": codes[0]}, session)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.True(t, pendingSecret(t, e))
	})
	t.Run("passkey assertion accepted, then TOTP confirms", func(t *testing.T) {
		e := newWAEnv(t)
		va := e.auth()
		_, _, session := e.enroll(va, "Key", e.session())
		secret, _ := e.enableTOTP(session)
		assert.True(t, e.reload().TOTPEnabled)
		assert.NotEmpty(t, secret)
	})
	t.Run("malformed body rejected", func(t *testing.T) {
		e := newWAEnv(t)
		_, _, session := e.enroll(e.auth(), "Key", e.session())
		req := sessionRequest("POST", "/users/2fa/setup", nil, session)
		req.ContentLength = 5
		req.Body = nopCloser("{bad")
		w, _ := doRequest(e.router, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
	t.Run("passkey lookup failure surfaces as 500", func(t *testing.T) {
		e := newWAEnv(t)
		session := e.session()
		dbtest.HideTable(t, e.db, "webauthn_credentials")
		w, _ := e.do("POST", "/users/2fa/setup", nil, session)
		assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	})
	t.Run("already-enabled TOTP still conflicts", func(t *testing.T) {
		e := newWAEnv(t)
		_, session := e.enableTOTP(e.session())
		w, _ := e.do("POST", "/users/2fa/setup", nil, session)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
	t.Run("OIDC account still excluded", func(t *testing.T) {
		e := newWAEnv(t)
		sub := "oidc-subject-1337"
		require.NoError(t, e.db.Model(&models.User{}).Where("id = ?", e.user.ID).Update("oidc_subject", sub).Error)
		w, _ := e.do("POST", "/users/2fa/setup", nil, e.session())
		assert.Equal(t, http.StatusForbidden, w.Code)
		w, _ = e.do("POST", "/webauthn/register/begin", nil, e.session())
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestEnrollmentProof_GuessingIsLockedOut(t *testing.T) {
	e := newWAEnv(t)
	resetEnrollLock(t, e)
	_, codes, session := e.enroll(e.auth(), "Owner", e.session())

	var last int
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"code": "ZZZZZ-ZZZZZ-ZZZZZ"}, session)
		last = w.Code
	}
	assert.Equal(t, http.StatusTooManyRequests, last, "the attempt that trips the limit is refused with 429")

	// Locked: even a genuine proof is refused on both enrollment routes, and
	// the recovery code is not consumed.
	w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"code": codes[0]}, session)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	w, _ = e.do("POST", "/users/2fa/setup", map[string]string{"code": codes[0]}, session)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount())

	// It never locks the owner's sign-in.
	pending, _ := e.passwordStep()
	assert.NotNil(t, pending)
}

func TestEnrollmentProof_AttackFromStolenSessionCannotRemoveOwnerPasskey(t *testing.T) {
	e := newWAEnv(t)
	resetEnrollLock(t, e)
	owner := e.auth()
	ownerID, _, session := e.enroll(owner, "Owner", e.session())

	// The attacker holds only the session. They cannot enroll their own
	// authenticator, so they have no "different passkey" to prove removal with.
	attacker := e.auth()
	w, _ := e.do("POST", "/webauthn/register/begin", map[string]string{"name": strings.Repeat("a", 5)}, session)
	require.Equal(t, http.StatusBadRequest, w.Code)
	w, _ = e.do("POST", "/webauthn/register/finish", attacker.attestation("eA"), session)
	require.Equal(t, http.StatusConflict, w.Code)
	w, _ = e.do("DELETE", "/webauthn/credentials/"+ownerID, map[string]any{"assertion": attacker.assertion("eA")}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, int64(1), e.credentialCount())
}
