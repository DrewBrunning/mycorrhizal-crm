package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func doCadenceJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req, _ := http.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCreateCadencePolicy(t *testing.T) {
	db, router := setupRouter()
	router.POST("/cadence-policies", withValidated(func() any { return &models.CadencePolicyInput{} }), CreateCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	w := doCadenceJSON(t, router, "POST", "/cadence-policies", models.CadencePolicyInput{
		EntityID:           contact.VCardUID,
		TargetIntervalDays: 30,
		QualifyingTypes:    []string{models.InteractionTypeCall, models.InteractionTypeVisit},
	})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		CadencePolicy cadencePolicyWithHealth `json:"cadence_policy"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.CadencePolicy.ID)
	assert.Equal(t, 30, resp.CadencePolicy.TargetIntervalDays)
	// A contact with no qualifying interaction ever has undefined health.
	assert.False(t, resp.CadencePolicy.Health.HasQualifyingInteraction)
}

func TestCreateCadencePolicyRejectsDuplicate(t *testing.T) {
	db, router := setupRouter()
	router.POST("/cadence-policies", withValidated(func() any { return &models.CadencePolicyInput{} }), CreateCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30})

	w := doCadenceJSON(t, router, "POST", "/cadence-policies", models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 60,
	})
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestCreateCadencePolicyRejectsForeignContact(t *testing.T) {
	db, router := setupRouter()
	router.POST("/cadence-policies", withValidated(func() any { return &models.CadencePolicyInput{} }), CreateCadencePolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)

	w := doCadenceJSON(t, router, "POST", "/cadence-policies", models.CadencePolicyInput{
		EntityID: othersContact.VCardUID, TargetIntervalDays: 30,
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetCadencePolicyNotFound(t *testing.T) {
	_, router := setupRouter()
	router.GET("/cadence-policies/:id", GetCadencePolicy)

	w := doCadenceJSON(t, router, "GET", "/cadence-policies/does-not-exist", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetCadencePolicyHappyPath(t *testing.T) {
	db, router := setupRouter()
	router.GET("/cadence-policies/:id", GetCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.CadencePolicy{
		UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30,
		QualifyingTypes: []string{models.InteractionTypeCall},
	}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "GET", "/cadence-policies/"+policy.ID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp cadencePolicyWithHealth
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, policy.ID, resp.ID)
	assert.Equal(t, 30, resp.TargetIntervalDays)
	assert.False(t, resp.Health.HasQualifyingInteraction)
}

func TestGetCadencePolicyScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/cadence-policies/:id", GetCadencePolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	contact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&contact)
	policy := models.CadencePolicy{UserID: otherUser.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "GET", "/cadence-policies/"+policy.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "a policy owned by another user must not be readable")
}

func TestListCadencePoliciesByEntity(t *testing.T) {
	db, router := setupRouter()
	router.GET("/cadence-policies", ListCadencePolicies)

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: alice.VCardUID, TargetIntervalDays: 30})
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: bob.VCardUID, TargetIntervalDays: 60})

	w := doCadenceJSON(t, router, "GET", "/cadence-policies?entity_id="+alice.VCardUID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		CadencePolicies []cadencePolicyWithHealth `json:"cadence_policies"`
		Total           int                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.CadencePolicies, 1)
	assert.Equal(t, alice.VCardUID, resp.CadencePolicies[0].EntityID)
	assert.Equal(t, 1, resp.Total)
}

func TestListCadencePoliciesUnfiltered(t *testing.T) {
	db, router := setupRouter()
	router.GET("/cadence-policies", ListCadencePolicies)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30})
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID + "-extra", TargetIntervalDays: 60})

	// Without entity_id, all policies should be returned.
	w := doCadenceJSON(t, router, "GET", "/cadence-policies", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		CadencePolicies []cadencePolicyWithHealth `json:"cadence_policies"`
		Total           int                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.CadencePolicies, 2, "unfiltered list must include all policies")
	assert.EqualValues(t, 2, resp.Total)
	// Both must carry embedded health, even when undefined.
	for _, p := range resp.CadencePolicies {
		assert.False(t, p.Health.HasQualifyingInteraction)
	}
}

func TestListCadencePoliciesScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/cadence-policies", ListCadencePolicies)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	contact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&contact)
	db.Create(&models.CadencePolicy{UserID: otherUser.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30})

	w := doCadenceJSON(t, router, "GET", "/cadence-policies", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		CadencePolicies []cadencePolicyWithHealth `json:"cadence_policies"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.CadencePolicies, "another user's policies must not leak into the list")
}

