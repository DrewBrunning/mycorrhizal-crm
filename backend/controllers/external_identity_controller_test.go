package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2203: ExternalIdentity list filters and cross-user isolation with
// another user's data present, including the same system name.

func registerExternalIdentityRoutes(router *gin.Engine) {
	router.GET("/external-identities", ListExternalIdentities)
	router.GET("/external-identities/:id", GetExternalIdentity)
	router.PUT("/external-identities/:id", withValidated(func() any { return &models.ExternalIdentityInput{} }), UpdateExternalIdentity)
	router.DELETE("/external-identities/:id", DeleteExternalIdentity)
}

func TestListExternalIdentities_ContactAndSystemFiltersAreOwnerScoped(t *testing.T) {
	db, router := setupRouter(t)
	registerExternalIdentityRoutes(router)
	f := newScopeFixture(t, db)

	mk := func(userID uint, entityID, system, extID string) string {
		m := models.ExternalIdentity{
			UserID: userID, EntityID: entityID, System: system, ExternalID: extID,
			SyncStatus: models.ExternalIdentitySyncStatusIdle,
		}
		require.NoError(t, db.Create(&m).Error)
		return m.ID
	}
	aliceGH := mk(f.me.ID, f.alice.VCardUID, "github", "a-gh")
	aliceLI := mk(f.me.ID, f.alice.VCardUID, "linkedin", "a-li")
	bobGH := mk(f.me.ID, f.bob.VCardUID, "github", "b-gh")
	// Other user's rows share the system name.
	theirGH := mk(f.other.ID, f.theirs.VCardUID, "github", "t-gh")

	const key = "external_identities"
	assert.ElementsMatch(t, []string{aliceGH, aliceLI},
		idsFromList(t, router, "/external-identities?contact_id="+f.alice.VCardUID, key))
	assert.ElementsMatch(t, []string{aliceGH, bobGH},
		idsFromList(t, router, "/external-identities?system=github", key),
		"system filter must not leak the other user's github identity")
	assert.Equal(t, []string{aliceGH},
		idsFromList(t, router, "/external-identities?contact_id="+f.alice.VCardUID+"&system=github", key))
	assert.Empty(t, idsFromList(t, router, "/external-identities?contact_id="+f.theirs.VCardUID, key),
		"another user's contact uid must yield an empty list")
	assert.Empty(t, idsFromList(t, router, "/external-identities?contact_id="+f.theirs.VCardUID+"&system=github", key))
	all := idsFromList(t, router, "/external-identities", key)
	assert.ElementsMatch(t, []string{aliceGH, aliceLI, bobGH}, all)
	assert.NotContains(t, all, theirGH)
}

func TestExternalIdentity_CrossUserGetUpdateDeleteAre404(t *testing.T) {
	db, router := setupRouter(t)
	registerExternalIdentityRoutes(router)
	f := newScopeFixture(t, db)

	theirs := models.ExternalIdentity{
		UserID: f.other.ID, EntityID: f.theirs.VCardUID, System: "github", ExternalID: "t-gh",
		SyncStatus: models.ExternalIdentitySyncStatusIdle,
	}
	require.NoError(t, db.Create(&theirs).Error)

	w := doScopeJSON(router, "GET", "/external-identities/"+theirs.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	w = doScopeJSON(router, "PUT", "/external-identities/"+theirs.ID, models.ExternalIdentityInput{
		EntityID: f.alice.VCardUID, System: "github", ExternalID: "hijacked",
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	w = doScopeJSON(router, "DELETE", "/external-identities/"+theirs.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	var reloaded models.ExternalIdentity
	require.NoError(t, db.First(&reloaded, "id = ?", theirs.ID).Error, "other user's identity must survive")
	assert.Equal(t, "t-gh", reloaded.ExternalID)
	assert.Equal(t, f.theirs.VCardUID, reloaded.EntityID)
	assert.Equal(t, f.other.ID, reloaded.UserID)
}
