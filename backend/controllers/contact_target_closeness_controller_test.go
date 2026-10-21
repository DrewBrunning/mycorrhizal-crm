package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #1796: PUT/DELETE /contacts/:id/target-closeness (ADR 0023 amendment).

func targetRouter(t *testing.T) (*gorm.DB, *gin.Engine, models.User) {
	t.Helper()
	db, router := setupRouter(t)
	var user models.User
	require.NoError(t, db.First(&user).Error)
	router.PUT("/contacts/:id/target-closeness", middleware.ValidateJSONMiddleware(&models.ContactTargetClosenessInput{}), SetContactTargetCloseness)
	router.DELETE("/contacts/:id/target-closeness", ClearContactTargetCloseness)
	router.PUT("/contacts/:id", withValidated(func() any { return &models.ContactRecordInput{} }), UpdateContact)
	router.POST("/contacts", withValidated(func() any { return &models.ContactRecordInput{} }), CreateContact)
	router.GET("/contacts/:id", GetContact)
	router.GET("/contacts/:id/score", GetContactScore)
	return db, router, user
}

func tcJSON(router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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

func rawJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
	return m
}

func targetPath(c models.Contact) string {
	return "/contacts/" + idString(c.ID) + "/target-closeness"
}

func TestSetContactTargetCloseness_SetsAndOffersPolicy(t *testing.T) {
	db, router, user := targetRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	var before models.Contact
	require.NoError(t, db.First(&before, contact.ID).Error)

	w := tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "close"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.JSONEq(t, `{
		"contact_id": `+idString(contact.ID)+`,
		"target_closeness": "close",
		"default_interval_days": 30,
		"cadence_policy_offer": {"entity_id": "`+contact.VCardUID+`", "target_interval_days": 30}
	}`, w.Body.String())

	var after models.Contact
	require.NoError(t, db.First(&after, contact.ID).Error)
	require.NotNil(t, after.TargetCloseness)
	assert.Equal(t, "close", *after.TargetCloseness)
	assert.Equal(t, before.Revision+1, after.Revision, "hooks fire, so the revision bumps exactly once")
}

func TestSetContactTargetCloseness_NoOfferWhenPolicyExists(t *testing.T) {
	db, router, user := targetRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.CadencePolicy{UserID: user.ID, EntityID: contact.VCardUID, TargetIntervalDays: 10}).Error)

	w := tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "distant"})
	require.Equal(t, http.StatusOK, w.Code)
	m := rawJSON(t, w)
	assert.Equal(t, "distant", m["target_closeness"])
	assert.EqualValues(t, 60, m["default_interval_days"])
	v, present := m["cadence_policy_offer"]
	assert.True(t, present, "the key is always present")
	assert.Nil(t, v)

	// A soft-deleted policy does not count: the offer comes back.
	require.NoError(t, db.Where("entity_id = ?", contact.VCardUID).Delete(&models.CadencePolicy{}).Error)
	w = tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "distant"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, rawJSON(t, w)["cadence_policy_offer"])
}

