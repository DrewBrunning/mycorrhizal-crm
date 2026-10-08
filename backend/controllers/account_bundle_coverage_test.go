package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccountBundleHandlers_RequireUser exercises the currentUserID guard on
// every account-bundle handler.
func TestAccountBundleHandlers_RequireUser(t *testing.T) {
	db, _ := setupRouter(t)
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	registerMycorrhizalRoutes(router)
	router.GET("/export/account", ExportAccountBundle)

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/export/account"},
		{http.MethodPost, "/import/mycorrhizal/upload"},
		{http.MethodGet, "/import/mycorrhizal/status"},
		{http.MethodGet, "/import/mycorrhizal/preview"},
		{http.MethodPost, "/import/mycorrhizal/cancel"},
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		alAssertError(t, w, http.StatusUnauthorized, "UNAUTHORIZED", "")
	}

	// fetch/confirm validate their JSON body first, so give them a valid one —
	// the handler's own currentUserID guard must still reject.
	for _, req := range []*http.Request{
		newJSONRequest(t, "/import/mycorrhizal/fetch", models.MycorrhizalFetchRequest{SessionID: "x"}),
		newJSONRequest(t, "/import/mycorrhizal/confirm", models.SourceImportConfirmRequest{SessionID: "x", Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}}}),
	} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		alAssertError(t, w, http.StatusUnauthorized, "UNAUTHORIZED", "")
	}
}

// TestExportAccountBundle_GuardAndBuildFailures covers the export handler's
// database-error branches via dbtest.HideTable.
func TestExportAccountBundle_GuardAndBuildFailures(t *testing.T) {
	logtest.AllowWarnings(t, "the path under test (or its test config) legitimately logs: Failed to count contacts for export; Request error; Failed to build account bundle")
	t.Run("count failure", func(t *testing.T) {
		db, router := setupRouter(t)
		router.GET("/export/account", ExportAccountBundle)
		dbtest.HideTable(t, db, "contacts")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export/account", nil))
		alAssertError(t, w, http.StatusInternalServerError, "INTERNAL_ERROR", "")
		assert.NotContains(t, w.Header().Get("Content-Disposition"), "attachment", "a failed export must not look like a download")
	})

	t.Run("build failure", func(t *testing.T) {
		db, router := setupRouter(t)
		router.GET("/export/account", ExportAccountBundle)
		// The contact count succeeds; a later section's query fails.
		dbtest.HideTable(t, db, "notes")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export/account", nil))
		alAssertError(t, w, http.StatusInternalServerError, "INTERNAL_ERROR", "")
		assert.NotContains(t, w.Header().Get("Content-Disposition"), "attachment", "a failed export must not look like a download")
	})
}

// TestUploadMycorrhizalBundle_SessionLimit covers the per-user concurrent
// session cap (429).
func TestUploadMycorrhizalBundle_SessionLimit(t *testing.T) {
	_, router := setupRouter(t)
	registerMycorrhizalRoutes(router)

	body := testBundleBytes(t)
	for i := 0; i < services.MaxMycorrhizalImportSessionsPerUser; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", body))
		require.Equal(t, http.StatusOK, w.Code, "session %d should open", i)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", body))
	alAssertError(t, w, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", "")
}

// TestMycorrhizalHandlers_ErrorBranches covers the remaining validation and
// not-ready branches across status/preview/confirm/cancel/fetch.
func TestMycorrhizalHandlers_ErrorBranches(t *testing.T) {
	_, router := setupRouter(t)
	registerMycorrhizalRoutes(router)

	// fetch with an invalid JSON body.
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/import/mycorrhizal/fetch", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// confirm with an invalid JSON body.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/import/mycorrhizal/confirm", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// status/preview/confirm with an unknown session.
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/import/mycorrhizal/status?session_id=nope"},
		{http.MethodGet, "/import/mycorrhizal/preview?session_id=nope"},
	} {
		w = httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		assert.Equal(t, http.StatusNotFound, w.Code, tc.path)
	}

	// preview/confirm before ready.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", testBundleBytes(t)))
	require.Equal(t, http.StatusOK, w.Code)
	var up models.MycorrhizalUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/import/mycorrhizal/preview?session_id="+up.SessionID, nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/import/mycorrhizal/confirm",
		models.SourceImportConfirmRequest{SessionID: up.SessionID}))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// cancel with a missing session id.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/import/mycorrhizal/cancel", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// cancel a live (connecting) session -> 200.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/import/mycorrhizal/cancel?session_id="+up.SessionID, nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// TestMycorrhizalImport_ConfirmUnknownSession covers Confirm's session lookup
// failure (a valid JSON body naming no session).
func TestMycorrhizalImport_ConfirmUnknownSession(t *testing.T) {
	_, router := setupRouter(t)
	registerMycorrhizalRoutes(router)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/import/mycorrhizal/confirm",
		models.SourceImportConfirmRequest{SessionID: "nope", Actions: []models.RowImportAction{{RowIndex: 0, Action: "add"}}}))
	alAssertError(t, w, http.StatusNotFound, "NOT_FOUND", "")
}
