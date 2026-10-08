package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
