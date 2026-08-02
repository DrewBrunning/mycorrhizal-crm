package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCustomFieldsRealMigratedSchema is the real-DB check for T6
// (docs/fork-plan/tickets/11-T6-custom-fields-api.md): the ticket's own trap
// is that AutoMigrate-based tests cannot see a schema mismatch between the
// Go struct tags and the hand-written migration SQL (the recurring bug class
// behind ContactSyncLink.ETag). This test runs the FieldDefinition/FieldValue
// handlers against a database.InitDB-migrated real file database, round-
// tripping a definition + a typed value through the HTTP surface, and
// pinning the FK cascade that AutoMigrate does not replicate.
func TestCustomFieldsRealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "custom-fields-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "realdbtester", Password: "password123!A", Email: "realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/field-definitions", withValidated(func() any { return &models.FieldDefinitionInput{} }), CreateFieldDefinition)
	router.GET("/contacts/:id/field-values", ListContactFieldValues)
	router.PUT("/contacts/:id/field-values", withValidated(func() any { return &models.ContactFieldValuesInput{} }), ReplaceContactFieldValues)
	router.DELETE("/field-definitions/:id", DeleteFieldDefinition)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Create a Multi enum definition through the real HTTP surface.
	createResp := doJSON("POST", "/field-definitions", models.FieldDefinitionInput{
		Label:      "Pronouns",
		Key:        "pronouns",
		Type:       models.FieldTypeEnum,
		Projection: "vcard:X-PRONOUNS",
		Constraints: models.FieldConstraints{
			Values: []string{"she/her", "he/him", "they/them"},
			Multi:  true,
		},
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		FieldDefinition models.FieldDefinition `json:"field_definition"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	defID := created.FieldDefinition.ID
	require.NotEmpty(t, defID)

	// Set a Multi value; the column-name derivation (field_definitions.* /
	// field_values.*) is what this real-DB test exists to catch.
	putResp := doJSON("PUT", "/contacts/"+strconv.Itoa(int(contact.ID))+"/field-values", models.ContactFieldValuesInput{
		FieldValues: []models.FieldValueInput{
			{FieldDefinitionID: defID, Value: json.RawMessage(`["she/her","they/them"]`)},
		},
	})
	require.Equal(t, http.StatusOK, putResp.Code, putResp.Body.String())

	// Read it back.
	getResp := doJSON("GET", "/contacts/"+strconv.Itoa(int(contact.ID))+"/field-values", nil)
	require.Equal(t, http.StatusOK, getResp.Code)
	var list struct {
		FieldValues []models.FieldValue `json:"field_values"`
	}
	require.NoError(t, json.Unmarshal(getResp.Body.Bytes(), &list))
	require.Len(t, list.FieldValues, 1)
	assert.Equal(t, defID, list.FieldValues[0].FieldDefinitionID)
	assert.JSONEq(t, `["she/her","they/them"]`, string(list.FieldValues[0].Value))

	// A secret-sensitivity definition round-trips through the internal API
	// (owner's own data) but its value is kept out of the vCard projection
	// (projectCustomFields filters sensitivity='normal' in the query) — both
	// halves of the ticket's "respect sensitivity" requirement.
	secretResp := doJSON("POST", "/field-definitions", models.FieldDefinitionInput{
		Label:       "HIV Status",
		Key:         "hiv_status",
		Type:        models.FieldTypeString,
		Projection:  "vcard:X-HIV-STATUS",
		Sensitivity: models.RelationshipSensitivitySecret,
	})
	require.Equal(t, http.StatusCreated, secretResp.Code, secretResp.Body.String())
	var secretCreated struct {
		FieldDefinition models.FieldDefinition `json:"field_definition"`
	}
	require.NoError(t, json.Unmarshal(secretResp.Body.Bytes(), &secretCreated))

	putSecret := doJSON("PUT", "/contacts/"+strconv.Itoa(int(contact.ID))+"/field-values", models.ContactFieldValuesInput{
		FieldValues: []models.FieldValueInput{
			{FieldDefinitionID: secretCreated.FieldDefinition.ID, Value: json.RawMessage(`"positive"`)},
		},
	})
	require.Equal(t, http.StatusOK, putSecret.Code, putSecret.Body.String())

	var storedSecret models.FieldValue
	require.NoError(t, db.Where("field_definition_id = ?", secretCreated.FieldDefinition.ID).First(&storedSecret).Error)
	assert.Equal(t, `"positive"`, string(storedSecret.Value), "the internal API stores and returns the secret value")

	record := models.RecordForContact(&contact, "", db)
	for _, prop := range record.Passthrough.VCard {
		assert.NotEqual(t, "X-HIV-STATUS", prop.Name, "a secret-sensitivity field must not project to vCard")
	}

	// FK cascade: deleting the definition (real ON DELETE CASCADE from the
	// hand-written migration, which AutoMigrate does not replicate) removes
	// its values too.
	delResp := doJSON("DELETE", "/field-definitions/"+defID, nil)
	require.Equal(t, http.StatusOK, delResp.Code, delResp.Body.String())
	var orphanCount int64
	require.NoError(t, db.Model(&models.FieldValue{}).Where("field_definition_id = ?", defID).Count(&orphanCount).Error)
	assert.Zero(t, orphanCount, "FieldValues cascade-delete with their definition on the real schema")
}
