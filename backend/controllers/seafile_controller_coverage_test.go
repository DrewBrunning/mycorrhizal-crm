package controllers

// Coverage tests for milestone-30 issue #1623 §3: the low-coverage
// validation-rejection, SSRF-guard and ownership error paths in
// seafile_controller.go. Kept in a separate file so the focused existing suite
// (seafile_controller_test.go) stays untouched.

import (
	"encoding/json"
	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func m30SeedSeafileUser(t *testing.T, db *gorm.DB, tag string) models.User {
	t.Helper()
	user := models.User{
		Username: "seafile-cov-" + tag,
		Password: "password123!A",
		Email:    "seafile-cov-" + tag + "@example.com",
	}
	require.NoError(t, db.Create(&user).Error)
	require.NotZero(t, user.ID)
	return user
}

func m30SeedSeafileConfig(t *testing.T, db *gorm.DB, userID uint, baseURL, token string) {
	t.Helper()
	require.NoError(t, db.Create(&models.SeafileConfig{
		UserID: userID, BaseURL: baseURL, APITokenEncrypted: m30SeedEncrypted(t, token),
	}).Error)
}

func m30SeafileRouter(t *testing.T, db *gorm.DB, userID uint, cfg config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(m30CovContext(db, userID, cfg))
	router.GET("/seafile/config", GetSeafileConfig)
	router.PUT("/seafile/config", withValidated(func() any { return &models.SeafileConfigInput{} }), SaveSeafileConfig)
	router.DELETE("/seafile/config", DeleteSeafileConfig)
	router.POST("/seafile/test-connection", TestSeafileConnection)
	router.GET("/seafile/libraries", ListSeafileLibraries)
	router.GET("/seafile/libraries/:repo_id/dir", ListSeafileDir)
	router.POST("/seafile/contacts/:vcard_uid/link", LinkSeafileContact)
	router.DELETE("/seafile/contacts/:vcard_uid/links/:identity_id", UnlinkSeafileContact)
	return router
}

// TestSeafileCov_SaveMissingValidatedContext exercises the GetValidated error
// arm.
func TestSeafileCov_SaveMissingValidatedContext(t *testing.T) {
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(m30CovContext(db, 1, seafileControllerConfig()))
	router.PUT("/seafile/config", SaveSeafileConfig)

	w := m30DoJSON(t, router, "PUT", "/seafile/config", map[string]any{"base_url": "https://seafile.example"})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "request")
}

// TestSeafileCov_SaveInvalidURLIsValidation exercises the service-level
// NormalizeSeafileBaseURL rejection and the abortSeafileServiceError
// ErrSeafileInvalidURL arm.
func TestSeafileCov_SaveInvalidURLIsValidation(t *testing.T) {
	db := dbtest.New(t)
	router := gin.New()
	router.Use(m30CovContext(db, 1, seafileControllerConfig()))
	router.PUT("/seafile/config", m30InjectValidated(&models.SeafileConfigInput{
		BaseURL: "not-a-valid-url", APIToken: "token",
	}), SaveSeafileConfig)

	w := m30DoJSON(t, router, "PUT", "/seafile/config", map[string]any{})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeValidation, "")
}

// TestSeafileCov_SaveOriginChangeWithoutToken exercises the credential-exfil
// guard: a new origin with no token is rejected and the stored row is
// untouched.
func TestSeafileCov_SaveOriginChangeWithoutToken(t *testing.T) {
	db := dbtest.New(t)
	user := m30SeedSeafileUser(t, db, "origin")
	m30SeedSeafileConfig(t, db, user.ID, "https://seafile.example", "stored-token")

	router := gin.New()
	router.Use(m30CovContext(db, user.ID, seafileControllerConfig()))
	router.PUT("/seafile/config", m30InjectValidated(&models.SeafileConfigInput{
		BaseURL: "https://other.example",
	}), SaveSeafileConfig)

	w := m30DoJSON(t, router, "PUT", "/seafile/config", map[string]any{})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "api_token")

	var stored models.SeafileConfig
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&stored).Error)
	assert.Equal(t, "https://seafile.example", stored.BaseURL, "a rejected origin change must not persist")
}

