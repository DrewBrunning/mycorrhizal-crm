package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issues #2662 (UTF-8 BOM) and #2669 (non-UTF-8 CSV): the upload decoder must
// strip a BOM and read a non-UTF-8 file as Windows-1252 with a visible warning,
// never leave U+FFFD in the stored data. The fixtures are real bytes on disk
// (testdata/import/*), not strings built in the test, so the encoding under
// test is the encoding on the wire.

func importFixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "import", name))
	require.NoError(t, err)
	return b
}

type encodingFileDiag struct {
	Severity string `json:"severity"`
	Concept  string `json:"concept"`
	Message  string `json:"message"`
}

func fileDiagsFrom(t *testing.T, body []byte) []encodingFileDiag {
	t.Helper()
	var raw struct {
		FileDiagnostics []encodingFileDiag `json:"file_diagnostics"`
	}
	require.NoError(t, json.Unmarshal(body, &raw))
	return raw.FileDiagnostics
}

func TestImportUploadCSV_BOMHeaderMapsFirstColumn(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "bom.csv", importFixtureBytes(t, "bom.csv")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Headers)
	assert.Equal(t, "First Name", resp.Headers[0], "the BOM must not leak into the first header")
	require.NotEmpty(t, resp.SuggestedMappings)
	assert.Equal(t, "firstname", resp.SuggestedMappings[0].ContactField)
	assert.Empty(t, fileDiagsFrom(t, w.Body.Bytes()), "a BOM is not worth a warning")
}

func TestImportUploadCSV_Latin1ReadAsWindows1252WithWarning(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "latin1.csv", importFixtureBytes(t, "latin1.csv")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.SampleData, 1)
	assert.Equal(t, []string{"José", "Müller", "jose.latin1@example.com"}, resp.SampleData[0])
	assert.False(t, strings.ContainsRune(w.Body.String(), '\uFFFD'))

	diags := fileDiagsFrom(t, w.Body.Bytes())
	require.Len(t, diags, 1)
	assert.Equal(t, "warn", diags[0].Severity)
	assert.Equal(t, "encoding", diags[0].Concept)
	assert.Equal(t, "file was not UTF-8; read as Windows-1252", diags[0].Message)
}

func TestImportCSV_Latin1PreviewAndConfirmStoreDecodedText(t *testing.T) {
	db, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/upload", "latin1.csv", importFixtureBytes(t, "latin1.csv")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var up models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))

	pw := httptest.NewRecorder()
	router.ServeHTTP(pw, newJSONRequest(t, "/contacts/import/preview", models.ImportPreviewRequest{
		SessionID: up.SessionID,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Last Name", ContactField: "lastname"},
			{CSVColumn: "Email", ContactField: "email"},
		},
	}))
	require.Equal(t, http.StatusOK, pw.Code, pw.Body.String())
	assert.False(t, strings.ContainsRune(pw.Body.String(), '\uFFFD'))
	diags := fileDiagsFrom(t, pw.Body.Bytes())
	require.Len(t, diags, 1, "the encoding warning must survive into the preview response")
	assert.Equal(t, "encoding", diags[0].Concept)

	cw := httptest.NewRecorder()
	router.ServeHTTP(cw, newJSONRequest(t, "/contacts/import/confirm", models.ImportConfirmRequest{
		SessionID: up.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
	}))
	require.Equal(t, http.StatusOK, cw.Code, cw.Body.String())

	var c models.Contact
	require.NoError(t, db.Where("firstname = ?", "José").First(&c).Error)
	assert.Equal(t, "Müller", c.Lastname)
	assert.False(t, strings.ContainsRune(c.Firstname+c.Lastname, '�'))
}

func TestImportUploadVCF_BOMIsStripped(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/vcf/upload", "bom.vcf", importFixtureBytes(t, "bom.vcf")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.TotalRows, "a leading BOM must not hide the first BEGIN:VCARD line")
	assert.Equal(t, 1, resp.ValidRows)
	assert.Empty(t, fileDiagsFrom(t, w.Body.Bytes()))
}

func TestImportUploadVCF_Latin1ReadAsWindows1252WithWarning(t *testing.T) {
	_, router := setupRouter(t)
	registerImportRoutes(router, &config.Config{})

	w := httptest.NewRecorder()
	router.ServeHTTP(w, newFileUploadRequest(t, "/contacts/import/vcf/upload", "latin1.vcf", importFixtureBytes(t, "latin1.vcf")))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "José")
	assert.False(t, strings.ContainsRune(w.Body.String(), '\uFFFD'))
	diags := fileDiagsFrom(t, w.Body.Bytes())
	require.Len(t, diags, 1)
	assert.Equal(t, "encoding", diags[0].Concept)
	assert.Equal(t, "file was not UTF-8; read as Windows-1252", diags[0].Message)
}
