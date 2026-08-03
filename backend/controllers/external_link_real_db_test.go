package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExternalLinkSubstrate_RealMigratedSchema is the real-DB check for T14:
// every other controller test uses AutoMigrate against :memory: sqlite, which
// derives its schema from the same Go struct tags the application code uses —
// it cannot catch a GORM column-tag mismatch against the real hand-written
// migration SQL (this fork's own recurring bug class, e.g. ContactSyncLink.
// ETag). This test exercises the ExternalIdentity/ExternalActivity CRUD
// routes plus DeleteContact's cascade against a database.InitDB-migrated real
// file database instead.
func TestExternalLinkSubstrate_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "external-link-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "ext-realdb", Password: "password123!A", Email: "ext-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	router.POST("/external-identities", withValidated(func() any { return &models.ExternalIdentityInput{} }), CreateExternalIdentity)
	router.POST("/external-activities", withValidated(func() any { return &models.ExternalActivityInput{} }), CreateExternalActivity)
	router.DELETE("/contacts/:id", DeleteContact)

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

	// Create an identity + an activity through the real routes. The metadata /
	// payload JSON columns must survive the real schema's serializer.
	idResp := doJSON("POST", "/external-identities", models.ExternalIdentityInput{
		EntityID: contact.VCardUID, System: "immich", ExternalID: "person-real",
		URL:      "https://immich.example/people/person-real",
		Metadata: map[string]interface{}{"person_name": "Alice", "photo_count": 12},
	})
	require.Equal(t, http.StatusCreated, idResp.Code, idResp.Body.String())

	actResp := doJSON("POST", "/external-activities", models.ExternalActivityInput{
		EntityID: contact.VCardUID, SourceSystem: "immich", ExternalID: "asset-real",
		Type: "photo-appearance", OccurredAt: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC),
		Payload: map[string]interface{}{"asset_id": "asset-real", "person_name": "Alice"},
	})
	require.Equal(t, http.StatusCreated, actResp.Code, actResp.Body.String())

	// Round-trip check on the real schema: reload and confirm the JSON
	// columns parse back (a serializer/column-name mismatch would show here).
	var identity models.ExternalIdentity
	require.NoError(t, db.Where("system = ? AND external_id = ? AND user_id = ?", "immich", "person-real", user.ID).First(&identity).Error)
	assert.Equal(t, "Alice", identity.Metadata["person_name"])
	assert.EqualValues(t, 12, identity.Metadata["photo_count"])

	var activity models.ExternalActivity
	require.NoError(t, db.Where("source_system = ? AND external_id = ? AND user_id = ?", "immich", "asset-real", user.ID).First(&activity).Error)
	assert.Equal(t, "Alice", activity.Payload["person_name"])

	// Unique constraint: a duplicate (system, external_id, user) is rejected
	// at the DB level too (belt-and-braces beyond the controller's 409).
	dup := models.ExternalIdentity{UserID: user.ID, EntityID: contact.VCardUID, System: "immich", ExternalID: "person-real"}
	require.Error(t, db.Create(&dup).Error, "unique (system, external_id, user_id) must reject a duplicate")

	// Cascade on contact delete: both rows must be gone after DeleteContact.
	delResp := doJSON("DELETE", "/contacts/"+strconv.Itoa(int(contact.ID)), nil)
	require.Equal(t, http.StatusOK, delResp.Code, delResp.Body.String())
	var idCount, actCount int64
	require.NoError(t, db.Model(&models.ExternalIdentity{}).Where("user_id = ?", user.ID).Count(&idCount).Error)
	require.NoError(t, db.Model(&models.ExternalActivity{}).Where("user_id = ?", user.ID).Count(&actCount).Error)
	assert.EqualValues(t, 0, idCount, "ExternalIdentity rows must cascade on contact delete")
	assert.EqualValues(t, 0, actCount, "ExternalActivity rows must cascade on contact delete")
}
