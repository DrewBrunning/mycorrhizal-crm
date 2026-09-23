package controllers

import (
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetUpcomingOccasions_RealMigratedSchema is the real-DB check (CLAUDE.md
// backend trap #1) for docs/adrs/0024-occasions.md / issue #387, ticket
// #1224: the aggregate query's raw SQL (anchor_month/anchor_day IS NOT NULL,
// sensitivity, active) must run against the real hand-written migration
// columns, not just AutoMigrate's derived schema.
func TestGetUpcomingOccasions_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "occasion-upcoming-realdb", Password: "password123!A", Email: "occasion-upcoming-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)

	now := time.Now().UTC()
	soon := now.AddDate(0, 0, 5)
	contact := models.Contact{UserID: user.ID, Firstname: "Rae", Birthday: soon.Format("2006-01-02")}
	require.NoError(t, db.Create(&contact).Error)

	obligationContact := models.Contact{UserID: user.ID, Firstname: "Obi"}
	require.NoError(t, db.Create(&obligationContact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: obligationContact.VCardUID, Kind: "card", Label: "Holiday card",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	registerOccasionRoutes(router)

	resp := doOccasionGET(router, "/occasions/upcoming?days=30")
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())

	var body struct {
		Occasions []models.UpcomingOccasion `json:"occasions"`
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))

	sources := map[string]bool{}
	for _, o := range body.Occasions {
		sources[o.Source] = true
	}
	assert.True(t, sources["birthday"])
	assert.True(t, sources["obligation"])
}