// TestSeafileCov_SaveGetConfigDBError exercises SaveSeafileConfig's database
// branch.
func TestSeafileCov_SaveGetConfigDBError(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "seafile_configs")

	router := gin.New()
	router.Use(m30CovContext(db, 1, seafileControllerConfig()))
	router.PUT("/seafile/config", m30InjectValidated(&models.SeafileConfigInput{
		BaseURL: "https://seafile.example", APIToken: "token",
	}), SaveSeafileConfig)

	w := m30DoJSON(t, router, "PUT", "/seafile/config", map[string]any{})
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestSeafileCov_GetConfigPopulatedAndOwnership pins the populated success
// response and cross-user isolation.
func TestSeafileCov_GetConfigPopulatedAndOwnership(t *testing.T) {
	db := dbtest.New(t)
	userA := m30SeedSeafileUser(t, db, "a")
	userB := m30SeedSeafileUser(t, db, "b")
	m30SeedSeafileConfig(t, db, userA.ID, "https://seafile.example", "sekret")

	wA := m30DoJSON(t, m30SeafileRouter(t, db, userA.ID, seafileControllerConfig()), "GET", "/seafile/config", nil)
	require.Equal(t, http.StatusOK, wA.Code, wA.Body.String())
	var respA services.SeafileConfigResponse
	require.NoError(t, json.Unmarshal(wA.Body.Bytes(), &respA))
	assert.Equal(t, "https://seafile.example", respA.BaseURL)
	assert.True(t, respA.HasAPIToken)

	wB := m30DoJSON(t, m30SeafileRouter(t, db, userB.ID, seafileControllerConfig()), "GET", "/seafile/config", nil)
	require.Equal(t, http.StatusOK, wB.Code, wB.Body.String())
	var respB services.SeafileConfigResponse
	require.NoError(t, json.Unmarshal(wB.Body.Bytes(), &respB))
	assert.Empty(t, respB.BaseURL, "user B must not see user A's Seafile config")
	assert.False(t, respB.HasAPIToken)
}

