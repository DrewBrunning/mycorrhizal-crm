package controllers

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pendingTOTPSecret runs POST /users/2fa/setup WITHOUT confirming, leaving a
// pending (totp_enabled = false) secret on the account, and returns it.
func (e *waEnv) pendingTOTPSecret(token string) string {
	e.t.Helper()
	var proof map[string]any
	if e.lastAuth != nil {
		proof = e.proofFrom(e.lastAuth, token) // issue #1337
	}
	w, _ := e.do("POST", "/users/2fa/setup", proof, token)
	require.Equal(e.t, http.StatusOK, w.Code, w.Body.String())
	var setup struct {
		Secret string `json:"secret"`
	}
	require.NoError(e.t, json.Unmarshal(w.Body.Bytes(), &setup))
	u := e.reload()
	require.False(e.t, u.TOTPEnabled)
	require.NotNil(e.t, u.TOTPSecretEncrypted)
	return setup.Secret
}

// Issue #1306: a pending (never-confirmed) TOTP secret on a passkey-only
// account must not be accepted as a second factor anywhere.
func TestWebAuthn_PendingTOTPSecretNeverSatisfiesProof(t *testing.T) {
	e := newWAEnv(t)
	va := e.auth()
	id, codes, session := e.enroll(va, "Key", e.session())
	secret := e.pendingTOTPSecret(session)
	code := totpCodeAt(t, secret, time.Now())

	// /login/2fa: rejected, no session cookie.
	pending, methods := e.passwordStep()
	assert.Equal(t, []string{"webauthn"}, methods)
	w, cookies := doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": code}, ""), pending))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Nil(t, cookies["auth_token"])

	// Recovery-code regeneration: rejected, codes untouched.
	w, _ = e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": code}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount())

	// Passkey deletion: rejected, credential intact.
	w, _ = e.do("DELETE", "/webauthn/credentials/"+id, map[string]string{"code": code}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, int64(1), e.credentialCount())
	assert.False(t, e.reload().TOTPEnabled)

	// Recovery codes still work for the passkey-only account: login, then regenerate.
	w, cookies = doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": codes[0]}, ""), pending))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotNil(t, cookies["auth_token"])
	w, _ = e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": codes[1]}, session)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// The confirmed-TOTP paths keep working: after setup+confirm the secret is a
// valid proof for login, regenerate and passkey deletion. Each runs in its own
// account because a TOTP step is single-use (issue #873) and only ±1 step is
// accepted.
func TestWebAuthn_ConfirmedTOTPStillSatisfiesProof(t *testing.T) {
	setup := func(t *testing.T) (*waEnv, string, string, string) {
		e := newWAEnv(t)
		secret, session := e.enableTOTP(e.session())
		id, _, session := e.enroll(e.auth(), "Key", session)
		return e, secret, id, session
	}
	code := totpCodeAt

	t.Run("login", func(t *testing.T) {
		e, secret, _, _ := setup(t)
		pending, _ := e.passwordStep()
		w, cookies := doRequest(e.router, withPending(sessionRequest("POST", "/login/2fa", map[string]string{"code": code(t, secret, time.Now())}, ""), pending))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.NotNil(t, cookies["auth_token"])
	})
	t.Run("regenerate", func(t *testing.T) {
		e, secret, _, session := setup(t)
		w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": code(t, secret, time.Now())}, session)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
	t.Run("delete passkey", func(t *testing.T) {
		e, secret, id, session := setup(t)
		w, _ := e.do("DELETE", "/webauthn/credentials/"+id, map[string]string{"code": code(t, secret, time.Now())}, session)
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}
