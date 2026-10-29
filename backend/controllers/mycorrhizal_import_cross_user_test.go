package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2203: an account-bundle import session that belongs to another user
// must be answered exactly like an unknown id (404 NOT_FOUND, same body), so
// the response is not an existence oracle.
func TestMycorrhizalImport_SessionIsOwnerScoped(t *testing.T) {
	db, ownerRouter := setupRouter(t)
	registerMycorrhizalRoutes(ownerRouter)

	w := httptest.NewRecorder()
	ownerRouter.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", testBundleBytes(t)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var up models.MycorrhizalUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))
	require.NotEmpty(t, up.SessionID)

	// Same session manager (package global), different user. Only the
	// session_id routes are registered; registering the upload route would
	// not reset the manager, but nothing here needs it.
	other := routerForUser(db, 2)
	other.GET("/import/mycorrhizal/status", GetMycorrhizalImportStatus)
	other.GET("/import/mycorrhizal/preview", GetMycorrhizalImportPreview)
	other.POST("/import/mycorrhizal/cancel", CancelMycorrhizalImport)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/import/mycorrhizal/status"},
		{http.MethodGet, "/import/mycorrhizal/preview"},
		{http.MethodPost, "/import/mycorrhizal/cancel"},
	} {
		t.Run(route.path, func(t *testing.T) {
			foreign := httptest.NewRecorder()
			other.ServeHTTP(foreign, httptest.NewRequest(route.method, route.path+"?session_id="+up.SessionID, nil))
			unknown := httptest.NewRecorder()
			other.ServeHTTP(unknown, httptest.NewRequest(route.method, route.path+"?session_id=no-such-session", nil))

			env := mzErrorCode(t, foreign, http.StatusNotFound, "NOT_FOUND")
			assert.Equal(t, "Import session expired or not found", env.Error.Message)
			assert.JSONEq(t, unknown.Body.String(), foreign.Body.String(), "a foreign session must not be distinguishable from an unknown one")
			assert.Equal(t, unknown.Code, foreign.Code)
		})
	}

	// The foreign cancel did not drop the owner's session.
	rec := httptest.NewRecorder()
	ownerRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/import/mycorrhizal/status?session_id="+up.SessionID, nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
