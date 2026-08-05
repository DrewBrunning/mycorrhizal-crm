package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkFieldType_RealMigratedSchema is the real-DB check for T34 (per
// CLAUDE.md trap #1): runs LinkFieldType CRUD, lazy per-user default
// seeding, the partial-unique-index-after-soft-delete case, the duplicate-
// name 409, and reorder against a database.InitDB-migrated real file
// database rather than AutoMigrate, so a column-tag/migration-SQL mismatch
// (this fork's own recurring bug class) would be caught here.
func TestLinkFieldType_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "link-field-types-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "linkfieldtype-realdb", Password: "password123!A", Email: "linkfieldtype-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	// Uses the real ValidateJSONMiddleware (not the withValidated test shim)
	// specifically so the safeurl validator genuinely runs — the case below
	// that asserts a javascript: Protocol is rejected needs real validation,
	// not just JSON binding.
	router.POST("/link-field-types", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeInput{}), CreateLinkFieldType)
	router.GET("/link-field-types", ListLinkFieldTypes)
	router.GET("/link-field-types/:id", GetLinkFieldType)
	router.PUT("/link-field-types/reorder", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeReorderInput{}), ReorderLinkFieldTypes)
	router.PUT("/link-field-types/:id", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeInput{}), UpdateLinkFieldType)
	router.DELETE("/link-field-types/:id", DeleteLinkFieldType)

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

	// First list call lazily seeds the defaults — must include every seeded
	// name, land in the real `link_field_types` table (not just AutoMigrate's
	// derived schema), and be flagged is_default.
	listResp := doJSON("GET", "/link-field-types", nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		LinkFieldTypes []models.LinkFieldType `json:"link_field_types"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.LinkFieldTypes, len(models.LinkFieldTypeDefaults))
	byName := map[string]models.LinkFieldType{}
	for _, lt := range listed.LinkFieldTypes {
		byName[lt.Name] = lt
	}
	whatsapp, ok := byName["WhatsApp"]
	require.True(t, ok, "WhatsApp default must be seeded")
	assert.True(t, whatsapp.IsDefault)
	assert.Equal(t, "https://wa.me/{value}", whatsapp.Protocol)
	// Discord has no stable public profile-by-handle URL — seeded with an
	// empty (not missing) protocol, per the ticket's own call.
	discord, ok := byName["Discord"]
	require.True(t, ok, "Discord default must be seeded even with an empty protocol")
	assert.Empty(t, discord.Protocol)

	// A second list call must not duplicate the seed set.
	listResp2 := doJSON("GET", "/link-field-types", nil)
	require.Equal(t, http.StatusOK, listResp2.Code, listResp2.Body.String())
	var listed2 struct {
		LinkFieldTypes []models.LinkFieldType `json:"link_field_types"`
	}
	require.NoError(t, json.Unmarshal(listResp2.Body.Bytes(), &listed2))
	require.Len(t, listed2.LinkFieldTypes, len(models.LinkFieldTypeDefaults), "seeding must not run twice")

	// Create a custom type.
	createResp := doJSON("POST", "/link-field-types", models.LinkFieldTypeInput{
		Name: "Custom Service", Protocol: "https://example.com/{value}", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		LinkFieldType models.LinkFieldType `json:"link_field_type"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	customID := created.LinkFieldType.ID
	require.NotEmpty(t, customID)
	assert.False(t, created.LinkFieldType.IsDefault)

	// Duplicate active name -> 409, not a sniffed constraint error.
	dupResp := doJSON("POST", "/link-field-types", models.LinkFieldTypeInput{
		Name: "Custom Service", Protocol: "https://example.com/{value}", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusConflict, dupResp.Code, dupResp.Body.String())

	// javascript: scheme in Protocol -> rejected by the safeurl validator at
	// the JSON-binding layer (400), never reaches the database.
	unsafeResp := doJSON("POST", "/link-field-types", models.LinkFieldTypeInput{
		Name: "Malicious", Protocol: "javascript:alert(1)//{value}", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusBadRequest, unsafeResp.Code, unsafeResp.Body.String())

	// Update (full replace) — editing a seeded default keeps it flagged
	// is_default (provenance, not "unmodified since seeding").
	discordID := byName["Discord"].ID
	updateResp := doJSON("PUT", "/link-field-types/"+discordID, models.LinkFieldTypeInput{
		Name: "Discord", Protocol: "https://discord.com/users/{value}", Category: models.LinkFieldTypeCategoryMessaging,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())
	var updatedDiscord models.LinkFieldType
	require.NoError(t, json.Unmarshal(updateResp.Body.Bytes(), &updatedDiscord))
	assert.Equal(t, "https://discord.com/users/{value}", updatedDiscord.Protocol)
	assert.True(t, updatedDiscord.IsDefault, "editing a default must not clear is_default")

	// Soft-delete, then recreate with the same name — the partial unique
	// index idx_link_field_types_user_name must not block it.
	deleteResp := doJSON("DELETE", "/link-field-types/"+customID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	recreateResp := doJSON("POST", "/link-field-types", models.LinkFieldTypeInput{
		Name: "Custom Service", Protocol: "https://example.org/{value}", Category: models.LinkFieldTypeCategoryOther,
	})
	require.Equal(t, http.StatusCreated, recreateResp.Code, recreateResp.Body.String())
	var recreated struct {
		LinkFieldType models.LinkFieldType `json:"link_field_type"`
	}
	require.NoError(t, json.Unmarshal(recreateResp.Body.Bytes(), &recreated))
	assert.NotEqual(t, customID, recreated.LinkFieldType.ID, "recreated row is a new soft-delete-surviving row, not the old one")

	// Getting the soft-deleted original by ID now 404s (scoped queries
	// exclude soft-deleted rows).
	getDeletedResp := doJSON("GET", "/link-field-types/"+customID, nil)
	require.Equal(t, http.StatusNotFound, getDeletedResp.Code, getDeletedResp.Body.String())

	// Reorder: move the recreated custom type to the front.
	finalListResp := doJSON("GET", "/link-field-types", nil)
	require.Equal(t, http.StatusOK, finalListResp.Code, finalListResp.Body.String())
	var finalListed struct {
		LinkFieldTypes []models.LinkFieldType `json:"link_field_types"`
	}
	require.NoError(t, json.Unmarshal(finalListResp.Body.Bytes(), &finalListed))
	order := make([]string, len(finalListed.LinkFieldTypes))
	order[0] = recreated.LinkFieldType.ID
	i := 1
	for _, lt := range finalListed.LinkFieldTypes {
		if lt.ID == recreated.LinkFieldType.ID {
			continue
		}
		order[i] = lt.ID
		i++
	}
	reorderResp := doJSON("PUT", "/link-field-types/reorder", models.LinkFieldTypeReorderInput{Order: order})
	require.Equal(t, http.StatusOK, reorderResp.Code, reorderResp.Body.String())
	var reordered struct {
		LinkFieldTypes []models.LinkFieldType `json:"link_field_types"`
	}
	require.NoError(t, json.Unmarshal(reorderResp.Body.Bytes(), &reordered))
	var reorderedCustom models.LinkFieldType
	require.NoError(t, db.First(&reorderedCustom, "id = ?", recreated.LinkFieldType.ID).Error)
	assert.Equal(t, 0, reorderedCustom.Position)

	// Reorder with an ID that doesn't belong to the user -> rejected outright.
	badReorderResp := doJSON("PUT", "/link-field-types/reorder", models.LinkFieldTypeReorderInput{Order: []string{"00000000-0000-0000-0000-000000000000"}})
	require.Equal(t, http.StatusBadRequest, badReorderResp.Code, badReorderResp.Body.String())
}
