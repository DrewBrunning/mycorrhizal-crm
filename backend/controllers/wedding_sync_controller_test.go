package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mycorrhizal/contactmodel"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Controller-level wiring tests for the wedding-anniversary <-> married
// LifeEvent sync (services/wedding_sync.go). The service itself is covered in
// services/wedding_sync_test.go; these pin that the HTTP handlers actually
// call it in both directions.

func weddingContactPayload(date *contactmodel.PartialDate) models.ContactRecordInput {
	return models.ContactRecordInput{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}}},
			Anniversaries: []contactmodel.Anniversary{
				{Kind: "wedding", Date: contactmodel.AnniversaryDate{Partial: date}},
			},
		},
	}
}

func createControllerContactWithWedding(t *testing.T, db *gorm.DB, userID uint) models.Contact {
	t.Helper()
	rec := &contactmodel.Record{
		Card: contactmodel.Card{
			Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}}},
			Anniversaries: []contactmodel.Anniversary{
				{Kind: "wedding", Date: contactmodel.AnniversaryDate{
					Partial: &contactmodel.PartialDate{Year: intPtr(2009), Month: intPtr(8), Day: intPtr(16)},
				}},
			},
		},
	}
	contact := models.Contact{UserID: userID}
	models.ApplyRecordToContact(&contact, rec, "")
	require.NoError(t, db.Create(&contact).Error)
	return contact
}

func TestCreateContactWeddingAnniversaryCreatesMarriedLifeEvent(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)

	jsonValue, _ := json.Marshal(weddingContactPayload(
		&contactmodel.PartialDate{Year: intPtr(2009), Month: intPtr(8), Day: intPtr(16)},
	))

	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var user models.User
	db.First(&user)
	var event models.LifeEvent
	require.NoError(t, db.Where("user_id = ? AND type = ?", user.ID, models.LifeEventTypeMarried).First(&event).Error)
	require.NotNil(t, event.Date)
	assert.Equal(t, 2009, *event.Date.Year)
	assert.Equal(t, 8, *event.Date.Month)
	assert.Equal(t, 16, *event.Date.Day)
}

func TestUpdateContactWeddingAnniversaryUpdatesMarriedLifeEvent(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)

	var user models.User
	db.First(&user)

	// Create a contact with a wedding anniversary through the API.
	jsonValue, _ := json.Marshal(weddingContactPayload(
		&contactmodel.PartialDate{Year: intPtr(2009), Month: intPtr(8), Day: intPtr(16)},
	))
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	id := uint(responseBody["contact"].(map[string]any)["id"].(float64))

	var created models.Contact
	require.NoError(t, db.First(&created, id).Error)
	var event models.LifeEvent
	require.NoError(t, db.Where("entity_id = ? AND user_id = ? AND type = ?", created.VCardUID, user.ID, models.LifeEventTypeMarried).First(&event).Error)
	require.NotNil(t, event.Date)
	assert.Equal(t, 2009, *event.Date.Year)

	// Move the wedding date; the life event follows.
	jsonValue, _ = json.Marshal(weddingContactPayload(
		&contactmodel.PartialDate{Year: intPtr(2011), Month: intPtr(5), Day: intPtr(20)},
	))
	req, _ = http.NewRequest("PUT", fmt.Sprintf("/contacts/%d", id), bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var reloaded models.LifeEvent
	require.NoError(t, db.Where("id = ?", event.ID).First(&reloaded).Error)
	require.NotNil(t, reloaded.Date)
	assert.Equal(t, 2011, *reloaded.Date.Year)
	assert.Equal(t, 5, *reloaded.Date.Month)
	assert.Equal(t, 20, *reloaded.Date.Day)
}

func TestCreateMarriedLifeEventSetsWeddingAnniversary(t *testing.T) {
	db, router := setupRouter()
	router.POST("/life-events", withValidated(func() any { return &models.LifeEventInput{} }), CreateLifeEvent)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	payload := models.LifeEventInput{
		EntityID: contact.VCardUID,
		Type:     models.LifeEventTypeMarried,
		Date:     &contactmodel.PartialDate{Year: intPtr(2011), Month: intPtr(5), Day: intPtr(20)},
	}
	jsonValue, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "/life-events", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	date := services.WeddingDateFromCard(&reloaded.Card)
	require.NotNil(t, date)
	assert.Equal(t, 2011, *date.Year)
	assert.Equal(t, 5, *date.Month)
	assert.Equal(t, 20, *date.Day)
}

func TestDeleteMarriedLifeEventClearsWeddingAnniversary(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/life-events/:id", DeleteLifeEvent)

	var user models.User
	db.First(&user)
	contact := createControllerContactWithWedding(t, db, user.ID)

	event := models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: models.LifeEventTypeMarried,
		Date: &contactmodel.PartialDate{Year: intPtr(2009), Month: intPtr(8), Day: intPtr(16)},
	}
	require.NoError(t, db.Create(&event).Error)

	req, _ := http.NewRequest("DELETE", "/life-events/"+event.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Contact
	require.NoError(t, db.First(&reloaded, contact.ID).Error)
	assert.Nil(t, services.WeddingDateFromCard(&reloaded.Card))
}
