package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2103 (ADR 0038): the import-assistant handlers carry a reason_code in
// the raw JSON. These tests read the wire body (wireReason/requireReason from
// sign_in_reason_codes_test.go), so a dropped .WithReason fails them.

func TestImportReasonCodes_CSVUpload(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	t.Run("no file", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/contacts/import/upload"))
		requireReason(t, w, http.StatusBadRequest, "import_file_missing")
	})
	t.Run("too large", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "big.csv", make([]byte, services.MaxCSVSize+1024)))
		requireReason(t, w, http.StatusBadRequest, "csv_file_too_large")
		_, params := wireReason(t, w)
		assert.EqualValues(t, services.MaxCSVSize/(1024*1024), params["max_mb"])
	})
	t.Run("wrong extension", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "contacts.txt", csvFixture([3]string{"A", "B", "a@example.com"})))
		requireReason(t, w, http.StatusBadRequest, "csv_file_not_csv")
	})
	t.Run("parser error is not migrated", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "empty.csv", []byte("First Name,Last Name,Email\n")))
		requireReason(t, w, http.StatusBadRequest, "")
	})
}

func TestImportReasonCodes_VCFUpload(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	t.Run("no file", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/contacts/import/vcf/upload"))
		requireReason(t, w, http.StatusBadRequest, "import_file_missing")
	})
	t.Run("too large", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/vcf/upload", "big.vcf", make([]byte, services.MaxVCFSize+1024)))
		requireReason(t, w, http.StatusBadRequest, "vcard_file_too_large")
		_, params := wireReason(t, w)
		assert.EqualValues(t, services.MaxVCFSize/(1024*1024), params["max_mb"])
	})
	t.Run("wrong extension", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/vcf/upload", "contacts.txt", vcfFixture("A", "B", "a@example.com", "")))
		requireReason(t, w, http.StatusBadRequest, "vcard_file_not_vcf")
	})
}

func TestImportReasonCodes_JSContactUpload(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	t.Run("no file", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/contacts/import/jscontact/upload"))
		requireReason(t, w, http.StatusBadRequest, "import_file_missing")
	})
	t.Run("too large", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/jscontact/upload", "big.json", make([]byte, services.MaxVCFSize+1024)))
		requireReason(t, w, http.StatusBadRequest, "import_jscontact_file_too_large")
		_, params := wireReason(t, w)
		assert.EqualValues(t, services.MaxVCFSize/(1024*1024), params["max_mb"])
	})
	t.Run("wrong extension", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/jscontact/upload", "cards.txt", jsContactFixture("u1", "A", "B", "a@example.com")))
		requireReason(t, w, http.StatusBadRequest, "import_file_not_jscontact")
	})
}

func TestImportReasonCodes_MonicaHandlers(t *testing.T) {
	srv := mockMonicaAPI(t)
	defer srv.Close()
	_, router := monicaRouterDB(t, 1)

	t.Run("bad token", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newJSONRequest(t, "/contacts/import/monica/connect", models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: "nope"}))
		requireReason(t, w, http.StatusBadRequest, "monica_token_rejected")
	})
	t.Run("unknown session", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/contacts/import/monica/status?session_id=nope", nil))
		requireReason(t, w, http.StatusNotFound, "monica_session_not_found")
	})
}

func TestImportReasonCodes_BundleHandlers(t *testing.T) {
	_, router := setupRouter(t)
	registerMycorrhizalRoutes(router)

	t.Run("no file", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, newFileUploadRequestNoFile(t, "/import/mycorrhizal/upload"))
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		requireReason(t, w, http.StatusBadRequest, "bundle_upload_missing")
	})
	t.Run("unknown session", func(t *testing.T) {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/import/mycorrhizal/status?session_id=nope", nil))
		requireReason(t, w, http.StatusNotFound, "bundle_session_not_found")
	})
}
