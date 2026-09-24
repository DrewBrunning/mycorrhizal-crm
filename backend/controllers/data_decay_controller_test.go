package controllers

import (
	"encoding/json"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateDataDecayPolicy(t *testing.T) {
	db, router := setupRouter()
	router.POST("/data-decay-policies", withValidated(func() any { return &models.DataDecayPolicyInput{} }), CreateDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	w := doCadenceJSON(t, router, "POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 365,
	})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		DataDecayPolicy dataDecayPolicyWithHealth `json:"data_decay_policy"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.DataDecayPolicy.ID)
	assert.Equal(t, 365, resp.DataDecayPolicy.IntervalDays)
	assert.True(t, resp.DataDecayPolicy.Active, "created without an explicit active flag must default to true")
	assert.Nil(t, resp.DataDecayPolicy.LastVerifiedAt)
	// A freshly created policy is anchored on CreatedAt, so it is never
	// immediately overdue.
	assert.Zero(t, resp.DataDecayPolicy.Health.OverdueBy)
}

func TestCreateDataDecayPolicyExplicitInactive(t *testing.T) {
	db, router := setupRouter()
	router.POST("/data-decay-policies", withValidated(func() any { return &models.DataDecayPolicyInput{} }), CreateDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)

	inactive := false
	w := doCadenceJSON(t, router, "POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 365, Active: &inactive,
	})
	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var reloaded models.DataDecayPolicy
	db.First(&reloaded, "entity_id = ?", contact.VCardUID)
	assert.False(t, reloaded.Active, "an explicit active:false must survive creation")
}

func TestCreateDataDecayPolicyRejectsDuplicate(t *testing.T) {
	db, router := setupRouter()
	router.POST("/data-decay-policies", withValidated(func() any { return &models.DataDecayPolicyInput{} }), CreateDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true})

	w := doCadenceJSON(t, router, "POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 180,
	})
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestCreateDataDecayPolicyRejectsForeignContact(t *testing.T) {
	db, router := setupRouter()
	router.POST("/data-decay-policies", withValidated(func() any { return &models.DataDecayPolicyInput{} }), CreateDataDecayPolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	othersContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&othersContact)

	w := doCadenceJSON(t, router, "POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: othersContact.VCardUID, IntervalDays: 365,
	})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestGetDataDecayPolicyNotFound(t *testing.T) {
	_, router := setupRouter()
	router.GET("/data-decay-policies/:id", GetDataDecayPolicy)

	w := doCadenceJSON(t, router, "GET", "/data-decay-policies/does-not-exist", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDataDecayPolicyScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.GET("/data-decay-policies/:id", GetDataDecayPolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	contact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&contact)
	policy := models.DataDecayPolicy{UserID: otherUser.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "GET", "/data-decay-policies/"+policy.ID, nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "a policy owned by another user must not be readable")
}

func TestListDataDecayPoliciesByEntity(t *testing.T) {
	db, router := setupRouter()
	router.GET("/data-decay-policies", ListDataDecayPolicies)

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)
	db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: alice.VCardUID, IntervalDays: 365, Active: true})
	db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: bob.VCardUID, IntervalDays: 180, Active: true})

	w := doCadenceJSON(t, router, "GET", "/data-decay-policies?entity_id="+alice.VCardUID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		DataDecayPolicies []dataDecayPolicyWithHealth `json:"data_decay_policies"`
		Total             int64                       `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.DataDecayPolicies, 1)
	assert.Equal(t, alice.VCardUID, resp.DataDecayPolicies[0].EntityID)
	assert.EqualValues(t, 1, resp.Total)
}

