package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGiftDefaultsToIdea(t *testing.T) {
	db, router := setupRouter()
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	payload := models.GiftInput{EntityID: contact.VCardUID, Description: "She mentioned she liked X"}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var responseBody struct {
		Gift models.Gift `json:"gift"`
	}
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Equal(t, models.GiftStatusIdea, responseBody.Gift.Status, "a new gift must default to idea")

	var count int64
	db.Model(&models.Gift{}).Count(&count)
	assert.EqualValues(t, 1, count)
}

func TestCreateGiftWithExplicitStatusAndValue(t *testing.T) {
	db, router := setupRouter()
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	now := time.Now()
	payload := models.GiftInput{
		EntityID: contact.VCardUID, Status: models.GiftStatusGiven,
		Occasion: "birthday", Description: "The espresso machine",
		Date: &now, ValueCents: 12000, Currency: "EUR",
	}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var gift models.Gift
	db.First(&gift, "entity_id = ?", contact.VCardUID)
	assert.Equal(t, models.GiftStatusGiven, gift.Status)
	assert.Equal(t, "birthday", gift.Occasion)
	assert.EqualValues(t, 12000, gift.ValueCents)
	assert.Equal(t, "EUR", gift.Currency)
	require.NotNil(t, gift.Date)
}

func TestCreateGiftRejectsContactFromAnotherUser(t *testing.T) {
	db, router := setupRouter()
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)

	payload := models.GiftInput{EntityID: othersContact.VCardUID, Description: "Idea"}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var count int64
	db.Model(&models.Gift{}).Count(&count)
	assert.Zero(t, count, "a rejected create must not leave a row")
}

func TestCreateGiftRequiresCurrencyWithValue(t *testing.T) {
	db, router := setupRouter()
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	// Value without currency.
	payload := models.GiftInput{EntityID: contact.VCardUID, Description: "Watch", ValueCents: 5000}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "a value without an explicit currency must be rejected")

	// Currency without a value is meaningless too.
	payload2 := models.GiftInput{EntityID: contact.VCardUID, Description: "Watch", Currency: "USD"}
	jsonValue2, _ := json.Marshal(payload2)
	req2, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusBadRequest, w2.Code, "a currency without a value must be rejected")

	var count int64
	db.Model(&models.Gift{}).Count(&count)
	assert.Zero(t, count)
}

func TestCreateGiftRejectsForeignLifeEvent(t *testing.T) {
	db, router := setupRouter()
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersEvent := models.LifeEvent{UserID: otherUser.ID, EntityID: contact.VCardUID, Type: "married"}
	db.Create(&othersEvent)

	payload := models.GiftInput{EntityID: contact.VCardUID, Description: "Anniversary gift", LifeEventID: othersEvent.ID}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "linking another user's life event must be rejected")
}

func TestCreateGiftRejectsForeignActivity(t *testing.T) {
	db, router := setupRouter()
	router.POST("/gifts", withValidated(func() any { return &models.GiftInput{} }), CreateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	otherActivity := models.Activity{UserID: otherUser.ID, Title: "Someone else's activity", Date: time.Now()}
	db.Create(&otherActivity)

	payload := models.GiftInput{EntityID: contact.VCardUID, Description: "Gift from the meetup", ActivityID: &otherActivity.ID}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/gifts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "linking another user's activity must be rejected")
}

func TestGetGift(t *testing.T) {
	db, router := setupRouter()
	router.GET("/gifts/:id", GetGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "A candle"}
	db.Create(&gift)

	req, _ := http.NewRequest("GET", "/gifts/"+gift.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetGiftScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/gifts/:id", GetGift)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)
	othersGift := models.Gift{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Description: "Secret"}
	db.Create(&othersGift)

	req, _ := http.NewRequest("GET", "/gifts/"+othersGift.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "another user's gift must not be readable")
}

func TestListGiftsFiltersByEntityID(t *testing.T) {
	db, router := setupRouter()
	router.GET("/gifts", ListGifts)

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)
	db.Create(&models.Gift{UserID: user.ID, EntityID: alice.VCardUID, Description: "Idea for Alice"})
	db.Create(&models.Gift{UserID: user.ID, EntityID: bob.VCardUID, Description: "Idea for Bob"})

	req, _ := http.NewRequest("GET", "/gifts?entity_id="+alice.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Len(t, responseBody["gifts"], 1)
}

func TestListGiftsScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/gifts", ListGifts)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)
	db.Create(&models.Gift{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Description: "Secret"})

	req, _ := http.NewRequest("GET", "/gifts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Empty(t, responseBody["gifts"], "another user's gifts must not leak into the list")
}

func TestUpdateGift(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/gifts/:id", withValidated(func() any { return &models.GiftInput{} }), UpdateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "Old idea"}
	db.Create(&gift)

	payload := models.GiftInput{EntityID: contact.VCardUID, Status: models.GiftStatusPurchased, Description: "Now bought"}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/gifts/"+gift.ID, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.Gift
	db.First(&reloaded, "id = ?", gift.ID)
	assert.Equal(t, "Now bought", reloaded.Description)
	assert.Equal(t, models.GiftStatusPurchased, reloaded.Status)
}

func TestUpdateGiftScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/gifts/:id", withValidated(func() any { return &models.GiftInput{} }), UpdateGift)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)
	othersGift := models.Gift{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Description: "Secret"}
	db.Create(&othersGift)

	payload := models.GiftInput{EntityID: othersContact.VCardUID, Description: "Updated secret"}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/gifts/"+othersGift.ID, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "another user's gift must not be updatable")
}

func TestUpdateGiftRejectsForeignLifeEvent(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/gifts/:id", withValidated(func() any { return &models.GiftInput{} }), UpdateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "Old idea"}
	db.Create(&gift)

	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersEvent := models.LifeEvent{UserID: otherUser.ID, EntityID: contact.VCardUID, Type: "married"}
	db.Create(&othersEvent)

	payload := models.GiftInput{EntityID: contact.VCardUID, Description: "Now linked", LifeEventID: othersEvent.ID}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/gifts/"+gift.ID, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "linking another user's life event on update must be rejected")
}

func TestUpdateGiftRejectsForeignActivity(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/gifts/:id", withValidated(func() any { return &models.GiftInput{} }), UpdateGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "Old idea"}
	db.Create(&gift)

	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	otherActivity := models.Activity{UserID: otherUser.ID, Title: "Someone else's activity", Date: time.Now()}
	db.Create(&otherActivity)

	payload := models.GiftInput{EntityID: contact.VCardUID, Description: "Now linked", ActivityID: &otherActivity.ID}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/gifts/"+gift.ID, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "linking another user's activity on update must be rejected")
}

func TestDeleteGiftSoftDeletes(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/gifts/:id", DeleteGift)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Description: "Old idea"}
	db.Create(&gift)

	req, _ := http.NewRequest("DELETE", "/gifts/"+gift.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var count int64
	db.Model(&models.Gift{}).Count(&count)
	assert.Zero(t, count)

	var unscopedCount int64
	db.Unscoped().Model(&models.Gift{}).Where("id = ?", gift.ID).Count(&unscopedCount)
	assert.EqualValues(t, 1, unscopedCount, "must be a soft delete, not a hard delete")
}