func TestSetContactTargetCloseness_Validation(t *testing.T) {
	db, router, user := targetRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	w := tcJSON(router, http.MethodPut, targetPath(contact), map[string]string{"tier": "bogus"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = tcJSON(router, http.MethodPut, targetPath(contact), map[string]string{})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var after models.Contact
	require.NoError(t, db.First(&after, contact.ID).Error)
	assert.Nil(t, after.TargetCloseness)
}

func TestClearContactTargetCloseness_ClearsDismissesAndIsIdempotent(t *testing.T) {
	db, router, user := targetRouter(t)
	tier := "close"
	contact := models.Contact{UserID: user.ID, Firstname: "Alice", TargetCloseness: &tier}
	require.NoError(t, db.Create(&contact).Error)
	pending := models.ReachOutSuggestion{UserID: user.ID, ContactVCardUID: contact.VCardUID, Kind: models.ReachOutKindTargetBehind,
		OldValue: "close", NewValue: "2030-01-01T00:00:00Z", Status: models.ReachOutStatusPending}
	require.NoError(t, db.Create(&pending).Error)
	orgRow := models.ReachOutSuggestion{UserID: user.ID, ContactVCardUID: contact.VCardUID, Kind: models.ReachOutKindOrganization,
		NewValue: "X", AuditEventID: 1, Status: models.ReachOutStatusPending}
	require.NoError(t, db.Create(&orgRow).Error)

	for i := 0; i < 2; i++ { // the second call is the idempotent one
		w := tcJSON(router, http.MethodDelete, targetPath(contact), nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		m := rawJSON(t, w)
		assert.EqualValues(t, contact.ID, m["contact_id"])
		for _, k := range []string{"target_closeness", "default_interval_days", "cadence_policy_offer"} {
			v, present := m[k]
			assert.Truef(t, present, "%s must be present", k)
			assert.Nilf(t, v, "%s must be null", k)
		}
	}

	var after models.Contact
	require.NoError(t, db.First(&after, contact.ID).Error)
	assert.Nil(t, after.TargetCloseness)

	var got models.ReachOutSuggestion
	require.NoError(t, db.First(&got, "id = ?", pending.ID).Error)
	assert.Equal(t, models.ReachOutStatusDismissed, got.Status)
	var gotOrg models.ReachOutSuggestion
	require.NoError(t, db.First(&gotOrg, "id = ?", orgRow.ID).Error)
	assert.Equal(t, models.ReachOutStatusPending, gotOrg.Status, "other kinds are untouched")
}

func TestTargetCloseness_OwnershipAndMissing(t *testing.T) {
	db, router, _ := targetRouter(t)
	other := models.User{Username: "intruder-target", Password: "password123!A", Email: "intruder-target@example.com"}
	require.NoError(t, db.Create(&other).Error)
	tier := "close"
	theirs := models.Contact{UserID: other.ID, Firstname: "Theirs", TargetCloseness: &tier}
	require.NoError(t, db.Create(&theirs).Error)

	assert.Equal(t, http.StatusNotFound, tcJSON(router, http.MethodPut, targetPath(theirs), models.ContactTargetClosenessInput{Tier: "casual"}).Code)
	assert.Equal(t, http.StatusNotFound, tcJSON(router, http.MethodDelete, targetPath(theirs), nil).Code)
	assert.Equal(t, http.StatusNotFound, tcJSON(router, http.MethodPut, "/contacts/999999/target-closeness", models.ContactTargetClosenessInput{Tier: "casual"}).Code)

	var after models.Contact
	require.NoError(t, db.First(&after, theirs.ID).Error)
	require.NotNil(t, after.TargetCloseness)
	assert.Equal(t, "close", *after.TargetCloseness, "another user's target is never read or changed")
}

func TestTargetCloseness_DBFailuresAre500(t *testing.T) {
	for _, tc := range []struct{ name, method, table string }{
		{"put count", http.MethodPut, "cadence_policies"},
		{"delete dismiss", http.MethodDelete, "reach_out_suggestions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, router, user := targetRouter(t)
			logtest.AllowWarnings(t, "the injected DB failure is logged by the error envelope")
			contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
			require.NoError(t, db.Create(&contact).Error)
			dbtest.HideTable(t, db, tc.table)
			var body any
			if tc.method == http.MethodPut {
				body = models.ContactTargetClosenessInput{Tier: "close"}
			}
			w := tcJSON(router, tc.method, targetPath(contact), body)
			assert.Equal(t, http.StatusInternalServerError, w.Code)
		})
	}
}

// The contact PUT/POST never write target_closeness, whatever crm says.
func TestContactWrites_NeverTouchTargetCloseness(t *testing.T) {
	db, router, user := targetRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)
	require.Equal(t, http.StatusOK, tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "close"}).Code)

	card := contactmodel.Card{Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Alice"}}}}
	distant := "distant"
	// A PUT that forges a different value, and one that omits the key.
	for _, crm := range []contactmodel.CRMEnvelope{{TargetCloseness: &distant}, {}} {
		w := tcJSON(router, http.MethodPut, "/contacts/"+idString(contact.ID), models.ContactRecordInput{Card: card, CRM: crm})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var stored models.Contact
		require.NoError(t, db.First(&stored, contact.ID).Error)
		require.NotNil(t, stored.TargetCloseness)
		assert.Equal(t, "close", *stored.TargetCloseness)
		// The response envelope echoes the flat column, not the forged value.
		crmOut, _ := rawJSON(t, w)["crm"].(map[string]any)
		assert.Equal(t, "close", crmOut["target_closeness"])
		// And the persisted crm JSON column agrees (BeforeSave mirrors the flat column).
		require.NotNil(t, stored.CRM.TargetCloseness)
		assert.Equal(t, "close", *stored.CRM.TargetCloseness)
	}

	// POST with the field set stores NULL.
	w := tcJSON(router, http.MethodPost, "/contacts", models.ContactRecordInput{Card: card, CRM: contactmodel.CRMEnvelope{TargetCloseness: &distant}})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var created models.Contact
	require.NoError(t, db.Where("id != ?", contact.ID).Order("id DESC").First(&created).Error)
	assert.Nil(t, created.TargetCloseness)
}

func TestGetContact_TargetClosenessFieldPresence(t *testing.T) {
	db, router, user := targetRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	w := tcJSON(router, http.MethodGet, "/contacts/"+idString(contact.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	crm, _ := rawJSON(t, w)["crm"].(map[string]any)
	_, present := crm["target_closeness"]
	assert.False(t, present, "absent when no target (raw JSON)")

	require.Equal(t, http.StatusOK, tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "casual"}).Code)
	w = tcJSON(router, http.MethodGet, "/contacts/"+idString(contact.ID), nil)
	crm, _ = rawJSON(t, w)["crm"].(map[string]any)
	assert.Equal(t, "casual", crm["target_closeness"])
}

func TestGetContactScore_TargetFieldPresence(t *testing.T) {
	db, router, user := targetRouter(t)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	w := tcJSON(router, http.MethodGet, "/contacts/"+idString(contact.ID)+"/score", nil)
	require.Equal(t, http.StatusOK, w.Code)
	m := rawJSON(t, w)
	v, present := m["target"]
	assert.True(t, present, `"target" is always present`)
	assert.Nil(t, v)

	require.Equal(t, http.StatusOK, tcJSON(router, http.MethodPut, targetPath(contact), models.ContactTargetClosenessInput{Tier: "close"}).Code)
	w = tcJSON(router, http.MethodGet, "/contacts/"+idString(contact.ID)+"/score", nil)
	target, ok := rawJSON(t, w)["target"].(map[string]any)
	require.True(t, ok, w.Body.String())
	assert.Equal(t, "close", target["tier"])
	assert.Equal(t, "target_tier", target["interval_source"])
	assert.EqualValues(t, 30, target["interval_days"])
	assert.Contains(t, []any{"on_track", "drifting", "behind"}, target["status"])
	for _, k := range []string{"value", "reason"} {
		assert.Contains(t, target, k)
	}
}
