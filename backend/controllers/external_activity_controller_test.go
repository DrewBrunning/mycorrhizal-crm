package controllers

import (
	"net/http"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2203: ExternalActivity list filters and cross-user isolation with
// another user's data present, including the same source system.

func registerExternalActivityRoutes(router *gin.Engine) {
	router.GET("/external-activities", ListExternalActivities)
	router.GET("/external-activities/:id", GetExternalActivity)
	router.PUT("/external-activities/:id", withValidated(func() any { return &models.ExternalActivityInput{} }), UpdateExternalActivity)
	router.DELETE("/external-activities/:id", DeleteExternalActivity)
}

func TestListExternalActivities_ContactAndSystemFiltersAreOwnerScoped(t *testing.T) {
	db, router := setupRouter(t)
	registerExternalActivityRoutes(router)
	f := newScopeFixture(t, db)

	when := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	mk := func(userID uint, entityID, system, extID string) string {
		m := models.ExternalActivity{
			UserID: userID, EntityID: entityID, SourceSystem: system, ExternalID: extID,
			Type: "photo-appearance", OccurredAt: when,
			Provenance: models.ExternalActivityProvenanceExternal, SyncState: models.ExternalActivitySyncStateSynced,
		}
		require.NoError(t, db.Create(&m).Error)
		return m.ID
	}
	aliceImmich := mk(f.me.ID, f.alice.VCardUID, "immich", "a-1")
	aliceStrava := mk(f.me.ID, f.alice.VCardUID, "strava", "a-2")
	bobImmich := mk(f.me.ID, f.bob.VCardUID, "immich", "b-1")
	// Other user's rows share the source system.
	theirImmich := mk(f.other.ID, f.theirs.VCardUID, "immich", "t-1")

	const key = "external_activities"
	assert.ElementsMatch(t, []string{aliceImmich, aliceStrava},
		idsFromList(t, router, "/external-activities?contact_id="+f.alice.VCardUID, key))
	assert.ElementsMatch(t, []string{aliceImmich, bobImmich},
		idsFromList(t, router, "/external-activities?system=immich", key),
		"system filter must not leak the other user's immich activity")
	assert.Equal(t, []string{aliceImmich},
		idsFromList(t, router, "/external-activities?contact_id="+f.alice.VCardUID+"&system=immich", key))
	assert.Empty(t, idsFromList(t, router, "/external-activities?contact_id="+f.theirs.VCardUID, key),
		"another user's contact uid must yield an empty list")
	assert.Empty(t, idsFromList(t, router, "/external-activities?contact_id="+f.theirs.VCardUID+"&system=immich", key))
	all := idsFromList(t, router, "/external-activities", key)
	assert.ElementsMatch(t, []string{aliceImmich, aliceStrava, bobImmich}, all)
	assert.NotContains(t, all, theirImmich)
}

func TestExternalActivity_CrossUserGetUpdateDeleteAre404(t *testing.T) {
	db, router := setupRouter(t)
	registerExternalActivityRoutes(router)
	f := newScopeFixture(t, db)

	when := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	theirs := models.ExternalActivity{
		UserID: f.other.ID, EntityID: f.theirs.VCardUID, SourceSystem: "immich", ExternalID: "t-1",
		Type: "photo-appearance", OccurredAt: when,
		Provenance: models.ExternalActivityProvenanceExternal, SyncState: models.ExternalActivitySyncStateSynced,
	}
	require.NoError(t, db.Create(&theirs).Error)

	w := doScopeJSON(router, "GET", "/external-activities/"+theirs.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	w = doScopeJSON(router, "PUT", "/external-activities/"+theirs.ID, models.ExternalActivityInput{
		EntityID: f.alice.VCardUID, SourceSystem: "immich", ExternalID: "hijacked",
		Type: "photo-appearance", OccurredAt: when,
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	w = doScopeJSON(router, "DELETE", "/external-activities/"+theirs.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var reloaded models.ExternalActivity
	require.NoError(t, db.First(&reloaded, "id = ?", theirs.ID).Error, "other user's activity must survive")
	assert.Equal(t, "t-1", reloaded.ExternalID)
	assert.Equal(t, f.theirs.VCardUID, reloaded.EntityID)
	assert.Equal(t, f.other.ID, reloaded.UserID)
}
