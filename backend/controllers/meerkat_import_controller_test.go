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
	"mycorrhizal/internal/meerkatapifake"
	"mycorrhizal/internal/meerkatfixture"
	"mycorrhizal/meerkatapi"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func meerkatRouter(t *testing.T) (*gin.Engine, uint) {
	t.Helper()
	_, router, userID := meerkatRouterDB(t)
	return router, userID
}

// meerkatRouterDB is meerkatRouter plus the backing DB, for tests that assert
// a rejected request wrote nothing.
func meerkatRouterDB(t *testing.T) (*gorm.DB, *gin.Engine, uint) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "meerkat-ctrl", Password: "password123!A", Email: "meerkat-ctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, &config.Config{ProfilePhotoDir: t.TempDir()})
	return db, router, user.ID
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
	db, router, _ := meerkatRouterDB(t)

	// no file
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/contacts/import/meerkat/upload"))
	mzErrorField(t, w, http.StatusBadRequest, "INVALID_INPUT", "file")

	// not a sqlite file
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/meerkat/upload", "x.db", []byte("nope not sqlite")))
	mzErrorField(t, w, http.StatusBadRequest, "INVALID_INPUT", "file")

	// wrong extension
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/meerkat/upload", "x.txt", meerkatFixtureFile(t)))
	mzErrorField(t, w, http.StatusBadRequest, "INVALID_INPUT", "file")
	mzAssertNoImportWrites(t, db)
}

func TestMeerkatImport_CancelAndFetchEdgeCases(t *testing.T) {
	db, router, _ := meerkatRouterDB(t)

	// cancel without session_id
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/contacts/import/meerkat/cancel", nil))
	mzErrorField(t, w, http.StatusBadRequest, "MISSING_FIELD", "session_id")

	// fetch unknown session
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/fetch",
		models.MeerkatFetchRequest{SessionID: "nope"}))
	mzErrorCode(t, w, http.StatusNotFound, "NOT_FOUND")

	// status missing session_id
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/status", nil))
	mzErrorField(t, w, http.StatusBadRequest, "MISSING_FIELD", "session_id")
	mzAssertNoImportWrites(t, db)
}

func init() { meerkatapi.DisableRateLimitForTesting() }

func apiFake(t *testing.T) *meerkatapifake.Server {
	t.Helper()
	srv := meerkatapifake.New(t)
	for i, name := range []string{"Ada", "Bea", "Cy"} {
		srv.Contacts = append(srv.Contacts, meerkatapifake.Contact(i+1, name))
	}
	srv.Contacts[0]["photo"] = "a.png"
	srv.Pictures[1] = meerkatapifake.PNG()
	return srv
}

func postConnect(t *testing.T, router *gin.Engine, body any) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/connect", body))
	return w
}

func TestConnectMeerkatImport_RejectsBadBodies(t *testing.T) {
	db, router, _ := meerkatRouterDB(t)

	w := postConnect(t, router, map[string]string{})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "BaseURL")

	w = postConnect(t, router, models.MeerkatConnectRequest{BaseURL: "http://example.invalid"})
	require.Equal(t, http.StatusBadRequest, w.Code, "no credentials")

	var n int64
	require.NoError(t, db.Unscoped().Model(&models.Contact{}).Count(&n).Error)
	require.Zero(t, n, "a rejected connect writes nothing")
}

func TestConnectMeerkatImport_FullFlowWithPhoto(t *testing.T) {
	db, router, userID := meerkatRouterDB(t)
	srv := apiFake(t)

	w := postConnect(t, router, models.MeerkatConnectRequest{BaseURL: srv.URL, APIToken: meerkatapifake.Token})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.MeerkatConnectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Version.Supported)
	require.Equal(t, 3, resp.Totals.Contacts)
	require.NotContains(t, w.Body.String(), meerkatapifake.Token, "the response never echoes the credential")
	sid := resp.SessionID

	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/fetch", models.MeerkatFetchRequest{SessionID: sid}))
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	pollMeerkatPhase(t, router, sid, models.SourceImportPhaseReady)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/preview?session_id="+sid, nil))
	require.Equal(t, http.StatusOK, w.Code)
	var pv models.SourceImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	actions := make([]models.RowImportAction, len(pv.Rows))
	for i, row := range pv.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/meerkat/confirm", models.SourceImportConfirmRequest{SessionID: sid, Actions: actions}))
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	st := pollMeerkatPhase(t, router, sid, models.SourceImportPhaseDone)
	require.NotNil(t, st.Result)
	require.Equal(t, 3, st.Result.Created)
	require.Equal(t, 1, st.Result.PhotosSaved)

	var c models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", userID, "Ada").First(&c).Error)
	require.NotEmpty(t, c.Photo)
}

func TestConnectMeerkatImport_SessionCap(t *testing.T) {
	_, router, _ := meerkatRouterDB(t)
	srv := apiFake(t)
	body := models.MeerkatConnectRequest{BaseURL: srv.URL, APIToken: meerkatapifake.Token}
	for i := 0; i < services.MaxMeerkatImportSessionsPerUser; i++ {
		require.Equal(t, http.StatusOK, postConnect(t, router, body).Code)
	}
	w := postConnect(t, router, body)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestConnectMeerkatImport_SessionIsOwnerScoped(t *testing.T) {
	db, router, _ := meerkatRouterDB(t)
	srv := apiFake(t)
	w := postConnect(t, router, models.MeerkatConnectRequest{BaseURL: srv.URL, APIToken: meerkatapifake.Token})
	require.Equal(t, http.StatusOK, w.Code)
	var resp models.MeerkatConnectResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	other := models.User{Username: "other-meerkat", Password: "password123!A", Email: "other-meerkat@example.com"}
	require.NoError(t, db.Create(&other).Error)
	otherRouter := routerForUser(db, other.ID)
	// Not registerImportRoutes: that would replace the shared session manager.
	otherRouter.GET("/contacts/import/meerkat/status", GetMeerkatImportStatus)
	rec := httptest.NewRecorder()
	otherRouter.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/status?session_id="+resp.SessionID, nil))
	require.Equal(t, http.StatusNotFound, rec.Code, "the session exists but belongs to another user: %s", rec.Body.String())

	// Byte-identical to the response for an id that never existed.
	unknown := httptest.NewRecorder()
	otherRouter.ServeHTTP(unknown, httptest.NewRequest(http.MethodGet, "/contacts/import/meerkat/status?session_id=no-such-session", nil))
	require.Equal(t, http.StatusNotFound, unknown.Code)
	assert.JSONEq(t, unknown.Body.String(), rec.Body.String(), "a foreign session must not be distinguishable from an unknown one")
}

func TestConnectMeerkatImport_BadCredentialsAre400AndLeakNothing(t *testing.T) {
	_, router, _ := meerkatRouterDB(t)
	srv := apiFake(t)
	w := postConnect(t, router, models.MeerkatConnectRequest{BaseURL: srv.URL, APIToken: "SECRET-NOPE"})
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.NotContains(t, w.Body.String(), "SECRET-NOPE")
}
