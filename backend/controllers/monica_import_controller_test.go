package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/monica"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMonicaAPI serves the minimal Monica list API the connect probe needs
// (two contacts, everything else empty). Rejects any token but "good".
func mockMonicaAPI(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		total := 0
		if r.URL.Path == "/api/contacts" {
			total = 2
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{},
			"meta": map[string]int{"current_page": 1, "last_page": 1, "total": total},
		})
	}))
}

func monicaRouter(t *testing.T, userID uint) *gin.Engine {
	t.Helper()
	monica.DisableRateLimitForTesting()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	router := routerForUser(db, userID)
	registerImportRoutes(router, &config.Config{ProfilePhotoDir: t.TempDir()})
	return router
}

func TestConnectMonicaImport_RejectsEmptyBody(t *testing.T) {
	router := monicaRouter(t, 1)
	req := newJSONRequest(t, "/contacts/import/monica/connect", map[string]string{})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestConnectMonicaImport_Succeeds(t *testing.T) {
	srv := mockMonicaAPI(t)
	defer srv.Close()

	router := monicaRouter(t, 1)
	req := newJSONRequest(t, "/contacts/import/monica/connect", models.MonicaConnectRequest{
		BaseURL: srv.URL, APIToken: "good",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.MonicaConnectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.SessionID)
	assert.Equal(t, 2, resp.Totals.Contacts)
}

func TestConnectMonicaImport_BadTokenIsFieldError(t *testing.T) {
	srv := mockMonicaAPI(t)
	defer srv.Close()

	router := monicaRouter(t, 1)
	req := newJSONRequest(t, "/contacts/import/monica/connect", models.MonicaConnectRequest{
		BaseURL: srv.URL, APIToken: "nope",
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestGetMonicaImportStatus_UnknownSessionIs404(t *testing.T) {
	router := monicaRouter(t, 1)
	req := httptest.NewRequest(http.MethodGet, "/contacts/import/monica/status?session_id=nope", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetMonicaImportStatus_MissingSessionIDIs400(t *testing.T) {
	router := monicaRouter(t, 1)
	req := httptest.NewRequest(http.MethodGet, "/contacts/import/monica/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestConnectMonicaImport_SessionCapReturns429(t *testing.T) {
	srv := mockMonicaAPI(t)
	defer srv.Close()

	router := monicaRouter(t, 1)
	body := models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: "good"}
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/monica/connect", body))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/monica/connect", body))
	assert.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
}
