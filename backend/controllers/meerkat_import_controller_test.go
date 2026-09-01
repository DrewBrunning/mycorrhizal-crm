package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/meerkatfixture"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func meerkatRouter(t *testing.T) (*gin.Engine, uint) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "meerkat-ctrl", Password: "password123!A", Email: "meerkat-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, &config.Config{ProfilePhotoDir: t.TempDir()})
	return router, user.ID
}

func meerkatFixtureFile(t *testing.T) []byte {
	t.Helper()
	m, err := meerkatfixture.Read()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "fixture.sqlite")
	require.NoError(t, meerkatfixture.Populate(path, m))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func pollMeerkatPhase(t *testing.T, router *gin.Engine, sid, want string) models.SourceImportStatus {
	t.Helper()
	for i := 0; i < 400; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/status?session_id="+sid, nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var st models.SourceImportStatus
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
		if st.Phase == want {
			return st
		}
		if st.Phase == models.SourceImportPhaseFailed {
			t.Fatalf("meerkat session failed: %s", st.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for meerkat phase %q", want)
	return models.SourceImportStatus{}
}

func TestMeerkatImport_FullControllerFlow(t *testing.T) {
	router, _ := meerkatRouter(t)

	// upload
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/meerkat/upload", "meerkat.db", meerkatFixtureFile(t)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var up models.MeerkatUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))
	require.Len(t, up.SourceUsers, 2)
	sid := up.SessionID

	// fetch -> 202
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/fetch",
		models.MeerkatFetchRequest{SessionID: sid, SourceUserID: up.DefaultSourceUserID}))
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	pollMeerkatPhase(t, router, sid, models.SourceImportPhaseReady)

	// preview
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/preview?session_id="+sid, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var pv models.SourceImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	require.NotNil(t, pv.LossReport)

	actions := make([]models.RowImportAction, len(pv.Rows))
	for i, row := range pv.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/confirm",
		models.SourceImportConfirmRequest{SessionID: sid, Actions: actions}))
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	st := pollMeerkatPhase(t, router, sid, models.SourceImportPhaseDone)
	require.NotNil(t, st.Result)
}

func TestUploadMeerkatDatabase_RejectsBadUploads(t *testing.T) {
	router, _ := meerkatRouter(t)

	// no file
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/contacts/import/meerkat/upload"))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// not a sqlite file
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/meerkat/upload", "x.db", []byte("nope not sqlite")))
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// wrong extension
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/meerkat/upload", "x.txt", meerkatFixtureFile(t)))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMeerkatImport_CancelAndFetchEdgeCases(t *testing.T) {
	router, _ := meerkatRouter(t)

	// cancel without session_id
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/contacts/import/meerkat/cancel", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// fetch unknown session
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/fetch",
		models.MeerkatFetchRequest{SessionID: "nope"}))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// status missing session_id
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/status", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
