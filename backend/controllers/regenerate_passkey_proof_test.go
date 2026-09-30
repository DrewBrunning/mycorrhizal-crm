package controllers

import (
	"encoding/json"
	"net/http"
	"testing"

	"mycorrhizal/middleware"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1354: a passkey-only account (no TOTP) regenerates recovery codes
// with an assertion from one of its passkeys.

func passkeyOnlyEnv(t *testing.T) (e *waEnv, va *virtualAuthenticator, tok string, codes []string) {
	t.Helper()
	e = newWAEnv(t)
	t.Cleanup(func() { middleware.GetAccountRateLimiter().RecordSuccessfulLogin(secondFactorProofLockKey(e.user.ID)) })
	va = e.auth()
	_, codes, tok = e.enroll(va, "Key", e.session())
	require.False(t, e.reload().TOTPEnabled)
	return e, va, tok, codes
}

func TestRegenerateRecoveryCodes_PasskeyOnlyAssertion(t *testing.T) {
	e, va, tok, oldCodes := passkeyOnlyEnv(t)

	w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", e.proofFrom(va, tok), tok)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var out struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Len(t, out.RecoveryCodes, recoveryCodeCount)
	assert.False(t, e.hasRecoveryCode(oldCodes[0]), "the old set is dead")
	assert.True(t, e.hasRecoveryCode(out.RecoveryCodes[0]), "the new set is live")
	assert.Equal(t, int64(recoveryCodeCount), e.recoveryCount())
}

func TestRegenerateRecoveryCodes_PasskeyOnlyStillAcceptsRecoveryCode(t *testing.T) {
	e, _, tok, codes := passkeyOnlyEnv(t)
	w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]string{"code": codes[0]}, tok)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestRegenerateRecoveryCodes_AssertionWithoutCeremonyRejected(t *testing.T) {
	e, va, tok, oldCodes := passkeyOnlyEnv(t)
	// No assert/begin: the assertion has no ceremony behind it.
	w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", map[string]any{"assertion": va.assertion("bm8tY2VyZW1vbnk")}, tok)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.True(t, e.hasRecoveryCode(oldCodes[0]), "nothing changed")
}

func TestRegenerateRecoveryCodes_WrongAssertionsCountTowardLockout(t *testing.T) {
	e, va, tok, oldCodes := passkeyOnlyEnv(t)
	stranger := e.auth() // a passkey the account does not hold

	var last int
	for i := 0; i < middleware.MaxLoginAttempts; i++ {
		w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", e.proofFrom(stranger, tok), tok)
		last = w.Code
		if i < middleware.MaxLoginAttempts-1 {
			require.Equal(t, http.StatusBadRequest, last, "miss %d", i+1)
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, last)

	// Locked: even the owner's genuine assertion is refused, and nothing changes.
	w, _ := e.do("POST", "/users/2fa/recovery-codes/regenerate", e.proofFrom(va, tok), tok)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.True(t, e.hasRecoveryCode(oldCodes[0]))
}
