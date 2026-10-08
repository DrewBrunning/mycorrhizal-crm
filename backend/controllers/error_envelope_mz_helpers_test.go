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

// mzSeedEdge creates two contacts and a suggested relationship edge between
// them for userID, so a not-found test can prove the edge was left untouched.
func mzSeedEdge(t *testing.T, db *gorm.DB, userID uint) models.RelationshipEdge {
	t.Helper()
	a := models.Contact{UserID: userID, Firstname: "MzA"}
	b := models.Contact{UserID: userID, Firstname: "MzB"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	edge := models.RelationshipEdge{
		UserID: userID, SourceID: a.VCardUID, TargetID: b.VCardUID, Type: "friend_of",
		Source: models.RelationshipSourceHouseholdInferred, Confidence: 0.7, Status: models.RelationshipStatusSuggested,
	}
	require.NoError(t, db.Create(&edge).Error)
	return edge
}

// mzAssertEdgeUntouched re-reads the edge and asserts it still exists with its
// seeded type and status.
func mzAssertEdgeUntouched(t *testing.T, db *gorm.DB, edge models.RelationshipEdge) {
	t.Helper()
	var got models.RelationshipEdge
	require.NoError(t, db.First(&got, "id = ?", edge.ID).Error)
	assert.Equal(t, edge.Type, got.Type)
	assert.Equal(t, edge.Status, got.Status)
}
