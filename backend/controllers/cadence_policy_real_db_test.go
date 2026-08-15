package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/database"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCadencePolicy_RealMigratedSchema is the real-DB check for T19: every
// other controller test runs against AutoMigrate on:memory: sqlite, which
// derives its schema from the same Go struct tags the application code uses —
// it cannot catch a GORM column-tag or serializer mismatch against the real
// hand-written migration SQL (this fork's own recurring bug class, e.g.
// ContactSyncLink.ETag). This test runs the CadencePolicy CRUD + overdue
// surface against a database.InitDB-migrated real file database instead:
// create (with a JSON qualifying_types list) -> list by entity (health
// embedded) -> update -> soft-delete -> recreate (partial unique index) ->
// overdue list.
func TestCadencePolicy_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cadence-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "cadence-realdb", Password: "password123!A", Email: "cadence-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/cadence-policies", withValidated(func() any { return &models.CadencePolicyInput{} }), CreateCadencePolicy)
	router.GET("/cadence-policies", ListCadencePolicies)
	router.PUT("/cadence-policies/:id", withValidated(func() any { return &models.CadencePolicyInput{} }), UpdateCadencePolicy)
	router.DELETE("/cadence-policies/:id", DeleteCadencePolicy)
	router.GET("/cadence-policies/overdue", GetOverdueCadences)

	doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
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

	// Create with a JSON qualifying_types array — must survive the real
	// TEXT/json serializer column, not just AutoMigrate's derived schema.
	createResp := doJSON("POST", "/cadence-policies", models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 30,
		QualifyingTypes: []string{models.InteractionTypeCall, models.InteractionTypeVisit},
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		CadencePolicy cadencePolicyWithHealth `json:"cadence_policy"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	policyID := created.CadencePolicy.ID
	require.NotEmpty(t, policyID)
	assert.Equal(t, []string{models.InteractionTypeCall, models.InteractionTypeVisit}, created.CadencePolicy.QualifyingTypes)

	// List by entity: the policy returns with undefined health (no
	// qualifying interaction yet).
	listResp := doJSON("GET", "/cadence-policies?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		CadencePolicies []cadencePolicyWithHealth `json:"cadence_policies"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.CadencePolicies, 1)
	assert.False(t, listed.CadencePolicies[0].Health.HasQualifyingInteraction)

	// Record a qualifying interaction ~40 days ago and verify health turns
	// overdue through the real schema (the activity_contacts join + contacts
	// vcard_uid join must resolve correctly on real migration column names).
	activity := models.Activity{
		UserID: user.ID, Title: "Last call", Date: time.Now().AddDate(0, 0, -40),
		Type: models.InteractionTypeCall, Contacts: []models.Contact{contact},
	}
	require.NoError(t, db.Create(&activity).Error)

	overdueResp := doJSON("GET", "/cadence-policies/overdue", nil)
	require.Equal(t, http.StatusOK, overdueResp.Code, overdueResp.Body.String())
	var overdue struct {
		Overdue []services.OverdueCadence `json:"overdue"`
	}
	require.NoError(t, json.Unmarshal(overdueResp.Body.Bytes(), &overdue))
	require.Len(t, overdue.Overdue, 1)
	assert.Equal(t, policyID, overdue.Overdue[0].Policy.ID)
	assert.Equal(t, contact.ID, overdue.Overdue[0].ContactID)
	assert.True(t, overdue.Overdue[0].Health.OverdueBy > 0)

	// Update (full replace).
	updateResp := doJSON("PUT", "/cadence-policies/"+policyID, models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 90,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())
	var reloaded models.CadencePolicy
	require.NoError(t, db.First(&reloaded, "id = ?", policyID).Error)
	assert.Equal(t, 90, reloaded.TargetIntervalDays)
	assert.Empty(t, reloaded.QualifyingTypes)

	// Soft-delete, then recreate for the same contact (the partial unique
	// index idx_cadence_policies_user_entity must not block it).
	deleteResp := doJSON("DELETE", "/cadence-policies/"+policyID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	recreateResp := doJSON("POST", "/cadence-policies", models.CadencePolicyInput{
		EntityID: contact.VCardUID, TargetIntervalDays: 60,
	})
	require.Equal(t, http.StatusCreated, recreateResp.Code, recreateResp.Body.String())
}
