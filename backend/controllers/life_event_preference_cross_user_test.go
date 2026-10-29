package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2203: cross-user list/get/update/delete for life events and
// preferences. Another user's row must be indistinguishable from a missing
// one (404, never 403) and must be left untouched.

func doScopeJSON(router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
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

func TestLifeEvents_CrossUserIsolation(t *testing.T) {
	db, router := setupRouter(t)
	router.GET("/life-events", ListLifeEvents)
	router.GET("/life-events/:id", GetLifeEvent)
	router.PUT("/life-events/:id", withValidated(func() any { return &models.LifeEventInput{} }), UpdateLifeEvent)
	router.DELETE("/life-events/:id", DeleteLifeEvent)

	f := newScopeFixture(t, db)
	mine := models.LifeEvent{UserID: f.me.ID, EntityID: f.alice.VCardUID, Type: models.LifeEventTypeMoved, Description: "mine"}
	theirs := models.LifeEvent{UserID: f.other.ID, EntityID: f.theirs.VCardUID, Type: models.LifeEventTypeMoved, Description: "theirs"}
	require.NoError(t, db.Create(&mine).Error)
	require.NoError(t, db.Create(&theirs).Error)

	t.Run("list excludes other user's events", func(t *testing.T) {
		assert.Equal(t, []string{mine.ID}, idsFromList(t, router, "/life-events", "life_events"))
	})
	t.Run("get is 404", func(t *testing.T) {
		w := doScopeJSON(router, "GET", "/life-events/"+theirs.ID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})
	t.Run("update is 404 and does not modify", func(t *testing.T) {
		w := doScopeJSON(router, "PUT", "/life-events/"+theirs.ID, models.LifeEventInput{
			EntityID: f.alice.VCardUID, Type: models.LifeEventTypeRetired, Description: "hijacked",
		})
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		var reloaded models.LifeEvent
		require.NoError(t, db.First(&reloaded, "id = ?", theirs.ID).Error)
		assert.Equal(t, "theirs", reloaded.Description)
		assert.Equal(t, f.theirs.VCardUID, reloaded.EntityID)
		assert.Equal(t, f.other.ID, reloaded.UserID)
	})
	t.Run("delete is 404 and does not delete", func(t *testing.T) {
		w := doScopeJSON(router, "DELETE", "/life-events/"+theirs.ID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		var count int64
		require.NoError(t, db.Model(&models.LifeEvent{}).Where("id = ?", theirs.ID).Count(&count).Error)
		assert.EqualValues(t, 1, count, "other user's life event must survive")
	})
}

func TestPreferences_CrossUserIsolation(t *testing.T) {
	db, router := setupRouter(t)
	registerPreferenceRoutes(t, router)

	f := newScopeFixture(t, db)
	mine := models.Preference{UserID: f.me.ID, EntityID: f.alice.VCardUID, Category: models.PreferenceCategoryHobby, Value: "mine", Sensitivity: models.RelationshipSensitivityNormal}
	theirs := models.Preference{UserID: f.other.ID, EntityID: f.theirs.VCardUID, Category: models.PreferenceCategoryHobby, Value: "theirs", Sensitivity: models.RelationshipSensitivityNormal}
	require.NoError(t, db.Create(&mine).Error)
	require.NoError(t, db.Create(&theirs).Error)

	t.Run("list excludes other user's preferences", func(t *testing.T) {
		assert.Equal(t, []string{mine.ID}, idsFromList(t, router, "/preferences", "preferences"))
	})
	t.Run("get is 404", func(t *testing.T) {
		w := doScopeJSON(router, "GET", "/preferences/"+theirs.ID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	})
	t.Run("update is 404 and does not modify", func(t *testing.T) {
		w := doScopeJSON(router, "PUT", "/preferences/"+theirs.ID, models.PreferenceInput{
			EntityID: f.alice.VCardUID, Category: models.PreferenceCategoryHobby, Value: "hijacked",
		})
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		var reloaded models.Preference
		require.NoError(t, db.First(&reloaded, "id = ?", theirs.ID).Error)
		assert.Equal(t, "theirs", reloaded.Value)
		assert.Equal(t, f.theirs.VCardUID, reloaded.EntityID)
		assert.Equal(t, f.other.ID, reloaded.UserID)
	})
	t.Run("delete is 404 and does not delete", func(t *testing.T) {
		w := doScopeJSON(router, "DELETE", "/preferences/"+theirs.ID, nil)
		assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
		var count int64
		require.NoError(t, db.Model(&models.Preference{}).Where("id = ?", theirs.ID).Count(&count).Error)
		assert.EqualValues(t, 1, count, "other user's preference must survive")
	})
}
