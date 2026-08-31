package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImportVCF_CustomFieldCandidateSurface_PreviewAndConfirm exercises the
// issue #514 HTTP surface end to end against the real migrated schema
// (dbtest.New, per CLAUDE.md backend trap #1 — the AutoMigrate helpers that
// predate field_definitions would silently drop the whole feature): the VCF
// upload surfaces the discovered X-* property as a custom_field_candidate on
// the preview, and the confirm with a "create" mapping promotes it into a
// first-class custom field while stripping it from the saved passthrough.
func TestImportVCF_CustomFieldCandidateSurface_PreviewAndConfirm(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "cfmctrl", Password: "password123!A", Email: "cfmctrl@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	registerImportRoutes(router, &config.Config{})

	vcf := "BEGIN:VCARD\r\n" +
		"VERSION:4.0\r\n" +
		"FN:Cora Imported\r\n" +
		"N:Imported;Cora;;;\r\n" +
		"EMAIL:cora@example.com\r\n" +
		"UID:55555555-5555-4555-8555-555555555555\r\n" +
		"X-HOMETOWN:Springfield\r\n" +
		"END:VCARD\r\n"

	uploadReq := newFileUploadRequest(t, "/contacts/import/vcf/upload", "contacts.vcf", []byte(vcf))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, uploadReq)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	require.Len(t, preview.Rows, 1)
	require.Len(t, preview.Rows[0].CustomFieldCandidates, 1)
	cand := preview.Rows[0].CustomFieldCandidates[0]
	assert.Equal(t, "x-hometown", cand.Name)
	assert.Equal(t, "Springfield", cand.Value)
	assert.Empty(t, cand.MatchedDefinitionID, "no definition matches a third-party X- property")

	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: preview.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-hometown", Action: "create"},
		},
	})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, confirmReq)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var result models.ImportResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, 1, result.Created)

	var def models.FieldDefinition
	require.NoError(t, db.Where("user_id = ? AND key = ?", user.ID, "hometown").First(&def).Error)
	assert.Equal(t, "vcard:X-HOMETOWN", def.Projection)

	var contact models.Contact
	require.NoError(t, db.Where("user_id = ? AND email = ?", user.ID, "cora@example.com").First(&contact).Error)

	var fv models.FieldValue
	require.NoError(t, db.Where("entity_id = ? AND field_definition_id = ?", contact.VCardUID, def.ID).First(&fv).Error)
	assert.JSONEq(t, `"Springfield"`, string(fv.Value))

	var pt models.Contact
	require.NoError(t, db.Model(&pt).Select("passthrough").Where("id = ?", contact.ID).First(&pt).Error)
	for _, p := range pt.Passthrough.VCard {
		assert.NotEqual(t, "x-hometown", p.Name, "promoted property must not survive in passthrough")
	}
}

// TestImportVCF_CustomFieldMapping_RejectsForeignDefinition drives the
// ownership failure over HTTP: mapping a property into another user's
// definition is a 400 INVALID_INPUT on field_mappings, not a 500, and the
// import does not half-apply.
func TestImportVCF_CustomFieldMapping_RejectsForeignDefinition(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "cfmowner", Password: "password123!A", Email: "cfmowner@example.com"}
	require.NoError(t, db.Create(&user).Error)
	other := models.User{Username: "cfmforeign", Password: "password123!A", Email: "cfmforeign@example.com"}
	require.NoError(t, db.Create(&other).Error)

	foreignDef := models.FieldDefinition{
		UserID: other.ID, Label: "Foreign", Key: "foreign",
		Target: models.FieldDefinitionTargetContact, Type: models.FieldTypeText,
		Projection: "internal-only", Sensitivity: "normal",
	}
	require.NoError(t, db.Create(&foreignDef).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	registerImportRoutes(router, &config.Config{})

	vcf := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Cora Owner\r\nN:Owner;Cora;;;\r\nUID:66666666-6666-4666-8666-666666666666\r\nX-HOMETOWN:Springfield\r\nEND:VCARD\r\n"
	uploadReq := newFileUploadRequest(t, "/contacts/import/vcf/upload", "contacts.vcf", []byte(vcf))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, uploadReq)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var preview models.ImportPreviewResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))

	confirmReq := newJSONRequest(t, "/contacts/import/vcf/confirm", models.ImportConfirmRequest{
		SessionID: preview.SessionID,
		Actions:   []models.RowImportAction{{RowIndex: 0, Action: "add"}},
		FieldMappings: []models.ImportFieldMapping{
			{PropertyName: "x-hometown", Action: "map", FieldDefinitionID: foreignDef.ID},
		},
	})
	w = httptest.NewRecorder()
	router.ServeHTTP(w, confirmReq)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	env := decodeError(t, w)
	assert.Equal(t, "INVALID_INPUT", env.Error.Code)
	assert.Contains(t, strings.ToLower(env.Error.Details["reason"].(string)), "not one of your definitions")

	// Failed closed: no contact, no value on the foreign definition.
	var contactCount int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contactCount).Error)
	assert.Zero(t, contactCount)
	var valueCount int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("field_definition_id = ?", foreignDef.ID).Count(&valueCount).Error)
	assert.Zero(t, valueCount)
}