// TestSeafileCov_GetConfigDBError exercises the retrieval-failure branch.
func TestSeafileCov_GetConfigDBError(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "seafile_configs")

	w := m30DoJSON(t, m30SeafileRouter(t, db, 1, seafileControllerConfig()), "GET", "/seafile/config", nil)
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestSeafileCov_DeleteOwnershipIsolated proves a delete never removes another
// user's connection.
func TestSeafileCov_DeleteOwnershipIsolated(t *testing.T) {
	db := dbtest.New(t)
	userA := m30SeedSeafileUser(t, db, "a")
	userB := m30SeedSeafileUser(t, db, "b")
	m30SeedSeafileConfig(t, db, userA.ID, "https://seafile.example", "sekret")

	w := m30DoJSON(t, m30SeafileRouter(t, db, userB.ID, seafileControllerConfig()), "DELETE", "/seafile/config", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	require.NoError(t, db.Model(&models.SeafileConfig{}).Where("user_id = ?", userA.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count, "user A's config must survive user B's delete")
}

// TestSeafileCov_DeleteDBError exercises the delete-failure branch.
func TestSeafileCov_DeleteDBError(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "seafile_configs")

	w := m30DoJSON(t, m30SeafileRouter(t, db, 1, seafileControllerConfig()), "DELETE", "/seafile/config", nil)
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestSeafileCov_ServiceErrorMapping drives abortSeafileServiceError's
// not-found, redirect, request-failed, invalid-data and unreachable arms
// through ListSeafileLibraries.
func TestSeafileCov_ServiceErrorMapping(t *testing.T) {
	cases := []struct {
		name       string
		failStatus int
		wantStatus int
		wantCode   string
	}{
		{"not_found", http.StatusNotFound, http.StatusNotFound, apperrors.ErrCodeNotFound},
		{"redirect", http.StatusTemporaryRedirect, http.StatusServiceUnavailable, apperrors.ErrCodeExternal},
		{"request_failed", http.StatusInternalServerError, http.StatusServiceUnavailable, apperrors.ErrCodeExternal},
		{"invalid_data", http.StatusOK, http.StatusServiceUnavailable, apperrors.ErrCodeExternal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := seedSeafileControllerDB(t)
			router := seafileTestRouter(t, db)

			fake := newSeafileTestServer(t, "sekret")
			defer fake.Close()
			fake.FailWithStatus = tc.failStatus
			connectSeafileControllerConfig(t, db, fake.URL())

			w := seafileDoJSON(t, router, "GET", "/seafile/libraries", nil)
			mzErrorCode(t, w, tc.wantStatus, tc.wantCode)
		})
	}

	t.Run("unreachable", func(t *testing.T) {
		db := seedSeafileControllerDB(t)
		router := seafileTestRouter(t, db)
		connectSeafileControllerConfig(t, db, m30ClosedPortURL(t))

		w := seafileDoJSON(t, router, "GET", "/seafile/libraries", nil)
		mzErrorCode(t, w, http.StatusServiceUnavailable, apperrors.ErrCodeExternal)
	})
}

// TestSeafileCov_PrivateURLGuard proves the SSRF guard: with
// SEAFILE_BLOCK_PRIVATE_URLS on, a loopback base URL is refused and the
// request never reaches the listening server.
func TestSeafileCov_PrivateURLGuard(t *testing.T) {
	var hits int32
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer listener.Close()

	cfg := seafileControllerConfig()
	cfg.SeafileBlockPrivateURLs = true

	db := seedSeafileControllerDB(t)
	m30SeedSeafileConfig(t, db, 1, listener.URL, "sekret")
	router := m30SeafileRouter(t, db, 1, cfg)

	w := m30DoJSON(t, router, "GET", "/seafile/libraries", nil)
	mzErrorCode(t, w, http.StatusServiceUnavailable, apperrors.ErrCodeExternal)

	// Test connection diagnoses instead of erroring (200, ok:false) but still
	// must not dial.
	wTest := m30DoJSON(t, router, "POST", "/seafile/test-connection", nil)
	require.Equal(t, http.StatusOK, wTest.Code, wTest.Body.String())
	var result services.SeafileConnectionTestResult
	require.NoError(t, json.Unmarshal(wTest.Body.Bytes(), &result))
	assert.False(t, result.OK)

	assert.Equal(t, int32(0), atomic.LoadInt32(&hits),
		"the SSRF guard must prevent the request from ever reaching the loopback listener")
}

// TestSeafileCov_LinkValidationRejected covers the malformed-body, struct-tag
// and post-trim empty-identifier rejections of LinkSeafileContact.
func TestSeafileCov_LinkValidationRejected(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)
	base := "/seafile/contacts/" + contact.VCardUID + "/link"

	wBadJSON := m30DoRaw(t, router, "POST", base, "{not json")
	assertAppErrorResponse(t, wBadJSON, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "request body")

	wMissing := seafileDoJSON(t, router, "POST", base, map[string]any{"name": "contract.pdf"})
	mzErrorCode(t, wMissing, http.StatusBadRequest, apperrors.ErrCodeValidation)

	wType := seafileDoJSON(t, router, "POST", base, map[string]any{
		"repo_id": "repo-1", "path": "/contract.pdf", "name": "contract.pdf", "type": "bogus",
	})
	mzErrorCode(t, wType, http.StatusBadRequest, apperrors.ErrCodeValidation)

	wBlank := seafileDoJSON(t, router, "POST", base, map[string]any{
		"repo_id": "repo-1", "path": "   ", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, wBlank, http.StatusBadRequest, apperrors.ErrCodeInvalidInput, "repo_id/path")
}

// TestSeafileCov_LinkLookupDBError exercises the non-ErrRecordNotFound lookup
// branch.
func TestSeafileCov_LinkLookupDBError(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)
	dbtest.HideTable(t, db, "external_identities")

	w := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+contact.VCardUID+"/link", map[string]any{
		"repo_id": "repo-1", "path": "/contract.pdf", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, w, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestSeafileCov_LinkForeignContactNotFound pins the ownership rejection's
// NOT_FOUND code.
func TestSeafileCov_LinkForeignContactNotFound(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	other := models.User{Username: "seafile-cov-foreign", Password: "password123!A", Email: "seafile-cov-foreign@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreign := models.Contact{UserID: other.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&foreign).Error)

	w := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+foreign.VCardUID+"/link", map[string]any{
		"repo_id": "repo-1", "path": "/contract.pdf", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, w, http.StatusNotFound, apperrors.ErrCodeNotFound, "")
}

// TestSeafileCov_UnlinkCrossUserAndDBError covers the not-found (cross-user)
// and database branches of UnlinkSeafileContact.
func TestSeafileCov_UnlinkCrossUserAndDBError(t *testing.T) {
	db := seedSeafileControllerDB(t)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	other := models.User{Username: "seafile-cov-owner", Password: "password123!A", Email: "seafile-cov-owner@example.com"}
	require.NoError(t, db.Create(&other).Error)
	foreignIdentity := models.ExternalIdentity{
		UserID: other.ID, EntityID: contact.VCardUID,
		System: services.ExternalSystemSeafile, ExternalID: "repo-1:/secret.pdf",
	}
	require.NoError(t, db.Create(&foreignIdentity).Error)

	router := seafileTestRouter(t, db)
	w := seafileDoJSON(t, router, "DELETE", "/seafile/contacts/"+contact.VCardUID+"/links/"+foreignIdentity.ID, nil)
	assertAppErrorResponse(t, w, http.StatusNotFound, apperrors.ErrCodeNotFound, "")
	assert.Equal(t, foreignIdentity.ID, mustLoadIdentity(t, db, foreignIdentity.ID).ID)

	dbtest.HideTable(t, db, "external_identities")
	wDB := seafileDoJSON(t, router, "DELETE", "/seafile/contacts/"+contact.VCardUID+"/links/"+foreignIdentity.ID, nil)
	assertAppErrorResponse(t, wDB, http.StatusInternalServerError, apperrors.ErrCodeDatabase, "")
}

// TestSeafileCov_LinkNoConfigIsValidation covers the service-error arm of
// LinkSeafileContact: no configured connection is the caller's setup -> 400.
func TestSeafileCov_LinkNoConfigIsValidation(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)
	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	w := seafileDoJSON(t, router, "POST", "/seafile/contacts/"+contact.VCardUID+"/link", map[string]any{
		"repo_id": "repo-1", "path": "/contract.pdf", "name": "contract.pdf", "type": "file",
	})
	assertAppErrorResponse(t, w, http.StatusBadRequest, apperrors.ErrCodeValidation, "")
}

// TestSeafileCov_Unauthenticated covers every handler's currentUserID !ok arm.
func TestSeafileCov_Unauthenticated(t *testing.T) {
	db := dbtest.New(t)
	router := m30SeafileRouter(t, db, 0, seafileControllerConfig())
	router.PUT("/seafile/config/direct", m30InjectValidated(&models.SeafileConfigInput{
		BaseURL: "https://seafile.example", APIToken: "token",
	}), SaveSeafileConfig)

	cases := []struct {
		name, method, path string
	}{
		{"get_config", "GET", "/seafile/config"},
		{"save_config", "PUT", "/seafile/config/direct"},
		{"delete_config", "DELETE", "/seafile/config"},
		{"test_connection", "POST", "/seafile/test-connection"},
		{"list_libraries", "GET", "/seafile/libraries"},
		{"list_dir", "GET", "/seafile/libraries/repo-1/dir"},
		{"link", "POST", "/seafile/contacts/vcard-uid/link"},
		{"unlink", "DELETE", "/seafile/contacts/vcard-uid/links/identity-id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := m30DoJSON(t, router, tc.method, tc.path, nil)
			assertAppErrorResponse(t, w, http.StatusUnauthorized, apperrors.ErrCodeUnauthorized, "")
		})
	}
}

// TestSeafileCov_ListDirNormalizesBarePath covers the path-without-leading-slash
// normalization branch of ListSeafileDir.
func TestSeafileCov_ListDirNormalizesBarePath(t *testing.T) {
	db := seedSeafileControllerDB(t)
	router := seafileTestRouter(t, db)

	fake := newSeafileTestServer(t, "sekret")
	defer fake.Close()
	fake.addLib("repo-1", "Personal")
	fake.addDirItem("repo-1", "/Documents", "contract.pdf", "file", 2048, 2000)
	connectSeafileControllerConfig(t, db, fake.URL())

	w := seafileDoJSON(t, router, "GET", "/seafile/libraries/repo-1/dir?path=Documents", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Items []services.SeafileItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Items, 1)
	assert.Equal(t, "contract.pdf", resp.Items[0].Name)
}
