package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// alEnvelope is the REST error envelope body ({"error":{code,message,details}}).
// Self-contained (issue #1596) so these helpers do not depend on another
// test file's decoder.
type alEnvelope struct {
	Error struct {
		Code    string                 `json:"code"`
		Message string                 `json:"message"`
		Details map[string]interface{} `json:"details"`
	} `json:"error"`
}

func alDecode(t *testing.T, w *httptest.ResponseRecorder) alEnvelope {
	t.Helper()
	var env alEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body: %s", w.Body.String())
	return env
}

// alAssertError pins the HTTP status AND the apperrors code. If field is
// non-empty it also pins details.field. A bare status check can pass for the
// wrong reason (many failures share a 400/404).
func alAssertError(t *testing.T, w *httptest.ResponseRecorder, status int, code, field string) alEnvelope {
	t.Helper()
	require.Equal(t, status, w.Code, "body: %s", w.Body.String())
	env := alDecode(t, w)
	require.Equal(t, code, env.Error.Code, "body: %s", w.Body.String())
	if field != "" {
		assert.Equal(t, field, env.Error.Details["field"], "body: %s", w.Body.String())
	}
	return env
}

// alAssertValidation asserts a 400 VALIDATION_ERROR whose details name field.
func alAssertValidation(t *testing.T, w *httptest.ResponseRecorder, field string) alEnvelope {
	t.Helper()
	require.Equal(t, 400, w.Code, "body: %s", w.Body.String())
	env := alDecode(t, w)
	require.Equal(t, "VALIDATION_ERROR", env.Error.Code, "body: %s", w.Body.String())
	assert.Contains(t, env.Error.Details, field, "body: %s", w.Body.String())
	return env
}

// alSeedApiToken creates a user with one active API token, so a rejected
// request has something to (not) revoke or rotate.
func alSeedApiToken(t *testing.T, db *gorm.DB) models.ApiToken {
	t.Helper()
	user := models.User{Username: "alseed", Password: "password123!A", Email: "alseed@example.com"}
	require.NoError(t, db.Create(&user).Error)
	tok := models.ApiToken{UserID: user.ID, Name: "seed", TokenHash: "alseed-hash", Scope: "full"}
	require.NoError(t, db.Create(&tok).Error)
	return tok
}

// alAssertApiTokenUntouched asserts the seeded token is still the only token
// and still active (neither revoked nor re-hashed by a rejected request).
func alAssertApiTokenUntouched(t *testing.T, db *gorm.DB, tok models.ApiToken) {
	t.Helper()
	var n int64
	require.NoError(t, db.Model(&models.ApiToken{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "a rejected request must not create tokens")
	var after models.ApiToken
	require.NoError(t, db.First(&after, tok.ID).Error)
	assert.Nil(t, after.RevokedAt, "a rejected request must not revoke the token")
	assert.Equal(t, tok.TokenHash, after.TokenHash, "a rejected request must not rotate the token")
}