func TestUpdateCadencePolicy(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/cadence-policies/:id", withValidated(func() any { return &models.CadencePolicyInput{} }), UpdateCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30, QualifyingTypes: []string{models.InteractionTypeCall}}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "PUT", "/cadence-policies/"+policy.ID, models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 90,
	})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.CadencePolicy
	db.First(&reloaded, "id = ?", policy.ID)
	assert.Equal(t, 90, reloaded.TargetIntervalDays)
	assert.Empty(t, reloaded.QualifyingTypes, "full-replace semantics must clear the previous types")
}

func TestUpdateCadencePolicyScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/cadence-policies/:id", withValidated(func() any { return &models.CadencePolicyInput{} }), UpdateCadencePolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	contact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&contact)
	policy := models.CadencePolicy{UserID: otherUser.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "PUT", "/cadence-policies/"+policy.ID, models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 90,
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateCadencePolicyRejectsEntityChangeToExisting(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/cadence-policies/:id", withValidated(func() any { return &models.CadencePolicyInput{} }), UpdateCadencePolicy)

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)

	// Two contacts, each with a policy. Changing one policy's entity to
	// the other contact must 409.
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: alice.VCardUID, TargetIntervalDays: 30})
	policyB := models.CadencePolicy{UserID: user.ID, EntityID: bob.VCardUID, TargetIntervalDays: 60}
	db.Create(&policyB)

	w := doCadenceJSON(t, router, "PUT", "/cadence-policies/"+policyB.ID, models.CadencePolicyInput{
		EntityID: alice.VCardUID, TargetIntervalDays: 90,
	})
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestDeleteCadencePolicySoftDeletesAndAllowsRecreate(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/cadence-policies/:id", DeleteCadencePolicy)
	router.POST("/cadence-policies", withValidated(func() any { return &models.CadencePolicyInput{} }), CreateCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "DELETE", "/cadence-policies/"+policy.ID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	// Soft-deleted: gone from normal queries...
	var count int64
	db.Model(&models.CadencePolicy{}).Where("id = ?", policy.ID).Count(&count)
	assert.Zero(t, count)
	// ...but physically present for the tombstone/trace.
	var unscoped int64
	db.Unscoped().Model(&models.CadencePolicy{}).Where("id = ?", policy.ID).Count(&unscoped)
	assert.EqualValues(t, 1, unscoped, "CadencePolicy must soft-delete (user-authored content, T26)")

	// And the partial unique index must allow re-creating for the same contact.
	recreate := doCadenceJSON(t, router, "POST", "/cadence-policies", models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 60,
	})
	assert.Equal(t, http.StatusCreated, recreate.Code, recreate.Body.String())
}

func TestGetOverdueCadences(t *testing.T) {
	db, router := setupRouter()
	router.GET("/cadence-policies/overdue", GetOverdueCadences)

	var user models.User
	db.First(&user)
	// Overdue: last qualifying interaction ~40 days ago, 30-day interval.
	overdueContact := models.Contact{UserID: user.ID, Firstname: "Neglected"}
	db.Create(&overdueContact)
	oldActivity := models.Activity{
		UserID: user.ID, Title: "Last call", Date: time.Now().AddDate(0, 0, -40),
		Type: models.InteractionTypeCall, Contacts: []models.Contact{overdueContact},
	}
	db.Create(&oldActivity)
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: overdueContact.VCardUID, TargetIntervalDays: 30})

	// Not overdue: last interaction 5 days ago, 30-day interval.
	recentContact := models.Contact{UserID: user.ID, Firstname: "Fresh"}
	db.Create(&recentContact)
	recentActivity := models.Activity{
		UserID: user.ID, Title: "Recent chat", Date: time.Now().AddDate(0, 0, -5),
		Type: models.InteractionTypeVisit, Contacts: []models.Contact{recentContact},
	}
	db.Create(&recentActivity)
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: recentContact.VCardUID, TargetIntervalDays: 30})

	// Never interacted: cannot be overdue.
	neverContact := models.Contact{UserID: user.ID, Firstname: "Brand New"}
	db.Create(&neverContact)
	db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: neverContact.VCardUID, TargetIntervalDays: 30})

	w := doCadenceJSON(t, router, "GET", "/cadence-policies/overdue", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Overdue []services.OverdueCadence `json:"overdue"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Overdue, 1, "only the truly overdue policy appears")
	assert.Equal(t, overdueContact.VCardUID, resp.Overdue[0].Policy.EntityID)
	assert.Equal(t, overdueContact.ID, resp.Overdue[0].ContactID)
	assert.Equal(t, "Neglected", resp.Overdue[0].ContactName)
	assert.True(t, resp.Overdue[0].Health.OverdueBy > 0)
}

func TestGetOverdueCadencesEmptyIsNullSafe(t *testing.T) {
	_, router := setupRouter()
	router.GET("/cadence-policies/overdue", GetOverdueCadences)

	w := doCadenceJSON(t, router, "GET", "/cadence-policies/overdue", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"overdue":[]`, "an empty overdue list must serialize as [] not null (frontend crash guard)")
}

