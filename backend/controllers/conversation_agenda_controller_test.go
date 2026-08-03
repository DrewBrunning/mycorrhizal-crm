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

func TestCreateConversationAgenda(t *testing.T) {
	db, router := setupRouter()
	router.POST("/conversation-agenda", withValidated(func() any { return &models.ConversationAgendaInput{} }), CreateConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	payload := models.ConversationAgendaInput{
		EntityID: contact.VCardUID, Content: "Ask about her mother's surgery",
	}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/conversation-agenda", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var count int64
	db.Model(&models.ConversationAgenda{}).Count(&count)
	assert.EqualValues(t, 1, count)
}

func TestCreateConversationAgendaRejectsContactFromAnotherUser(t *testing.T) {
	db, router := setupRouter()
	router.POST("/conversation-agenda", withValidated(func() any { return &models.ConversationAgendaInput{} }), CreateConversationAgenda)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)

	payload := models.ConversationAgendaInput{EntityID: othersContact.VCardUID, Content: "Ask about something"}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/conversation-agenda", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetConversationAgenda(t *testing.T) {
	db, router := setupRouter()
	router.GET("/conversation-agenda/:id", GetConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	item := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the trip"}
	db.Create(&item)

	req, _ := http.NewRequest("GET", "/conversation-agenda/"+item.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetConversationAgendaScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/conversation-agenda/:id", GetConversationAgenda)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)
	othersItem := models.ConversationAgenda{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Content: "Secret"}
	db.Create(&othersItem)

	req, _ := http.NewRequest("GET", "/conversation-agenda/"+othersItem.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "another user's agenda item must not be readable")
}

func TestListConversationAgendaFiltersByEntityID(t *testing.T) {
	db, router := setupRouter()
	router.GET("/conversation-agenda", ListConversationAgenda)

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)
	db.Create(&models.ConversationAgenda{UserID: user.ID, EntityID: alice.VCardUID, Content: "Ask Alice about her garden"})
	db.Create(&models.ConversationAgenda{UserID: user.ID, EntityID: bob.VCardUID, Content: "Ask Bob about his new job"})

	req, _ := http.NewRequest("GET", "/conversation-agenda?entity_id="+alice.VCardUID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Len(t, responseBody["conversation_agenda"], 1)
}

func TestListConversationAgendaScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/conversation-agenda", ListConversationAgenda)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)
	db.Create(&models.ConversationAgenda{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Content: "Secret"})

	req, _ := http.NewRequest("GET", "/conversation-agenda", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var responseBody map[string]any
	json.Unmarshal(w.Body.Bytes(), &responseBody)
	assert.Empty(t, responseBody["conversation_agenda"], "another user's agenda items must not leak into the list")
}

func TestUpdateConversationAgendaPreservesDiscussedState(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/conversation-agenda/:id", withValidated(func() any { return &models.ConversationAgendaInput{} }), UpdateConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	item := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the old topic"}
	db.Create(&item)

	now := time.Now()
	item.DiscussedAt = &now
	require.NoError(t, db.Save(&item).Error)

	payload := models.ConversationAgendaInput{EntityID: contact.VCardUID, Content: "Ask about the new topic"}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PUT", "/conversation-agenda/"+item.ID, bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.ConversationAgenda
	db.First(&reloaded, "id = ?", item.ID)
	assert.Equal(t, "Ask about the new topic", reloaded.Content)
	require.NotNil(t, reloaded.DiscussedAt, "editing content must not silently re-open a discussed item")
	assert.Equal(t, now.Unix(), reloaded.DiscussedAt.Unix())
}

func TestDeleteConversationAgendaSoftDeletes(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/conversation-agenda/:id", DeleteConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	item := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the trip"}
	db.Create(&item)

	req, _ := http.NewRequest("DELETE", "/conversation-agenda/"+item.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var count int64
	db.Model(&models.ConversationAgenda{}).Count(&count)
	assert.Zero(t, count)

	var unscopedCount int64
	db.Unscoped().Model(&models.ConversationAgenda{}).Where("id = ?", item.ID).Count(&unscopedCount)
	assert.EqualValues(t, 1, unscopedCount, "must be a soft delete, not a hard delete")
}

func TestDiscussConversationAgendaWithoutActivity(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/conversation-agenda/:id/discuss", withValidated(func() any { return &models.ConversationAgendaDiscussInput{} }), DiscussConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	item := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the garden"}
	db.Create(&item)

	payload := models.ConversationAgendaDiscussInput{}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/conversation-agenda/"+item.ID+"/discuss", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.ConversationAgenda
	db.First(&reloaded, "id = ?", item.ID)
	require.NotNil(t, reloaded.DiscussedAt, "marking discussed must set discussed_at")
	assert.Nil(t, reloaded.ActivityID, "no activity supplied means no activity linked")
}

func TestDiscussConversationAgendaWithActivity(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/conversation-agenda/:id/discuss", withValidated(func() any { return &models.ConversationAgendaDiscussInput{} }), DiscussConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	item := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the house"}
	db.Create(&item)
	activity := models.Activity{UserID: user.ID, Title: "Coffee catch-up", Contacts: []models.Contact{contact}}
	db.Create(&activity)

	payload := models.ConversationAgendaDiscussInput{ActivityID: &activity.ID}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/conversation-agenda/"+item.ID+"/discuss", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var reloaded models.ConversationAgenda
	db.First(&reloaded, "id = ?", item.ID)
	require.NotNil(t, reloaded.DiscussedAt)
	require.NotNil(t, reloaded.ActivityID)
	assert.Equal(t, activity.ID, *reloaded.ActivityID)
}

func TestDiscussConversationAgendaRejectsForeignActivity(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/conversation-agenda/:id/discuss", withValidated(func() any { return &models.ConversationAgendaDiscussInput{} }), DiscussConversationAgenda)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	item := models.ConversationAgenda{UserID: user.ID, EntityID: contact.VCardUID, Content: "Ask about the house"}
	db.Create(&item)

	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	otherActivity := models.Activity{UserID: otherUser.ID, Title: "Someone else's activity"}
	db.Create(&otherActivity)

	payload := models.ConversationAgendaDiscussInput{ActivityID: &otherActivity.ID}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/conversation-agenda/"+item.ID+"/discuss", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "linking another user's activity must be rejected")

	var reloaded models.ConversationAgenda
	db.First(&reloaded, "id = ?", item.ID)
	assert.Nil(t, reloaded.DiscussedAt, "a rejected discuss must not mark the item discussed")
}

func TestDiscussConversationAgendaScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.PATCH("/conversation-agenda/:id/discuss", withValidated(func() any { return &models.ConversationAgendaDiscussInput{} }), DiscussConversationAgenda)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)
	othersItem := models.ConversationAgenda{UserID: otherUser.ID, EntityID: othersContact.VCardUID, Content: "Secret"}
	db.Create(&othersItem)

	payload := models.ConversationAgendaDiscussInput{}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/conversation-agenda/"+othersItem.ID+"/discuss", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code, "another user's agenda item must not be discussable")
}
