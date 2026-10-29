package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2203: Monica import session_id endpoints with another user's
// session. Mirrors TestConnectMeerkatImport_SessionIsOwnerScoped.
//
// Current behaviour (pinned here, same as Meerkat): a session that exists but
// belongs to someone else is rejected with 401 UNAUTHORIZED
// ("Session does not belong to current user"), while an unknown id is 404.
// That 401-vs-404 difference is an existence oracle; session ids are random,
// so it is low risk, and changing it is a handler decision outside this
// test-only change. What these tests guarantee is that the foreign caller
// gets no status/preview data, cannot confirm or cancel, and the owner's
// session is untouched.
func TestMonicaImport_SessionIsOwnerScoped(t *testing.T) {
	srv := mockMonicaAPI(t)
	defer srv.Close()

	// Owner is user 1; registerImportRoutes installs the shared session manager.
	db, ownerRouter := monicaRouterDB(t, 1)
	w := httptest.NewRecorder()
	ownerRouter.ServeHTTP(w, newJSONRequest(t, "/contacts/import/monica/connect", models.MonicaConnectRequest{
		BaseURL: srv.URL, APIToken: "good",
	}))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var connected models.MonicaConnectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &connected))
	sessionID := connected.SessionID
	require.NotEmpty(t, sessionID)

	// Another user's router over the SAME session manager. Not
	// registerImportRoutes: that would replace the shared manager.
	other := routerForUser(db, 2)
	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	other.GET("/contacts/import/monica/status", GetMonicaImportStatus)
	other.GET("/contacts/import/monica/preview", GetMonicaImportPreview)
	other.POST("/contacts/import/monica/fetch", middleware.ValidateJSONMiddleware(&models.MonicaFetchRequest{}), StartMonicaFetch)
	other.POST("/contacts/import/monica/confirm", middleware.ValidateJSONMiddleware(&models.MonicaConfirmRequest{}), func(c *gin.Context) {
		ConfirmMonicaImport(c, cfg)
	})
	other.POST("/contacts/import/monica/cancel", CancelMonicaImport)

	assertForeignRejected := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		mzErrorCode(t, rec, http.StatusUnauthorized, "UNAUTHORIZED")
		assert.NotEqual(t, http.StatusForbidden, rec.Code)
		assert.NotContains(t, rec.Body.String(), `"phase"`, "no session status may leak")
		assert.NotContains(t, rec.Body.String(), srv.URL, "no session detail may leak")
	}
	sid := "?session_id=" + sessionID

	t.Run("status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		other.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/contacts/import/monica/status"+sid, nil))
		assertForeignRejected(t, rec)
	})
	t.Run("preview", func(t *testing.T) {
		rec := httptest.NewRecorder()
		other.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/contacts/import/monica/preview"+sid, nil))
		assertForeignRejected(t, rec)
	})
	t.Run("fetch", func(t *testing.T) {
		rec := httptest.NewRecorder()
		other.ServeHTTP(rec, newJSONRequest(t, "/contacts/import/monica/fetch", models.MonicaFetchRequest{SessionID: sessionID}))
		assertForeignRejected(t, rec)
	})
	t.Run("confirm", func(t *testing.T) {
		rec := httptest.NewRecorder()
		other.ServeHTTP(rec, newJSONRequest(t, "/contacts/import/monica/confirm", map[string]any{
			"session_id": sessionID, "actions": []any{},
		}))
		assertForeignRejected(t, rec)
	})
	t.Run("cancel", func(t *testing.T) {
		rec := httptest.NewRecorder()
		other.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/contacts/import/monica/cancel"+sid, nil))
		assertForeignRejected(t, rec)
	})

	// The owner's session survived every foreign attempt (cancel would have
	// dropped it) and nothing was imported.
	rec := httptest.NewRecorder()
	ownerRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/contacts/import/monica/status"+sid, nil))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var status models.MonicaImportStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	assert.Equal(t, models.MonicaPhaseConnecting, status.Phase, "foreign calls must not advance the owner's session")
	mzAssertNoImportWrites(t, db)
}
