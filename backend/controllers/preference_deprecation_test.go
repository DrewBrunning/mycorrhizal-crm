package controllers

// Issue #1738: the hobby Preference category is deprecated, not removed. The
// route-based deprecation registry cannot key on a body value, so the
// preference controller emits the same RFC 8594 headers on every response
// that carries a hobby preference. These tests pin the header contract on
// create/read/update/list and pair every "headers present" case with a
// non-hobby control, so an always-on header would fail.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func preferenceRequest(t *testing.T, router *gin.Engine, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertHobbyDeprecated(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "true", w.Header().Get("Deprecation"))
	assert.Equal(t, hobbyPreferenceDeprecationSunset, w.Header().Get("Sunset"))
	link := w.Header().Get("Link")
	assert.Contains(t, link, `rel="deprecation"`)
	assert.Contains(t, link, "deprecation-policy")
}

func assertNotDeprecated(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Empty(t, w.Header().Get("Deprecation"))
	assert.Empty(t, w.Header().Get("Sunset"))
	assert.Empty(t, w.Header().Get("Link"))
}

func createPreferenceForTest(t *testing.T, router *gin.Engine, contact models.Contact, category, key, value string) *httptest.ResponseRecorder {
	t.Helper()
	return preferenceRequest(t, router, http.MethodPost, "/preferences", models.PreferenceInput{
		EntityID: contact.VCardUID, Category: category, Key: key, Value: value,
	})
}

func TestPreferenceDeprecation_CreateStillWorksAndAnnouncesHobby(t *testing.T) {
	db, router := setupRouter(t)
	registerPreferenceRoutes(t, router)
	var user models.User
	db.First(&user)
	contact := seedPreferenceContact(t, db, user.ID)

	w := createPreferenceForTest(t, router, contact, models.PreferenceCategoryHobby, "favorite", "Piano")
	require.Equal(t, http.StatusCreated, w.Code, "a deprecated category must still create (no API narrowing)")
	assertHobbyDeprecated(t, w)

	var body struct {
		Preference models.Preference `json:"preference"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, models.PreferenceCategoryHobby, body.Preference.Category)
	assert.Equal(t, "Piano", body.Preference.Value)

	// Control: an ordinary category is unchanged and carries no signal.
	w = createPreferenceForTest(t, router, contact, models.PreferenceCategoryFood, "like", "Ramen")
	require.Equal(t, http.StatusCreated, w.Code)
	assertNotDeprecated(t, w)
}

func TestPreferenceDeprecation_ReadAndUpdate(t *testing.T) {
	db, router := setupRouter(t)
	registerPreferenceRoutes(t, router)
	var user models.User
	db.First(&user)
	contact := seedPreferenceContact(t, db, user.ID)

	created := createPreferenceForTest(t, router, contact, models.PreferenceCategoryHobby, "like", "Chess")
	require.Equal(t, http.StatusCreated, created.Code)
	var createdBody struct {
		Preference models.Preference `json:"preference"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdBody))

	w := preferenceRequest(t, router, http.MethodGet, "/preferences/"+createdBody.Preference.ID, nil)
	require.Equal(t, http.StatusOK, w.Code)
	assertHobbyDeprecated(t, w)

	w = preferenceRequest(t, router, http.MethodPut, "/preferences/"+createdBody.Preference.ID, models.PreferenceInput{
		EntityID: contact.VCardUID, Category: models.PreferenceCategoryHobby, Key: "favorite", Value: "Chess",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assertHobbyDeprecated(t, w)

	// Control: reading a food preference has no deprecation signal.
	food := createPreferenceForTest(t, router, contact, models.PreferenceCategoryFood, "like", "Ramen")
	var foodBody struct {
		Preference models.Preference `json:"preference"`
	}
	require.NoError(t, json.Unmarshal(food.Body.Bytes(), &foodBody))
	assertNotDeprecated(t, preferenceRequest(t, router, http.MethodGet, "/preferences/"+foodBody.Preference.ID, nil))
}

func TestPreferenceDeprecation_ListSignalsWhenAPageCarriesHobby(t *testing.T) {
	db, router := setupRouter(t)
	registerPreferenceRoutes(t, router)
	var user models.User
	db.First(&user)
	contact := seedPreferenceContact(t, db, user.ID)

	// Only a food preference: no signal.
	food := createPreferenceForTest(t, router, contact, models.PreferenceCategoryFood, "like", "Ramen")
	require.Equal(t, http.StatusCreated, food.Code)
	assertNotDeprecated(t, preferenceRequest(t, router, http.MethodGet, "/preferences", nil))

	hobby := createPreferenceForTest(t, router, contact, models.PreferenceCategoryHobby, "favorite", "Piano")
	require.Equal(t, http.StatusCreated, hobby.Code)
	w := preferenceRequest(t, router, http.MethodGet, "/preferences", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assertHobbyDeprecated(t, w)

	var body struct {
		Preferences []models.Preference `json:"preferences"`
		Total       int                 `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, 2, body.Total)
	assert.Len(t, body.Preferences, 2)
}

func TestPreferenceDeprecation_NewDislikeCategoryIsNotDeprecated(t *testing.T) {
	db, router := setupRouter(t)
	registerPreferenceRoutes(t, router)
	var user models.User
	db.First(&user)
	contact := seedPreferenceContact(t, db, user.ID)

	w := createPreferenceForTest(t, router, contact, models.PreferenceCategoryDislikeActivity, "", "Crowded parties")
	require.Equal(t, http.StatusCreated, w.Code, "dislike_activity is the current home for dislikes")
	assertNotDeprecated(t, w)

	// And it is not projected into a Card.
	var pref models.Preference
	require.NoError(t, db.First(&pref).Error)
	record := models.RecordForContact(&contact, "", db)
	assert.Empty(t, record.Card.PersonalInfo)
}
