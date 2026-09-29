package controllers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allowedIDs decodes the allowCredentials ids from an assert/begin response.
func allowedIDs(t *testing.T, body []byte) []string {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Allow []struct {
				ID string `json:"id"`
			} `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	require.NoError(t, json.Unmarshal(body, &opts))
	ids := make([]string, 0, len(opts.PublicKey.Allow))
	for _, a := range opts.PublicKey.Allow {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestWebAuthn_ProofBeginExcludeID(t *testing.T) {
	e := newWAEnv(t)
	va1, va2, va3 := e.auth(), e.auth(), e.auth()
	id1, _, session := e.enroll(va1, "One", e.session())
	id2, _, session := e.enroll(va2, "Two", session)
	_, _, session = e.enroll(va3, "Three", session)

	t.Run("excluded credential is absent from allowCredentials", func(t *testing.T) {
		w, _ := e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": id1}, session)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		ids := allowedIDs(t, w.Body.Bytes())
		assert.ElementsMatch(t, []string{b64(va2.credentialID), b64(va3.credentialID)}, ids)
		assert.NotContains(t, ids, b64(va1.credentialID))
	})

	t.Run("no body is backward compatible and lists every passkey", func(t *testing.T) {
		w, _ := e.do("POST", "/webauthn/assert/begin", nil, session)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Len(t, allowedIDs(t, w.Body.Bytes()), 3)
	})

	t.Run("empty exclude_id behaves like no body", func(t *testing.T) {
		w, _ := e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": ""}, session)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Len(t, allowedIDs(t, w.Body.Bytes()), 3)
	})

	t.Run("malformed body is a 400", func(t *testing.T) {
		w, _ := e.do("POST", "/webauthn/assert/begin", "not-an-object", session)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	})

	t.Run("the excluded passkey cannot answer the begun ceremony", func(t *testing.T) {
		w, _ := e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": id1}, session)
		require.Equal(t, http.StatusOK, w.Code)
		w, _ = e.do("DELETE", "/webauthn/credentials/"+id1,
			map[string]any{"assertion": va1.assertion(challengeFrom(t, w.Body.Bytes()))}, session)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, int64(3), e.credentialCount())
	})

	t.Run("full flow: exclude then remove with another passkey", func(t *testing.T) {
		w, _ := e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": id2}, session)
		require.Equal(t, http.StatusOK, w.Code)
		w, _ = e.do("DELETE", "/webauthn/credentials/"+id2,
			map[string]any{"assertion": va3.assertion(challengeFrom(t, w.Body.Bytes()))}, session)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, int64(2), e.credentialCount())
	})
}

func TestWebAuthn_ProofBeginExcludeIDOwnershipAndLastPasskey(t *testing.T) {
	e := newWAEnv(t)
	other := newWAEnv(t)
	otherID, _, _ := other.enroll(other.auth(), "Theirs", other.session())
	id1, _, session := e.enroll(e.auth(), "Mine", e.session())

	// Another user's id and a made-up id are indistinguishable (404, same body).
	w, _ := e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": otherID}, session)
	assert.Equal(t, http.StatusNotFound, w.Code)
	wUnknown, _ := e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": "does-not-exist"}, session)
	assert.Equal(t, http.StatusNotFound, wUnknown.Code)
	assert.JSONEq(t, wUnknown.Body.String(), w.Body.String())

	// Excluding the only passkey leaves nothing to prove with -> 409.
	w, _ = e.do("POST", "/webauthn/assert/begin", map[string]string{"exclude_id": id1}, session)
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestWebAuthn_DeleteWithTargetPasskeyGivesDistinctError(t *testing.T) {
	e := newWAEnv(t)
	va1, va2 := e.auth(), e.auth()
	id1, _, session := e.enroll(va1, "One", e.session())
	_, _, session = e.enroll(va2, "Two", session)

	// Legacy no-body begin still offers the target; using it is rejected with a
	// message that says what to do rather than the generic "Invalid code".
	w, _ := e.do("POST", "/webauthn/assert/begin", nil, session)
	require.Equal(t, http.StatusOK, w.Code)
	w, _ = e.do("DELETE", "/webauthn/credentials/"+id1,
		map[string]any{"assertion": va1.assertion(challengeFrom(t, w.Body.Bytes()))}, session)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "different passkey")
	assert.Equal(t, int64(2), e.credentialCount())
}
