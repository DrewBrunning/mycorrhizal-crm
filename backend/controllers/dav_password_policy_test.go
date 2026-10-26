package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateDAVPasswordPolicy_TogglesForUserWithoutSecondFactor(t *testing.T) {
	db, router := setupRouter(t)

	router.PATCH("/users/dav-password-policy",
		middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}),
		UpdateDAVPasswordPolicy)

	allow := true
	body, _ := json.Marshal(models.DAVPasswordPolicyInput{Allow: &allow})
	req, _ := http.NewRequest(http.MethodPatch, "/users/dav-password-policy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var user models.User
	require.NoError(t, db.First(&user).Error)
	assert.True(t, user.AllowDAVPassword)
}

func TestUpdateDAVPasswordPolicy_CanDisable(t *testing.T) {
	db, router := setupRouter(t)
	require.NoError(t, db.Model(&models.User{}).Where("1 = 1").Update("allow_dav_password", true).Error)

	router.PATCH("/users/dav-password-policy",
		middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}),
		UpdateDAVPasswordPolicy)

	allow := false
	body, _ := json.Marshal(models.DAVPasswordPolicyInput{Allow: &allow})
	req, _ := http.NewRequest(http.MethodPatch, "/users/dav-password-policy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, false, resp["allow_dav_password"])

	var user models.User
	require.NoError(t, db.First(&user).Error)
	assert.False(t, user.AllowDAVPassword)
}

func TestUpdateDAVPasswordPolicy_RejectedWhileSecondFactorEnrolled(t *testing.T) {
	db, router := setupRouter(t)
	require.NoError(t, db.Model(&models.User{}).Where("1 = 1").Updates(map[string]any{
		"totp_enabled":          true,
		"totp_secret_encrypted": "encrypted-secret",
	}).Error)

	router.PATCH("/users/dav-password-policy",
		middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}),
		UpdateDAVPasswordPolicy)

	allow := true
	body, _ := json.Marshal(models.DAVPasswordPolicyInput{Allow: &allow})
	req, _ := http.NewRequest(http.MethodPatch, "/users/dav-password-policy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var user models.User
	require.NoError(t, db.First(&user).Error)
	assert.False(t, user.AllowDAVPassword)
}

func TestGetCurrentUser_ReportsDAVPasswordPolicy(t *testing.T) {
	db, router := setupRouter(t)
	require.NoError(t, db.Model(&models.User{}).Where("1 = 1").Update("allow_dav_password", true).Error)
	router.GET("/users/me", GetCurrentUser)

	req, _ := http.NewRequest(http.MethodGet, "/users/me", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.CurrentUserResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, resp.AllowDAVPassword)
}

func TestUpdateDAVPasswordPolicy_MissingAllowRejected(t *testing.T) {
	db, router := setupRouter(t)
	router.PATCH("/users/dav-password-policy",
		middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}),
		UpdateDAVPasswordPolicy)

	req, _ := http.NewRequest(http.MethodPatch, "/users/dav-password-policy", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var user models.User
	require.NoError(t, db.First(&user).Error)
	assert.False(t, user.AllowDAVPassword)
}

func TestUpdateDAVPasswordPolicy_UserLookupFailure(t *testing.T) {
	logtest.AllowWarnings(t, "the test deliberately hides the users table to drive the query-failure branch")
	db, router := setupRouter(t)
	dbtest.HideTable(t, db, "users")
	router.PATCH("/users/dav-password-policy",
		middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}),
		UpdateDAVPasswordPolicy)

	allow := true
	body, _ := json.Marshal(models.DAVPasswordPolicyInput{Allow: &allow})
	req, _ := http.NewRequest(http.MethodPatch, "/users/dav-password-policy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}

func TestUpdateDAVPasswordPolicy_SecondFactorLookupFailure(t *testing.T) {
	logtest.AllowWarnings(t, "the test deliberately hides the webauthn_credentials table to drive the query-failure branch")
	db, router := setupRouter(t)
	dbtest.HideTable(t, db, "webauthn_credentials")
	router.PATCH("/users/dav-password-policy",
		middleware.ValidateJSONMiddleware(&models.DAVPasswordPolicyInput{}),
		UpdateDAVPasswordPolicy)

	allow := true
	body, _ := json.Marshal(models.DAVPasswordPolicyInput{Allow: &allow})
	req, _ := http.NewRequest(http.MethodPatch, "/users/dav-password-policy", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
}
