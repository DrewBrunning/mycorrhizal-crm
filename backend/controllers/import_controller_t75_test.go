package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfirmImport_UpdateMergePreservesCardOnlyData pins T75 trigger 2 at
// the handler level against the real migrated schema (CLAUDE.md trap 1):
// importing a CSV row that matches an existing contact merges into it via
// MergeImportedContact (which mutates flat fields only) followed by
// tx.Save(&existing) — a plain save with no ApplyRecordToContact. Before
// T75's BeforeSave merge, that save silently destroyed the existing
// contact's Card-only data (pronouns, hobbies, apartment, pet kind,
// passthrough). The flow here is the real one: upload CSV → preview (the
// row is detected as a duplicate by email) → confirm with the "update"
// action.
func TestConfirmImport_UpdateMergePreservesCardOnlyData(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t75importc", Password: "password123!A", Email: "t75importc@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(existing, richCardOnlyRecordCtrl(), "")
	require.NoError(t, db.Create(existing).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", *cfg)
		c.Next()
	})
	registerImportRoutes(router, cfg)

	// A CSV row that shares the existing contact's email (so duplicate
	// detection flags it) and carries a phone the contact does not yet have
	// (so the merge actually changes something).
	csv := "First Name,Last Name,Email,Phone\nImported,Name,ada@example.com,+15559090909\n"
	uploadReq := newFileUploadRequest(t, "/contacts/import/upload", "contacts.csv", []byte(csv))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))

	previewReq := newJSONRequest(t, "/contacts/import/preview", models.ImportPreviewRequest{
		SessionID: uploadResp.SessionID,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Last Name", ContactField: "lastname"},
			{CSVColumn: "Email", ContactField: "email", Group: 0},
			{CSVColumn: "Phone", ContactField: "phone", Group: 0},
		},
	})
	previewW := httptest.NewRecorder()
	router.ServeHTTP(previewW, previewReq)
	require.Equal(t, http.StatusOK, previewW.Code, previewW.Body.String())
	var previewResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(previewW.Body.Bytes(), &previewResp))
	require.Len(t, previewResp.Rows, 1)
	require.NotNil(t, previewResp.Rows[0].DuplicateMatch, "the row must be detected as a duplicate of the existing contact")
	assert.Equal(t, existing.ID, previewResp.Rows[0].DuplicateMatch.ExistingContactID)
	assert.Equal(t, "update", previewResp.Rows[0].SuggestedAction)

	confirmReq := newJSONRequest(t, "/contacts/import/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())
	var result models.ImportResult
	require.NoError(t, json.Unmarshal(confirmW.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Updated)

	var persisted models.Contact
	require.NoError(t, db.First(&persisted, existing.ID).Error)
	assert.Equal(t, "Imported", persisted.Firstname, "the merge must have applied the incoming flat firstname")

	phoneValues := make([]string, 0, len(persisted.Phones))
	for _, p := range persisted.Phones {
		phoneValues = append(phoneValues, p.Value)
	}
	assert.Contains(t, phoneValues, "+15559090909", "the incoming phone must have been merged onto the existing contact")

	assertCardOnlyDataPreserved(t, persisted)
}

// TestConfirmImport_UpdateMergeNewAddressPreservesExistingUnprojected pins the
// per-entry address rule (T75 item 2) at the handler level: when an import
// merge APPENDS a different flat address, the existing address's unprojected
// components (apartment) must survive — the ticket's original whole-array
// dirty-comparison rule would have rebuilt the whole array from flat and
// destroyed them.
func TestConfirmImport_UpdateMergeNewAddressPreservesExistingUnprojected(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "t75importa", Password: "password123!A", Email: "t75importa@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(existing, richCardOnlyRecordCtrl(), "")
	require.NoError(t, db.Create(existing).Error)

	cfg := &config.Config{ProfilePhotoDir: t.TempDir()}
	gin.SetMode(gin.ReleaseMode)
	router := routerForUser(db, user.ID)
	router.Use(func(c *gin.Context) {
		c.Set("cfg", *cfg)
		c.Next()
	})
	registerImportRoutes(router, cfg)

	// A CSV row that matches by email and carries a DIFFERENT address than the
	// existing contact's (so the flat Addresses array grows).
	csv := "First Name,Last Name,Email,Address\nImported,Name,ada@example.com,456 Oak Ave\n"
	uploadReq := newFileUploadRequest(t, "/contacts/import/upload", "contacts.csv", []byte(csv))
	uploadW := httptest.NewRecorder()
	router.ServeHTTP(uploadW, uploadReq)
	require.Equal(t, http.StatusOK, uploadW.Code, uploadW.Body.String())
	var uploadResp models.ImportUploadResponse
	require.NoError(t, json.Unmarshal(uploadW.Body.Bytes(), &uploadResp))

	previewReq := newJSONRequest(t, "/contacts/import/preview", models.ImportPreviewRequest{
		SessionID: uploadResp.SessionID,
		Mappings: []models.ColumnMapping{
			{CSVColumn: "First Name", ContactField: "firstname"},
			{CSVColumn: "Last Name", ContactField: "lastname"},
			{CSVColumn: "Email", ContactField: "email", Group: 0},
			{CSVColumn: "Address", ContactField: "address_street", Group: 0},
		},
	})
	previewW := httptest.NewRecorder()
	router.ServeHTTP(previewW, previewReq)
	require.Equal(t, http.StatusOK, previewW.Code, previewW.Body.String())
	var previewResp models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(previewW.Body.Bytes(), &previewResp))
	require.NotNil(t, previewResp.Rows[0].DuplicateMatch)

	confirmReq := newJSONRequest(t, "/contacts/import/confirm", models.ImportConfirmRequest{
		SessionID: uploadResp.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "update"}},
	})
	confirmW := httptest.NewRecorder()
	router.ServeHTTP(confirmW, confirmReq)
	require.Equal(t, http.StatusOK, confirmW.Code, confirmW.Body.String())

	var persisted models.Contact
	require.NoError(t, db.First(&persisted, existing.ID).Error)
	require.Len(t, persisted.Card.Addresses, 2, "the imported address must be appended alongside the existing one")

	components := map[string]string{}
	for _, comp := range persisted.Card.Addresses[0].Components {
		components[comp.Kind] = comp.Value
	}
	if components["apartment"] != "Apt 3B" {
		t.Errorf("existing address apartment lost when a new address was imported (components=%v)", components)
	}

	// The appended address is the flat-only street from the CSV.
	var appended string
	for _, comp := range persisted.Card.Addresses[1].Components {
		if comp.Kind == "name" {
			appended = comp.Value
		}
	}
	assert.Equal(t, "456 Oak Ave", appended)
}