func TestListDataDecayPoliciesScopedToUser(t *testing.T) {
	db, router := setupRouter()
	router.GET("/data-decay-policies", ListDataDecayPolicies)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	otherContact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&otherContact)
	db.Create(&models.DataDecayPolicy{UserID: otherUser.ID, EntityID: otherContact.VCardUID, IntervalDays: 365, Active: true})

	w := doCadenceJSON(t, router, "GET", "/data-decay-policies", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		DataDecayPolicies []dataDecayPolicyWithHealth `json:"data_decay_policies"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.DataDecayPolicies, "another user's policy must never leak into this user's list")
}

func TestUpdateDataDecayPolicy(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/data-decay-policies/:id", withValidated(func() any { return &models.DataDecayPolicyInput{} }), UpdateDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true}
	db.Create(&policy)

	inactive := false
	w := doCadenceJSON(t, router, "PUT", "/data-decay-policies/"+policy.ID, models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 90, Active: &inactive,
	})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.DataDecayPolicy
	db.First(&reloaded, "id = ?", policy.ID)
	assert.Equal(t, 90, reloaded.IntervalDays)
	assert.False(t, reloaded.Active)
}

func TestUpdateDataDecayPolicyDoesNotTouchLastVerifiedAt(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/data-decay-policies/:id", withValidated(func() any { return &models.DataDecayPolicyInput{} }), UpdateDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	verifiedAt := time.Now().AddDate(0, 0, -10)
	policy := models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true, LastVerifiedAt: &verifiedAt}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "PUT", "/data-decay-policies/"+policy.ID, models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 90,
	})
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded models.DataDecayPolicy
	db.First(&reloaded, "id = ?", policy.ID)
	require.NotNil(t, reloaded.LastVerifiedAt)
	assert.WithinDuration(t, verifiedAt, *reloaded.LastVerifiedAt, time.Second, "a plain update must never touch last_verified_at")
}

func TestUpdateDataDecayPolicyScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/data-decay-policies/:id", withValidated(func() any { return &models.DataDecayPolicyInput{} }), UpdateDataDecayPolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	contact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&contact)
	policy := models.DataDecayPolicy{UserID: otherUser.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "PUT", "/data-decay-policies/"+policy.ID, models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 90,
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateDataDecayPolicyRejectsEntityChangeToExisting(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/data-decay-policies/:id", withValidated(func() any { return &models.DataDecayPolicyInput{} }), UpdateDataDecayPolicy)

	var user models.User
	db.First(&user)
	alice := models.Contact{UserID: user.ID, Firstname: "Alice"}
	bob := models.Contact{UserID: user.ID, Firstname: "Bob"}
	db.Create(&alice)
	db.Create(&bob)

	db.Create(&models.DataDecayPolicy{UserID: user.ID, EntityID: alice.VCardUID, IntervalDays: 365, Active: true})
	policyB := models.DataDecayPolicy{UserID: user.ID, EntityID: bob.VCardUID, IntervalDays: 180, Active: true}
	db.Create(&policyB)

	w := doCadenceJSON(t, router, "PUT", "/data-decay-policies/"+policyB.ID, models.DataDecayPolicyInput{
		EntityID: alice.VCardUID, IntervalDays: 90,
	})
	assert.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestDeleteDataDecayPolicySoftDeletesAndAllowsRecreate(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/data-decay-policies/:id", DeleteDataDecayPolicy)
	router.POST("/data-decay-policies", withValidated(func() any { return &models.DataDecayPolicyInput{} }), CreateDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	policy := models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "DELETE", "/data-decay-policies/"+policy.ID, nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var count int64
	db.Model(&models.DataDecayPolicy{}).Where("id = ?", policy.ID).Count(&count)
	assert.Zero(t, count)
	var unscoped int64
	db.Unscoped().Model(&models.DataDecayPolicy{}).Where("id = ?", policy.ID).Count(&unscoped)
	assert.EqualValues(t, 1, unscoped, "DataDecayPolicy must soft-delete (user-authored content, T26)")

	recreate := doCadenceJSON(t, router, "POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 180,
	})
	assert.Equal(t, http.StatusCreated, recreate.Code, recreate.Body.String())
}

func TestVerifyDataDecayPolicyStampsLastVerifiedAt(t *testing.T) {
	db, router := setupRouter()
	router.POST("/data-decay-policies/:id/verify", VerifyDataDecayPolicy)

	var user models.User
	db.First(&user)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	db.Create(&contact)
	// Overdue from the start: created "long ago" with a short interval.
	policy := models.DataDecayPolicy{UserID: user.ID, EntityID: contact.VCardUID, IntervalDays: 1, Active: true}
	db.Create(&policy)
	db.Model(&policy).UpdateColumn("created_at", time.Now().AddDate(0, 0, -30))

	before := time.Now()
	w := doCadenceJSON(t, router, "POST", "/data-decay-policies/"+policy.ID+"/verify", nil)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp dataDecayPolicyWithHealth
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.LastVerifiedAt)
	assert.True(t, !resp.LastVerifiedAt.Before(before), "last_verified_at must be stamped to (about) now")
	assert.Zero(t, resp.Health.OverdueBy, "verifying resets the health baseline, so it is no longer overdue")

	var reloaded models.DataDecayPolicy
	db.First(&reloaded, "id = ?", policy.ID)
	require.NotNil(t, reloaded.LastVerifiedAt)
}

func TestVerifyDataDecayPolicyScopedToOwner(t *testing.T) {
	db, router := setupRouter()
	router.POST("/data-decay-policies/:id/verify", VerifyDataDecayPolicy)

	var user models.User
	db.First(&user)
	otherUser := models.User{Username: "other", Password: "x", Email: "other@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	contact := models.Contact{UserID: otherUser.ID, Firstname: "Not Yours"}
	db.Create(&contact)
	policy := models.DataDecayPolicy{UserID: otherUser.ID, EntityID: contact.VCardUID, IntervalDays: 365, Active: true}
	db.Create(&policy)

	w := doCadenceJSON(t, router, "POST", "/data-decay-policies/"+policy.ID+"/verify", nil)
	assert.Equal(t, http.StatusNotFound, w.Code, "verifying another user's policy must 404")

	var reloaded models.DataDecayPolicy
	db.First(&reloaded, "id = ?", policy.ID)
	assert.Nil(t, reloaded.LastVerifiedAt, "the foreign policy must be untouched")
}

func TestGetOverdueDataDecayPolicies(t *testing.T) {
	db, router := setupRouter()
	router.GET("/data-decay-policies/overdue", GetOverdueDataDecayPolicies)

	var user models.User
	db.First(&user)

	overdueContact := models.Contact{UserID: user.ID, Firstname: "Stale"}
	db.Create(&overdueContact)
	overduePolicy := models.DataDecayPolicy{UserID: user.ID, EntityID: overdueContact.VCardUID, IntervalDays: 30, Active: true}
	db.Create(&overduePolicy)
	overdueVerified := time.Now().AddDate(0, 0, -40)
	db.Model(&overduePolicy).UpdateColumn("last_verified_at", overdueVerified)

	freshContact := models.Contact{UserID: user.ID, Firstname: "Fresh"}
	db.Create(&freshContact)
	freshPolicy := models.DataDecayPolicy{UserID: user.ID, EntityID: freshContact.VCardUID, IntervalDays: 30, Active: true}
	db.Create(&freshPolicy)
	freshVerified := time.Now().AddDate(0, 0, -1)
	db.Model(&freshPolicy).UpdateColumn("last_verified_at", freshVerified)

	w := doCadenceJSON(t, router, "GET", "/data-decay-policies/overdue", nil)
	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Overdue []services.OverdueDataDecayPolicy `json:"overdue"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Overdue, 1, "only the truly overdue policy appears")
	assert.Equal(t, overdueContact.VCardUID, resp.Overdue[0].Policy.EntityID)
	assert.Equal(t, overdueContact.ID, resp.Overdue[0].ContactID)
	assert.Equal(t, "Stale", resp.Overdue[0].ContactName)
	assert.True(t, resp.Overdue[0].Health.OverdueBy > 0)
}

func TestGetOverdueDataDecayPoliciesEmptyIsNullSafe(t *testing.T) {
	_, router := setupRouter()
	router.GET("/data-decay-policies/overdue", GetOverdueDataDecayPolicies)

	w := doCadenceJSON(t, router, "GET", "/data-decay-policies/overdue", nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"overdue":[]`, "an empty overdue list must serialize as [] not null (frontend crash guard)")
}
