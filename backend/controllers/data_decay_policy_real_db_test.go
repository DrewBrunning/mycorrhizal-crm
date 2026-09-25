package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDataDecayPolicy_RealMigratedSchema is the real-DB check for issue #352,
// mirroring TestCadencePolicy_RealMigratedSchema: every other controller
// test in this package runs against AutoMigrate on :memory: sqlite, which
// derives its schema from the same Go struct tags the application code
// uses — it cannot catch a GORM column-tag mismatch against the real
// hand-written migration SQL (this fork's own recurring bug class, e.g.
// ContactSyncLink.ETag). This runs the DataDecayPolicy CRUD + verify +
// overdue surface against a database.InitDB-migrated real file database:
// create -> list by entity -> push overdue -> verify (resets health) ->
// update -> soft-delete -> recreate (partial unique index).
func TestDataDecayPolicy_RealMigratedSchema(t *testing.T) {
	db := dbtest.New(t)

	user := models.User{Username: "decay-realdb", Password: "password123!A", Email: "decay-realdb@example.com"}
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
	router.POST("/data-decay-policies", withValidated(func() any { return &models.DataDecayPolicyInput{} }), CreateDataDecayPolicy)
	router.GET("/data-decay-policies", ListDataDecayPolicies)
	router.PUT("/data-decay-policies/:id", withValidated(func() any { return &models.DataDecayPolicyInput{} }), UpdateDataDecayPolicy)
	router.DELETE("/data-decay-policies/:id", DeleteDataDecayPolicy)
	router.POST("/data-decay-policies/:id/verify", VerifyDataDecayPolicy)
	router.GET("/data-decay-policies/overdue", GetOverdueDataDecayPolicies)

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

	createResp := doJSON("POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 30,
	})
	require.Equal(t, http.StatusCreated, createResp.Code, createResp.Body.String())
	var created struct {
		DataDecayPolicy dataDecayPolicyWithHealth `json:"data_decay_policy"`
	}
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &created))
	policyID := created.DataDecayPolicy.ID
	require.NotEmpty(t, policyID)
	assert.Nil(t, created.DataDecayPolicy.LastVerifiedAt)

	// List by entity: the policy round-trips through the real `active`
	// column (BOOLEAN NOT NULL DEFAULT 1) and `interval_days`.
	listResp := doJSON("GET", "/data-decay-policies?entity_id="+contact.VCardUID, nil)
	require.Equal(t, http.StatusOK, listResp.Code, listResp.Body.String())
	var listed struct {
		DataDecayPolicies []dataDecayPolicyWithHealth `json:"data_decay_policies"`
	}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listed))
	require.Len(t, listed.DataDecayPolicies, 1)
	assert.True(t, listed.DataDecayPolicies[0].Active)

	// Push the policy's created_at into the past through the real
	// `created_at` column so the baseline (no last_verified_at yet) is
	// overdue.
	require.NoError(t, db.Model(&models.DataDecayPolicy{}).Where("id = ?", policyID).
		UpdateColumn("created_at", time.Now().AddDate(0, 0, -40)).Error)

	overdueResp := doJSON("GET", "/data-decay-policies/overdue", nil)
	require.Equal(t, http.StatusOK, overdueResp.Code, overdueResp.Body.String())
	var overdue struct {
		Overdue []services.OverdueDataDecayPolicy `json:"overdue"`
	}
	require.NoError(t, json.Unmarshal(overdueResp.Body.Bytes(), &overdue))
	require.Len(t, overdue.Overdue, 1)
	assert.Equal(t, policyID, overdue.Overdue[0].Policy.ID)
	assert.Equal(t, contact.ID, overdue.Overdue[0].ContactID)
	assert.True(t, overdue.Overdue[0].Health.OverdueBy > 0)

	// Verify ("confirm still current") through the real `last_verified_at`
	// column: the policy must drop out of the overdue list.
	verifyResp := doJSON("POST", "/data-decay-policies/"+policyID+"/verify", nil)
	require.Equal(t, http.StatusOK, verifyResp.Code, verifyResp.Body.String())
	var verified dataDecayPolicyWithHealth
	require.NoError(t, json.Unmarshal(verifyResp.Body.Bytes(), &verified))
	require.NotNil(t, verified.LastVerifiedAt)
	assert.Zero(t, verified.Health.OverdueBy)

	overdueAfterVerify := doJSON("GET", "/data-decay-policies/overdue", nil)
	require.Equal(t, http.StatusOK, overdueAfterVerify.Code)
	var overdueAfter struct {
		Overdue []services.OverdueDataDecayPolicy `json:"overdue"`
	}
	require.NoError(t, json.Unmarshal(overdueAfterVerify.Body.Bytes(), &overdueAfter))
	assert.Empty(t, overdueAfter.Overdue, "verifying must drop the policy out of the overdue list")

	// Update (full replace).
	updateResp := doJSON("PUT", "/data-decay-policies/"+policyID, models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 90,
	})
	require.Equal(t, http.StatusOK, updateResp.Code, updateResp.Body.String())
	var reloaded models.DataDecayPolicy
	require.NoError(t, db.First(&reloaded, "id = ?", policyID).Error)
	assert.Equal(t, 90, reloaded.IntervalDays)

	// Soft-delete, then recreate for the same contact (the partial unique
	// index idx_data_decay_policies_user_entity must not block it).
	deleteResp := doJSON("DELETE", "/data-decay-policies/"+policyID, nil)
	require.Equal(t, http.StatusOK, deleteResp.Code, deleteResp.Body.String())
	recreateResp := doJSON("POST", "/data-decay-policies", models.DataDecayPolicyInput{
		EntityID: contact.VCardUID, IntervalDays: 60,
	})
	require.Equal(t, http.StatusCreated, recreateResp.Code, recreateResp.Body.String())
}