// TestListCadencePoliciesSinceFeedTombstoneAtOriginalPosition proves the
// AfterDelete hook advances updated_at correctly: a cursor at the policy's
// ORIGINAL (pre-delete) position must still surface the tombstone, because
// the AfterDelete hook has bumped it forward. Without the hook the tombstone
// sits at the old position and any cursor >= that position misses it forever.
func TestListCadencePoliciesSinceFeedTombstoneAtOriginalPosition(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	router.GET("/cadence-policies", ListCadencePolicies)
	router.DELETE("/cadence-policies/:id", DeleteCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	db.Create(&policy)

	// Cursor at the policy's exact position — not before it. Without the
	// AfterDelete hook this cursor would miss the tombstone.
	cursor := EncodeCursor(policy.UpdatedAt, policy.ID)

	w := doCadenceJSON(t, router, "DELETE", "/cadence-policies/"+policy.ID, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w2 := doCadenceJSON(t, router, "GET", "/cadence-policies?since="+cursor, nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())
	var feed struct {
		CadencePolicies []models.CadencePolicy `json:"cadence_policies"`
	}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &feed))
	require.Len(t, feed.CadencePolicies, 1)
	assert.Equal(t, policy.ID, feed.CadencePolicies[0].ID)
	assert.True(t, feed.CadencePolicies[0].Deleted,
		"AfterDelete must advance updated_at so a cursor at the original position still surfaces the tombstone")
}

// TestListCadencePoliciesSinceFeed pins the T17 incremental contract: cadence
// policies soft-delete, so the ?since= change feed must surface tombstones
// (deleted:true) and carry the incremental sync-mode meta. Without the
// retention window the setupRouter default would 410 every cursor, so this
// uses setupRouterWithRetention like the other feed tests.
func TestListCadencePoliciesSinceFeed(t *testing.T) {
	db, router := setupRouterWithRetention(30)
	router.GET("/cadence-policies", ListCadencePolicies)
	router.DELETE("/cadence-policies/:id", DeleteCadencePolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 30}
	db.Create(&policy)

	cursor := EncodeCursor(policy.UpdatedAt.Add(-time.Hour), policy.ID)

	var feed struct {
		CadencePolicies []models.CadencePolicy `json:"cadence_policies"`
		Sync            struct {
			Mode string `json:"mode"`
		} `json:"sync"`
	}
	w := doCadenceJSON(t, router, "GET", "/cadence-policies?since="+cursor, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &feed))
	require.Len(t, feed.CadencePolicies, 1)
	assert.Equal(t, policy.ID, feed.CadencePolicies[0].ID)
	assert.False(t, feed.CadencePolicies[0].Deleted)
	assert.Equal(t, "incremental", feed.Sync.Mode)

	// Soft-delete, then re-run the feed: the tombstone must come through.
	w2 := doCadenceJSON(t, router, "DELETE", "/cadence-policies/"+policy.ID, nil)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	w3 := doCadenceJSON(t, router, "GET", "/cadence-policies?since="+cursor, nil)
	require.Equal(t, http.StatusOK, w3.Code, w3.Body.String())
	var feedAfterDelete struct {
		CadencePolicies []models.CadencePolicy `json:"cadence_policies"`
	}
	require.NoError(t, json.Unmarshal(w3.Body.Bytes(), &feedAfterDelete))
	require.Len(t, feedAfterDelete.CadencePolicies, 1)
	assert.Equal(t, policy.ID, feedAfterDelete.CadencePolicies[0].ID)
	assert.True(t, feedAfterDelete.CadencePolicies[0].Deleted, "soft-deleted policy must surface as a tombstone")
}
