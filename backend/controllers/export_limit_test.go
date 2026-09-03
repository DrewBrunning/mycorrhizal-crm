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

// Issue #498: ExportData / ExportContactsAsVCF / ExportContactsAsJSContact
// each load every contact (and, for the CSV backup, every related row) into
// memory and buffer the whole payload before writing a byte. MaxExportContacts
// is the safety ceiling that turns a pathological "export a million rows"
// request into a 507 with a pointer at the paginated API, instead of an OOM.

func TestExport_RefusesWhenContactCountExceedsLimit(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	require.NoError(t, db.First(&user).Error)

	for _, name := range []string{"Ada", "Bea", "Cyd"} {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: name, Lastname: "X"}).Error)
	}

	prev := exportContactsLimit
	exportContactsLimit = 2
	t.Cleanup(func() { exportContactsLimit = prev })

	router.GET("/export", ExportData)
	registerVCFRoute(router, t.TempDir())
	registerJSContactRoute(router, t.TempDir())

	cases := []struct {
		name string
		path string
	}{
		{"csv", "/export"},
		{"vcf", "/export/vcf"},
		{"jscontact", "/export/jscontact"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusInsufficientStorage, w.Code, "an over-limit export is a 507")
			var body struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "INSUFFICIENT_STORAGE", body.Error.Code)
			assert.Contains(t, body.Error.Message, "exceeds the single-request limit")
		})
	}
}

func TestExport_ProceedsWhenUnderLimit(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	require.NoError(t, db.First(&user).Error)
	for _, name := range []string{"Ada", "Bea"} {
		require.NoError(t, db.Create(&models.Contact{UserID: user.ID, Firstname: name, Lastname: "X"}).Error)
	}

	prev := exportContactsLimit
	exportContactsLimit = 100
	t.Cleanup(func() { exportContactsLimit = prev })

	router.GET("/export", ExportData)
	registerVCFRoute(router, t.TempDir())
	registerJSContactRoute(router, t.TempDir())

	for _, path := range []string{"/export", "/export/vcf", "/export/jscontact"} {
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "%s under the limit exports normally", path)
	}
}

// TestExportVCF_SingleContactExportIsNotCapped confirms the ?vcard_uid=
// single-contact export path skips the count guard — it is inherently bounded.
func TestExportVCF_SingleContactExportIsNotCapped(t *testing.T) {
	db, router := setupRouter()
	var user models.User
	require.NoError(t, db.First(&user).Error)
	var target models.Contact
	for _, name := range []string{"Ada", "Bea", "Cyd"} {
		c := models.Contact{UserID: user.ID, Firstname: name, Lastname: "X"}
		require.NoError(t, db.Create(&c).Error)
		target = c
	}

	prev := exportContactsLimit
	exportContactsLimit = 1
	t.Cleanup(func() { exportContactsLimit = prev })

	registerVCFRoute(router, t.TempDir())
	req, _ := http.NewRequest(http.MethodGet, "/export/vcf?vcard_uid="+target.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "a single-contact export is not subject to the full-export cap")
}
