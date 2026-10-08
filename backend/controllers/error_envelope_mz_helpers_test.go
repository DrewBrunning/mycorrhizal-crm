package controllers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mzEnvelope is the REST error envelope's wire shape.
type mzEnvelope struct {
	Error struct {
		Code    string                 `json:"code"`
		Message string                 `json:"message"`
		Details map[string]interface{} `json:"details"`
	} `json:"error"`
}

func mzDecode(t *testing.T, w *httptest.ResponseRecorder) mzEnvelope {
	t.Helper()
	var env mzEnvelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "response body: %s", w.Body.String())
	return env
}

// mzErrorCode asserts the response status and the envelope's error code, and
// returns the decoded envelope for further (details) assertions.
func mzErrorCode(t *testing.T, w *httptest.ResponseRecorder, status int, code string) mzEnvelope {
	t.Helper()
	require.Equal(t, status, w.Code, "body: %s", w.Body.String())
	env := mzDecode(t, w)
	require.Equal(t, code, env.Error.Code, "body: %s", w.Body.String())
	return env
}

// mzErrorField is mzErrorCode plus an assertion on details.field.
func mzErrorField(t *testing.T, w *httptest.ResponseRecorder, status int, code, field string) mzEnvelope {
	t.Helper()
	env := mzErrorCode(t, w, status, code)
	assert.Equal(t, field, env.Error.Details["field"], "body: %s", w.Body.String())
	return env
}
