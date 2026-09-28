package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerMycorrhizalRoutes(router *gin.Engine) {
	// The source-import managers are package globals; reset so a session left
	// by another test cannot trip the per-user cap.
	mycorrhizalImportSessions = services.NewMycorrhizalImportManager()
	router.POST("/import/mycorrhizal/upload", UploadMycorrhizalBundle)
	router.POST("/import/mycorrhizal/fetch", middleware.ValidateJSONMiddleware(&models.MycorrhizalFetchRequest{}), StartMycorrhizalFetch)
	router.GET("/import/mycorrhizal/status", GetMycorrhizalImportStatus)
	router.GET("/import/mycorrhizal/preview", GetMycorrhizalImportPreview)
	router.POST("/import/mycorrhizal/confirm", middleware.ValidateJSONMiddleware(&models.SourceImportConfirmRequest{}), ConfirmMycorrhizalImport)
	router.POST("/import/mycorrhizal/cancel", CancelMycorrhizalImport)
}

func testBundleBytes(t *testing.T) []byte {
	t.Helper()
	mk := func(given string) models.AccountBundleContact {
		uid := uuid.New().String()
		return models.AccountBundleContact{
			UID: uid,
			Card: contactmodel.Card{
				UID: uid,
				Name: &contactmodel.Name{Components: []contactmodel.NameComponent{
					{Kind: "given", Value: given},
					{Kind: "surname", Value: "Bundle"},
				}},
			},
			CRM:         contactmodel.CRMEnvelope{},
			Passthrough: contactmodel.Passthrough{},
		}
	}
	bundle := models.AccountBundle{
		Format:     models.AccountBundleFormat,
		Version:    models.AccountBundleVersion,
		ExportedAt: time.Now().UTC(),
		Plan: models.AccountBundlePlan{
			Contacts: []models.AccountBundleContact{mk("Ada"), mk("Bob")},
		},
		Omitted: models.AccountBundleOmissions{Attachments: true},
	}
	b, err := json.Marshal(bundle)
	require.NoError(t, err)
	return b
}

// TestExportAccountBundle covers the export handler: the body is JSON with
// every plan section present as an array (never absent), and the version/photo
// counts ride in response headers.
func TestExportAccountBundle(t *testing.T) {
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Export"}).Error)

	router.GET("/export/account", ExportAccountBundle)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/export/account", nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, "1", w.Header().Get("X-Mycorrhizal-Bundle-Version"))
	assert.Equal(t, "1", w.Header().Get("X-Mycorrhizal-Bundle-Contacts"))
	assert.Equal(t, "0", w.Header().Get("X-Mycorrhizal-Bundle-Photos"))

	var raw struct {
		Format string                     `json:"format"`
		Plan   map[string]json.RawMessage `json:"plan"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, models.AccountBundleFormat, raw.Format)
	for _, key := range []string{"contacts", "relationships", "notes", "life_events", "occasion_events"} {
		require.Contains(t, raw.Plan, key)
		assert.NotEqual(t, "null", string(raw.Plan[key]), "plan.%s must be [] not null", key)
	}
}

// TestMycorrhizalImport_FullControllerFlow drives upload → fetch → preview →
// confirm through the real handlers.
func TestMycorrhizalImport_FullControllerFlow(t *testing.T) {
	db, router := setupRouter(t)
	registerMycorrhizalRoutes(router)

	// upload
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "bundle.json", testBundleBytes(t)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var up models.MycorrhizalUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))
	assert.Equal(t, models.AccountBundleVersion, up.Version)
	assert.Equal(t, 2, up.Totals.Contacts)
	sid := up.SessionID

	// fetch -> 202
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/import/mycorrhizal/fetch", models.MycorrhizalFetchRequest{SessionID: sid}))
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	status := pollMycorrhizalPhase(t, router, sid, models.SourceImportPhaseReady)
	assert.NotNil(t, status)

	// preview
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/import/mycorrhizal/preview?session_id="+sid, nil))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var pv models.SourceImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &pv))
	require.Len(t, pv.Rows, 2)

	actions := make([]models.RowImportAction, len(pv.Rows))
	for i, row := range pv.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/import/mycorrhizal/confirm",
		models.SourceImportConfirmRequest{SessionID: sid, Actions: actions}))
	require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

	done := pollMycorrhizalPhase(t, router, sid, models.SourceImportPhaseDone)
	require.NotNil(t, done.Result)
	assert.Equal(t, 2, done.Result.Created)

	var n int64
	var user models.User
	require.NoError(t, db.First(&user).Error)
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&n).Error)
	assert.EqualValues(t, 2, n)

	// The import is recorded in the history with the mycorrhizal format.
	var runs []models.ImportRun
	require.NoError(t, db.Where("user_id = ? AND format = ?", user.ID, models.ImportFormatMycorrhizal).Find(&runs).Error)
	assert.Len(t, runs, 1)
}

// TestMycorrhizalImport_RejectsBadBundle covers the format/version gates and
// the missing-file/unknown-session branches.
func TestMycorrhizalImport_RejectsBadBundle(t *testing.T) {
	_, router := setupRouter(t)
	registerMycorrhizalRoutes(router)

	// no file
	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/import/mycorrhizal/upload"))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// not JSON
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "b.json", []byte("nope")))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// wrong version -> 422
	badVersion, err := json.Marshal(models.AccountBundle{Format: models.AccountBundleFormat, Version: 99})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "b.json", badVersion))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())

	// wrong format -> 422
	badFormat, err := json.Marshal(models.AccountBundle{Format: "nope", Version: 1})
	require.NoError(t, err)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/import/mycorrhizal/upload", "b.json", badFormat))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	// status missing session_id
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/import/mycorrhizal/status", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// fetch unknown session
	w = httptest.NewRecorder()
	router.ServeHTTP(w, newJSONRequest(t, "/import/mycorrhizal/fetch", models.MycorrhizalFetchRequest{SessionID: "nope"}))
	assert.Equal(t, http.StatusNotFound, w.Code)

	// cancel unknown session
	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/import/mycorrhizal/cancel?session_id=nope", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func pollMycorrhizalPhase(t *testing.T, router *gin.Engine, sid, want string) models.SourceImportStatus {
	t.Helper()
	for i := 0; i < 400; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/import/mycorrhizal/status?session_id="+sid, nil))
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var st models.SourceImportStatus
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &st))
		if st.Phase == want {
			return st
		}
		if st.Phase == models.SourceImportPhaseFailed {
			t.Fatalf("mycorrhizal session failed: %s", st.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for mycorrhizal phase %q", want)
	return models.SourceImportStatus{}
}
