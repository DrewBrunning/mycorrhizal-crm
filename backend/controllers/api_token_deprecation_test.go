package controllers

// Issue #2887: the legacy ApiToken `scope` field is deprecated in v1.7.0 and
// removed in v2.0.0 (#2023). Every response that carries the field — token
// list, create and rotate — announces the deprecation with the RFC 8594
// headers. Revoke responses do not carry the field, so they must stay silent.
// The response shape itself is unchanged for the whole window.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assertApiTokenScopeDeprecated(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "true", w.Header().Get("Deprecation"))
	assert.Equal(t, legacyScopeDeprecationSunset, w.Header().Get("Sunset"))
	link := w.Header().Get("Link")
	assert.Contains(t, link, `rel="deprecation"`)
	assert.Contains(t, link, "deprecation-policy")
}

func assertNotDeprecatedApiTokenScope(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Empty(t, w.Header().Get("Deprecation"))
	assert.Empty(t, w.Header().Get("Sunset"))
	assert.Empty(t, w.Header().Get("Link"))
}

func apiTokenCreateRequest(t *testing.T, router *gin.Engine, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req, _ := http.NewRequest(http.MethodPost, "/api-tokens", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestApiTokenScopeDeprecation_ListSignalsForAnyReturnedToken(t *testing.T) {
	db, router := setupRouter(t)

	var user models.User
	db.First(&user)
	require.NoError(t, db.Create(&models.ApiToken{UserID: user.ID, Name: "legacy", TokenHash: "h1"}).Error)

	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest(http.MethodGet, "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assertApiTokenScopeDeprecated(t, w)

	var body struct {
		Tokens []models.ApiTokenResponse `json:"tokens"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Tokens, 1)
	assert.Equal(t, "full", body.Tokens[0].Scope, "the legacy field keeps its v1.x shape for the window")
}

func TestApiTokenScopeDeprecation_ListEmptyDoesNotSignal(t *testing.T) {
	_, router := setupRouter(t)

	router.GET("/api-tokens", ListApiTokens)

	req, _ := http.NewRequest(http.MethodGet, "/api-tokens", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assertNotDeprecatedApiTokenScope(t, w)
}

func TestApiTokenScopeDeprecation_CreateSignals(t *testing.T) {
	_, router := setupRouter(t)
	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	w := apiTokenCreateRequest(t, router, models.ApiTokenInput{
		Name:   "scoped",
		Scopes: &[]string{"carddav", "caldav"},
	})

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assertApiTokenScopeDeprecated(t, w)

	var resp models.ApiTokenCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, []string{"carddav", "caldav"}, resp.Scopes)
	assert.Equal(t, "carddav", resp.Scope, "the legacy response field is still populated during the window")
}

func TestApiTokenScopeDeprecation_CreateLegacyRequestSignals(t *testing.T) {
	_, router := setupRouter(t)
	router.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), CreateApiToken)

	w := apiTokenCreateRequest(t, router, models.ApiTokenInput{Name: "legacy", Scope: "carddav"})

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assertApiTokenScopeDeprecated(t, w)
}

func TestApiTokenScopeDeprecation_RotateSignals(t *testing.T) {
	db, router := setupRouter(t)

	var user models.User
	db.First(&user)
	old := models.ApiToken{UserID: user.ID, Name: "rotate-me", TokenHash: "oldhash", Scopes: []string{"api:read"}}
	require.NoError(t, db.Create(&old).Error)

	router.POST("/api-tokens/:id/rotate", RotateApiToken)

	req, _ := http.NewRequest(http.MethodPost, "/api-tokens/"+strconv.Itoa(int(old.ID))+"/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assertApiTokenScopeDeprecated(t, w)
}

func TestApiTokenScopeDeprecation_RevokeDoesNotSignal(t *testing.T) {
	db, router := setupRouter(t)

	var user models.User
	db.First(&user)
	token := models.ApiToken{UserID: user.ID, Name: "revoke-me", TokenHash: "hash"}
	require.NoError(t, db.Create(&token).Error)

	router.DELETE("/api-tokens/:id", RevokeApiToken)

	req, _ := http.NewRequest(http.MethodDelete, "/api-tokens/"+strconv.Itoa(int(token.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assertNotDeprecatedApiTokenScope(t, w)
}
