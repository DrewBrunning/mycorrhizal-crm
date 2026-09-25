package controllers

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOccasionObligation_RealMigratedSchema is the real-DB check (CLAUDE.md
// backend trap #1) for docs/adrs/0024-occasions.md / issue #1222: when written, every
// other controller test in this package ran against AutoMigrate on :memory:
// sqlite, which derives its schema from the same Go struct tags the
// application code uses — it cannot catch a GORM column-tag mismatch against
// the real hand-written migration SQL (this fork's own recurring bug class).
// This test runs the OccasionObligation CRUD surface against a
// database.InitDB-migrated real file database instead: create (encrypted
// notes column + anchor month/day) -> list by entity -> update (deactivate)
// -> soft-delete -> recreate (no natural-key unique index to block it).
func TestOccasionObligation_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "occasion-realdb", Password: "password123!A", Email: "occasion-realdb@example.com"}
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
	registerOccasionObligationRoutes(t, router)

	// Create — the encrypted `notes` column and the anchor_month/anchor_day
	// columns must round-trip through the real migration, not just
	// AutoMigrate's derived schema.
	createResp := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:     contact.VCardUID,
		Kind:         models.OccasionObligationKindGift,
		Label:        "Birthday gift",
		AnchorMonth:  intPtr(7),
		AnchorDay:    intPtr(4),
		LeadTimeDays: 10,
		Notes:        "Likes board games",
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		OccasionObligation models.OccasionObligation `json:"occasion_obligation"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	obligationID := created.OccasionObligation.ID
	require.NotEmpty(t, obligationID)

	var stored models.OccasionObligation
	require.NoError(t, db.First(&stored, "id = ?", obligationID).Error)
	assert.Equal(t, "Likes board games", stored.Notes, "encrypted notes column must round-trip")
	require.NotNil(t, stored.AnchorMonth)
	require.NotNil(t, stored.AnchorDay)
	assert.Equal(t, 7, *stored.AnchorMonth)
	assert.Equal(t, 4, *stored.AnchorDay)
	assert.True(t, stored.Active)
	assert.Equal(t, models.RelationshipSensitivityNormal, stored.Sensitivity)

	// List by entity.
	listResp := doOccasionJSON(router, "GET", "/occasion-obligations?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		OccasionObligations []models.OccasionObligation `json:"occasion_obligations"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.OccasionObligations, 1)

	// Update: deactivate.
	inactive := false
	updateResp := doOccasionJSON(router, "PUT", "/occasion-obligations/"+obligationID, models.OccasionObligationInput{
		EntityID: contact.VCardUID,
		Kind:     models.OccasionObligationKindGift,
		Label:    "Birthday gift",
		Active:   &inactive,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())
	var reloaded models.OccasionObligation
	require.NoError(t, db.First(&reloaded, "id = ?", obligationID).Error)
	assert.False(t, reloaded.Active)
	assert.Nil(t, reloaded.AnchorMonth, "full-replace update must clear an omitted anchor")

	// Soft-delete, then recreate for the same contact/kind (no natural-key
	// unique index to block it, unlike CadencePolicy's partial index).
	deleteResp := doOccasionJSON(router, "DELETE", "/occasion-obligations/"+obligationID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	recreateResp := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID: contact.VCardUID,
		Kind:     models.OccasionObligationKindGift,
		Label:    "Birthday gift",
	})
	require.Equal(t, http.StatusCreated, recreateResp.Code, recreateResp.Body.String())
}
