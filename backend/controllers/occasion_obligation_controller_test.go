package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func registerOccasionObligationRoutes(t *testing.T, router *gin.Engine) {
	router.POST("/occasion-obligations", withValidated(func() any { return &models.OccasionObligationInput{} }), CreateOccasionObligation)
	router.GET("/occasion-obligations", ListOccasionObligations)
	router.GET("/occasion-obligations/:id", GetOccasionObligation)
	router.PUT("/occasion-obligations/:id", withValidated(func() any { return &models.OccasionObligationInput{} }), UpdateOccasionObligation)
	router.DELETE("/occasion-obligations/:id", DeleteOccasionObligation)
}

func seedOccasionObligationContact(t *testing.T, db *gorm.DB, userID uint) models.Contact {
	t.Helper()
	contact := models.Contact{UserID: userID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func doOccasionJSON(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCreateOccasionObligation(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	payload := models.OccasionObligationInput{
		EntityID:     contact.VCardUID,
		Kind:         models.OccasionObligationKindCard,
		Label:        "Christmas card",
		AnchorMonth:  intPtr(12),
		AnchorDay:    intPtr(25),
		LeadTimeDays: 14,
		Notes:        "Send to their new address",
	}
	w := doOccasionJSON(router, "POST", "/occasion-obligations", payload)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 1, count)

	var obligation models.OccasionObligation
	db.First(&obligation)
	assert.Equal(t, models.RelationshipSensitivityNormal, obligation.Sensitivity, "omitted sensitivity must default to normal")
	assert.True(t, obligation.Active, "omitted active must default to true")
	assert.Equal(t, "Christmas card", obligation.Label)
	assert.Equal(t, 12, *obligation.AnchorMonth)
	assert.Equal(t, 25, *obligation.AnchorDay)
	assert.Equal(t, "Send to their new address", obligation.Notes, "notes must persist on create")
}

func TestCreateOccasionObligationRejectsContactFromAnotherUser(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := seedOccasionObligationContact(t, db, otherUser.ID)

	payload := models.OccasionObligationInput{
		EntityID: othersContact.VCardUID,
		Kind:     models.OccasionObligationKindCard,
		Label:    "Christmas card",
	}
	w := doOccasionJSON(router, "POST", "/occasion-obligations", payload)
	assert.Equal(t, http.StatusNotFound, w.Code)

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count)
}

func TestCreateOccasionObligationRejectsPartialAnchor(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	// Month with no day.
	w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:    contact.VCardUID,
		Kind:        models.OccasionObligationKindCard,
		Label:       "Christmas card",
		AnchorMonth: intPtr(12),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// Day with no month.
	w = doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID:  contact.VCardUID,
		Kind:      models.OccasionObligationKindCard,
		Label:     "Christmas card",
		AnchorDay: intPtr(25),
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count, "neither partial-anchor request should have persisted a row")
}

func TestCreateOccasionObligationAllowsNilAnchor(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	// Neither month nor day — a valid "date TBD" obligation (ADR 0024).
	w := doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID: contact.VCardUID,
		Kind:     models.OccasionObligationKindInvite,
		Label:    "Annual summer BBQ",
	})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestListOccasionObligationsFiltersByEntity(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contactA := seedOccasionObligationContact(t, db, user.ID)
	contactB := seedOccasionObligationContact(t, db, user.ID)

	require.NoError(t, db.Create(&models.OccasionObligation{UserID: user.ID, EntityID: contactA.VCardUID, Kind: "card", Label: "A card"}).Error)
	require.NoError(t, db.Create(&models.OccasionObligation{UserID: user.ID, EntityID: contactB.VCardUID, Kind: "card", Label: "B card"}).Error)

	w := doOccasionJSON(router, "GET", "/occasion-obligations?entity_id="+contactA.VCardUID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		OccasionObligations []models.OccasionObligation `json:"occasion_obligations"`
		Total               int64                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.OccasionObligations, 1)
	assert.Equal(t, "A card", resp.OccasionObligations[0].Label)
	assert.EqualValues(t, 1, resp.Total)
}

func TestUpdateOccasionObligationDeactivate(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	obligation := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card", Active: true}
	require.NoError(t, db.Create(&obligation).Error)

	inactive := false
	w := doOccasionJSON(router, "PUT", "/occasion-obligations/"+obligation.ID, models.OccasionObligationInput{
		EntityID: contact.VCardUID,
		Kind:     "card",
		Label:    "Christmas card",
		Active:   &inactive,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.OccasionObligation
	require.NoError(t, db.First(&reloaded, "id = ?", obligation.ID).Error)
	assert.False(t, reloaded.Active, "explicit active=false must persist, not default back to true")
}

func TestDeleteOccasionObligationSoftDeletesAndAllowsRecreate(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	contact := seedOccasionObligationContact(t, db, user.ID)

	obligation := models.OccasionObligation{UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Christmas card"}
	require.NoError(t, db.Create(&obligation).Error)

	w := doOccasionJSON(router, "DELETE", "/occasion-obligations/"+obligation.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var count int64
	db.Model(&models.OccasionObligation{}).Count(&count)
	assert.EqualValues(t, 0, count, "soft-deleted row must not appear in the default-scoped count")

	var unscopedCount int64
	db.Unscoped().Model(&models.OccasionObligation{}).Count(&unscopedCount)
	assert.EqualValues(t, 1, unscopedCount, "soft-deleted row must still exist (tombstone)")

	// No natural-key unique constraint: recreating for the same contact/kind
	// must succeed even though the old row still exists, soft-deleted.
	w = doOccasionJSON(router, "POST", "/occasion-obligations", models.OccasionObligationInput{
		EntityID: contact.VCardUID,
		Kind:     "card",
		Label:    "Christmas card",
	})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
}

func TestGetOccasionObligationRejectsOtherUser(t *testing.T) {
	db, router := setupRouter()
	registerOccasionObligationRoutes(t, router)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other2", Password: "x", Email: "other2@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := seedOccasionObligationContact(t, db, otherUser.ID)

	obligation := models.OccasionObligation{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Kind: "card", Label: "Not yours"}
	require.NoError(t, db.Create(&obligation).Error)

	w := doOccasionJSON(router, "GET", "/occasion-obligations/"+obligation.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
