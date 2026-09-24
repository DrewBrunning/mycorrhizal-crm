package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/contactmodel"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func suggestionsGet(t *testing.T, router *gin.Engine, path string) []models.LifeEventSuggestion {
	t.Helper()
	req, _ := http.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body struct {
		Suggestions []models.LifeEventSuggestion `json:"suggestions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Suggestions
}

func TestLifeEventSuggestions_OfferResolveAndSuppress(t *testing.T) {
	db, router := setupRouter()
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.GET("/contacts/:id/life-event-suggestions", GetLifeEventSuggestions)
	router.POST("/life-event-suggestions/resolve",
		middleware.ValidateJSONMiddleware(&models.LifeEventSuggestionResolutionInput{}), ResolveLifeEventSuggestion)

	payload := contactPeriodInput("addr-1", contactmodel.TemporalRange{
		Start: &contactmodel.PartialDate{Year: intPtr(2019)},
		End:   &contactmodel.PartialDate{Year: intPtr(2024)},
	})
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	// A dated address offers a moved candidate.
	suggestions := suggestionsGet(t, router, "/contacts/1/life-event-suggestions")
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.LifeEventTypeMoved, suggestions[0].Type)
	assert.Equal(t, "addr-1", suggestions[0].SourceEntryID)
	require.NotNil(t, suggestions[0].Date)
	assert.Equal(t, 2019, *suggestions[0].Date.Year)

	// Dismiss it, then confirm it is not offered again.
	resolve := models.LifeEventSuggestionResolutionInput{
		EntityID: contact.VCardUID, SourceKind: contactmodel.PeriodKindAddress,
		SourceEntryID: "addr-1", EventType: models.LifeEventTypeMoved,
		Resolution: models.LifeEventSuggestionDismissed,
	}
	body, _ := json.Marshal(resolve)
	req, _ = http.NewRequest("POST", "/life-event-suggestions/resolve", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Empty(t, suggestionsGet(t, router, "/contacts/1/life-event-suggestions"),
		"a resolved candidate must not be offered again")
}

func TestLifeEventSuggestions_UnknownContactIs404(t *testing.T) {
	_, router := setupRouter()
	router.GET("/contacts/:id/life-event-suggestions", GetLifeEventSuggestions)

	req, _ := http.NewRequest("GET", "/contacts/9999/life-event-suggestions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResolveLifeEventSuggestionRoutes(t *testing.T) {
	db, router := setupRouter()
	router.POST("/life-event-suggestions/resolve",
		middleware.ValidateJSONMiddleware(&models.LifeEventSuggestionResolutionInput{}), ResolveLifeEventSuggestion)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada"}
	require.NoError(t, db.Create(&contact).Error)

	t.Run("invalid resolution value is rejected", func(t *testing.T) {
		body, _ := json.Marshal(models.LifeEventSuggestionResolutionInput{
			EntityID: contact.VCardUID, SourceKind: "address", SourceEntryID: "a1",
			EventType: "moved", Resolution: "maybe",
		})
		req, _ := http.NewRequest("POST", "/life-event-suggestions/resolve", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("unknown contact is 404", func(t *testing.T) {
		body, _ := json.Marshal(models.LifeEventSuggestionResolutionInput{
			EntityID: "00000000-0000-4000-8000-000000000000", SourceKind: "address",
			SourceEntryID: "a1", EventType: "moved", Resolution: "dismissed",
		})
		req, _ := http.NewRequest("POST", "/life-event-suggestions/resolve", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
