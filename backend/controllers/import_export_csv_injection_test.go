package controllers

// TEST-04 (issue #432): the CSV formula-injection neutralization
// (export_csv_injection_test.go pins csvSafe as a pure function;
// hostile_input_e2e_test.go proves it through /export against hand-seeded
// rows) must also hold when the hostile value arrives through the REAL
// import path. inj-csv-formula-note.vcf is imported via the VCF upload →
// confirm handlers, then exported as CSV: the raw formula prefixes must
// survive import into the DB untouched (the import side never mangles
// them), and the export boundary must then neutralize them.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/adversarial"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportToExport_CSVFormulaPrefixesNeutralized(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "importcsv", Password: "password123!A", Email: "importcsv@example.com"}
	require.NoError(t, db.Create(&user).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	router := routerForUser(db, user.ID)
	registerImportRoutes(router, cfg)
	router.GET("/export", ExportData)

	// 1. Import the hostile fixture through the real VCF upload + confirm path.
	uploadReq := newFileUploadRequest(t, "/contacts/import/vcf/upload", "inj-csv-formula-note.vcf", adversarial.LoadFixture("inj-csv-formula-note.vcf"))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))
	require.Len(t, uploadResp.Rows, 1)
	require.Equal(t, "add", uploadResp.Rows[0].SuggestedAction)

	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Created)

	// 2. The DB row must hold the raw prefixes — import preserves, it never
	//    "fixes" or drops them.
	var persisted models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&persisted).Error)
	assert.Equal(t, "+SUM(A1)", persisted.Lastname, "leading-+ must survive import into the DB")
	assert.Equal(t, "-1", persisted.MiddleName, "leading-- must survive import into the DB")
	assert.Equal(t, "@EVALUATE(1)", persisted.Nickname, "leading-@ must survive import into the DB")
	assert.Equal(t, "-Acme Corp", persisted.Organization, "leading-- org must survive import into the DB")

	// 3. The CSV export boundary neutralizes the formula prefixes that reach
	//    an exported column (Firstname/Lastname/Nickname...). MiddleName and
	//    Organization have no CSV column — their preservation is pinned by
	//    the DB assertions above.
	exportReq, _ := http.NewRequest(http.MethodGet, "/export", nil)
	exportW := httptest.NewRecorder()
	router.ServeHTTP(exportW, exportReq)
	require.Equal(t, http.StatusOK, exportW.Code, exportW.Body.String())
	body := exportW.Body.Bytes()
	for _, want := range []string{
		"'+SUM(A1)",
		"'@EVALUATE(1)",
	} {
		assert.Containsf(t, string(body), want, "exported CSV must neutralize the formula prefix %q", want)
	}
	// And the raw (un-neutralized) cells must be gone from the wire.
	assert.NotContains(t, string(body), "\r\n+SUM(A1)", "the un-neutralized cell must not appear")
	assert.NotContains(t, string(body), "\r\n@EVALUATE(1)", "the un-neutralized cell must not appear")
}
