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
	db, router := setupRouter(t)
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

	// A dated address offers both an arrival (moved, at the start) and, with no
	// successor, a departure (moved_out, at the end).
	suggestions := suggestionsGet(t, router, "/contacts/1/life-event-suggestions")
	require.Len(t, suggestions, 2)
	byType := map[string]models.LifeEventSuggestion{}
	for _, suggestion := range suggestions {
		byType[suggestion.Type] = suggestion
	}
	moved, ok := byType[models.LifeEventTypeMoved]
	require.True(t, ok, "a dated address start must offer a moved candidate")
	assert.Equal(t, "addr-1", moved.SourceEntryID)
	require.NotNil(t, moved.Date)
	assert.Equal(t, 2019, *moved.Date.Year)
	departure, ok := byType[models.LifeEventTypeMovedOut]
	require.True(t, ok, "an address end with no successor must offer a departure")
	require.NotNil(t, departure.Date)
	assert.Equal(t, 2024, *departure.Date.Year)

	// Dismiss both, then confirm neither is offered again.
	for _, eventType := range []string{models.LifeEventTypeMoved, models.LifeEventTypeMovedOut} {
		resolve := models.LifeEventSuggestionResolutionInput{
			EntityID: contact.VCardUID, SourceKind: contactmodel.PeriodKindAddress,
			SourceEntryID: "addr-1", EventType: eventType,
			Resolution: models.LifeEventSuggestionDismissed,
		}
		body, _ := json.Marshal(resolve)
		req, _ = http.NewRequest("POST", "/life-event-suggestions/resolve", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}

	assert.Empty(t, suggestionsGet(t, router, "/contacts/1/life-event-suggestions"),
		"a resolved candidate must not be offered again")
}

// Issue #1233: a title period is a valid inference source, and resolving it
// must pass the request-body oneof (address|organization|title).
func TestLifeEventSuggestions_TitleSourceKindIsAccepted(t *testing.T) {
	db, router := setupRouter(t)
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.GET("/contacts/:id/life-event-suggestions", GetLifeEventSuggestions)
	router.POST("/life-event-suggestions/resolve",
		middleware.ValidateJSONMiddleware(&models.LifeEventSuggestionResolutionInput{}), ResolveLifeEventSuggestion)

	payload := models.ContactRecordInput{
		Card: contactmodel.Card{
			Name:   &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Titles: []contactmodel.Title{{ID: "title-1", Name: "Engineer", Kind: "title"}},
		},
		CRM: contactmodel.CRMEnvelope{Periods: []contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindTitle, EntryID: "title-1", Range: contactmodel.TemporalRange{
				Start: &contactmodel.PartialDate{Year: intPtr(2020)},
			}},
		}},
	}
	jsonValue, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/contacts", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var contact models.Contact
	require.NoError(t, db.First(&contact).Error)

	suggestions := suggestionsGet(t, router, "/contacts/1/life-event-suggestions")
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.LifeEventTypeJobChange, suggestions[0].Type)
	assert.Equal(t, contactmodel.PeriodKindTitle, suggestions[0].SourceKind)

	resolve := models.LifeEventSuggestionResolutionInput{
		EntityID: contact.VCardUID, SourceKind: contactmodel.PeriodKindTitle,
		SourceEntryID: "title-1", EventType: models.LifeEventTypeJobChange,
		Resolution: models.LifeEventSuggestionDismissed,
	}
	body, _ := json.Marshal(resolve)
	req, _ = http.NewRequest("POST", "/life-event-suggestions/resolve", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, suggestionsGet(t, router, "/contacts/1/life-event-suggestions"))
}

func TestLifeEventSuggestions_UnknownContactIs404(t *testing.T) {
	_, router := setupRouter(t)
	router.GET("/contacts/:id/life-event-suggestions", GetLifeEventSuggestions)

	req, _ := http.NewRequest("GET", "/contacts/9999/life-event-suggestions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestResolveLifeEventSuggestionRoutes(t *testing.T) {
	db, router := setupRouter(t)
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
